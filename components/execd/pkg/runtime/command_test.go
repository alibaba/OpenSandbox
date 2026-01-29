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
	"path/filepath"
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
	if err := os.WriteFile(logFile, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	var got []string
	c := &Controller{}
	nextPos := c.readFromPos(mutex, logFile, 0, func(s string) { got = append(got, s) }, false)

	want := []string{"line1", "prog 10%", "prog 20%", "prog 30%", "last"}
	if len(got) != len(want) {
		t.Fatalf("unexpected token count: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d]: got %q want %q", i, got[i], want[i])
		}
	}

	// append more content and ensure incremental read only yields the new part
	appendPart := "tail1\r\ntail2\n"
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	if _, err := f.WriteString(appendPart); err != nil {
		f.Close()
		t.Fatalf("append write: %v", err)
	}
	_ = f.Close()

	got = got[:0]
	c.readFromPos(mutex, logFile, nextPos, func(s string) { got = append(got, s) }, false)
	want = []string{"tail1", "tail2"}
	if len(got) != len(want) {
		t.Fatalf("incremental token count: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("incremental token[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestReadFromPos_LongLine(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "stdout.log")

	// construct a single line larger than the default 64KB, but under 5MB
	longLine := strings.Repeat("x", 256*1024) + "\n" // 256KB
	if err := os.WriteFile(logFile, []byte(longLine), 0o644); err != nil {
		t.Fatalf("write long line: %v", err)
	}

	var got []string
	c := &Controller{}
	c.readFromPos(&sync.Mutex{}, logFile, 0, func(s string) { got = append(got, s) }, false)

	if len(got) != 1 {
		t.Fatalf("expected one token, got %d", len(got))
	}
	if got[0] != strings.TrimSuffix(longLine, "\n") {
		t.Fatalf("long line mismatch: got %d chars want %d chars", len(got[0]), len(longLine)-1)
	}
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
				t.Fatalf("unexpected error hook: %+v", err)
			},
			OnExecuteComplete: func(_ time.Duration) {
				completeCh <- struct{}{}
			},
		},
	}

	if err := c.runCommand(ctx, req); err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}

	select {
	case <-completeCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for completion hook")
	}

	if sessionID == "" {
		t.Fatalf("expected session id to be set")
	}
	if len(stdoutLines) != 1 || stdoutLines[0] != "hello" {
		t.Fatalf("unexpected stdout: %#v", stdoutLines)
	}
	if len(stderrLines) != 1 || stderrLines[0] != "errline" {
		t.Fatalf("unexpected stderr: %#v", stderrLines)
	}
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

	if err := c.runCommand(ctx, req); err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}

	select {
	case <-completeCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for completion hook")
	}

	if sessionID == "" {
		t.Fatalf("expected session id to be set")
	}
	if len(stdoutLines) == 0 || stdoutLines[0] != "before" {
		t.Fatalf("unexpected stdout: %#v", stdoutLines)
	}
	if len(stderrLines) != 0 {
		t.Fatalf("expected no stderr, got %#v", stderrLines)
	}
	if gotErr == nil {
		t.Fatalf("expected error hook to be called")
	}
	if gotErr.EName != "CommandExecError" || gotErr.EValue != "3" {
		t.Fatalf("unexpected error payload: %+v", gotErr)
	}
}
