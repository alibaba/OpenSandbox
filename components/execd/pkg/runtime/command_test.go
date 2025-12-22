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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFromPos_SplitsOnCRAndLF(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "stdout.log")

	initial := "line1\nprog 10%\rprog 20%\rprog 30%\nlast\n"
	if err := os.WriteFile(logFile, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	var got []string
	c := &Controller{}
	nextPos := c.readFromPos(logFile, 0, func(s string) { got = append(got, s) })

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
	c.readFromPos(logFile, nextPos, func(s string) { got = append(got, s) })
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
	c.readFromPos(logFile, 0, func(s string) { got = append(got, s) })

	if len(got) != 1 {
		t.Fatalf("expected one token, got %d", len(got))
	}
	if got[0] != strings.TrimSuffix(longLine, "\n") {
		t.Fatalf("long line mismatch: got %d chars want %d chars", len(got[0]), len(longLine)-1)
	}
}
