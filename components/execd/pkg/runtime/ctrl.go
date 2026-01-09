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

package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter"
)

var kernelWaitingBackoff = wait.Backoff{
	Steps:    60,
	Duration: 500 * time.Millisecond,
	Factor:   1.5,
	Jitter:   0.1,
}

// Controller manages code execution across runtimes.
type Controller struct {
	baseURL                        string
	token                          string
	mu                             sync.RWMutex
	jupyterClientMap               map[string]*jupyterKernel
	defaultLanguageJupyterSessions map[Language]string
	commandClientMap               map[string]*commandKernel
	db                             *sql.DB
	dbOnce                         sync.Once
}

type jupyterKernel struct {
	mu       sync.Mutex
	kernelID string
	client   *jupyter.Client
	language Language
}

type commandKernel struct {
	pid          int
	stdoutPath   string
	stderrPath   string
	startedAt    time.Time
	finishedAt   *time.Time
	exitCode     *int
	errMsg       string
	running      bool
	isBackground bool
	content      string
}

// NewController creates a runtime controller.
func NewController(baseURL, token string) *Controller {
	return &Controller{
		baseURL: baseURL,
		token:   token,

		jupyterClientMap:               make(map[string]*jupyterKernel),
		defaultLanguageJupyterSessions: make(map[Language]string),
		commandClientMap:               make(map[string]*commandKernel),
	}
}

// Execute dispatches a request to the correct backend.
func (c *Controller) Execute(request *ExecuteCodeRequest) error {
	var cancel context.CancelFunc
	var ctx context.Context
	if request.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), request.Timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	switch request.Language {
	case Command:
		return c.runCommand(ctx, request)
	case BackgroundCommand:
		return c.runBackgroundCommand(ctx, request)
	case Bash, Python, Java, JavaScript, TypeScript, Go:
		return c.runJupyter(ctx, request)
	case SQL:
		return c.runSQL(ctx, request)
	default:
		return fmt.Errorf("unknown language: %s", request.Language)
	}
}
