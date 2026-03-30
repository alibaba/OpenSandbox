//go:build e2e

package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func createCodeInterpreter(t *testing.T) (context.Context, *opensandbox.CodeInterpreter) {
	t.Helper()
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	ci, err := opensandbox.CreateCodeInterpreter(ctx, config, opensandbox.CodeInterpreterCreateOptions{
		Metadata: map[string]string{
			"test": "go-e2e-code-interpreter",
		},
		ReadyTimeout:        60 * time.Second,
		HealthCheckInterval: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("CreateCodeInterpreter: %v", err)
	}
	t.Cleanup(func() { ci.Kill(context.Background()) })
	return ctx, ci
}

func TestCodeInterpreter_CreateAndPing(t *testing.T) {
	ctx, ci := createCodeInterpreter(t)

	if !ci.IsHealthy(ctx) {
		t.Error("Code interpreter should be healthy")
	}

	metrics, err := ci.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	t.Logf("Code interpreter metrics: cpu=%.0f, mem=%.0fMiB", metrics.CPUCount, metrics.MemTotalMB)
}

func TestCodeInterpreter_PythonExecution(t *testing.T) {
	ctx, ci := createCodeInterpreter(t)

	exec, err := ci.Execute(ctx, "python", `print("hello from python")`, nil)
	if err != nil {
		t.Fatalf("Execute python: %v", err)
	}

	text := exec.Text()
	if !strings.Contains(text, "hello from python") {
		t.Errorf("Expected python output, got: %q", text)
	}
	t.Logf("Python output: %s", text)
}

func TestCodeInterpreter_PythonContextPersistence(t *testing.T) {
	ctx, ci := createCodeInterpreter(t)

	// Create a context
	codeCtx, err := ci.CreateContext(ctx, opensandbox.CreateContextRequest{Language: "python"})
	if err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	t.Logf("Created context: %s", codeCtx.ID)

	// Set a variable
	exec, err := ci.ExecuteInContext(ctx, codeCtx.ID, "python", `x = 42`, nil)
	if err != nil {
		t.Fatalf("Execute (set var): %v", err)
	}
	_ = exec

	// Read variable back — should persist in context
	exec, err = ci.ExecuteInContext(ctx, codeCtx.ID, "python", `print(f"x is {x}")`, nil)
	if err != nil {
		t.Fatalf("Execute (read var): %v", err)
	}

	text := exec.Text()
	if !strings.Contains(text, "x is 42") {
		t.Errorf("Expected variable persistence, got: %q", text)
	}
	t.Logf("Context persistence: %s", text)

	// Cleanup
	err = ci.DeleteContext(ctx, codeCtx.ID)
	if err != nil {
		t.Logf("DeleteContext: %v", err)
	}
}

func TestCodeInterpreter_ContextManagement(t *testing.T) {
	ctx, ci := createCodeInterpreter(t)

	// Create context
	codeCtx, err := ci.CreateContext(ctx, opensandbox.CreateContextRequest{Language: "python"})
	if err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	// List contexts
	contexts, err := ci.ListContexts(ctx, "python")
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	if len(contexts) == 0 {
		t.Error("Expected at least one context")
	}
	t.Logf("Listed %d python contexts", len(contexts))

	// Delete context
	err = ci.DeleteContext(ctx, codeCtx.ID)
	if err != nil {
		t.Fatalf("DeleteContext: %v", err)
	}
	t.Log("Context management passed")
}

func TestCodeInterpreter_ContextIsolation(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ci, err := opensandbox.CreateCodeInterpreter(ctx, config, opensandbox.CodeInterpreterCreateOptions{
		ReadyTimeout:        60 * time.Second,
		HealthCheckInterval: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("CreateCodeInterpreter: %v", err)
	}
	defer ci.Kill(context.Background())

	// Create two contexts
	ctx1, err := ci.CreateContext(ctx, opensandbox.CreateContextRequest{Language: "python"})
	if err != nil {
		t.Fatalf("CreateContext 1: %v", err)
	}
	ctx2, err := ci.CreateContext(ctx, opensandbox.CreateContextRequest{Language: "python"})
	if err != nil {
		t.Fatalf("CreateContext 2: %v", err)
	}

	// Set variable in context 1
	ci.ExecuteInContext(ctx, ctx1.ID, "python", `isolated_var = "ctx1_only"`, nil)

	// Try to read it in context 2 — should get NameError
	exec, err := ci.ExecuteInContext(ctx, ctx2.ID, "python", `print("ISOLATED") if "isolated_var" not in dir() else print(isolated_var)`, nil)
	if err != nil {
		t.Fatalf("Execute in ctx2: %v", err)
	}

	text := exec.Text()
	if !strings.Contains(text, "ISOLATED") {
		t.Errorf("Contexts should be isolated, got: %q", text)
	}
	t.Log("Context isolation verified")

	ci.DeleteContext(ctx, ctx1.ID)
	ci.DeleteContext(ctx, ctx2.ID)
}

func TestCodeInterpreter_ExecutionWithHandlers(t *testing.T) {
	ctx, ci := createCodeInterpreter(t)

	var stdoutLines []string
	handlers := &opensandbox.ExecutionHandlers{
		OnStdout: func(msg opensandbox.OutputMessage) error {
			stdoutLines = append(stdoutLines, msg.Text)
			return nil
		},
	}

	_, err := ci.Execute(ctx, "python", `
for i in range(3):
    print(f"line {i}")
`, handlers)
	if err != nil {
		t.Fatalf("Execute with handlers: %v", err)
	}

	if len(stdoutLines) == 0 {
		t.Error("Expected handler to receive stdout")
	}
	t.Logf("Handler received %d stdout events", len(stdoutLines))
}
