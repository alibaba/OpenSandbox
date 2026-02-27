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

	_ "go.uber.org/automaxprocs/maxprocs"

	"github.com/alibaba/opensandbox/execd/pkg/flag"
	"github.com/alibaba/opensandbox/execd/pkg/log"
	_ "github.com/alibaba/opensandbox/execd/pkg/util/safego"
	"github.com/alibaba/opensandbox/execd/pkg/web"
	"github.com/alibaba/opensandbox/execd/pkg/web/controller"
	"github.com/alibaba/opensandbox/internal/version"
)

// main initializes and starts the execd server.
func main() {
	version.EchoVersion("OpenSandbox Execd")

	flag.InitFlags()

	log.Init(flag.ServerLogLevel)

	controller.InitCodeRunner()
	engine := web.NewRouter(flag.ServerAccessToken)
	addr := fmt.Sprintf(":%d", flag.ServerPort)
	log.Info("execd listening on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Error("failed to start execd server: %v", err)
	}
}
