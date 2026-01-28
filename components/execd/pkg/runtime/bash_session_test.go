// Copyright 2026 Alibaba Group Holding Ltd.
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
	"strings"
	"testing"
	"time"
)

func TestBashSessionEnvAndExitCode(t *testing.T) {
	session := newBashSession(nil)
	t.Cleanup(func() { _ = session.close() })

	if err := session.start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	var (
		initCalls     int
		completeCalls int
		stdoutLines   []string
	)

	hooks := ExecuteResultHook{
		OnExecuteInit: func(ctx string) {
			if ctx != session.config.Session {
				t.Fatalf("unexpected session in OnExecuteInit: %s", ctx)
			}
			initCalls++
		},
		OnExecuteStdout: func(text string) {
			t.Log(text)
			stdoutLines = append(stdoutLines, text)
		},
		OnExecuteComplete: func(_ time.Duration) {
			completeCalls++
		},
	}

	// 1) export an env var
	if err := session.run("export FOO=hello", 3*time.Second, &hooks); err != nil {
		t.Fatalf("runCommand(export) error = %v", err)
	}
	exportStdoutCount := len(stdoutLines)

	// 2) verify env is persisted
	if err := session.run("echo $FOO", 3*time.Second, &hooks); err != nil {
		t.Fatalf("runCommand(echo) error = %v", err)
	}
	echoLines := stdoutLines[exportStdoutCount:]
	foundHello := false
	for _, line := range echoLines {
		if strings.TrimSpace(line) == "hello" {
			foundHello = true
			break
		}
	}
	if !foundHello {
		t.Fatalf("expected echo $FOO to output 'hello', got %v", echoLines)
	}

	// 3) ensure exit code of previous command is reflected in shell state
	prevCount := len(stdoutLines)
	if err := session.run("false; echo EXIT:$?", 3*time.Second, &hooks); err != nil {
		t.Fatalf("runCommand(exitcode) error = %v", err)
	}
	exitLines := stdoutLines[prevCount:]
	foundExit := false
	for _, line := range exitLines {
		if strings.Contains(line, "EXIT:1") {
			foundExit = true
			break
		}
	}
	if !foundExit {
		t.Fatalf("expected exit code output 'EXIT:1', got %v", exitLines)
	}

	if initCalls != 3 {
		t.Fatalf("OnExecuteInit expected 3 calls, got %d", initCalls)
	}
	if completeCalls != 3 {
		t.Fatalf("OnExecuteComplete expected 3 calls, got %d", completeCalls)
	}
}

func TestBashSessionEnvLargeOutputChained(t *testing.T) {
	session := newBashSession(nil)
	t.Cleanup(func() { _ = session.close() })

	if err := session.start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	var (
		initCalls     int
		completeCalls int
		stdoutLines   []string
	)

	hooks := ExecuteResultHook{
		OnExecuteInit: func(ctx string) {
			if ctx != session.config.Session {
				t.Fatalf("unexpected session in OnExecuteInit: %s", ctx)
			}
			initCalls++
		},
		OnExecuteStdout: func(text string) {
			t.Log(text)
			stdoutLines = append(stdoutLines, text)
		},
		OnExecuteComplete: func(_ time.Duration) {
			completeCalls++
		},
	}

	runAndCollect := func(cmd string) []string {
		start := len(stdoutLines)
		if err := session.run(cmd, 10*time.Second, &hooks); err != nil {
			t.Fatalf("runCommand(%q) error = %v", cmd, err)
		}
		return append([]string(nil), stdoutLines[start:]...)
	}

	lines1 := runAndCollect("export FOO=hello1; for i in $(seq 1 60); do echo A${i}:$FOO; done")
	if len(lines1) < 60 {
		t.Fatalf("expected >=60 lines for cmd1, got %d", len(lines1))
	}
	if !containsLine(lines1, "A1:hello1") || !containsLine(lines1, "A60:hello1") {
		t.Fatalf("env not reflected in cmd1 output, got %v", lines1[:3])
	}

	lines2 := runAndCollect("export FOO=${FOO}_next; export BAR=bar1; for i in $(seq 1 60); do echo B${i}:$FOO:$BAR; done")
	if len(lines2) < 60 {
		t.Fatalf("expected >=60 lines for cmd2, got %d", len(lines2))
	}
	if !containsLine(lines2, "B1:hello1_next:bar1") || !containsLine(lines2, "B60:hello1_next:bar1") {
		t.Fatalf("env not propagated to cmd2 output, sample %v", lines2[:3])
	}

	lines3 := runAndCollect("export BAR=${BAR}_last; for i in $(seq 1 60); do echo C${i}:$FOO:$BAR; done; echo FINAL_FOO=$FOO; echo FINAL_BAR=$BAR")
	if len(lines3) < 62 { // 60 lines + 2 finals
		t.Fatalf("expected >=62 lines for cmd3, got %d", len(lines3))
	}
	if !containsLine(lines3, "C1:hello1_next:bar1_last") || !containsLine(lines3, "C60:hello1_next:bar1_last") {
		t.Fatalf("env not propagated to cmd3 output, sample %v", lines3[:3])
	}
	if !containsLine(lines3, "FINAL_FOO=hello1_next") || !containsLine(lines3, "FINAL_BAR=bar1_last") {
		t.Fatalf("final env lines missing, got %v", lines3[len(lines3)-5:])
	}

	if initCalls != 3 {
		t.Fatalf("OnExecuteInit expected 3 calls, got %d", initCalls)
	}
	if completeCalls != 3 {
		t.Fatalf("OnExecuteComplete expected 3 calls, got %d", completeCalls)
	}
}

func containsLine(lines []string, target string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) == target {
			return true
		}
	}
	return false
}
