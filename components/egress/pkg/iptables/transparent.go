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

package iptables

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/alibaba/opensandbox/egress/pkg/log"
)

// SetupTransparentHTTP redirects locally originated TCP 80/443 to localPort for processes
// whose UID is not mitmUID.
//
// IPv4 only.
func SetupTransparentHTTP(localPort, mitmUID int) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("iptables transparent: only supported on linux")
	}

	if localPort <= 0 || mitmUID < 0 {
		return fmt.Errorf("iptables transparent: invalid port or uid")
	}
	target := strconv.Itoa(localPort)
	uid := strconv.Itoa(mitmUID)
	log.Infof("installing iptables transparent: OUTPUT tcp dport 80,443 -> 127.0.0.1:%s (skip uid %s)", target, uid)

	// Do not redirect loopback destinations (local services).
	loopRules := [][]string{
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "-d", "127.0.0.0/8", "-j", "RETURN"},
	}
	redir := [][]string{
		{
			"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp",
			"-m", "owner", "!", "--uid-owner", uid,
			"-m", "multiport", "--dports", "80,443",
			"-j", "REDIRECT", "--to-ports", target,
		},
	}
	rules := append(loopRules, redir...)

	for _, args := range rules {
		if output, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables transparent: %v (output: %s)", err, output)
		}
	}
	log.Infof("iptables transparent rules installed successfully")
	return nil
}
