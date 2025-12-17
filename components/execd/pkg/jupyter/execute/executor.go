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

package execute

// Executor is the interface for code execution
type Executor struct {
	// Internal client
	client *Client
	// WebSocket URL
	wsURL string
}

// NewExecutor creates a new code executor
func NewExecutor(wsURL string, httpClient HTTPClient) *Executor {
	client := NewClient("", httpClient)
	return &Executor{
		client: client,
		wsURL:  wsURL,
	}
}

// Connect connects to the kernel
func (e *Executor) Connect() error {
	return e.client.Connect(e.wsURL)
}

// Disconnect disconnects from the kernel
func (e *Executor) Disconnect() {
	e.client.Disconnect()
}

// ExecuteCodeStream executes code in streaming mode, sending results to the provided channel
func (e *Executor) ExecuteCodeStream(code string, resultChan chan *ExecutionResult) error {
	return e.client.ExecuteCodeStream(code, resultChan)
}

// ExecuteCodeWithCallback executes code using callback functions
func (e *Executor) ExecuteCodeWithCallback(code string, handler CallbackHandler) error {
	return e.client.ExecuteCodeWithCallback(code, handler)
}
