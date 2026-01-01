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
	"net/http"
	"strconv"

	"github.com/beego/beego/v2/server/web"

	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

type basicController struct {
	web.Controller
}

func (c *basicController) RespondError(status int, code model.ErrorCode, message ...string) {
	c.Ctx.Output.SetStatus(status)
	c.Data["json"] = model.ErrorResponse{
		Code: code,
		Message: func() string {
			if len(message) > 0 {
				return message[0]
			}
			return ""
		}(),
	}
	_ = c.ServeJSON()
}

func (c *basicController) RespondSuccess(data any) {
	c.Ctx.Output.SetStatus(http.StatusOK)
	if data != nil {
		c.Data["json"] = data
	}
	_ = c.ServeJSON()
}

func (c *basicController) QueryInt64(query string, defaultValue int64) int64 {
	val, err := strconv.ParseInt(query, 10, 64)
	if err != nil {
		return defaultValue
	}
	return val
}
