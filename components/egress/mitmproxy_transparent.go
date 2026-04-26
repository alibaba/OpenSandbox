// Copyright 2026 Alibaba Group Holding Ltd.
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
	"os"
	"strings"
	"time"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
	"github.com/alibaba/opensandbox/egress/pkg/iptables"
	"github.com/alibaba/opensandbox/egress/pkg/log"
	"github.com/alibaba/opensandbox/egress/pkg/mitmproxy"
)

type mitmTransparent struct {
	running *mitmproxy.Running
	port    int
	uid     uint32
}

// startMitmproxyTransparentIfEnabled launches Python mitmdump in transparent mode and installs iptables.
func startMitmproxyTransparentIfEnabled() (*mitmTransparent, error) {
	if !constants.IsTruthy(os.Getenv(constants.EnvMitmproxyTransparent)) {
		return nil, nil
	}

	mpPort := constants.EnvIntOrDefault(constants.EnvMitmproxyPort, constants.DefaultMitmproxyPort)
	mpUID, _, mpHome, err := mitmproxy.LookupUser(mitmproxy.RunAsUser)
	if err != nil {
		return nil, fmt.Errorf("lookup user %q: %w (ensure this user exists in the image)", mitmproxy.RunAsUser, err)
	}

	running, err := mitmproxy.Launch(mitmproxy.Config{
		ListenPort: mpPort,
		UserName:   mitmproxy.RunAsUser,
		ConfDir:    strings.TrimSpace(os.Getenv(constants.EnvMitmproxyConfDir)),
		ScriptPath: strings.TrimSpace(os.Getenv(constants.EnvMitmproxyScript)),
	})
	if err != nil {
		return nil, fmt.Errorf("start mitmdump: %w", err)
	}

	waitAddr := fmt.Sprintf("127.0.0.1:%d", mpPort)
	if err := mitmproxy.WaitListenPort(waitAddr, 15*time.Second); err != nil {
		return nil, fmt.Errorf("wait listen %s: %w", waitAddr, err)
	}
	if err := iptables.SetupTransparentHTTP(mpPort, mpUID); err != nil {
		return nil, fmt.Errorf("iptables transparent: %w", err)
	}
	log.Infof("mitmproxy: transparent intercept active (OUTPUT tcp 80,443 -> %d; trust mitm CA in clients)", mpPort)

	confDir := strings.TrimSpace(os.Getenv(constants.EnvMitmproxyConfDir))
	if err := mitmproxy.SyncRootCA(confDir, mpHome); err != nil {
		return nil, fmt.Errorf("mitm CA export: %w", err)
	}
	return &mitmTransparent{running: running, port: mpPort, uid: mpUID}, nil
}
