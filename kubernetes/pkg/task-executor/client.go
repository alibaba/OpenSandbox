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

package task_executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		klog.Warning("baseURL is empty, client may not work properly")
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Set creates or updates a task on the remote server.
// If task is nil, it sends a delete request.
func (c *Client) Set(ctx context.Context, task *Task) (*Task, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	var req *http.Request
	var err error

	if task == nil {
		// Delete request - send nil to clear tasks
		req, err = http.NewRequestWithContext(ctx, "POST", c.baseURL+"/setTasks", bytes.NewReader([]byte("[]")))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
	} else {
		// Create/Update request
		data, err := json.Marshal([]Task{*task})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal task: %w", err)
		}
		req, err = http.NewRequestWithContext(ctx, "POST", c.baseURL+"/setTasks", bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request with retry
	var resp *http.Response
	resp, err = c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error after retries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// Parse response - expect array of tasks
	var tasks []Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if task != nil && len(tasks) > 0 {
		// Find the task we just set
		for i := range tasks {
			if tasks[i].Name == task.Name {
				return &tasks[i], nil
			}
		}
	}

	if task == nil {
		// Delete succeeded
		return nil, nil
	}

	return task, nil
}

// Get retrieves the current task list from the remote server.
func (c *Client) Get(ctx context.Context) (*Task, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/getTasks", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// Parse response - expect array of tasks
	var tasks []Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Return the first task (single task mode)
	if len(tasks) > 0 {
		return &tasks[0], nil
	}

	// No tasks
	return nil, nil
}

// Reset calls the reset API on the task-executor to prepare the pod for reuse.
// Reset is only supported in sidecar mode.
// This method is idempotent - if a reset is already in progress, it returns the current status.
func (c *Client) Reset(ctx context.Context, req *ResetRequest) (*ResetResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reset request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/reset", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var resetResp ResetResponse
	if err := json.NewDecoder(resp.Body).Decode(&resetResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &resetResp, nil
}

// NewClientWithTimeout creates a new client with a custom timeout.
func NewClientWithTimeout(baseURL string, timeout time.Duration) *Client {
	if baseURL == "" {
		klog.Warning("baseURL is empty, client may not work properly")
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}
