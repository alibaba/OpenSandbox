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
	"bufio"
	"io"
	"os"
	"path/filepath"
	"time"
)

// tailStdPipe streams appended log data until the process finishes.
func (c *Controller) tailStdPipe(file string, onExecute func(text string), done <-chan struct{}) {
	lastPos := int64(0)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			c.readFromPos(file, lastPos, onExecute)
			return
		case <-ticker.C:
			newPos := c.readFromPos(file, lastPos, onExecute)
			lastPos = newPos
		}
	}
}

// getCommandKernel retrieves a command execution context.
func (c *Controller) getCommandKernel(sessionID string) *commandKernel {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.commandClientMap[sessionID]
}

// storeCommandKernel registers a command execution context.
func (c *Controller) storeCommandKernel(sessionID string, kernel *commandKernel) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.commandClientMap[sessionID] = kernel
}

// stdLogDescriptor creates temporary files for capturing command output.
func (c *Controller) stdLogDescriptor(session string) (io.WriteCloser, io.WriteCloser, error) {
	stdout, err := os.OpenFile(c.stdoutFileName(session), os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return nil, nil, err
	}
	stderr, err := os.OpenFile(c.stderrFileName(session), os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return nil, nil, err
	}

	return stdout, stderr, nil
}

func (c *Controller) combinedOutputDescriptor(session string) (io.WriteCloser, error) {
	return os.OpenFile(c.combinedOutputFileName(session), os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
}

// stdoutFileName constructs the stdout log path.
func (c *Controller) stdoutFileName(session string) string {
	return filepath.Join(os.TempDir(), session+".stdout")
}

// stderrFileName constructs the stderr log path.
func (c *Controller) stderrFileName(session string) string {
	return filepath.Join(os.TempDir(), session+".stderr")
}

func (c *Controller) combinedOutputFileName(session string) string {
	return filepath.Join(os.TempDir(), session+".output")
}

// readFromPos streams new content from a file starting at startPos.
func (c *Controller) readFromPos(filepath string, startPos int64, onExecute func(string)) int64 {
	file, err := os.Open(filepath)
	if err != nil {
		return startPos
	}
	defer file.Close()

	_, _ = file.Seek(startPos, 0) //nolint:errcheck

	scanner := bufio.NewScanner(file)
	// Support long lines and treat both \n and \r as delimiters to keep progress output.
	scanner.Buffer(make([]byte, 0, 64*1024), 5*1024*1024) // 5MB max token
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		for i, b := range data {
			if b == '\n' || b == '\r' {
				// Treat \r\n as a single delimiter to avoid empty tokens.
				if b == '\r' && i+1 < len(data) && data[i+1] == '\n' {
					return i + 2, data[:i], nil
				}
				return i + 1, data[:i], nil
			}
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})

	for scanner.Scan() {
		onExecute(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return startPos
	}

	endPos, _ := file.Seek(0, 1)
	return endPos
}
