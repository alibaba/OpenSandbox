// Example: Simple Agent Loop
//
// Demonstrates a basic AI agent that:
// 1. Gets a task from the user
// 2. Asks an LLM to write Python code
// 3. Executes the code in an OpenSandbox sandbox
// 4. Returns the result
//
// Usage:
//
//	export OPEN_SANDBOX_DOMAIN=localhost:8080
//	export OPEN_SANDBOX_API_KEY=your-api-key
//	export LLM_ENDPOINT=http://localhost:8080/v1  # OpenAI-compatible endpoint
//	export LLM_MODEL=gpt-4o-mini                   # or azure/gpt-4o-mini for Bifrost
//	go run main.go "Calculate the first 20 prime numbers"
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go"
)

func main() {
	task := "Calculate the first 10 Fibonacci numbers and print them"
	if len(os.Args) > 1 {
		task = strings.Join(os.Args[1:], " ")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Create sandbox
	fmt.Println("Creating sandbox...")
	config := opensandbox.ConnectionConfig{}
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: "python:3.11-slim",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create sandbox: %v\n", err)
		os.Exit(1)
	}
	defer sb.Kill(context.Background())
	fmt.Printf("Sandbox ready: %s\n", sb.ID())

	// 2. Ask LLM for code
	fmt.Printf("Task: %s\n", task)
	code, err := askLLM(ctx, task)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LLM error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated code:\n%s\n\n", code)

	// 3. Execute in sandbox
	fmt.Println("Executing in sandbox...")
	sb.RunCommand(ctx, fmt.Sprintf("cat > /tmp/task.py << 'EOF'\n%s\nEOF", code), nil)
	exec, err := sb.RunCommand(ctx, "python3 /tmp/task.py", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
		os.Exit(1)
	}

	// 4. Show result
	fmt.Printf("Result:\n%s\n", exec.Text())
	if exec.ExitCode != nil {
		fmt.Printf("Exit code: %d\n", *exec.ExitCode)
	}
}

func askLLM(ctx context.Context, task string) (string, error) {
	endpoint := os.Getenv("LLM_ENDPOINT")
	if endpoint == "" {
		return "", fmt.Errorf("LLM_ENDPOINT not set")
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	body, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "Respond ONLY with a Python code block. No explanation."},
			{"role": "user", "content": task},
		},
		"max_tokens": 1024,
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", endpoint+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(data, &result)
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM: %s", string(data))
	}

	text := result.Choices[0].Message.Content
	// Extract code from markdown block
	if start := strings.Index(text, "```python"); start != -1 {
		text = text[start+9:]
		if end := strings.Index(text, "```"); end != -1 {
			text = text[:end]
		}
	} else if start := strings.Index(text, "```"); start != -1 {
		text = text[start+3:]
		if end := strings.Index(text, "```"); end != -1 {
			text = text[:end]
		}
	}
	return strings.TrimSpace(text), nil
}
