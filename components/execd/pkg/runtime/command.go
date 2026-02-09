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
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
	"github.com/alibaba/opensandbox/execd/pkg/log"
	"github.com/alibaba/opensandbox/execd/pkg/util/safego"
)

// runCommand executes shell commands and streams their output.
func (c *Controller) runCommand(ctx context.Context, request *ExecuteCodeRequest) error {
	session := c.newContextID()

	signals := make(chan os.Signal, 1)
	defer close(signals)
	signal.Notify(signals)
	defer signal.Reset()

	stdout, stderr, err := c.stdLogDescriptor(session)
	if err != nil {
		return fmt.Errorf("failed to get stdlog descriptor: %w", err)
	}
	stdoutPath := c.stdoutFileName(session)
	stderrPath := c.stderrFileName(session)

	startAt := time.Now()
	log.Info("received command: %v", request.Code)
	cmd := exec.CommandContext(ctx, "bash", "-c", request.Code)

	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = mergeEnvs(os.Environ(), loadExtraEnvFromFile())

	done := make(chan struct{}, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	safego.Go(func() {
		defer wg.Done()
		c.tailStdPipe(stdoutPath, request.Hooks.OnExecuteStdout, done)
	})
	safego.Go(func() {
		defer wg.Done()
		c.tailStdPipe(stderrPath, request.Hooks.OnExecuteStderr, done)
	})

	cmd.Dir = request.Cwd
	// use a dedicated process group so signals propagate to children.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err = cmd.Start()
	if err != nil {
		request.Hooks.OnExecuteInit(session)
		request.Hooks.OnExecuteError(&execute.ErrorOutput{EName: "CommandExecError", EValue: err.Error()})
		log.Error("CommandExecError: error starting commands: %v", err)
		return nil
	}

	kernel := &commandKernel{
		pid:          cmd.Process.Pid,
		stdoutPath:   stdoutPath,
		stderrPath:   stderrPath,
		startedAt:    startAt,
		running:      true,
		content:      request.Code,
		isBackground: false,
	}
	c.storeCommandKernel(session, kernel)
	request.Hooks.OnExecuteInit(session)

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
	wg.Wait()
	if err != nil {
		var eName, eValue string
		var eCode int
		var traceback []string

		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode := exitError.ExitCode()
			eName = "CommandExecError"
			eValue = strconv.Itoa(exitCode)
			eCode = exitCode
		} else {
			eName = "CommandExecError"
			eValue = err.Error()
			eCode = 1
		}
		traceback = []string{err.Error()}

		request.Hooks.OnExecuteError(&execute.ErrorOutput{
			EName:     eName,
			EValue:    eValue,
			Traceback: traceback,
		})

		log.Error("CommandExecError: error running commands: %v", err)
		c.markCommandFinished(session, eCode, err.Error())
		return nil
	}

	c.markCommandFinished(session, 0, "")
	request.Hooks.OnExecuteComplete(time.Since(startAt))
	return nil
}

// runBackgroundCommand executes shell commands in detached mode.
func (c *Controller) runBackgroundCommand(ctx context.Context, cancel context.CancelFunc, request *ExecuteCodeRequest) error {
	session := c.newContextID()
	request.Hooks.OnExecuteInit(session)

	pipe, err := c.combinedOutputDescriptor(session)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to get combined output descriptor: %w", err)
	}
	stdoutPath := c.combinedOutputFileName(session)
	stderrPath := c.combinedOutputFileName(session)

	signals := make(chan os.Signal, 1)
	defer close(signals)
	signal.Notify(signals)
	defer signal.Reset()

	startAt := time.Now()
	log.Info("received command: %v", request.Code)
	cmd := exec.CommandContext(ctx, "bash", "-c", request.Code)

	cmd.Dir = request.Cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = pipe
	cmd.Stderr = pipe
	cmd.Env = mergeEnvs(os.Environ(), loadExtraEnvFromFile())

	// use DevNull as stdin so interactive programs exit immediately.
	cmd.Stdin = os.NewFile(uintptr(syscall.Stdin), os.DevNull)

	err = cmd.Start()
	kernel := &commandKernel{
		pid:          -1,
		stdoutPath:   stdoutPath,
		stderrPath:   stderrPath,
		startedAt:    startAt,
		running:      true,
		content:      request.Code,
		isBackground: true,
	}
	if err != nil {
		cancel()
		log.Error("CommandExecError: error starting commands: %v", err)
		kernel.running = false
		c.storeCommandKernel(session, kernel)
		c.markCommandFinished(session, 255, err.Error())
		return fmt.Errorf("failed to start commands: %w", err)
	}

	safego.Go(func() {
		defer pipe.Close()

		kernel.running = true
		kernel.pid = cmd.Process.Pid
		c.storeCommandKernel(session, kernel)

		err = cmd.Wait()
		cancel()
		if err != nil {
			log.Error("CommandExecError: error running commands: %v", err)
			exitCode := 1
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				exitCode = exitError.ExitCode()
			}
			c.markCommandFinished(session, exitCode, err.Error())
			return
		}
		c.markCommandFinished(session, 0, "")
	})

	// ensure we kill the whole process group if the context is cancelled (e.g., timeout).
	safego.Go(func() {
		<-ctx.Done()
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // best-effort
		}
	})

	request.Hooks.OnExecuteComplete(time.Since(startAt))
	return nil
}
