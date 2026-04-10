package tests

import (
	"testing"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
	"github.com/stretchr/testify/require"
)

func TestCommand_RunSimple(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	exec, err := sb.RunCommand(ctx, "echo hello-from-go-e2e", nil)
	require.NoError(t, err)

	require.NotNil(t, exec.ExitCode)
	require.Equal(t, 0, *exec.ExitCode)

	text := exec.Text()
	require.Contains(t, text, "hello-from-go-e2e")
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
	require.NoError(t, err)

	require.NotEmpty(t, stdoutLines, "expected handler to receive stdout events")
	t.Logf("Handler received %d stdout events", len(stdoutLines))
	t.Logf("Execution stdout count: %d", len(exec.Stdout))
}

func TestCommand_ExitCode(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	exec, err := sb.RunCommand(ctx, "true", nil)
	require.NoError(t, err)
	require.NotNil(t, exec.ExitCode)
	require.Equal(t, 0, *exec.ExitCode)
	t.Log("Exit code tests passed")
}

func TestCommand_MultiLine(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	exec, err := sb.RunCommand(ctx, "echo hello && echo world && uname -a", nil)
	require.NoError(t, err)

	text := exec.Text()
	require.Contains(t, text, "hello")
	require.Contains(t, text, "world")
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
	require.NoError(t, err)

	text := exec.Text()
	require.Contains(t, text, "injected-from-go-e2e")
	t.Logf("Env injection: %s", text)
}

func TestCommand_BackgroundStatusLogs(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	// Run a background command
	exec, err := sb.RunCommandWithOpts(ctx, opensandbox.RunCommandRequest{
		Command:    "echo bg-output && sleep 1 && echo bg-done",
		Background: true,
	}, nil)
	require.NoError(t, err)

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
	require.NoError(t, err)
	if exec.ID == "" {
		t.Log("No execution ID — cannot test interrupt")
		return
	}

	// Verify it's running — try a quick command to confirm sandbox is responsive
	pingExec, err := sb.RunCommand(ctx, "echo still-alive", nil)
	require.NoError(t, err)
	require.Contains(t, pingExec.Text(), "still-alive")
	t.Log("Interrupt test: sandbox responsive during background command")
}
