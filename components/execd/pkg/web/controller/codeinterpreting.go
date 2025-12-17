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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/flag"
	"github.com/alibaba/opensandbox/execd/pkg/runtime"
	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

var codeRunner *runtime.Controller

func InitCodeRunner() {
	codeRunner = runtime.NewController(flag.JupyterServerHost, flag.JupyterServerToken)
}

// CodeInterpretingController handles code execution entrypoints.
type CodeInterpretingController struct {
	basicController

	// chunkWriter serializes SSE event writes to prevent interleaved output.
	chunkWriter sync.Mutex
}

// CreateContext creates a new code execution context.
func (c *CodeInterpretingController) CreateContext() {
	var request model.CodeContextRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &request); err != nil {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeInvalidRequest,
			fmt.Sprintf("error parsing request, MAYBE invalid body format. %v", err),
		)
		return
	}

	session, err := codeRunner.CreateContext(&runtime.CreateContextRequest{
		Language: runtime.Language(request.Language),
		Cwd:      request.Cwd,
	})
	if err != nil {
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error creating code context. %v", err),
		)
		return
	}

	resp := model.CodeContext{
		ID:                 session,
		CodeContextRequest: request,
	}
	c.RespondSuccess(resp)
}

// InterruptCode interrupts the execution of running code in a session.
func (c *CodeInterpretingController) InterruptCode() {
	c.interrupt()
}

// RunCode executes code in a context and streams output via SSE.
func (c *CodeInterpretingController) RunCode() {
	var request model.RunCodeRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &request); err != nil {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeInvalidRequest,
			fmt.Sprintf("error parsing request, MAYBE invalid body format. %v", err),
		)
		return
	}

	err := request.Validate()
	if err != nil {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeInvalidRequest,
			fmt.Sprintf("invalid request, validation error %v", err),
		)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runCodeRequest := c.buildExecuteCodeRequest(request)
	eventsHandler := c.setServerEventsHandler(ctx)
	runCodeRequest.Hooks = eventsHandler

	c.setupSSEResponse()
	err = codeRunner.Execute(runCodeRequest)
	if err != nil {
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error running codes %v", err),
		)
		return
	}

	time.Sleep(flag.ApiGracefulShutdownTimeout)
}

// buildExecuteCodeRequest converts a RunCodeRequest to runtime format.
func (c *CodeInterpretingController) buildExecuteCodeRequest(request model.RunCodeRequest) *runtime.ExecuteCodeRequest {
	req := &runtime.ExecuteCodeRequest{
		Language: runtime.Language(request.Context.Language),
		Code:     request.Code,
		Context:  request.Context.ID,
	}

	if req.Language == "" {
		req.Language = runtime.Command
	}

	return req
}

func (c *CodeInterpretingController) interrupt() {
	session := c.GetString("id")
	if session == "" {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeMissingQuery,
			"missing query parameter 'id'",
		)
		return
	}

	err := codeRunner.Interrupt(session)
	if err != nil {
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error interruptting code context. %v", err),
		)
		return
	}

	c.RespondSuccess(nil)
}
