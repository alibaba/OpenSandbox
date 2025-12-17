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

package main

import (
	"fmt"

	"github.com/beego/beego/v2/core/logs"
	"github.com/beego/beego/v2/server/web"

	_ "go.uber.org/automaxprocs/maxprocs"

	"github.com/alibaba/opensandbox/execd/pkg/flag"
	_ "github.com/alibaba/opensandbox/execd/pkg/util/safego"
	_ "github.com/alibaba/opensandbox/execd/pkg/web"
	myweb "github.com/alibaba/opensandbox/execd/pkg/web"
	"github.com/alibaba/opensandbox/execd/pkg/web/controller"
)

// main initializes and starts the execd server.
func main() {
	flag.InitFlags()

	logs.SetLevel(flag.ServerLogLevel)
	web.BConfig.Listen.HTTPPort = flag.ServerPort
	myweb.SetAccessToken(flag.ServerAccessToken)

	controller.InitCodeRunner()
	addr := fmt.Sprintf(":%d", flag.ServerPort)
	logs.Info("execd listening on %s", addr)
	web.RunWithMiddleWares(addr, myweb.ProxyMiddleware())
}

// init configures beego before main runs.
func init() {
	web.BConfig.CopyRequestBody = true
	web.BConfig.RecoverPanic = true
	web.BConfig.WebConfig.AutoRender = false
	web.BConfig.WebConfig.Session.SessionOn = false
	web.BConfig.RouterCaseSensitive = false
}
