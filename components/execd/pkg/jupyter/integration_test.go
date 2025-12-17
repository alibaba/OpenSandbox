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

package jupyter

import (
	"encoding/json"
	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// Test integration flow: authentication -> get kernel specs -> create session -> execute code -> close session
func TestIntegrationFlow(t *testing.T) {
	// Create mock HTTP server
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle authentication validation request
		if r.URL.Path == "/api/status" {
			// Check authentication token
			auth := r.Header.Get("Authorization")
			if auth != "token test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Return status information
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status": "ok"}`))
			return
		}

		// Handle kernel specs request
		if r.URL.Path == "/api/kernelspecs" {
			// Return kernel specs
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"default": "python3",
				"kernelspecs": {
					"python3": {
						"name": "python3",
						"display_name": "Python 3",
						"language": "python"
					}
				}
			}`))
			return
		}

		// Handle session-related requests
		if r.URL.Path == "/api/sessions" {
			if r.Method == http.MethodGet {
				// List sessions
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[{
					"id": "test-session-id",
					"path": "/path/to/notebook.ipynb",
					"name": "Test Session",
					"type": "notebook",
					"kernel": {
						"id": "test-kernel-id",
						"name": "python3"
					}
				}]`))
				return
			} else if r.Method == http.MethodPost {
				// Create session
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{
					"id": "test-session-id",
					"path": "/path/to/notebook.ipynb",
					"name": "Test Session",
					"type": "notebook",
					"kernel": {
						"id": "test-kernel-id",
						"name": "python3"
					}
				}`))
				return
			}
		}

		// Handle specific session requests
		if strings.HasPrefix(r.URL.Path, "/api/sessions/test-session-id") {
			if r.Method == http.MethodDelete {
				// Delete session
				w.WriteHeader(http.StatusNoContent)
				return
			} else if r.Method == http.MethodPatch {
				// Modify session
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"id": "test-session-id",
					"path": "/path/to/updated-notebook.ipynb",
					"name": "Updated Test Session",
					"type": "notebook",
					"kernel": {
						"id": "test-kernel-id",
						"name": "python3"
					}
				}`))
				return
			} else if r.Method == http.MethodGet {
				// Get session
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"id": "test-session-id",
					"path": "/path/to/notebook.ipynb",
					"name": "Test Session",
					"type": "notebook",
					"kernel": {
						"id": "test-kernel-id",
						"name": "python3"
					}
				}`))
				return
			}
		}

		// Handle kernel requests
		if r.URL.Path == "/api/kernels" {
			if r.Method == http.MethodGet {
				// List kernels
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[{
					"id": "test-kernel-id",
					"name": "python3",
					"execution_state": "idle"
				}]`))
				return
			}
		}

		// Handle specific kernel requests
		if strings.HasPrefix(r.URL.Path, "/api/kernels/test-kernel-id") {
			if r.Method == http.MethodGet {
				// Get kernel
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"id": "test-kernel-id",
					"name": "python3",
					"execution_state": "idle"
				}`))
				return
			} else if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/restart") {
				// Restart kernel
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"id": "test-kernel-id",
					"name": "python3",
					"restarted": true
				}`))
				return
			}
		}

		// If it's a WebSocket connection request, upgrade to WebSocket
		if strings.HasSuffix(r.URL.Path, "/channels") {
			// Return 404, as WebSocket connections will be handled by a dedicated WebSocket server
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// For other requests, return 404
		w.WriteHeader(http.StatusNotFound)
	}))
	defer httpServer.Close()

	// Create mock WebSocket server for code execution
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/channels") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Upgrade HTTP connection to WebSocket
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection to WebSocket: %v", err)
		}
		defer conn.Close()

		// Continuously handle WebSocket messages
		for {
			// Read request message
			var msg execute.Message
			err := conn.ReadJSON(&msg)
			if err != nil {
				break
			}

			// If it's an execute request, send mock response
			if msg.Header.MessageType == string(execute.MsgExecuteRequest) {
				// Send stream output
				streamContent, _ := json.Marshal(execute.StreamOutput{
					Name: execute.StreamStdout,
					Text: "Hello from test WebSocket!\n",
				})

				streamMsg := execute.Message{
					Header: execute.Header{
						MessageID:   "stream-msg-id",
						Session:     msg.Header.Session,
						MessageType: string(execute.MsgStream),
					},
					ParentHeader: msg.Header,
					Content:      json.RawMessage(streamContent),
				}
				conn.WriteJSON(streamMsg)

				// Send execution result
				resultContent, _ := json.Marshal(execute.ExecuteResult{
					ExecutionCount: 1,
					Data: map[string]interface{}{
						"text/plain": "Integration test result",
					},
					Metadata: map[string]interface{}{},
				})

				executeResultMsg := execute.Message{
					Header: execute.Header{
						MessageID:   "result-msg-id",
						Session:     msg.Header.Session,
						MessageType: string(execute.MsgExecuteResult),
					},
					ParentHeader: msg.Header,
					Content:      json.RawMessage(resultContent),
				}
				conn.WriteJSON(executeResultMsg)

				// Send status message
				statusContent, _ := json.Marshal(execute.StatusUpdate{
					ExecutionState: execute.StateIdle,
				})

				statusMsg := execute.Message{
					Header: execute.Header{
						MessageID:   "status-msg-id",
						Session:     msg.Header.Session,
						MessageType: string(execute.MsgStatus),
					},
					ParentHeader: msg.Header,
					Content:      json.RawMessage(statusContent),
				}
				conn.WriteJSON(statusMsg)
			}
		}
	}))
	defer wsServer.Close()

	// Create Jupyter client
	client := NewClient(httpServer.URL)
	client.SetToken("test-token")

	// Test 1: Validate authentication
	status, err := client.ValidateAuth()
	if err != nil {
		t.Fatalf("Authentication validation failed: %v", err)
	}
	if status != "ok" {
		t.Errorf("Authentication status incorrect, expected 'ok', got '%s'", status)
	}

	// Test 2: Get kernel specs
	specs, err := client.GetKernelSpecs()
	if err != nil {
		t.Fatalf("Failed to get kernel specs: %v", err)
	}
	if specs.Default != "python3" {
		t.Errorf("Default kernel incorrect, expected 'python3', got '%s'", specs.Default)
	}
	if len(specs.Kernelspecs) != 1 {
		t.Errorf("Kernel count incorrect, expected 1, got %d", len(specs.Kernelspecs))
	}

	// Test 3: Create session
	session, err := client.CreateSession("Test Session", "/path/to/notebook.ipynb", "python3")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	if session.ID != "test-session-id" {
		t.Errorf("Session ID incorrect, expected 'test-session-id', got '%s'", session.ID)
	}
	if session.Kernel.ID != "test-kernel-id" {
		t.Errorf("Kernel ID incorrect, expected 'test-kernel-id', got '%s'", session.Kernel.ID)
	}

	// Modify WebSocket URL to point to WebSocket test server
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http") + "/api/kernels/test-kernel-id/channels"

	// Test 4: Connect to kernel and execute code
	executor := execute.NewExecutor(wsURL, nil)
	err = executor.Connect()
	if err != nil {
		t.Fatalf("Failed to connect to kernel: %v", err)
	}
	defer executor.Disconnect()

	// Execute code
	err = executor.ExecuteCodeWithCallback("print('Hello from integration test!')", execute.CallbackHandler{})
	if err != nil {
		t.Fatalf("Failed to execute code: %v", err)
	}

	// Test 5: Delete session
	err = client.DeleteSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}
}
