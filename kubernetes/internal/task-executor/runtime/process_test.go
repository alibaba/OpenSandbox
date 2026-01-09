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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/api/v1alpha1"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/config"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/types"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/utils"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func setupTestExecutor(t *testing.T) (Executor, string) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDir:           dataDir,
		EnableSidecarMode: false,
	}
	executor, err := NewProcessExecutor(cfg)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	return executor, dataDir
}

func TestProcessExecutor_Lifecycle(t *testing.T) {
	// Skip if not running on Linux/Unix-like systems where sh is available
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found, skipping process executor test")
	}

	executor, _ := setupTestExecutor(t)
	pExecutor := executor.(*processExecutor)
	ctx := context.Background()

	// 1. Create a task that runs for a while
	task := &types.Task{
		Name: "long-running",
		Spec: v1alpha1.TaskSpec{
			Process: &v1alpha1.ProcessTask{
				Command: []string{"/bin/sh", "-c", "sleep 10"},
			},
		},
	}

	// Create task directory manually (normally handled by store)

	taskDir, err := utils.SafeJoin(pExecutor.rootDir, task.Name)
	assert.Nil(t, err)
	os.MkdirAll(taskDir, 0755)

	// 2. Start
	if err := executor.Start(ctx, task); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 3. Inspect (Running)
	status, err := executor.Inspect(ctx, task)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if status.State != types.TaskStateRunning {
		t.Errorf("Task should be running, got: %s", status.State)
	}

	// 4. Stop
	if err := executor.Stop(ctx, task); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 5. Inspect (Terminated)
	// Wait a bit for file to be written
	time.Sleep(100 * time.Millisecond)
	status, err = executor.Inspect(ctx, task)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	// sleep command killed by signal results in non-zero exit code, so it's Failed
	if status.State != types.TaskStateFailed {
		t.Errorf("Task should be failed (terminated), got: %s", status.State)
	}
}

func TestProcessExecutor_ShortLived(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found")
	}

	executor, _ := setupTestExecutor(t)
	pExecutor := executor.(*processExecutor)
	ctx := context.Background()

	task := &types.Task{
		Name: "short-lived",
		Spec: v1alpha1.TaskSpec{
			Process: &v1alpha1.ProcessTask{
				Command: []string{"echo", "done"},
			},
		},
	}
	taskDir, err := utils.SafeJoin(pExecutor.rootDir, task.Name)
	assert.Nil(t, err)
	os.MkdirAll(taskDir, 0755)

	if err := executor.Start(ctx, task); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for process to finish
	time.Sleep(200 * time.Millisecond)

	status, err := executor.Inspect(ctx, task)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if status.State != types.TaskStateSucceeded {
		t.Errorf("Task should be succeeded, got: %s", status.State)
	}
	if status.ExitCode != 0 {
		t.Errorf("Exit code should be 0, got %d", status.ExitCode)
	}
}

func TestProcessExecutor_Failure(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found")
	}

	executor, _ := setupTestExecutor(t)
	pExecutor := executor.(*processExecutor)
	ctx := context.Background()

	task := &types.Task{
		Name: "failing-task",
		Spec: v1alpha1.TaskSpec{
			Process: &v1alpha1.ProcessTask{
				Command: []string{"/bin/sh", "-c", "exit 1"},
			},
		},
	}
	taskDir, err := utils.SafeJoin(pExecutor.rootDir, task.Name)
	assert.Nil(t, err)
	os.MkdirAll(taskDir, 0755)

	if err := executor.Start(ctx, task); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	status, err := executor.Inspect(ctx, task)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if status.State != types.TaskStateFailed {
		t.Errorf("Task should be failed")
	} else if status.ExitCode != 1 {
		t.Errorf("Exit code should be 1, got %d", status.ExitCode)
	}
}

func TestProcessExecutor_InvalidArgs(t *testing.T) {
	exec, _ := setupTestExecutor(t)
	ctx := context.Background()

	// Nil task
	if err := exec.Start(ctx, nil); err == nil {
		t.Error("Start should fail with nil task")
	}

	// Missing process spec
	task := &types.Task{
		Name: "invalid",
		Spec: v1alpha1.TaskSpec{},
	}
	if err := exec.Start(ctx, task); err == nil {
		t.Error("Start should fail with missing process spec")
	}
}

func TestShellEscape(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{[]string{"echo", "hello"}, "'echo' 'hello'"},
		{[]string{"echo", "hello world"}, "'echo' 'hello world'"},
		{[]string{"foo'bar"}, "'foo'\\''bar'"},
	}

	for _, tt := range tests {
		got := shellEscape(tt.input)
		if got != tt.expected {
			t.Errorf("shellEscape(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNewExecutor(t *testing.T) {
	// 1. Container mode + Host Mode
	cfg := &config.Config{
		EnableContainerMode: true,
	}
	e, err := NewExecutor(cfg)
	if err != nil {
		t.Fatalf("NewExecutor(container) failed: %v", err)
	}
	if _, ok := e.(*compositeExecutor); !ok {
		t.Error("NewExecutor should return CompositeExecutor")
	}

	// 2. Process mode only
	cfg = &config.Config{
		EnableContainerMode: false,
		DataDir:             t.TempDir(),
	}
	e, err = NewExecutor(cfg)
	if err != nil {
		t.Fatalf("NewExecutor(process) failed: %v", err)
	}
	if _, ok := e.(*compositeExecutor); !ok {
		t.Error("NewExecutor should return CompositeExecutor")
	}

	// 3. Nil config
	if _, err := NewExecutor(nil); err == nil {
		t.Error("NewExecutor should fail with nil config")
	}
}

func TestProcessExecutor_EnvInheritance(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found")
	}

	// 1. Setup Host Environment
	expectedHostVar := "HOST_TEST_VAR=host_value"
	os.Setenv("HOST_TEST_VAR", "host_value")
	defer os.Unsetenv("HOST_TEST_VAR")

	executor, _ := setupTestExecutor(t)
	pExecutor := executor.(*processExecutor)
	ctx := context.Background()

	// 2. Define Task with Custom Env
	task := &types.Task{
		Name: "env-test",
		Spec: v1alpha1.TaskSpec{
			Process: &v1alpha1.ProcessTask{
				Command: []string{"env"},
				Env: []corev1.EnvVar{
					{Name: "TASK_TEST_VAR", Value: "task_value"},
				},
			},
		},
	}
	expectedTaskVar := "TASK_TEST_VAR=task_value"

	taskDir, err := utils.SafeJoin(pExecutor.rootDir, task.Name)
	assert.Nil(t, err)
	os.MkdirAll(taskDir, 0755)

	// 3. Start Task
	if err := executor.Start(ctx, task); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 4. Wait for completion
	time.Sleep(200 * time.Millisecond)

	status, err := executor.Inspect(ctx, task)
	assert.Nil(t, err)
	assert.Equal(t, types.TaskStateSucceeded, status.State)

	// 5. Verify Output
	stdoutPath := filepath.Join(taskDir, StdoutFile)
	output, err := os.ReadFile(stdoutPath)
	assert.Nil(t, err)
	outputStr := string(output)

	assert.Contains(t, outputStr, expectedHostVar, "Should inherit host environment variables")
	assert.Contains(t, outputStr, expectedTaskVar, "Should include task-specific environment variables")
}

func TestProcessExecutor_Sidecar_EnvInheritance(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sidecar mode tests depend on /proc and nsenter, only supported on Linux")
	}

	// 1. Setup Mock "Main Container" Process
	// We start a sleep process with a specific identifying environment variable
	mainContainerCmd := exec.Command("sleep", "100")
	mainContainerCmd.Env = append(os.Environ(),
		"SANDBOX_MAIN_CONTAINER=test-sidecar-main",
		"TARGET_CONTAINER_VAR=i_am_from_target",
	)
	if err := mainContainerCmd.Start(); err != nil {
		t.Fatalf("Failed to start mock main container: %v", err)
	}
	defer func() {
		_ = mainContainerCmd.Process.Kill()
	}()

	// 2. Setup Mock nsenter
	// Create a script named 'nsenter' that prints envs and behaves like the shim
	tempBin := t.TempDir()
	mockNsenterPath := filepath.Join(tempBin, "nsenter")
	// The mock script:
	// 1. Prints environment variables to stdout (for verification)
	// 2. Extracts the shim script command (last argument) and runs it (to behave correctly)
	// Note: The real Start() calls: nsenter ... -- /bin/sh -c shimScript
	// We want to verify the env vars passed to nsenter.
	scriptContent := `#!/bin/sh
echo "MOCK_NSENTER_CALLED=true"
env
# Find the actual command to run (shim script)
# In the test, we know the structure is: nsenter ... -- /bin/sh -c shimScript
# We just want to exit successfully to satisfy the test flow or simulate the shim.
# For simplicity in this env test, we just exit 0. 
# The shim script logic is tested in Host mode tests.
exit 0
`
	if err := os.WriteFile(mockNsenterPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write mock nsenter: %v", err)
	}

	// Prepend tempBin to PATH
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tempBin+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

	// 3. Configure Executor
	cfg := &config.Config{
		DataDir:           t.TempDir(),
		EnableSidecarMode: true,
		MainContainerName: "test-sidecar-main", // Matches the env value above
	}
	executor, err := NewProcessExecutor(cfg)
	assert.Nil(t, err)

	task := &types.Task{
		Name: "sidecar-env-test",
		Spec: v1alpha1.TaskSpec{
			Process: &v1alpha1.ProcessTask{
				Command: []string{"echo", "hello"},
			},
		},
	}

	// 4. Start Task
	// This should: find our sleep process, read its env, run mock nsenter with that env
	if err := executor.Start(context.Background(), task); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 5. Verify Output
	// We read the task's stdout.log
	pExecutor := executor.(*processExecutor)
	taskDir, _ := utils.SafeJoin(pExecutor.rootDir, task.Name)
	stdoutPath := filepath.Join(taskDir, StdoutFile)

	// Wait a bit for file to be written
	time.Sleep(100 * time.Millisecond)

	output, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("Failed to read stdout: %v", err)
	}
	outputStr := string(output)

	assert.Contains(t, outputStr, "MOCK_NSENTER_CALLED=true", "Should call mock nsenter")
	assert.Contains(t, outputStr, "TARGET_CONTAINER_VAR=i_am_from_target", "Should inherit env from target container")
}
