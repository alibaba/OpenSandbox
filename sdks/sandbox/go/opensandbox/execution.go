package opensandbox

import (
	"encoding/json"
	"strconv"
	"strings"
)

// OutputMessage represents a single stdout or stderr line from command execution.
type OutputMessage struct {
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

// ExecutionResult represents a result output from code execution.
type ExecutionResult struct {
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

// ExecutionError represents an error during code/command execution.
type ExecutionError struct {
	Name      string   `json:"name"`
	Value     string   `json:"value"`
	Timestamp int64    `json:"timestamp"`
	Traceback []string `json:"traceback"`
}

// ExecutionComplete represents the completion event from the server.
type ExecutionComplete struct {
	Timestamp     int64 `json:"timestamp"`
	ExecutionTime int64 `json:"execution_time"`
}

// ExecutionInit represents the initialization event from the server.
type ExecutionInit struct {
	ID        string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

// Execution is the structured result of a command or code execution.
type Execution struct {
	// ID is the execution/command identifier from the init event.
	ID string

	// Stdout contains all stdout messages in order.
	Stdout []OutputMessage

	// Stderr contains all stderr messages in order.
	Stderr []OutputMessage

	// Results contains execution results (for code interpreter).
	Results []ExecutionResult

	// Error is set if the execution failed.
	Error *ExecutionError

	// Complete is set when execution finishes.
	Complete *ExecutionComplete

	// ExitCode is the process exit code. Nil if not available.
	ExitCode *int
}

// Text returns the combined stdout text.
func (e *Execution) Text() string {
	var b strings.Builder
	for i, m := range e.Stdout {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.Text)
	}
	return b.String()
}

// ExecutionHandlers provides optional callbacks invoked during streaming execution.
// Return a non-nil error from any handler to abort the stream.
type ExecutionHandlers struct {
	OnInit     func(ExecutionInit) error
	OnStdout   func(OutputMessage) error
	OnStderr   func(OutputMessage) error
	OnResult   func(ExecutionResult) error
	OnComplete func(ExecutionComplete) error
	OnError    func(ExecutionError) error
}

// sseEvent is the raw JSON structure from execd SSE/NDJSON events.
type sseEvent struct {
	Type          string `json:"type"`
	Text          string `json:"text"`
	Timestamp     int64  `json:"timestamp"`
	ExitCode      *int   `json:"exit_code,omitempty"`
	ExecutionTime int64  `json:"execution_time,omitempty"`
	// Error fields (for code interpreter errors)
	EName     string   `json:"ename,omitempty"`
	EValue    string   `json:"evalue,omitempty"`
	Traceback []string `json:"traceback,omitempty"`
}

// processStreamEvent parses a raw StreamEvent into the Execution accumulator
// and invokes the appropriate handler.
func processStreamEvent(exec *Execution, event StreamEvent, handlers *ExecutionHandlers) error {
	data := event.Data
	if data == "" {
		return nil
	}

	var ev sseEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		// Not JSON — treat as raw stdout
		msg := OutputMessage{Text: data}
		exec.Stdout = append(exec.Stdout, msg)
		if handlers != nil && handlers.OnStdout != nil {
			return handlers.OnStdout(msg)
		}
		return nil
	}

	switch ev.Type {
	case "init":
		init := ExecutionInit{ID: ev.Text, Timestamp: ev.Timestamp}
		exec.ID = ev.Text
		if handlers != nil && handlers.OnInit != nil {
			return handlers.OnInit(init)
		}

	case "stdout":
		msg := OutputMessage{Text: ev.Text, Timestamp: ev.Timestamp}
		exec.Stdout = append(exec.Stdout, msg)
		if handlers != nil && handlers.OnStdout != nil {
			return handlers.OnStdout(msg)
		}

	case "stderr":
		msg := OutputMessage{Text: ev.Text, Timestamp: ev.Timestamp}
		exec.Stderr = append(exec.Stderr, msg)
		if handlers != nil && handlers.OnStderr != nil {
			return handlers.OnStderr(msg)
		}

	case "result":
		res := ExecutionResult{Text: ev.Text, Timestamp: ev.Timestamp}
		exec.Results = append(exec.Results, res)
		if handlers != nil && handlers.OnResult != nil {
			return handlers.OnResult(res)
		}

	case "error":
		execErr := ExecutionError{
			Name:      ev.EName,
			Value:     ev.EValue,
			Timestamp: ev.Timestamp,
			Traceback: ev.Traceback,
		}
		exec.Error = &execErr
		// Try to parse exit code from error value
		if code, err := strconv.Atoi(ev.EValue); err == nil {
			exec.ExitCode = &code
		}
		if handlers != nil && handlers.OnError != nil {
			return handlers.OnError(execErr)
		}

	case "execution_complete":
		complete := ExecutionComplete{
			Timestamp:     ev.Timestamp,
			ExecutionTime: ev.ExecutionTime,
		}
		exec.Complete = &complete
		// Foreground command exit code: 0 if no error
		if exec.ExitCode == nil && exec.Error == nil {
			zero := 0
			exec.ExitCode = &zero
		}
		if handlers != nil && handlers.OnComplete != nil {
			return handlers.OnComplete(complete)
		}

	case "ping":
		// Ignore ping events

	default:
		// Unknown event type — treat as stdout
		if ev.Text != "" {
			msg := OutputMessage{Text: ev.Text, Timestamp: ev.Timestamp}
			exec.Stdout = append(exec.Stdout, msg)
			if handlers != nil && handlers.OnStdout != nil {
				return handlers.OnStdout(msg)
			}
		}
	}

	return nil
}
