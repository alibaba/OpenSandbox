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
	"fmt"
	"os"
	"time"
)

// CommandStatus describes the lifecycle state of a command.
type CommandStatus struct {
	Session    string     `json:"session"`
	Running    bool       `json:"running"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// CommandOutput contains non-streamed stdout/stderr plus status.
type CommandOutput struct {
	CommandStatus
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

func (c *Controller) commandSnapshot(session string) *commandKernel {
	c.mu.RLock()
	defer c.mu.RUnlock()

	kernel, ok := c.commandClientMap[session]
	if !ok || kernel == nil {
		return nil
	}

	cp := *kernel
	return &cp
}

// GetCommandStatus returns the execution status for a command session.
func (c *Controller) GetCommandStatus(session string) (*CommandStatus, error) {
	kernel := c.commandSnapshot(session)
	if kernel == nil {
		return nil, fmt.Errorf("command not found: %s", session)
	}

	status := &CommandStatus{
		Session:    session,
		Running:    kernel.running,
		ExitCode:   kernel.exitCode,
		Error:      kernel.errMsg,
		StartedAt:  kernel.startedAt,
		FinishedAt: kernel.finishedAt,
	}
	return status, nil
}

// GetCommandOutput returns accumulated stdout/stderr and status for a session.
func (c *Controller) GetCommandOutput(session string) (*CommandOutput, error) {
	kernel := c.commandSnapshot(session)
	if kernel == nil {
		return nil, fmt.Errorf("command not found: %s", session)
	}

	status, err := c.GetCommandStatus(session)
	if err != nil {
		return nil, err
	}

	stdout, err := os.ReadFile(kernel.stdoutPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read stdout: %w", err)
	}
	stderr, err := os.ReadFile(kernel.stderrPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read stderr: %w", err)
	}

	return &CommandOutput{
		CommandStatus: *status,
		Stdout:        string(stdout),
		Stderr:        string(stderr),
	}, nil
}

// markCommandFinished updates bookkeeping when a command exits.
func (c *Controller) markCommandFinished(session string, exitCode int, errMsg string) {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	kernel, ok := c.commandClientMap[session]
	if !ok || kernel == nil {
		return
	}

	kernel.exitCode = &exitCode
	kernel.errMsg = errMsg
	kernel.running = false
	kernel.finishedAt = &now
}
