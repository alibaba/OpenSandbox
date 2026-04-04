package opensandbox

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRunCommandRequest_MarshalJSON_TimeoutMilliseconds(t *testing.T) {
	req := RunCommandRequest{
		Command: "echo hi",
		Timeout: 5 * time.Second,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)

	got, ok := raw["timeout"].(float64)
	if !ok {
		t.Fatalf("timeout field missing or not a number: %v", raw)
	}
	if got != 5000 {
		t.Errorf("timeout = %v, want 5000 (milliseconds)", got)
	}
}

func TestRunCommandRequest_MarshalJSON_ZeroTimeoutOmitted(t *testing.T) {
	req := RunCommandRequest{Command: "ls"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)

	if _, exists := raw["timeout"]; exists {
		t.Errorf("expected timeout to be omitted when zero, got %v", raw["timeout"])
	}
}

func TestRunInSessionRequest_MarshalJSON_TimeoutMilliseconds(t *testing.T) {
	req := RunInSessionRequest{
		Command: "sleep 1",
		Timeout: 10 * time.Second,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)

	got, ok := raw["timeout"].(float64)
	if !ok {
		t.Fatalf("timeout field missing or not a number: %v", raw)
	}
	if got != 10000 {
		t.Errorf("timeout = %v, want 10000 (milliseconds)", got)
	}
}

func TestRunInSessionRequest_MarshalJSON_ZeroTimeoutOmitted(t *testing.T) {
	req := RunInSessionRequest{Command: "pwd"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)

	if _, exists := raw["timeout"]; exists {
		t.Errorf("expected timeout to be omitted when zero, got %v", raw["timeout"])
	}
}
