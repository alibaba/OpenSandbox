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
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// tailStdPipe streams appended log data until the process finishes.
func (c *Controller) tailStdPipe(file string, onExecute func(text string), done <-chan struct{}) {
	lastPos := int64(0)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	mutex := &sync.Mutex{}
	for {
		select {
		case <-done:
			c.readFromPos(mutex, file, lastPos, onExecute, true)
			return
		case <-ticker.C:
			newPos := c.readFromPos(mutex, file, lastPos, onExecute, false)
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
func (c *Controller) readFromPos(mutex *sync.Mutex, filepath string, startPos int64, onExecute func(string), flushIncomplete bool) int64 {
	if !mutex.TryLock() {
		return -1
	}
	defer mutex.Unlock()

	file, err := os.Open(filepath)
	if err != nil {
		return startPos
	}
	defer file.Close()

	_, _ = file.Seek(startPos, 0) //nolint:errcheck

	reader := bufio.NewReader(file)
	var buffer bytes.Buffer
	var currentPos int64 = startPos

	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				// If buffer has content but no newline, flush if needed, otherwise wait for next read
				if flushIncomplete && buffer.Len() > 0 {
					onExecute(buffer.String())
					buffer.Reset()
				}
			}
			break
		}
		currentPos++

		// Check if it's a line terminator (\n or \r)
		if b == '\n' || b == '\r' {
			// If buffer has content, output this line
			if buffer.Len() > 0 {
				onExecute(buffer.String())
				buffer.Reset()
			}
			// Skip line terminator
			continue
		}

		buffer.WriteByte(b)
	}

	endPos, _ := file.Seek(0, 1)
	// If the last read position doesn't end with a newline, return buffer start position and wait for next flush
	if !flushIncomplete && buffer.Len() > 0 {
		return currentPos - int64(buffer.Len())
	}
	return endPos
}
