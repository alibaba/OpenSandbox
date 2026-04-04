// Example: Multi-turn Code Interpreter Agent
//
// Demonstrates an AI agent that maintains persistent Python state across
// multiple conversation turns using OpenSandbox's code interpreter:
//
// Turn 1: LLM creates a dataset
// Turn 2: LLM analyzes it (variables persist from turn 1)
// Turn 3: LLM generates a summary
//
// Usage:
//
//	export OPEN_SANDBOX_DOMAIN=localhost:8080
//	export OPEN_SANDBOX_API_KEY=your-api-key
//	export LLM_ENDPOINT=http://localhost:8080/v1
//	export LLM_MODEL=gpt-4o-mini
//	go run main.go
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

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Create code interpreter sandbox
	fmt.Println("Creating code interpreter...")
	config := opensandbox.ConnectionConfig{}
	ci, err := opensandbox.CreateCodeInterpreter(ctx, config, opensandbox.CodeInterpreterCreateOptions{
		ReadyTimeout: 60 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
		os.Exit(1)
	}
	defer ci.Kill(context.Background())
	fmt.Printf("Ready: %s\n\n", ci.ID())

	// Create persistent Python context
	codeCtx, err := ci.CreateContext(ctx, opensandbox.CreateContextRequest{Language: "python"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "CreateContext: %v\n", err)
		os.Exit(1)
	}

	conversation := []map[string]string{
		{"role": "system", "content": "You are a data analysis assistant. Respond ONLY with a Python code block. Use only stdlib. Always print results."},
	}

	// Multi-turn conversation
	turns := []string{
		"Create a list called 'temps' with 12 monthly average temperatures (Celsius) for São Paulo: [22, 23, 22, 20, 18, 17, 16, 18, 19, 20, 21, 22]. Print it.",
		"Using the 'temps' variable, find the coldest month (1-indexed), hottest month, and the average temperature. Print all three.",
		"Using 'temps', classify each month as 'hot' (>20), 'warm' (18-20), or 'cool' (<18). Print the classification for each month.",
	}

	for i, prompt := range turns {
		fmt.Printf("--- Turn %d ---\n", i+1)
		fmt.Printf("User: %s\n\n", prompt)

		conversation = append(conversation, map[string]string{"role": "user", "content": prompt})

		code, err := chatLLM(ctx, conversation)
		if err != nil {
			fmt.Fprintf(os.Stderr, "LLM error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Code:\n%s\n\n", code)

		exec, err := ci.ExecuteInContext(ctx, codeCtx.ID, "python", code, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Execute error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Output:\n%s\n\n", exec.Text())

		conversation = append(conversation, map[string]string{"role": "assistant", "content": "```python\n" + code + "\n```"})
	}

	ci.DeleteContext(ctx, codeCtx.ID)
	fmt.Println("Done! All turns completed with persistent state.")
}

func chatLLM(ctx context.Context, messages []map[string]string) (string, error) {
	endpoint := os.Getenv("LLM_ENDPOINT")
	if endpoint == "" {
		return "", fmt.Errorf("LLM_ENDPOINT not set")
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"messages":   messages,
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
		return "", fmt.Errorf("no choices: %s", string(data))
	}

	text := result.Choices[0].Message.Content
	if start := strings.Index(text, "```python"); start != -1 {
		text = text[start+9:]
		if end := strings.Index(text, "```"); end != -1 {
			text = text[:end]
		}
	}
	return strings.TrimSpace(text), nil
}
