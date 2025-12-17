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
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/beego/beego/v2/core/logs"
)

// Interrupt stops execution in the specified session.
func (c *Controller) Interrupt(sessionID string) error {
	switch {
	case c.getJupyterKernel(sessionID) != nil:
		kernel := c.getJupyterKernel(sessionID)
		logs.Warning("Interrupting Jupyter kernel %s", kernel.kernelID)
		return kernel.client.InterruptKernel(kernel.kernelID)
	case c.getCommandKernel(sessionID) != nil:
		kernel := c.getCommandKernel(sessionID)
		return c.killPid(kernel.pid)
	default:
		return errors.New("no such session")
	}
}

// killPid sends SIGTERM followed by SIGKILL if needed.
func (c *Controller) killPid(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	logs.Warning("Attempting to terminate process %d", pid)

	if err := process.Signal(syscall.SIGTERM); err != nil {
		if strings.Contains(err.Error(), "already finished") {
			return nil
		}
		logs.Warning("SIGTERM failed for pid %d: %v, trying SIGKILL", pid, err)
	} else {
		done := make(chan error, 1)
		go func() {
			_, err := process.Wait()
			done <- err
		}()

		select {
		case err := <-done:
			if err == nil {
				logs.Info("Process %d terminated gracefully", pid)
				return nil
			}
		case <-time.After(3 * time.Second):
			logs.Warning("Process %d did not terminate after SIGTERM, using SIGKILL", pid)
		}
	}

	if err := process.Signal(syscall.SIGKILL); err != nil {
		if strings.Contains(err.Error(), "already finished") {
			return nil
		}
		return fmt.Errorf("failed to kill process %d: %w", pid, err)
	}

	for range 3 {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			if strings.Contains(err.Error(), "already finished") ||
				strings.Contains(err.Error(), "no such process") {
				logs.Info("Process %d confirmed terminated", pid)
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("process %d might still be running", pid)
}
