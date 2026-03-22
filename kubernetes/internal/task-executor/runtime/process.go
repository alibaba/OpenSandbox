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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/config"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/types"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/utils"
)

const (
	ExitFile   = "exit"
	PidFile    = "pid"
	StdoutFile = "stdout.log"
	StderrFile = "stderr.log"
)

// processExecutor handles both Host and Sidecar modes as they share the same
// shim-based process execution model.
type processExecutor struct {
	config  *config.Config
	rootDir string
}

func NewProcessExecutor(config *config.Config) (Executor, error) {
	return &processExecutor{rootDir: config.DataDir, config: config}, nil
}

func (e *processExecutor) Start(ctx context.Context, task *types.Task) error {
	if task == nil {
		return fmt.Errorf("task cannot be nil")
	}
	taskDir, err := utils.SafeJoin(e.rootDir, task.Name)
	if err != nil {
		return fmt.Errorf("invalid task name: %w", err)
	}
	pidPath := filepath.Join(taskDir, PidFile)
	exitPath := filepath.Join(taskDir, ExitFile)

	var cmdList []string
	if task.Process != nil {
		cmdList = append(task.Process.Command, task.Process.Args...)
	} else {
		return fmt.Errorf("process spec is required for process executor but task.Process is nil (task name: %s)", task.Name)
	}

	if len(cmdList) == 0 {
		return fmt.Errorf("no command specified in process spec (task name: %s)", task.Name)
	}

	safeCmdStr := shellEscape(cmdList)
	shimScript := e.buildShimScript(exitPath, safeCmdStr)

	var cmd *exec.Cmd

	if e.config.EnableSidecarMode {
		targetPID, err := e.findPidByEnvVar("SANDBOX_MAIN_CONTAINER", e.config.MainContainerName)
		if err != nil {
			return fmt.Errorf("failed to resolve target PID: %w", err)
		}

		targetEnv, err := getProcEnviron(targetPID)
		if err != nil {
			return fmt.Errorf("failed to read target process environment: %w", err)
		}

		nsenterArgs := []string{
			"-t", strconv.Itoa(targetPID),
			"--mount", "--uts", "--ipc", "--net", "--pid",
			"--",
			"/bin/sh", "-c", shimScript,
		}
		cmd = exec.Command("nsenter", nsenterArgs...)
		cmd.Env = targetEnv
		klog.InfoS("Starting sidecar task", "id", task.Name, "targetPID", targetPID)

	} else {
		cmd = exec.Command("/bin/sh", "-c", shimScript)
		cmd.Env = os.Environ()
		klog.InfoS("Starting host task", "name", task.Name, "cmd", safeCmdStr, "exitPath", exitPath)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	return e.executeCommand(task, cmd, pidPath)
}

// executeCommand handles log setup and process starting
func (e *processExecutor) executeCommand(task *types.Task, cmd *exec.Cmd, pidPath string) error {
	if task == nil || cmd == nil {
		return fmt.Errorf("task and cmd cannot be nil")
	}

	taskDir, err := utils.SafeJoin(e.rootDir, task.Name)
	if err != nil {
		return fmt.Errorf("invalid task name: %w", err)
	}

	stdoutPath := filepath.Join(taskDir, StdoutFile)
	stderrPath := filepath.Join(taskDir, StderrFile)

	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open stdout: %w", err)
	}

	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		stdoutFile.Close()
		return fmt.Errorf("failed to open stderr: %w", err)
	}

	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	if task.Process != nil {
		for _, env := range task.Process.Env {
			if env.Name != "" {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", env.Name, env.Value))
			}
		}

		if task.Process.WorkingDir != "" {
			cmd.Dir = task.Process.WorkingDir
			klog.InfoS("Set working directory", "name", task.Name, "workingDir", task.Process.WorkingDir)
		}
	}

	if err := cmd.Start(); err != nil {
		klog.ErrorS(err, "failed to start command", "name", task.Name)
		stdoutFile.Close()
		stderrFile.Close()
		return fmt.Errorf("failed to start cmd: %w", err)
	}

	// Write PID to file immediately (Host-side PID)
	// This fixes the issue where sidecar tasks would write the container-internal PID
	pid := cmd.Process.Pid
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		klog.ErrorS(err, "failed to write pid file", "name", task.Name)
		_ = cmd.Process.Kill()
		stdoutFile.Close()
		stderrFile.Close()
		return fmt.Errorf("failed to write pid file: %w", err)
	}

	klog.InfoS("Task command started successfully", "name", task.Name, "pid", pid)

	stdoutFile.Close()
	stderrFile.Close()

	go func() {
		if err := cmd.Wait(); err != nil {
			klog.ErrorS(err, "task process exited with error", "name", task.Name)
		} else {
			klog.InfoS("task process exited successfully", "name", task.Name)
		}
	}()
	return nil
}

func (e *processExecutor) buildShimScript(exitPath, cmdStr string) string {
	// The shim script acts as a mini-init process.
	// 1. It runs the user command in the background.
	// 2. It traps SIGTERM and forwards it to the child process.
	// 3. It waits for the child to exit and captures the exit code.
	// This ensures graceful shutdown propagation in sidecar/host modes.
	script := fmt.Sprintf(`
cleanup() {
    if [ -n "$CHILD_PID" ]; then
        kill -TERM "$CHILD_PID" 2>/dev/null
    fi
}
trap cleanup TERM

%s &
CHILD_PID=$!
wait "$CHILD_PID"
EXIT_CODE=$?

printf "%%d" $EXIT_CODE > %s
exit $EXIT_CODE
`, cmdStr, shellEscapePath(exitPath))
	klog.InfoS("Generated shim script", "exitPath", exitPath, "script", script)
	return script
}

func (e *processExecutor) Inspect(ctx context.Context, task *types.Task) (*types.Status, error) {
	taskDir, err := utils.SafeJoin(e.rootDir, task.Name)
	if err != nil {
		return nil, fmt.Errorf("invalid task name: %w", err)
	}
	exitPath := filepath.Join(taskDir, ExitFile)
	pidPath := filepath.Join(taskDir, PidFile)

	status := &types.Status{
		State: types.TaskStateUnknown,
	}
	subStatus := types.SubStatus{}
	var pid int
	if exitData, err := os.ReadFile(exitPath); err == nil {
		fileInfo, _ := os.Stat(exitPath)
		exitCode, _ := strconv.Atoi(string(exitData))

		subStatus.ExitCode = exitCode
		finishedAt := fileInfo.ModTime()
		subStatus.FinishedAt = &finishedAt

		if exitCode == 0 {
			status.State = types.TaskStateSucceeded
			subStatus.Reason = "Succeeded"
		} else {
			status.State = types.TaskStateFailed
			subStatus.Reason = "Failed"
		}

		if pidFileInfo, err := os.Stat(pidPath); err == nil {
			startedAt := pidFileInfo.ModTime()
			subStatus.StartedAt = &startedAt
		}

		status.SubStatuses = []types.SubStatus{subStatus}
		return status, nil
	}

	if pidData, err := os.ReadFile(pidPath); err == nil {
		pid, _ = strconv.Atoi(strings.TrimSpace(string(pidData)))
		fileInfo, _ := os.Stat(pidPath)
		startedAt := fileInfo.ModTime()
		subStatus.StartedAt = &startedAt

		if isProcessRunning(pid) {
			status.State = types.TaskStateRunning
			if task.Process != nil && task.Process.TimeoutSeconds != nil {
				timeout := time.Duration(*task.Process.TimeoutSeconds) * time.Second
				elapsed := time.Since(startedAt)
				if elapsed > timeout {
					status.State = types.TaskStateTimeout
					subStatus.Reason = "TaskTimeout"
					subStatus.Message = fmt.Sprintf("Task exceeded timeout of %d seconds", *task.Process.TimeoutSeconds)
				}
			}
		} else {
			status.State = types.TaskStateFailed
			subStatus.ExitCode = 137
			subStatus.Reason = "ProcessCrashed"
			subStatus.Message = "Process exited without writing exit code"
			subStatus.FinishedAt = &startedAt
		}
		status.SubStatuses = []types.SubStatus{subStatus}
		return status, nil
	}

	status.State = types.TaskStatePending
	subStatus.Reason = "Pending"
	status.SubStatuses = []types.SubStatus{subStatus}

	return status, nil
}

func (e *processExecutor) Stop(ctx context.Context, task *types.Task) error {
	taskDir, err := utils.SafeJoin(e.rootDir, task.Name)
	if err != nil {
		return fmt.Errorf("invalid task name: %w", err)
	}
	pidPath := filepath.Join(taskDir, PidFile)
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return nil
	}
	var pid int
	pid, err = strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil || pid == 0 {
		return nil
	}
	klog.InfoS("Read PID from pid file", "name", task.Name, "pid", pid)

	pgid := -pid

	targetPID := 0
	if e.config.EnableSidecarMode {
		children, err := getChildrenPIDs(pid)
		if err == nil && len(children) > 0 {
			targetPID = children[0]
			klog.InfoS("Sidecar mode: targeted Shim process via /proc/children", "nsenterPID", pid, "shimPID", targetPID)
		} else {
			klog.Warning("Sidecar mode: failed to find child process via /proc/children, falling back to PGID", "pid", pid, "err", err)
		}
	} else {
		targetPID = pid
	}

	killedShim := false
	if targetPID > 0 {
		if err := syscall.Kill(targetPID, syscall.SIGTERM); err == nil {
			killedShim = true
		} else if err != syscall.ESRCH {
			klog.ErrorS(err, "Failed to send SIGTERM to target process", "targetPID", targetPID)
		}
	}

	if !killedShim {
		_ = syscall.Kill(pgid, syscall.SIGTERM)
	}

	timeout := 10 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessRunning(pid) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	klog.InfoS("Process did not exit after timeout, sending SIGKILL", "pgid", pgid)
	if targetPID > 0 {
		_ = syscall.Kill(targetPID, syscall.SIGKILL)
	}
	_ = syscall.Kill(pgid, syscall.SIGKILL)

	return nil
}

// getChildrenPIDs reads /proc/<pid>/task/<pid>/children to find direct children
func getChildrenPIDs(pid int) ([]int, error) {
	path := fmt.Sprintf("/proc/%d/task/%d/children", pid, pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pids []int
	for _, field := range strings.Fields(string(data)) {
		if id, err := strconv.Atoi(field); err == nil {
			pids = append(pids, id)
		}
	}
	return pids, nil
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// shellEscape quotes arguments for safe shell execution
func shellEscape(args []string) string {
	quoted := make([]string, len(args))
	for i, s := range args {
		quoted[i] = shellEscapePath(s)
	}
	return strings.Join(quoted, " ")
}

// shellEscapePath escapes a single string for safe shell execution.
// It wraps the string in single quotes and escapes any embedded single quotes.
// e.g., foo'bar -> 'foo'\”bar'
func shellEscapePath(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// findPidByEnvVar finds a process by checking for a specific environment variable
func (e *processExecutor) findPidByEnvVar(envName, expectedValue string) (int, error) {
	procDir, err := os.Open("/proc")
	if err != nil {
		return 0, fmt.Errorf("failed to open /proc: %w", err)
	}
	defer procDir.Close()

	entries, err := procDir.Readdirnames(-1)
	if err != nil {
		return 0, fmt.Errorf("failed to read /proc entries: %w", err)
	}

	selfPID := os.Getpid()
	targetEnv := fmt.Sprintf("%s=%s", envName, expectedValue)

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry)
		if err != nil {
			continue
		}
		if pid == selfPID {
			continue
		}

		// Read process environment
		envPath := filepath.Join("/proc", entry, "environ")
		envData, err := os.ReadFile(envPath)
		if err != nil {
			continue
		}

		// Environment variables are null-separated
		envVars := strings.Split(string(envData), "\x00")
		for _, env := range envVars {
			if env == targetEnv {
				klog.InfoS("Found main container by environment variable", "pid", pid, "env", targetEnv)
				return pid, nil
			}
		}
	}

	return 0, fmt.Errorf("no process found with environment variable %s=%s", envName, expectedValue)
}

// getProcEnviron reads environment variables from /proc/<pid>/environ
func getProcEnviron(pid int) ([]string, error) {
	envPath := filepath.Join("/proc", strconv.Itoa(pid), "environ")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return nil, err
	}

	// Environment variables in /proc/<pid>/environ are separated by null bytes
	var envs []string
	for _, env := range strings.Split(string(data), "\x00") {
		if len(env) > 0 {
			envs = append(envs, env)
		}
	}
	return envs, nil
}

// RestartMainContainer restarts the main container by sending SIGTERM
// This is a public method that can be called directly for reset operations.
func (e *processExecutor) RestartMainContainer(ctx context.Context, mainContainerName string) error {
	return e.restartMainContainer(ctx, mainContainerName)
}

// restartMainContainer restarts the main container by sending SIGTERM first,
// then SIGKILL if the process doesn't exit gracefully.
func (e *processExecutor) restartMainContainer(ctx context.Context, mainContainerName string) error {
	// Find the main container PID by environment variable
	mainPID, err := e.findPidByEnvVar("SANDBOX_MAIN_CONTAINER", mainContainerName)
	if err != nil {
		return fmt.Errorf("failed to find main container: %w", err)
	}

	klog.InfoS("Found main container for restart", "pid", mainPID, "container", mainContainerName)

	// Step 1: Send SIGTERM for graceful shutdown
	if err := syscall.Kill(mainPID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM to main container: %w", err)
	}

	klog.InfoS("Sent SIGTERM to main container", "pid", mainPID)

	// Step 2: Wait for process to exit gracefully (container runtime will restart it based on restartPolicy)
	gracefulTimeout := 30 * time.Second
	gracefulDeadline := time.Now().Add(gracefulTimeout)
	for time.Now().Before(gracefulDeadline) {
		if !isProcessRunning(mainPID) {
			klog.InfoS("Main container process exited gracefully", "pid", mainPID)
			// Step 2.1: Wait for the new container to start
			if err := e.waitForNewContainer(ctx, mainPID, mainContainerName); err != nil {
				return fmt.Errorf("failed to wait for new container: %w", err)
			}
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Step 3: Process didn't exit gracefully, send SIGKILL for forceful termination
	klog.InfoS("Main container did not exit gracefully, sending SIGKILL", "pid", mainPID)
	if err := syscall.Kill(mainPID, syscall.SIGKILL); err != nil {
		return fmt.Errorf("failed to send SIGKILL to main container: %w", err)
	}

	// Step 4: Wait for SIGKILL to take effect
	forceKillWaitTimeout := 5 * time.Second
	forceKillDeadline := time.Now().Add(forceKillWaitTimeout)
	for time.Now().Before(forceKillDeadline) {
		if !isProcessRunning(mainPID) {
			klog.InfoS("Main container process killed forcefully", "pid", mainPID)
			// Step 4.1: Wait for the new container to start
			if err := e.waitForNewContainer(ctx, mainPID, mainContainerName); err != nil {
				return fmt.Errorf("failed to wait for new container: %w", err)
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Step 5: Process still running after SIGKILL - this is an error
	return fmt.Errorf("main container still running after SIGKILL, pid: %d", mainPID)
}

// waitForNewContainer waits for the new main container to start after the old one was terminated.
// It polls for a new PID with the same SANDBOX_MAIN_CONTAINER environment variable,
// ensuring the replacement container is running before returning.
func (e *processExecutor) waitForNewContainer(ctx context.Context, oldPID int, mainContainerName string) error {
	klog.InfoS("Waiting for new main container to start", "oldPID", oldPID, "container", mainContainerName)

	// Total timeout for new container to start
	startTimeout := 60 * time.Second
	startDeadline := time.Now().Add(startTimeout)

	// Small buffer after finding new PID to ensure container is fully ready
	readyBuffer := 2 * time.Second

	for time.Now().Before(startDeadline) {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled while waiting for new container: %w", ctx.Err())
		default:
		}

		// Try to find a new PID for the main container
		newPID, err := e.findPidByEnvVar("SANDBOX_MAIN_CONTAINER", mainContainerName)
		if err == nil && newPID != oldPID && isProcessRunning(newPID) {
			klog.InfoS("Found new main container process", "newPID", newPID, "oldPID", oldPID)

			// Wait a short buffer to ensure the container is fully ready
			time.Sleep(readyBuffer)

			// Verify the new PID is still running after the buffer
			if isProcessRunning(newPID) {
				klog.InfoS("New main container is ready", "newPID", newPID)
				return nil
			}
			klog.InfoS("New main container process exited during buffer, continuing to wait", "newPID", newPID)
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for new main container to start after %v", startTimeout)
}
