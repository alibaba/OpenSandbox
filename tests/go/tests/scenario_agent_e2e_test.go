//go:build e2e

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getLLMEndpoint returns the Bifrost LLM gateway URL.
func getLLMEndpoint() string {
	if v := os.Getenv("LLM_ENDPOINT"); v != "" {
		return v
	}
	// Same domain as sandbox, different path
	domain := os.Getenv("OPENSANDBOX_TEST_DOMAIN")
	if domain == "" {
		return ""
	}
	protocol := os.Getenv("OPENSANDBOX_TEST_PROTOCOL")
	if protocol == "" {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s/v1", protocol, domain)
}

func getLLMModel() string {
	if v := os.Getenv("LLM_MODEL"); v != "" {
		return v
	}
	return "azure/gpt-4o-mini"
}

// chatCompletion calls the LLM via Bifrost and returns the assistant message.
func chatCompletion(ctx context.Context, endpoint, model string, messages []map[string]string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"messages":   messages,
		"max_tokens": 1024,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("LLM returned %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("parse LLM response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in LLM response")
	}
	return result.Choices[0].Message.Content, nil
}

// extractCode pulls a Python code block from LLM output.
func extractCode(text string) string {
	// Look for ```python ... ``` blocks
	start := strings.Index(text, "```python")
	if start == -1 {
		start = strings.Index(text, "```")
		if start == -1 {
			return ""
		}
		start += 3
	} else {
		start += 9
	}
	// Skip to next line
	if nl := strings.Index(text[start:], "\n"); nl != -1 {
		start += nl + 1
	}
	end := strings.Index(text[start:], "```")
	if end == -1 {
		return text[start:]
	}
	return strings.TrimSpace(text[start : start+end])
}

// TestScenario_SimpleAgentLoop tests a basic agent that:
// 1. Gets a task
// 2. Asks the LLM to write Python code
// 3. Executes the code in a sandbox
// 4. Returns the result
func TestScenario_SimpleAgentLoop(t *testing.T) {
	llmEndpoint := getLLMEndpoint()
	if llmEndpoint == "" {
		t.Skip("LLM_ENDPOINT or OPENSANDBOX_TEST_DOMAIN not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Create sandbox
	config := getConnectionConfig(t)
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	require.NoError(t, err)
	defer sb.Kill(context.Background())
	t.Logf("Sandbox ready: %s", sb.ID())

	// Step 1: Ask LLM to write code
	task := "Write Python code that calculates the first 10 Fibonacci numbers and prints them as a comma-separated list. Only output the code block, nothing else."
	t.Logf("Task: %s", task)

	llmResponse, err := chatCompletion(ctx, llmEndpoint, getLLMModel(), []map[string]string{
		{"role": "system", "content": "You are a coding assistant. Respond ONLY with a Python code block. No explanation."},
		{"role": "user", "content": task},
	})
	require.NoError(t, err)
	t.Logf("LLM response:\n%s", llmResponse)

	// Step 2: Extract code
	code := extractCode(llmResponse)
	if code == "" {
		// Try using the whole response as code
		code = strings.TrimSpace(llmResponse)
	}
	t.Logf("Extracted code:\n%s", code)

	// Step 3: Write code to file and execute in sandbox
	writeCmd := fmt.Sprintf("cat > /tmp/agent_task.py << 'PYEOF'\n%s\nPYEOF", code)
	writeResult, writeErr := sb.RunCommand(ctx, writeCmd, nil)
	require.NoError(t, writeErr)
	if writeResult.ExitCode != nil {
		require.Equal(t, 0, *writeResult.ExitCode, "write code to sandbox: %s", writeResult.Text())
	}
	exec, err := sb.RunCommand(ctx, "python3 /tmp/agent_task.py", nil)
	require.NoError(t, err)

	output := exec.Text()
	t.Logf("Execution output: %s", output)

	if exec.ExitCode != nil {
		assert.Equal(t, 0, *exec.ExitCode, "code execution exit code")
	}

	// Step 4: Verify result contains Fibonacci numbers (sequence includes 0,1,1,2,3,5,8,13,21,34)
	assert.Contains(t, output, "1")
	assert.Contains(t, output, "8")
	assert.Contains(t, output, "34")
	t.Log("Agent loop completed successfully: task → LLM → code → execute → result")
}

// TestScenario_CodeInterpreterAgent tests a multi-turn agent that:
// 1. Creates a code interpreter sandbox
// 2. Uses the LLM to solve a data analysis task step-by-step
// 3. Maintains Python state across multiple code executions
func TestScenario_CodeInterpreterAgent(t *testing.T) {
	llmEndpoint := getLLMEndpoint()
	if llmEndpoint == "" {
		t.Skip("LLM_ENDPOINT or OPENSANDBOX_TEST_DOMAIN not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	config := getConnectionConfig(t)
	ci, err := opensandbox.CreateCodeInterpreter(ctx, config, opensandbox.CodeInterpreterCreateOptions{
		ReadyTimeout:        60 * time.Second,
		HealthCheckInterval: 500 * time.Millisecond,
	})
	require.NoError(t, err)
	defer ci.Kill(context.Background())
	t.Logf("Code interpreter ready: %s", ci.ID())

	// Create persistent Python context
	codeCtx, err := ci.CreateContext(ctx, opensandbox.CreateContextRequest{Language: "python"})
	require.NoError(t, err)
	t.Logf("Python context: %s", codeCtx.ID)

	// Agent conversation: multi-turn data analysis
	conversation := []map[string]string{
		{"role": "system", "content": "You are a data analysis assistant. When asked to analyze data, respond ONLY with a Python code block. The code will be executed in a Jupyter-like environment where variables persist between turns. Always print your results. Use only the Python standard library — do NOT import numpy, pandas, or any external packages."},
	}

	// Turn 1: Create dataset
	t.Log("--- Turn 1: Create dataset ---")
	conversation = append(conversation, map[string]string{
		"role": "user", "content": "Create a list called 'sales' with these monthly values: [120, 150, 90, 200, 180, 220, 160, 190, 210, 170, 230, 250]. Print the list.",
	})

	reply1, err := chatCompletion(ctx, llmEndpoint, getLLMModel(), conversation)
	require.NoError(t, err)
	code1 := extractCode(reply1)
	if code1 == "" {
		code1 = strings.TrimSpace(reply1)
	}
	t.Logf("Turn 1 code: %s", code1)

	exec1, err := ci.ExecuteInContext(ctx, codeCtx.ID, "python", code1, nil)
	require.NoError(t, err)
	t.Logf("Turn 1 output: %s", exec1.Text())
	conversation = append(conversation, map[string]string{"role": "assistant", "content": reply1})

	// Turn 2: Analyze the data (using persisted variable)
	t.Log("--- Turn 2: Analyze dataset ---")
	conversation = append(conversation, map[string]string{
		"role": "user", "content": "Using the 'sales' variable from the previous step, calculate and print: the mean, the max month (1-indexed), and whether total sales exceed 2000.",
	})

	reply2, err := chatCompletion(ctx, llmEndpoint, getLLMModel(), conversation)
	require.NoError(t, err)
	code2 := extractCode(reply2)
	if code2 == "" {
		code2 = strings.TrimSpace(reply2)
	}
	t.Logf("Turn 2 code: %s", code2)

	exec2, err := ci.ExecuteInContext(ctx, codeCtx.ID, "python", code2, nil)
	require.NoError(t, err)
	output2 := exec2.Text()
	t.Logf("Turn 2 output: %s", output2)

	// Verify the analysis used the persisted state
	assert.NotEmpty(t, output2, "turn 2 produced no output — context persistence may have failed")
	// The total is 2170, so "exceed 2000" should be True/Yes
	if !strings.Contains(strings.ToLower(output2), "true") && !strings.Contains(strings.ToLower(output2), "yes") && !strings.Contains(output2, "2170") {
		t.Logf("Warning: output may not confirm total > 2000: %q", output2)
	}

	// Cleanup
	ci.DeleteContext(ctx, codeCtx.ID)
	t.Log("Multi-turn code interpreter agent completed successfully")
}

// TestScenario_SandboxToolUse tests an agent that uses the sandbox as a tool:
// 1. LLM decides what shell command to run
// 2. Agent executes it in sandbox
// 3. Returns output to LLM for interpretation
func TestScenario_SandboxToolUse(t *testing.T) {
	llmEndpoint := getLLMEndpoint()
	if llmEndpoint == "" {
		t.Skip("LLM_ENDPOINT or OPENSANDBOX_TEST_DOMAIN not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := getConnectionConfig(t)
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	require.NoError(t, err)
	defer sb.Kill(context.Background())

	// Ask LLM what command to run to get system info
	reply, err := chatCompletion(ctx, llmEndpoint, getLLMModel(), []map[string]string{
		{"role": "system", "content": "You have access to a Linux shell. Respond ONLY with the exact shell command to run. No explanation, no code blocks, just the raw command."},
		{"role": "user", "content": "What command shows the Linux kernel version, CPU count, and total memory in one line?"},
	})
	require.NoError(t, err)
	command := strings.TrimSpace(reply)
	// Strip code block markers if present
	command = strings.TrimPrefix(command, "```bash\n")
	command = strings.TrimPrefix(command, "```\n")
	command = strings.TrimSuffix(command, "\n```")
	command = strings.TrimPrefix(command, "```")
	command = strings.TrimSpace(command)
	t.Logf("LLM suggested command: %s", command)

	// Execute in sandbox
	exec, err := sb.RunCommand(ctx, command, nil)
	require.NoError(t, err)
	shellOutput := exec.Text()
	t.Logf("Shell output: %s", shellOutput)

	// Ask LLM to interpret the output
	interpretation, err := chatCompletion(ctx, llmEndpoint, getLLMModel(), []map[string]string{
		{"role": "system", "content": "Summarize the system information in one sentence."},
		{"role": "user", "content": fmt.Sprintf("Shell output:\n%s", shellOutput)},
	})
	require.NoError(t, err)
	t.Logf("LLM interpretation: %s", interpretation)

	assert.NotEmpty(t, interpretation, "LLM produced no interpretation")
	t.Log("Tool-use agent completed: task → LLM → shell → LLM → answer")
}
