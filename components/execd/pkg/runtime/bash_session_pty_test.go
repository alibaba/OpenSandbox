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
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readOutputTimeout drains r for up to d, returning all collected bytes.
func readOutputTimeout(r io.Reader, d time.Duration) string {
	var buf strings.Builder
	deadline := time.Now().Add(d)
	tmp := make([]byte, 256)
	for time.Now().Before(deadline) {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			break
		}
	}
	return buf.String()
}

// TestPTY_BasicExecution verifies that a PTY session can run a command and
// the output is received on the PTY master.
func TestPTY_BasicExecution(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	s := newBashSession(t.TempDir())
	t.Cleanup(func() { _ = s.close() })

	require.NoError(t, s.StartPTY())
	require.True(t, s.IsRunning(), "expected bash process to be running after StartPTY")

	// Send a command via stdin.
	_, err := s.WriteStdin([]byte("echo hi\n"))
	require.NoError(t, err)

	// Read output via AttachOutput.
	outR, _, detach := s.AttachOutput()
	defer detach()
	out := readOutputTimeout(outR, 3*time.Second)
	assert.Contains(t, out, "hi", "expected 'hi' in PTY output, got: %q", out)
}

// TestPTY_ResizeUpdatesWinsize verifies that ResizePTY changes the terminal
// dimensions reported by the PTY (no error path; structural change verified).
func TestPTY_ResizeUpdatesWinsize(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	s := newBashSession(t.TempDir())
	t.Cleanup(func() { _ = s.close() })

	require.NoError(t, s.StartPTY())

	// Resize to known dimensions.
	require.NoError(t, s.ResizePTY(120, 40))

	// Verify via pty.GetsizeFull that the kernel registered the new size.
	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	require.NotNil(t, ptmx)

	ws, err := pty.GetsizeFull(ptmx)
	require.NoError(t, err)
	assert.Equal(t, uint16(120), ws.Cols, "expected cols=120 after resize")
	assert.Equal(t, uint16(40), ws.Rows, "expected rows=40 after resize")
}

// TestPTY_AnsiSequencesPresent verifies that PTY output contains ANSI escape
// sequences (the prompt), which distinguishes PTY mode from plain pipe mode.
func TestPTY_AnsiSequencesPresent(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	s := newBashSession(t.TempDir())
	t.Cleanup(func() { _ = s.close() })

	require.NoError(t, s.StartPTY())

	// Send a command that forces a prompt re-emission.
	_, err := s.WriteStdin([]byte("PS1='\\e[1;32m>>\\e[0m '; echo marker\n"))
	require.NoError(t, err)

	outR, _, detach := s.AttachOutput()
	defer detach()
	out := readOutputTimeout(outR, 3*time.Second)
	// ANSI escape sequences start with ESC (\x1b) followed by [
	assert.Contains(t, out, "\x1b[", "expected ANSI escape sequence in PTY output, got: %q", out)
	assert.Contains(t, out, "marker", "expected 'marker' in PTY output, got: %q", out)
}

// TestPTY_PipeModeUnchanged verifies that a session created without ?pty=1
// still uses plain pipes and has no PTY fd open — regression guard.
func TestPTY_PipeModeUnchanged(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	s := newBashSession(t.TempDir())
	t.Cleanup(func() { _ = s.close() })

	require.NoError(t, s.Start())
	require.True(t, s.IsRunning(), "expected bash process to be running")

	// PTY fields must be unset in pipe mode.
	s.mu.Lock()
	isPTY := s.isPTY
	ptmx := s.ptmx
	s.mu.Unlock()

	assert.False(t, isPTY, "isPTY must be false in pipe mode")
	assert.Nil(t, ptmx, "ptmx must be nil in pipe mode")

	// ResizePTY must be a no-op (no error) when not in PTY mode.
	require.NoError(t, s.ResizePTY(100, 30))

	// Attach output first so the broadcast goroutine has a PipeWriter in place,
	// then write stdin to avoid a race where output lands only in the replay buffer.
	outR, _, detach := s.AttachOutput()
	defer detach()

	// Stdin must still work via pipe.
	_, err := s.WriteStdin([]byte("echo pipe-ok\n"))
	require.NoError(t, err)

	// Poll the replay buffer until output appears — this is reliable regardless of
	// whether the output arrived before or after AttachOutput installed the PipeWriter.
	deadline := time.Now().Add(5 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		data, _ := s.replay.readFrom(0)
		got = string(data)
		if strings.Contains(got, "pipe-ok") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = outR // attached above; detach() will clean up
	assert.Contains(t, got, "pipe-ok", "expected pipe-mode echo output in replay buffer, got: %q", got)
}
