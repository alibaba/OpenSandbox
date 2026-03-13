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
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/alibaba/opensandbox/execd/pkg/runtime"
	"github.com/alibaba/opensandbox/execd/pkg/web/model"
	"github.com/stretchr/testify/require"
)

func TestBuildExecuteCodeRequestDefaultsToCommand(t *testing.T) {
	ctrl := &CodeInterpretingController{}
	req := model.RunCodeRequest{
		Code: "echo 1",
		Context: model.CodeContext{
			ID:                 "session-1",
			CodeContextRequest: model.CodeContextRequest{},
		},
	}

	execReq := ctrl.buildExecuteCodeRequest(req)

	require.Equal(t, runtime.Command, execReq.Language, "expected default language")
	require.Equal(t, "session-1", execReq.Context)
	require.Equal(t, "echo 1", execReq.Code)
}

func TestBuildExecuteCodeRequestRespectsLanguage(t *testing.T) {
	ctrl := &CodeInterpretingController{}
	req := model.RunCodeRequest{
		Code: "print(1)",
		Context: model.CodeContext{
			ID: "session-2",
			CodeContextRequest: model.CodeContextRequest{
				Language: "python",
			},
		},
	}

	execReq := ctrl.buildExecuteCodeRequest(req)

	require.Equal(t, runtime.Language("python"), execReq.Language)
}

func TestGetContext_NotFoundReturns404(t *testing.T) {
	ctx, w := newTestContext(http.MethodGet, "/code/contexts/missing", nil)
	ctx.Params = append(ctx.Params, gin.Param{Key: "contextId", Value: "missing"})
	ctrl := NewCodeInterpretingController(ctx)

	previous := codeRunner
	codeRunner = runtime.NewController("", "")
	t.Cleanup(func() { codeRunner = previous })

	ctrl.GetContext()

	require.Equal(t, http.StatusNotFound, w.Code)

	var resp model.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, model.ErrorCodeContextNotFound, resp.Code)
	require.Equal(t, "context missing not found", resp.Message)
}

func TestGetContext_MissingIDReturns400(t *testing.T) {
	ctx, w := newTestContext(http.MethodGet, "/code/contexts/", nil)
	ctrl := NewCodeInterpretingController(ctx)

	ctrl.GetContext()

	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp model.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, model.ErrorCodeMissingQuery, resp.Code)
	require.Equal(t, "missing path parameter 'contextId'", resp.Message)
}
