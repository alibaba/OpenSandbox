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
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/flag"
	"github.com/alibaba/opensandbox/execd/pkg/runtime"
	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

// RunCommand executes a shell command and streams the output via SSE.
func (c *CodeInterpretingController) RunCommand() {
	var request model.RunCommandRequest
	if err := c.bindJSON(&request); err != nil {
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

	ctx, cancel := context.WithCancel(c.ctx.Request.Context())
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

// GetCommandStatus returns command status by id.
func (c *CodeInterpretingController) GetCommandStatus() {
	commandID := c.ctx.Param("id")
	if commandID == "" {
		c.RespondError(http.StatusBadRequest, model.ErrorCodeInvalidRequest, "missing command execution id")
		return
	}

	status, err := codeRunner.GetCommandStatus(commandID)
	if err != nil {
		c.RespondError(http.StatusNotFound, model.ErrorCodeInvalidRequest, err.Error())
		return
	}

	resp := model.CommandStatusResponse{
		ID:       status.Session,
		Running:  status.Running,
		ExitCode: status.ExitCode,
		Error:    status.Error,
		Content:  status.Content,
	}
	if !status.StartedAt.IsZero() {
		resp.StartedAt = status.StartedAt
	}
	if status.FinishedAt != nil {
		resp.FinishedAt = status.FinishedAt
	}

	c.RespondSuccess(resp)
}

// GetBackgroundCommandOutput returns accumulated stdout/stderr for a command session as plain text.
func (c *CodeInterpretingController) GetBackgroundCommandOutput() {
	id := c.ctx.Param("id")
	if id == "" {
		c.RespondError(http.StatusBadRequest, model.ErrorCodeMissingQuery, "missing command execution id")
		return
	}

	cursor := c.QueryInt64(c.ctx.Query("cursor"), 0)
	output, lastCursor, err := codeRunner.SeekBackgroundCommandOutput(id, cursor)
	if err != nil {
		c.RespondError(http.StatusBadRequest, model.ErrorCodeInvalidRequest, err.Error())
		return
	}

	c.ctx.Header("EXECD-COMMANDS-TAIL-CURSOR", strconv.FormatInt(lastCursor, 10))
	c.ctx.Header("Content-Type", "text/plain; charset=utf-8")
	c.ctx.String(http.StatusOK, "%s", output)
}

func (c *CodeInterpretingController) buildExecuteCommandRequest(request model.RunCommandRequest) *runtime.ExecuteCodeRequest {
	timeout := time.Duration(request.TimeoutMs) * time.Millisecond
	if request.Background {
		return &runtime.ExecuteCodeRequest{
			Language: runtime.BackgroundCommand,
			Code:     request.Command,
			Cwd:      request.Cwd,
			Timeout:  timeout,
		}
	} else {
		return &runtime.ExecuteCodeRequest{
			Language: runtime.Command,
			Code:     request.Command,
			Cwd:      request.Cwd,
			Timeout:  timeout,
		}
	}
}
