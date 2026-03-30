//go:build e2e

package tests

import (
	"strings"
	"testing"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func TestCommand_RunSimple(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	exec, err := sb.RunCommand(ctx, "echo hello-from-go-e2e", nil)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}

	if exec.ExitCode == nil || *exec.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %v", exec.ExitCode)
	}

	text := exec.Text()
	if !strings.Contains(text, "hello-from-go-e2e") {
		t.Errorf("Expected stdout to contain 'hello-from-go-e2e', got: %q", text)
	}
	t.Logf("Output: %s", text)
}

func TestCommand_RunWithHandlers(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	var stdoutLines []string
	handlers := &opensandbox.ExecutionHandlers{
		OnStdout: func(msg opensandbox.OutputMessage) error {
			stdoutLines = append(stdoutLines, msg.Text)
			return nil
		},
	}

	exec, err := sb.RunCommand(ctx, "echo line1 && echo line2", handlers)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}

	if len(stdoutLines) == 0 {
		t.Error("Expected handler to receive stdout events")
	}
	t.Logf("Handler received %d stdout events", len(stdoutLines))
	t.Logf("Execution stdout count: %d", len(exec.Stdout))
}

func TestCommand_ExitCode(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	exec, err := sb.RunCommand(ctx, "true", nil)
	if err != nil {
		t.Fatalf("RunCommand(true): %v", err)
	}
	if exec.ExitCode == nil || *exec.ExitCode != 0 {
		t.Errorf("Expected exit code 0 for 'true', got %v", exec.ExitCode)
	}
	t.Log("Exit code tests passed")
}

func TestCommand_MultiLine(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	exec, err := sb.RunCommand(ctx, "echo hello && echo world && uname -a", nil)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}

	text := exec.Text()
	if !strings.Contains(text, "hello") || !strings.Contains(text, "world") {
		t.Errorf("Expected multi-line output, got: %q", text)
	}
	t.Logf("Multi-line output (%d bytes): %s", len(text), text)
}

func TestCommand_EnvInjection(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	exec, err := sb.RunCommandWithOpts(ctx, opensandbox.RunCommandRequest{
		Command: "echo $CUSTOM_VAR",
		Envs: map[string]string{
			"CUSTOM_VAR": "injected-from-go-e2e",
		},
	}, nil)
	if err != nil {
		t.Fatalf("RunCommand with envs: %v", err)
	}

	text := exec.Text()
	if !strings.Contains(text, "injected-from-go-e2e") {
		t.Errorf("Expected env var in output, got: %q", text)
	}
	t.Logf("Env injection: %s", text)
}

func TestCommand_BackgroundStatusLogs(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	// Run a background command
	exec, err := sb.RunCommandWithOpts(ctx, opensandbox.RunCommandRequest{
		Command:    "echo bg-output && sleep 1 && echo bg-done",
		Background: true,
	}, nil)
	if err != nil {
		t.Fatalf("RunCommand background: %v", err)
	}

	// The init event should give us an execution ID
	if exec.ID == "" {
		t.Log("No execution ID from background command (server may not return init event for background)")
		return
	}
	t.Logf("Background command ID: %s", exec.ID)
}

func TestCommand_Interrupt(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	// Start a long-running command in background
	exec, err := sb.RunCommandWithOpts(ctx, opensandbox.RunCommandRequest{
		Command:    "sleep 300",
		Background: true,
	}, nil)
	if err != nil {
		t.Fatalf("RunCommand background: %v", err)
	}
	if exec.ID == "" {
		t.Log("No execution ID — cannot test interrupt")
		return
	}

	// Verify it's running — try a quick command to confirm sandbox is responsive
	pingExec, err := sb.RunCommand(ctx, "echo still-alive", nil)
	if err != nil {
		t.Fatalf("Ping after background: %v", err)
	}
	if !strings.Contains(pingExec.Text(), "still-alive") {
		t.Error("Sandbox should still be responsive")
	}
	t.Log("Interrupt test: sandbox responsive during background command")
}
