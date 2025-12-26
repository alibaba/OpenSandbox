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
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/flag"
	"github.com/alibaba/opensandbox/execd/pkg/runtime"
	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

// RunCommand executes a shell command and streams the output via SSE.
func (c *CodeInterpretingController) RunCommand() {
	var request model.RunCommandRequest
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

	runCodeRequest := c.buildExecuteCommandRequest(request)
	eventsHandler := c.setServerEventsHandler(ctx)
	runCodeRequest.Hooks = eventsHandler

	c.setupSSEResponse()
	err = codeRunner.Execute(runCodeRequest)
	if err != nil {
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error running commands %v", err),
		)
		return
	}

	time.Sleep(flag.ApiGracefulShutdownTimeout)
}

// InterruptCommand stops a running shell command session.
func (c *CodeInterpretingController) InterruptCommand() {
	c.interrupt()
}

// GetCommandStatus returns command status by session id.
func (c *CodeInterpretingController) GetCommandStatus() {
	session := c.Ctx.Input.Param(":session")
	if session == "" {
		c.RespondError(http.StatusBadRequest, model.ErrorCodeInvalidRequest, "missing session")
		return
	}

	status, err := codeRunner.GetCommandStatus(session)
	if err != nil {
		c.RespondError(http.StatusNotFound, model.ErrorCodeInvalidRequest, err.Error())
		return
	}

	resp := model.CommandStatusResponse{
		Session:  status.Session,
		Running:  status.Running,
		ExitCode: status.ExitCode,
		Error:    status.Error,
	}
	if !status.StartedAt.IsZero() {
		resp.StartedAt = status.StartedAt
	}
	if status.FinishedAt != nil {
		resp.FinishedAt = status.FinishedAt
	}

	c.RespondSuccess(resp)
}

// GetCommandOutput returns accumulated stdout/stderr for a command session.
func (c *CodeInterpretingController) GetCommandOutput() {
	session := c.Ctx.Input.Param(":session")
	if session == "" {
		c.RespondError(http.StatusBadRequest, model.ErrorCodeInvalidRequest, "missing session")
		return
	}

	output, err := codeRunner.GetCommandOutput(session)
	if err != nil {
		c.RespondError(http.StatusNotFound, model.ErrorCodeInvalidRequest, err.Error())
		return
	}

	resp := model.CommandOutputResponse{
		CommandStatusResponse: model.CommandStatusResponse{
			Session:  output.Session,
			Running:  output.Running,
			ExitCode: output.ExitCode,
			Error:    output.Error,
		},
		Stdout: output.Stdout,
		Stderr: output.Stderr,
	}
	if !output.StartedAt.IsZero() {
		resp.StartedAt = output.StartedAt
	}
	if output.FinishedAt != nil {
		resp.FinishedAt = output.FinishedAt
	}

	c.RespondSuccess(resp)
}

func (c *CodeInterpretingController) buildExecuteCommandRequest(request model.RunCommandRequest) *runtime.ExecuteCodeRequest {
	if request.Background {
		return &runtime.ExecuteCodeRequest{
			Language: runtime.BackgroundCommand,
			Code:     request.Command,
			Cwd:      request.Cwd,
		}
	} else {
		return &runtime.ExecuteCodeRequest{
			Language: runtime.Command,
			Code:     request.Command,
			Cwd:      request.Cwd,
		}
	}
}
