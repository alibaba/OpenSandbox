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

package mitmproxy

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
	"github.com/alibaba/opensandbox/egress/pkg/log"
)

const RunAsUser = "mitmproxy"

// listenHostLoopback binds mitmdump to loopback only. Transparent mode receives traffic via iptables REDIRECT
// to this port; listening on 0.0.0.0 would expose an open proxy to any interface in the netns.
const listenHostLoopback = "127.0.0.1"

// Config controls mitmdump --mode transparent.
type Config struct {
	ListenPort int
	// UserName is the passwd entry used to run mitmdump (must match iptables ! --uid-owner).
	UserName string
	// ConfDir is passed as --set confdir=... (CA and state).
	ConfDir string
	// ScriptPath optional mitmproxy script (-s) for addons (e.g. inject headers).
	ScriptPath string
}

// Running is a started mitmdump. Call GracefulShutdown before process exit to send SIGTERM and reap it.
type Running struct {
	Cmd  *exec.Cmd
	done chan error
}

func LookupUser(userName string) (uid, gid int, home string, err error) {
	if strings.TrimSpace(userName) == "" {
		userName = RunAsUser
	}
	u, err := user.Lookup(userName)
	if err != nil {
		return 0, 0, "", err
	}
	uid64, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return 0, 0, "", err
	}
	gid64, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return 0, 0, "", err
	}
	if uid64 > uint64(math.MaxInt) {
		return 0, 0, "", fmt.Errorf("mitmproxy: UID %d overflows int", uid64)
	}
	if gid64 > uint64(math.MaxInt) {
		return 0, 0, "", fmt.Errorf("mitmproxy: GID %d overflows int", gid64)
	}
	return int(uid64), int(gid64), u.HomeDir, nil
}

// Launch starts mitmdump and returns immediately after the process is running.
func Launch(cfg Config) (*Running, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("mitmproxy: transparent mitmdump is only supported on linux")
	}

	if cfg.ListenPort <= 0 {
		return nil, fmt.Errorf("mitmproxy: invalid listen port")
	}
	uname := cfg.UserName
	if strings.TrimSpace(uname) == "" {
		uname = RunAsUser
	}
	uid, gid, home, err := LookupUser(uname)
	if err != nil {
		return nil, fmt.Errorf("mitmproxy: lookup user %q: %w", uname, err)
	}

	args := []string{
		"--mode", "transparent",
		"--listen-host", listenHostLoopback,
		"--listen-port", strconv.Itoa(cfg.ListenPort),
	}

	trustDir := strings.TrimSpace(os.Getenv(constants.EnvMitmproxyUpstreamTrustDir))
	if trustDir == "" {
		trustDir = "/etc/ssl/certs"
	}
	args = append(args, "--set", "ssl_verify_upstream_trusted_confdir="+trustDir)
	homeEnv := home
	if strings.TrimSpace(cfg.ConfDir) != "" {
		cd := strings.TrimSpace(cfg.ConfDir)
		args = append(args, "--set", "confdir="+cd)
		homeEnv = cd
	}
	if strings.TrimSpace(cfg.ScriptPath) != "" {
		args = append(args, "-s", strings.TrimSpace(cfg.ScriptPath))
	}

	// Passthrough: no TLS interception for matching host/IP (regex). Each pattern -> --set ignore_hosts=...
	// https://docs.mitmproxy.org/stable/concepts/options/ — transparent mode often works better with IP ranges.
	for _, p := range strings.Split(os.Getenv(constants.EnvMitmproxyIgnoreHosts), ";") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		args = append(args, "--set", "ignore_hosts="+p)
	}

	cmd := exec.Command("mitmdump", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
	}
	cmd.Env = append(os.Environ(), "HOME="+homeEnv)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mitmproxy: start mitmdump: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	log.Infof("[mitmproxy] mitmdump started (pid %d, transparent on %s:%d)", cmd.Process.Pid, listenHostLoopback, cfg.ListenPort)
	return &Running{Cmd: cmd, done: done}, nil
}
