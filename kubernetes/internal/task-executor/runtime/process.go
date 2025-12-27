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

	// 1. Construct the user command securely
	var cmdList []string
	if task.Spec.Process != nil {
		cmdList = append(task.Spec.Process.Command, task.Spec.Process.Args...)
	} else {
		return fmt.Errorf("process spec is required for process executor but task.Spec.Process is nil (task name: %s)", task.Name)
	}

	if len(cmdList) == 0 {
		return fmt.Errorf("no command specified in process spec (task name: %s)", task.Name)
	}

	// Use shell escaping to prevent command injection
	safeCmdStr := shellEscape(cmdList)
	shimScript := e.buildShimScript(exitPath, safeCmdStr)

	// 2. Prepare the execution command based on mode
	var cmd *exec.Cmd

	if e.config.EnableSidecarMode {
		// Sidecar Logic: Find target PID and use nsenter
		targetPID, err := e.findPidByEnvVar("SANDBOX_MAIN_CONTAINER", e.config.MainContainerName)
		if err != nil {
			return fmt.Errorf("failed to resolve target PID: %w", err)
		}

		nsenterArgs := []string{
			"-t", strconv.Itoa(targetPID),
			"--mount", "--uts", "--ipc", "--net", "--pid",
			"--",
			"/bin/sh", "-c", shimScript,
		}
		cmd = exec.Command("nsenter", nsenterArgs...)
		klog.InfoS("Starting sidecar task", "id", task.Name, "targetPID", targetPID)

	} else {
		// Host Logic: Direct execution
		// Use exec.Command instead of CommandContext to ensure the process survives
		// after the HTTP request context is canceled.
		cmd = exec.Command("/bin/sh", "-c", shimScript)
		klog.InfoS("Starting host task", "name", task.Name, "cmd", safeCmdStr, "exitPath", exitPath)
	}

	// Set process group ID to isolate from parent process lifecycle
	// This applies to both Host and Sidecar (nsenter) processes
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group
		Pgid:    0,    // Use PID as PGID
	}

	// 3. Execute common logic (logs, shim start)
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

	// Apply environment variables from ProcessTask spec
	if task.Spec.Process != nil {
		// Start with current environment
		cmd.Env = os.Environ()
		// Add task-specific environment variables
		for _, env := range task.Spec.Process.Env {
			if env.Name != "" {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", env.Name, env.Value))
			}
		}

		// Apply working directory
		if task.Spec.Process.WorkingDir != "" {
			cmd.Dir = task.Spec.Process.WorkingDir
			klog.InfoS("Set working directory", "name", task.Name, "workingDir", task.Spec.Process.WorkingDir)
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
		// Try to kill the process since we failed to track it
		_ = cmd.Process.Kill()
		stdoutFile.Close()
		stderrFile.Close()
		return fmt.Errorf("failed to write pid file: %w", err)
	}

	klog.InfoS("Task command started successfully", "name", task.Name, "pid", pid)

	// Close file descriptors in parent; child process has inherited them
	stdoutFile.Close()
	stderrFile.Close()

	// Wait for process in background
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
	var pid int

	// 1. Check Exit File (Completed)
	if exitData, err := os.ReadFile(exitPath); err == nil {
		fileInfo, _ := os.Stat(exitPath)
		exitCode, _ := strconv.Atoi(string(exitData))

		status.ExitCode = exitCode
		finishedAt := fileInfo.ModTime()
		status.FinishedAt = &finishedAt

		if exitCode == 0 {
			status.State = types.TaskStateSucceeded
			status.Reason = "Succeeded"
		} else {
			status.State = types.TaskStateFailed
			status.Reason = "Failed"
		}

		// Try to read start time from PID file
		if pidFileInfo, err := os.Stat(pidPath); err == nil {
			startedAt := pidFileInfo.ModTime()
			status.StartedAt = &startedAt
		}

		return status, nil
	}

	// 2. Check PID File (Running)
	if pidData, err := os.ReadFile(pidPath); err == nil {
		pid, _ = strconv.Atoi(strings.TrimSpace(string(pidData)))
		fileInfo, _ := os.Stat(pidPath)
		startedAt := fileInfo.ModTime()
		status.StartedAt = &startedAt

		if isProcessRunning(pid) {
			status.State = types.TaskStateRunning
		} else {
			// Process crashed
			status.State = types.TaskStateFailed
			status.ExitCode = 137 // Assume kill/crash
			status.Reason = "ProcessCrashed"
			status.Message = "Process exited without writing exit code"
			// Use ModTime as FinishedAt for crash approximation
			status.FinishedAt = &startedAt
		}
		return status, nil
	}

	// 3. Pending
	status.State = types.TaskStatePending
	status.Reason = "Pending"
	return status, nil
}

func (e *processExecutor) Stop(ctx context.Context, task *types.Task) error {
	// Read from pid file (Root PID: nsenter or sh)
	taskDir, err := utils.SafeJoin(e.rootDir, task.Name)
	if err != nil {
		return fmt.Errorf("invalid task name: %w", err)
	}
	pidPath := filepath.Join(taskDir, PidFile)
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return nil // pid file does not exist, process might not be started
	}
	var pid int
	pid, err = strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil || pid == 0 {
		return nil
	}
	klog.InfoS("Read PID from pid file", "name", task.Name, "pid", pid)

	// Target the process group (negative PID)
	pgid := -pid

	// Determine target PID to signal
	targetPID := 0
	if e.config.EnableSidecarMode {
		// In Sidecar mode, pid is nsenter. We need to signal its child (Shim).
		// We use /proc/<pid>/task/<pid>/children which is O(1) compared to scanning /proc.
		children, err := getChildrenPIDs(pid)
		if err == nil && len(children) > 0 {
			targetPID = children[0] // Assume first child is Shim
			klog.InfoS("Sidecar mode: targeted Shim process via /proc/children", "nsenterPID", pid, "shimPID", targetPID)
		} else {
			klog.Warning("Sidecar mode: failed to find child process via /proc/children, falling back to PGID", "pid", pid, "err", err)
		}
	} else {
		// In Host mode, pid is the Shim itself.
		targetPID = pid
	}

	// 1. Send SIGTERM
	// If we found a specific target (Shim), signal it. It will trap and forward to child.
	// If not (or if signal fails), fallback to signaling the group.
	killedShim := false
	if targetPID > 0 {
		if err := syscall.Kill(targetPID, syscall.SIGTERM); err == nil {
			killedShim = true
		} else if err != syscall.ESRCH {
			klog.ErrorS(err, "Failed to send SIGTERM to target process", "targetPID", targetPID)
		}
	}

	if !killedShim {
		// Fallback: kill the group.
		// Note: In Sidecar mode, this might kill nsenter before Shim exits, risking zombies.
		_ = syscall.Kill(pgid, syscall.SIGTERM)
	}

	// 2. Wait for process to exit (Graceful shutdown)
	// Poll every 500ms for up to 10 seconds
	timeout := 10 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessRunning(pid) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// 3. Force Kill (SIGKILL)
	klog.InfoS("Process did not exit after timeout, sending SIGKILL", "pgid", pgid)
	if targetPID > 0 {
		_ = syscall.Kill(targetPID, syscall.SIGKILL)
	}
	_ = syscall.Kill(pgid, syscall.SIGKILL)

	return nil
}

// getChildrenPIDs reads /proc/<pid>/task/<pid>/children to find direct children.
// This requires kernel 3.5+ and CONFIG_PROC_CHILDREN.
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

// Helpers
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

// findPidByEnvVar finds a process by checking for a specific environment variable.
// It looks for processes with SANDBOX_MAIN_CONTAINER=<expectedValue> in their environment.
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
