// Copyright 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows
// +build !windows

package runtime

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
	"github.com/stretchr/testify/require"
)

// TestRunCommand_CancelKillsChildren verifies that cancelling the context
// terminates not only the bash group leader but also its descendant
// processes. Regression test for
// https://github.com/alibaba/OpenSandbox/issues/922.
func TestRunCommand_CancelKillsChildren(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	pidFile := filepath.Join(t.TempDir(), "child.pid")

	c := NewController("", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	var once sync.Once

	req := &ExecuteCodeRequest{
		// Spawn a sleep child, record its pid, then wait so the bash
		// leader stays alive until the context is cancelled.
		Code:    `sleep 30 & echo $! > "` + pidFile + `"; echo READY; wait`,
		Cwd:     t.TempDir(),
		Timeout: 30 * time.Second,
		Hooks: ExecuteResultHook{
			OnExecuteInit: func(_ string) {},
			OnExecuteStdout: func(s string) {
				if strings.TrimSpace(s) == "READY" {
					once.Do(func() { close(started) })
				}
			},
			OnExecuteStderr:   func(_ string) {},
			OnExecuteError:    func(_ *execute.ErrorOutput) {},
			OnExecuteComplete: func(_ time.Duration) {},
		},
	}

	done := make(chan struct{})
	go func() {
		_ = c.runCommand(ctx, req)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		cancel()
		<-done
		t.Fatal("command did not emit READY in time")
	}

	pidBytes, err := os.ReadFile(pidFile)
	require.NoError(t, err, "expected child pid file")
	childPid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	require.NoError(t, err)
	require.Positive(t, childPid)

	require.NoError(t, syscall.Kill(childPid, 0), "child should be alive before cancel")

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runCommand did not return after cancel")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(childPid, 0); err != nil {
			require.True(t, errors.Is(err, syscall.ESRCH),
				"unexpected liveness probe error: %v", err)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("child pid %d still alive 2s after cancel — process leak", childPid)
}

// TestKillPid_TerminatesEntireProcessGroup verifies that killPid signals
// the whole process group, not just the leader. Regression test for
// https://github.com/alibaba/OpenSandbox/issues/922.
func TestKillPid_TerminatesEntireProcessGroup(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.Command("bash", "-c",
		`sleep 30 & echo $! > "`+pidFile+`"; wait`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	// Reap the leader concurrently so it doesn't linger as a zombie that
	// keeps the process group "alive" from killPid's liveness probe
	// perspective. Mirrors how runCommand's cmd.Wait() reaps in production.
	waitDone := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(waitDone)
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-waitDone
	})

	var childPid int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			if pid, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil && pid > 0 {
				childPid = pid
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.Positive(t, childPid, "failed to capture child pid")
	require.NoError(t, syscall.Kill(childPid, 0), "child should be alive before kill")

	c := &Controller{}
	require.NoError(t, c.killPid(cmd.Process.Pid))

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(childPid, 0); err != nil {
			require.True(t, errors.Is(err, syscall.ESRCH),
				"unexpected liveness probe error: %v", err)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("child pid %d still alive 2s after killPid — process leak", childPid)
}
