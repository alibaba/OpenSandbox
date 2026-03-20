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

import "os"

const (
	// DefaultPort is the default port for task-executor API
	DefaultPort = "5758"
	// PortEnvVar is the environment variable name for custom port
	PortEnvVar = "TASK_EXECUTOR_PORT"
)

// GetPort returns the task-executor port from environment variable or default
func GetPort() string {
	if port := os.Getenv(PortEnvVar); port != "" {
		return port
	}
	return DefaultPort
}
