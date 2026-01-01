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

package web

import (
	"net/http"

	"github.com/beego/beego/v2/core/logs"
	"github.com/beego/beego/v2/server/web"
	"github.com/beego/beego/v2/server/web/context"

	"github.com/alibaba/opensandbox/execd/pkg/web/controller"
	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

var accessToken string

// SetAccessToken configures the API token.
func SetAccessToken(token string) {
	accessToken = token
}

// AccessTokenFilter enforces a static token check.
func AccessTokenFilter(ctx *context.Context) {
	if accessToken == "" {
		return
	}

	requestedToken := ctx.Input.Header(model.ApiAccessTokenHeader)
	if requestedToken == "" || requestedToken != accessToken {
		ctx.Output.SetStatus(http.StatusUnauthorized)
		_ = ctx.Output.JSON(map[string]any{
			"error": "Unauthorized: invalid or missing header " + model.ApiAccessTokenHeader,
		}, true, false)
	}
}

// LogFilter logs incoming HTTP requests.
func LogFilter(ctx *context.Context) {
	logs.Info("Requested: %v - %v", ctx.Request.Method, ctx.Request.URL.String())
}

func init() {
	web.Router("/ping", &controller.MainController{}, "get:Ping")

	files := web.NewNamespace("/files",
		web.NSRouter("", &controller.FilesystemController{}, "delete:RemoveFiles"),
		web.NSRouter("/info", &controller.FilesystemController{}, "get:GetFilesInfo"),
		web.NSRouter("/mv", &controller.FilesystemController{}, "post:RenameFiles"),
		web.NSRouter("/permissions", &controller.FilesystemController{}, "post:ChmodFiles"),
		web.NSRouter("/search", &controller.FilesystemController{}, "get:SearchFiles"),
		web.NSRouter("/replace", &controller.FilesystemController{}, "post:ReplaceContent"),
		web.NSRouter("/upload", &controller.FilesystemController{}, "post:UploadFile"),
		web.NSRouter("/download", &controller.FilesystemController{}, "get:DownloadFile"),
	)

	directories := web.NewNamespace("/directories",
		web.NSRouter("", &controller.FilesystemController{}, "post:MakeDirs;delete:RemoveDirs"),
	)

	codeInterpreting := web.NewNamespace("/code",
		web.NSRouter("", &controller.CodeInterpretingController{}, "post:RunCode;delete:InterruptCode"),
		web.NSRouter("/context", &controller.CodeInterpretingController{}, "post:CreateContext"),
	)

	command := web.NewNamespace("/command",
		web.NSRouter("", &controller.CodeInterpretingController{}, "post:RunCommand;delete:InterruptCommand"),
		web.NSRouter("/status/:id", &controller.CodeInterpretingController{}, "get:GetCommandStatus"),
		web.NSRouter("/:id/logs", &controller.CodeInterpretingController{}, "get:GetBackgroundCommandOutput"),
	)

	metric := web.NewNamespace("metrics",
		web.NSRouter("", &controller.MetricController{}, "get:GetMetrics"),
		web.NSRouter("/watch", &controller.MetricController{}, "get:WatchMetrics"),
	)

	web.AddNamespace(files)
	web.AddNamespace(directories)
	web.AddNamespace(codeInterpreting)
	web.AddNamespace(command)
	web.AddNamespace(metric)

	web.InsertFilter("/*", web.BeforeRouter, AccessTokenFilter)
	web.InsertFilter("/*", web.BeforeRouter, LogFilter)
}
