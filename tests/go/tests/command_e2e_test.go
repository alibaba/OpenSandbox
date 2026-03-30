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
