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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

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
	*basicController

	// chunkWriter serializes SSE event writes to prevent interleaved output.
	chunkWriter sync.Mutex
}

func NewCodeInterpretingController(ctx *gin.Context) *CodeInterpretingController {
	return &CodeInterpretingController{
		basicController: newBasicController(ctx),
	}
}

// CreateContext creates a new code execution context.
func (c *CodeInterpretingController) CreateContext() {
	var request model.CodeContextRequest
	if err := c.bindJSON(&request); err != nil {
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

// GetContext returns a specific code context by id.
func (c *CodeInterpretingController) GetContext() {
	contextID := c.ctx.Param("contextId")
	if contextID == "" {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeMissingQuery,
			"missing path parameter 'contextId'",
		)
		return
	}

	codeContext, err := codeRunner.GetContext(contextID)
	if err != nil {
		if errors.Is(err, runtime.ErrContextNotFound) {
			c.RespondError(
				http.StatusNotFound,
				model.ErrorCodeContextNotFound,
				fmt.Sprintf("context %s not found", contextID),
			)
			return
		}
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error getting code context %s. %v", contextID, err),
		)
		return
	}
	c.RespondSuccess(codeContext)
}

// ListContexts returns active code contexts, optionally filtered by language.
func (c *CodeInterpretingController) ListContexts() {
	language := c.ctx.Query("language")

	contexts, err := codeRunner.ListContext(language)
	if err != nil {
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			err.Error(),
		)
		return
	}

	c.RespondSuccess(contexts)
}

// DeleteContextsByLanguage deletes all contexts for a given language.
func (c *CodeInterpretingController) DeleteContextsByLanguage() {
	language := c.ctx.Query("language")
	if language == "" {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeMissingQuery,
			"missing query parameter 'language'",
		)
		return
	}

	err := codeRunner.DeleteLanguageContext(runtime.Language(language))
	if err != nil {
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error deleting code context %s. %v", language, err),
		)
		return
	}

	c.RespondSuccess(nil)
}

// DeleteContext deletes a specific code context by id.
func (c *CodeInterpretingController) DeleteContext() {
	contextID := c.ctx.Param("contextId")
	if contextID == "" {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeMissingQuery,
			"missing path parameter 'contextId'",
		)
		return
	}

	err := codeRunner.DeleteContext(contextID)
	if err != nil {
		if errors.Is(err, runtime.ErrContextNotFound) {
			c.RespondError(
				http.StatusNotFound,
				model.ErrorCodeContextNotFound,
				fmt.Sprintf("context %s not found", contextID),
			)
			return
		} else {
			c.RespondError(
				http.StatusInternalServerError,
				model.ErrorCodeRuntimeError,
				fmt.Sprintf("error deleting code context %s. %v", contextID, err),
			)
			return
		}
	}

	c.RespondSuccess(nil)
}

// CreateSession creates a new bash session (create_session API).
// An empty body is allowed and is treated as default options (no cwd override).
func (c *CodeInterpretingController) CreateSession() {
	var request model.CreateSessionRequest
	if err := c.bindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeInvalidRequest,
			fmt.Sprintf("error parsing request. %v", err),
		)
		return
	}

	sessionID, err := codeRunner.CreateBashSession(&runtime.CreateContextRequest{
		Cwd: request.Cwd,
	})
	if err != nil {
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error creating session. %v", err),
		)
		return
	}

	c.RespondSuccess(model.CreateSessionResponse{SessionID: sessionID})
}

// RunInSession runs code in an existing bash session and streams output via SSE (run_in_session API).
func (c *CodeInterpretingController) RunInSession() {
	sessionID := c.ctx.Param("sessionId")
	if sessionID == "" {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeMissingQuery,
			"missing path parameter 'sessionId'",
		)
		return
	}

	var request model.RunInSessionRequest
	if err := c.bindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeInvalidRequest,
			fmt.Sprintf("error parsing request. %v", err),
		)
		return
	}
	if err := request.Validate(); err != nil {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeInvalidRequest,
			fmt.Sprintf("invalid request. %v", err),
		)
		return
	}

	timeout := time.Duration(request.TimeoutMs) * time.Millisecond
	runReq := &runtime.ExecuteCodeRequest{
		Language: runtime.Bash,
		Context:  sessionID,
		Code:     request.Code,
		Cwd:      request.Cwd,
		Timeout:  timeout,
	}
	ctx, cancel := context.WithCancel(c.ctx.Request.Context())
	defer cancel()
	runReq.Hooks = c.setServerEventsHandler(ctx)

	c.setupSSEResponse()

	// If ?since=<offset> is provided, replay buffered output before live stream.
	if sinceStr := c.ctx.Query("since"); sinceStr != "" {
		if since, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			if replayData, _, replayErr := codeRunner.ReplaySessionOutput(sessionID, since); replayErr == nil && len(replayData) > 0 {
				event := model.ServerStreamEvent{
					Type:      model.StreamEventTypeReplay,
					Text:      string(replayData),
					Timestamp: time.Now().UnixMilli(),
				}
				c.writeSingleEvent("Replay", event.ToJSON(), true, event.Summary())
			}
		}
	}

	err := codeRunner.RunInBashSession(ctx, runReq)
	if err != nil {
		if errors.Is(err, runtime.ErrContextNotFound) {
			c.RespondError(
				http.StatusNotFound,
				model.ErrorCodeContextNotFound,
				fmt.Sprintf("session not found. %v", err),
			)
			return
		}
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error running in session. %v", err),
		)
		return
	}

	time.Sleep(flag.ApiGracefulShutdownTimeout)
}

// GetSessionStatus returns status and replay buffer offset for a bash session.
func (c *CodeInterpretingController) GetSessionStatus() {
	sessionID := c.ctx.Param("sessionId")
	if sessionID == "" {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeMissingQuery,
			"missing path parameter 'sessionId'",
		)
		return
	}

	status, err := codeRunner.GetBashSessionStatus(sessionID)
	if err != nil {
		if errors.Is(err, runtime.ErrContextNotFound) {
			c.RespondError(
				http.StatusNotFound,
				model.ErrorCodeContextNotFound,
				fmt.Sprintf("session %s not found", sessionID),
			)
			return
		}
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error getting session status. %v", err),
		)
		return
	}

	c.RespondSuccess(model.SessionStatusResponse{
		SessionID:    status.SessionID,
		Running:      status.Running,
		OutputOffset: status.OutputOffset,
	})
}

// DeleteSession deletes a bash session (delete_session API).
func (c *CodeInterpretingController) DeleteSession() {
	sessionID := c.ctx.Param("sessionId")
	if sessionID == "" {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeMissingQuery,
			"missing path parameter 'sessionId'",
		)
		return
	}

	err := codeRunner.DeleteBashSession(sessionID)
	if err != nil {
		if errors.Is(err, runtime.ErrContextNotFound) {
			c.RespondError(
				http.StatusNotFound,
				model.ErrorCodeContextNotFound,
				fmt.Sprintf("session %s not found", sessionID),
			)
			return
		}
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error deleting session %s. %v", sessionID, err),
		)
		return
	}

	c.RespondSuccess(nil)
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
	session := c.ctx.Query("id")
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
