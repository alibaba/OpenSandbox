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

package runtime

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	goruntime "runtime"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
	"github.com/stretchr/testify/assert"
)

func TestReadFromPos_SplitsOnCRAndLF(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "stdout.log")

	mutex := &sync.Mutex{}

	initial := "line1\nprog 10%\rprog 20%\rprog 30%\nlast\n"
	if err := os.WriteFile(logFile, []byte(initial), 0o644); !assert.NoError(t, err, "write initial file") {
		return
	}

	var got []string
	c := &Controller{}
	nextPos := c.readFromPos(mutex, logFile, 0, func(s string) { got = append(got, s) }, false)

	want := []string{"line1", "prog 10%", "prog 20%", "prog 30%", "last"}
	assert.Equal(t, len(want), len(got), "unexpected token count")
	for i := range want {
		assert.Equalf(t, want[i], got[i], "token[%d]", i)
	}

	// append more content and ensure incremental read only yields the new part
	appendPart := "tail1\r\ntail2\n"
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if !assert.NoError(t, err, "open append") {
		return
	}
	if _, err := f.WriteString(appendPart); err != nil {
		_ = f.Close()
		assert.NoError(t, err, "append write")
		return
	}
	_ = f.Close()

	got = got[:0]
	c.readFromPos(mutex, logFile, nextPos, func(s string) { got = append(got, s) }, false)
	want = []string{"tail1", "tail2"}
	assert.Equal(t, len(want), len(got), "incremental token count")
	for i := range want {
		assert.Equalf(t, want[i], got[i], "incremental token[%d]", i)
	}
}

func TestReadFromPos_LongLine(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "stdout.log")

	// construct a single line larger than the default 64KB, but under 5MB
	longLine := strings.Repeat("x", 256*1024) + "\n" // 256KB
	if err := os.WriteFile(logFile, []byte(longLine), 0o644); !assert.NoError(t, err, "write long line") {
		return
	}

	var got []string
	c := &Controller{}
	c.readFromPos(&sync.Mutex{}, logFile, 0, func(s string) { got = append(got, s) }, false)

	assert.Equal(t, 1, len(got), "expected one token")
	assert.Equal(t, strings.TrimSuffix(longLine, "\n"), got[0], "long line mismatch")
}

func TestReadFromPos_FlushesTrailingLine(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "stdout.log")
	content := []byte("line1\nlastline-without-newline")
	err := os.WriteFile(file, content, 0o644)
	assert.NoError(t, err)

	c := NewController("", "")
	mutex := &sync.Mutex{}
	var lines []string
	onExecute := func(text string) {
		lines = append(lines, text)
	}

	// First read: should only get complete lines with newlines
	pos := c.readFromPos(mutex, file, 0, onExecute, false)
	assert.GreaterOrEqual(t, pos, int64(0))
	assert.Equal(t, []string{"line1"}, lines)

	// Flush at end: should output the last line (without newline)
	c.readFromPos(mutex, file, pos, onExecute, true)
	assert.Equal(t, []string{"line1", "lastline-without-newline"}, lines)
}

func TestRunCommand_Echo(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("bash not available on windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		sessionID   string
		stdoutLines []string
		stderrLines []string
		completeCh  = make(chan struct{}, 1)
	)

	req := &ExecuteCodeRequest{
		Code:    `echo "hello"; echo "errline" 1>&2`,
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
		Hooks: ExecuteResultHook{
			OnExecuteInit: func(s string) { sessionID = s },
			OnExecuteStdout: func(s string) {
				stdoutLines = append(stdoutLines, s)
			},
			OnExecuteStderr: func(s string) {
				stderrLines = append(stderrLines, s)
			},
			OnExecuteError: func(err *execute.ErrorOutput) {
				assert.Fail(t, "unexpected error hook", "%+v", err)
			},
			OnExecuteComplete: func(_ time.Duration) {
				completeCh <- struct{}{}
			},
		},
	}

	if !assert.NoError(t, c.runCommand(ctx, req)) {
		return
	}

	select {
	case <-completeCh:
	case <-time.After(2 * time.Second):
		assert.Fail(t, "timeout waiting for completion hook")
		return
	}

	assert.NotEmpty(t, sessionID, "expected session id to be set")
	assert.Equal(t, []string{"hello"}, stdoutLines)
	assert.Equal(t, []string{"errline"}, stderrLines)
}

func TestRunCommand_Error(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("bash not available on windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		sessionID   string
		gotErr      *execute.ErrorOutput
		completeCh  = make(chan struct{}, 2)
		stdoutLines []string
		stderrLines []string
	)

	req := &ExecuteCodeRequest{
		Code:    `echo "before"; exit 3`,
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
		Hooks: ExecuteResultHook{
			OnExecuteInit:   func(s string) { sessionID = s },
			OnExecuteStdout: func(s string) { stdoutLines = append(stdoutLines, s) },
			OnExecuteStderr: func(s string) { stderrLines = append(stderrLines, s) },
			OnExecuteError: func(err *execute.ErrorOutput) {
				gotErr = err
				completeCh <- struct{}{}
			},
			OnExecuteComplete: func(_ time.Duration) {
				completeCh <- struct{}{}
			},
		},
	}

	if !assert.NoError(t, c.runCommand(ctx, req)) {
		return
	}

	select {
	case <-completeCh:
	case <-time.After(2 * time.Second):
		assert.Fail(t, "timeout waiting for completion hook")
		return
	}

	assert.NotEmpty(t, sessionID, "expected session id to be set")
	if assert.NotEmpty(t, stdoutLines) {
		assert.Equal(t, "before", stdoutLines[0])
	}
	assert.Empty(t, stderrLines)
	if assert.NotNil(t, gotErr, "expected error hook to be called") {
		assert.Equal(t, "CommandExecError", gotErr.EName)
		assert.Equal(t, "3", gotErr.EValue)
	}
}

func TestRunCommand_WithUser(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("bash not available on windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}
	cur, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	c := NewController("", "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		sessionID  string
		gotErr     *execute.ErrorOutput
		completeCh = make(chan struct{}, 2)
	)

	req := &ExecuteCodeRequest{
		Code:    `echo "user-test"`,
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
		User:    &CommandUser{Username: &cur.Username},
		Hooks: ExecuteResultHook{
			OnExecuteInit:  func(s string) { sessionID = s },
			OnExecuteError: func(err *execute.ErrorOutput) { gotErr = err; completeCh <- struct{}{} },
			OnExecuteComplete: func(_ time.Duration) {
				completeCh <- struct{}{}
			},
		},
	}

	if !assert.NoError(t, c.runCommand(ctx, req)) {
		return
	}

	select {
	case <-completeCh:
	case <-time.After(2 * time.Second):
		assert.Fail(t, "timeout waiting for completion hook")
		return
	}

	if gotErr != nil {
		if strings.Contains(gotErr.EValue, "operation not permitted") {
			t.Skipf("skipping user credential test: %s", gotErr.EValue)
		}
		assert.Fail(t, "unexpected error hook", "%+v", gotErr)
		return
	}

	assert.NotEmpty(t, sessionID, "expected session id to be set")

	status, err := c.GetCommandStatus(sessionID)
	if !assert.NoError(t, err) {
		return
	}
	if assert.NotNil(t, status.User, "expected status user") {
		assert.NotNil(t, status.User.Username, "expected status username")
		if status.User.Username != nil {
			assert.Equal(t, cur.Username, *status.User.Username)
		}
	}
	if uidVal, parseErr := strconv.ParseInt(cur.Uid, 10, 64); parseErr == nil {
		if assert.NotNil(t, status.User) && assert.NotNil(t, status.User.UID) {
			assert.Equal(t, uidVal, *status.User.UID)
		}
	}
	if assert.NotNil(t, status.ExitCode) {
		assert.Equal(t, 0, *status.ExitCode)
	}
}
