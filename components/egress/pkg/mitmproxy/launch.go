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
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/alibaba/opensandbox/egress/pkg/log"
)

const RunAsUser = "mitmproxy"

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

// LookupUser resolves uid/gid and home directory for UserName (default mitmproxy).
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
	return int(uid64), int(gid64), u.HomeDir, nil
}

// Launch starts mitmdump and returns immediately after the process is running (Start, not Wait).
func Launch(ctx context.Context, cfg Config) (*exec.Cmd, error) {
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
		"--listen-host", "0.0.0.0",
		"--listen-port", strconv.Itoa(cfg.ListenPort),
		"--set", "block_global=false",
	}
	homeEnv := home
	if strings.TrimSpace(cfg.ConfDir) != "" {
		cd := strings.TrimSpace(cfg.ConfDir)
		args = append(args, "--set", "confdir="+cd)
		homeEnv = cd
	}
	if strings.TrimSpace(cfg.ScriptPath) != "" {
		args = append(args, "-s", strings.TrimSpace(cfg.ScriptPath))
	}

	cmd := exec.CommandContext(ctx, "mitmdump", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
	}
	cmd.Env = append(os.Environ(), "HOME="+homeEnv)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mitmproxy: start mitmdump: %w", err)
	}
	go func() {
		err := cmd.Wait()
		if err != nil && ctx.Err() == nil {
			log.Errorf("[mitmproxy] mitmdump exited: %v", err)
		}
	}()
	log.Infof("[mitmproxy] mitmdump started (pid %d, transparent on :%d)", cmd.Process.Pid, cfg.ListenPort)
	return cmd, nil
}
