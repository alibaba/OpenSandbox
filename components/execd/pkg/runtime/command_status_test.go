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
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGetCommandStatus_NotFound(t *testing.T) {
	c := NewController("", "")

	if _, err := c.GetCommandStatus("missing"); err == nil {
		t.Fatalf("expected error for missing session")
	}
}

func TestGetCommandStatus_Running(t *testing.T) {
	c := NewController("", "")

	var session string
	req := &ExecuteCodeRequest{
		Language: BackgroundCommand,
		Code:     "sleep 2",
		Hooks: ExecuteResultHook{
			OnExecuteInit:     func(id string) { session = id },
			OnExecuteComplete: func(time.Duration) {},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := c.runBackgroundCommand(ctx, cancel, req); err != nil {
		t.Fatalf("runBackgroundCommand error: %v", err)
	}
	if session == "" {
		t.Fatalf("session should be set by OnExecuteInit")
	}

	// Poll until status is registered (runBackgroundCommand stores kernel asynchronously).
	deadline := time.Now().Add(5 * time.Second)
	var (
		status *CommandStatus
		err    error
	)
	for time.Now().Before(deadline) {
		status, err = c.GetCommandStatus(session)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "not found") {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		t.Fatalf("GetCommandStatus unexpected error: %v", err)
	}
	if err != nil {
		t.Fatalf("GetCommandStatus error after retry: %v", err)
	}

	if status == nil || !status.Running {
		t.Fatalf("expected running=true")
	}
	if status.ExitCode != nil {
		t.Fatalf("expected exitCode to be nil while running")
	}
	if status.FinishedAt != nil {
		t.Fatalf("expected finishedAt to be nil while running")
	}
	if status.StartedAt.IsZero() {
		t.Fatalf("expected startedAt to be set")
	}
	t.Log(status)
}

func TestSeekBackgroundCommandOutput_Completed(t *testing.T) {
	c := NewController("", "")

	tmpDir := t.TempDir()
	session := "sess-done"
	stdoutPath := filepath.Join(tmpDir, session+".stdout")

	stdoutContent := "hello stdout"
	if err := os.WriteFile(stdoutPath, []byte(stdoutContent), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}

	started := time.Now().Add(-2 * time.Second)
	finished := time.Now()
	exitCode := 0
	kernel := &commandKernel{
		pid:          456,
		stdoutPath:   stdoutPath,
		isBackground: true,
		startedAt:    started,
		finishedAt:   &finished,
		exitCode:     &exitCode,
		errMsg:       "",
		running:      false,
	}
	c.storeCommandKernel(session, kernel)

	output, cursor, err := c.SeekBackgroundCommandOutput(session, 0)
	if err != nil {
		t.Fatalf("GetCommandOutput error: %v", err)
	}

	if cursor <= 0 {
		t.Fatalf("expected cursor>=0")
	}
	if string(output) != stdoutContent {
		t.Fatalf("expected output=%s, got %s", stdoutContent, string(output))
	}
}

func TestSeekBackgroundCommandOutput_WithRunBackgroundCommand(t *testing.T) {
	c := NewController("", "")

	expected := "line1\nline2\n"
	var session string
	req := &ExecuteCodeRequest{
		Language: BackgroundCommand,
		Code:     "printf 'line1\nline2\n'",
		Hooks: ExecuteResultHook{
			OnExecuteInit:     func(id string) { session = id },
			OnExecuteComplete: func(executionTime time.Duration) {},
			// other hooks unused in this test
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := c.runBackgroundCommand(ctx, cancel, req); err != nil {
		t.Fatalf("runBackgroundCommand error: %v", err)
	}
	if session == "" {
		t.Fatalf("session should be set by OnExecuteInit")
	}

	var (
		output []byte
		cursor int64
		err    error
	)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		output, cursor, err = c.SeekBackgroundCommandOutput(session, 0)
		if err == nil && len(output) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("SeekBackgroundCommandOutput error: %v", err)
	}
	if string(output) != expected {
		t.Fatalf("unexpected output: %q", string(output))
	}
	if cursor < int64(len(expected)) {
		t.Fatalf("cursor should advance to end of file, got %d", cursor)
	}

	// incremental seek from current cursor should return empty data and same-or-higher cursor
	output2, cursor2, err := c.SeekBackgroundCommandOutput(session, cursor)
	if err != nil {
		t.Fatalf("SeekBackgroundCommandOutput (second call) error: %v", err)
	}
	if len(output2) != 0 {
		t.Fatalf("expected no new output, got %q", string(output2))
	}
	if cursor2 < cursor {
		t.Fatalf("cursor should not move backwards: got %d < %d", cursor2, cursor)
	}
}
