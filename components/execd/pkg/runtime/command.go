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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/beego/beego/v2/core/logs"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
	"github.com/alibaba/opensandbox/execd/pkg/util/safego"
)

// runCommand executes shell commands and streams their output.
func (c *Controller) runCommand(ctx context.Context, request *ExecuteCodeRequest) error {
	session := c.newContextID()
	request.Hooks.OnExecuteInit(session)

	signals := make(chan os.Signal, 1)
	defer close(signals)
	signal.Notify(signals)
	defer signal.Reset()

	stdout, stderr, err := c.stdLogDescriptor(session)
	if err != nil {
		return fmt.Errorf("failed to get stdlog descriptor: %w", err)
	}

	startAt := time.Now()
	logs.Info("received command: %v", request.Code)
	cmd := exec.CommandContext(ctx, "bash", "-c", request.Code)

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	done := make(chan struct{}, 1)
	safego.Go(func() {
		c.tailStdPipe(c.stdoutFileName(session), request.Hooks.OnExecuteStdout, done)
	})
	safego.Go(func() {
		c.tailStdPipe(c.stderrFileName(session), request.Hooks.OnExecuteStderr, done)
	})

	cmd.Dir = request.Cwd
	cmd.Env = os.Environ()
	// use a dedicated process group so signals propagate to children.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err = cmd.Start()
	if err != nil {
		request.Hooks.OnExecuteError(&execute.ErrorOutput{EName: "CommandExecError", EValue: err.Error()})
		logs.Error("CommandExecError: error starting commands: %v", err)
		return nil
	}

	kernel := &commandKernel{
		pid: cmd.Process.Pid,
	}
	c.storeCommandKernel(session, kernel)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sig := <-signals:
				if sig == nil {
					continue
				}
				// DO NOT forward syscall.SIGURG to children processes.
				if sig != syscall.SIGCHLD && sig != syscall.SIGURG {
					_ = syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
				}
			}
		}
	}()

	err = cmd.Wait()
	close(done)
	if err != nil {
		var eName, eValue string
		var traceback []string

		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode := exitError.ExitCode()
			eName = "CommandExecError"
			eValue = strconv.Itoa(exitCode)
		} else {
			eName = "CommandExecError"
			eValue = err.Error()
		}
		traceback = []string{err.Error()}

		request.Hooks.OnExecuteError(&execute.ErrorOutput{
			EName:     eName,
			EValue:    eValue,
			Traceback: traceback,
		})

		logs.Error("CommandExecError: error running commands: %v", err)
		return nil
	}
	request.Hooks.OnExecuteComplete(time.Since(startAt))
	return nil
}

// runBackgroundCommand executes shell commands in detached mode.
func (c *Controller) runBackgroundCommand(_ context.Context, request *ExecuteCodeRequest) error {
	session := c.newContextID()
	request.Hooks.OnExecuteInit(session)

	signals := make(chan os.Signal, 1)
	defer close(signals)
	signal.Notify(signals)
	defer signal.Reset()

	startAt := time.Now()
	logs.Info("received command: %v", request.Code)
	cmd := exec.CommandContext(context.Background(), "bash", "-c", request.Code)

	cmd.Dir = request.Cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// use DevNull as stdin so interactive programs exit immediately.
	cmd.Stdin = os.NewFile(uintptr(syscall.Stdin), os.DevNull)

	safego.Go(func() {
		err := cmd.Start()
		if err != nil {
			logs.Error("CommandExecError: error starting commands: %v", err)
		}

		kernel := &commandKernel{
			pid: cmd.Process.Pid,
		}
		c.storeCommandKernel(session, kernel)

		err = cmd.Wait()
		if err != nil {
			logs.Error("CommandExecError: error running commands: %v", err)
		}
	})

	request.Hooks.OnExecuteComplete(time.Since(startAt))
	return nil
}

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

// stdoutFileName constructs the stdout log path.
func (c *Controller) stdoutFileName(session string) string {
	return filepath.Join(os.TempDir(), session+".stdout")
}

// stderrFileName constructs the stderr log path.
func (c *Controller) stderrFileName(session string) string {
	return filepath.Join(os.TempDir(), session+".stderr")
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
