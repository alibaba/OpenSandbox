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
	"strings"

	"github.com/alibaba/opensandbox/egress/pkg/log"
)

// transparentHTTPRules returns argv lines for transparent OUTPUT redirect (append or delete via op).
// op must be "-A" or "-D".
func transparentHTTPRules(localPort, mitmUID int, op string) [][]string {
	target := strconv.Itoa(localPort)
	uid := strconv.Itoa(mitmUID)
	loopRules := [][]string{
		{"iptables", "-t", "nat", op, "OUTPUT", "-p", "tcp", "-d", "127.0.0.0/8", "-j", "RETURN"},
	}
	redir := [][]string{
		{
			"iptables", "-t", "nat", op, "OUTPUT", "-p", "tcp",
			"-m", "owner", "!", "--uid-owner", uid,
			"-m", "multiport", "--dports", "80,443",
			"-j", "REDIRECT", "--to-ports", target,
		},
	}
	return append(loopRules, redir...)
}

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

	rules := transparentHTTPRules(localPort, mitmUID, "-A")
	for _, args := range rules {
		if output, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables transparent: %v (output: %s)", err, output)
		}
	}
	log.Infof("iptables transparent rules installed successfully")
	return nil
}

// RemoveTransparentHTTP deletes rules installed by SetupTransparentHTTP with the same port and mitmUID.
// Deletion order is reverse of insertion. Missing rules are ignored so teardown is best-effort.
func RemoveTransparentHTTP(localPort, mitmUID int) {
	if runtime.GOOS != "linux" {
		return
	}
	if localPort <= 0 || mitmUID < 0 {
		return
	}
	rules := transparentHTTPRules(localPort, mitmUID, "-D")
	for i := len(rules) - 1; i >= 0; i-- {
		args := rules[i]
		if output, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			log.Warnf("iptables transparent remove rule (ignored): %v (output: %s)", err, strings.TrimSpace(string(output)))
		}
	}
	log.Infof("iptables transparent rules removed")
}
