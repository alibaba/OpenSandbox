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
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
	"github.com/alibaba/opensandbox/egress/pkg/iptables"
	"github.com/alibaba/opensandbox/egress/pkg/log"
	"github.com/alibaba/opensandbox/egress/pkg/mitmproxy"
	"github.com/alibaba/opensandbox/internal/safego"
)

type mitmTransparent struct {
	mu        sync.Mutex
	running   *mitmproxy.Running
	port      int
	uid       uint32
	cfg       mitmproxy.Config
	restartCh chan error
}

func (m *mitmTransparent) getRunning() *mitmproxy.Running {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *mitmTransparent) setRunning(r *mitmproxy.Running) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = r
}

// startMitmproxyTransparentIfEnabled starts mitmdump in transparent mode, waits for the listener, and installs OUTPUT REDIRECT, then syncs the CA.
func startMitmproxyTransparentIfEnabled() (*mitmTransparent, error) {
	if !constants.IsTruthy(os.Getenv(constants.EnvMitmproxyTransparent)) {
		return nil, nil
	}

	mpPort := constants.EnvIntOrDefault(constants.EnvMitmproxyPort, constants.DefaultMitmproxyPort)
	mpUID, _, mpHome, err := mitmproxy.LookupUser(mitmproxy.RunAsUser)
	if err != nil {
		return nil, fmt.Errorf("lookup user %q: %w (ensure this user exists in the image)", mitmproxy.RunAsUser, err)
	}

	cfg := mitmproxy.Config{
		ListenPort: mpPort,
		UserName:   mitmproxy.RunAsUser,
		ConfDir:    strings.TrimSpace(os.Getenv(constants.EnvMitmproxyConfDir)),
		ScriptPath: strings.TrimSpace(os.Getenv(constants.EnvMitmproxyScript)),
	}
	restartCh := make(chan error, 1)
	cfg.OnExit = func(err error) {
		select {
		case restartCh <- err:
		default:
		}
	}
	running, err := mitmproxy.Launch(cfg)
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
	return &mitmTransparent{running: running, port: mpPort, uid: mpUID, cfg: cfg, restartCh: restartCh}, nil
}

// watchMitmproxy monitors mitmdump for unexpected exits, logs the error, and restarts it.
// Must be called after startMitmproxyTransparentIfEnabled.
func (m *mitmTransparent) watchMitmproxy(ctx context.Context, gate *mitmproxy.HealthGate) {
	safego.Go(func() {
		for {
			select {
			case err := <-m.restartCh:
				select {
				case <-ctx.Done():
					return
				default:
				}

				log.Errorf("[mitmproxy] mitmdump exited: %v; restarting...", err)
				gate.SetReady(false)
				m.restartWithBackoff(ctx, gate)

			case <-ctx.Done():
				return
			}
		}
	})
}

// restartWithBackoff retries mitmdump launch indefinitely with exponential backoff
// (1s, 2s, 4s, ..., capped at 30s) until it succeeds or ctx is cancelled.
// Transient OOM / resource pressure must not leave egress in a permanent dead state.
func (m *mitmTransparent) restartWithBackoff(ctx context.Context, gate *mitmproxy.HealthGate) {
	const (
		initialBackoff = time.Second
		maxBackoff     = 30 * time.Second
	)
	backoff := initialBackoff
	waitAddr := fmt.Sprintf("127.0.0.1:%d", m.cfg.ListenPort)

	for attempt := 1; ; attempt++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		newRunning, launchErr := mitmproxy.Launch(m.cfg)
		if launchErr == nil {
			if waitErr := mitmproxy.WaitListenPort(waitAddr, 15*time.Second); waitErr == nil {
				m.setRunning(newRunning)
				gate.SetReady(true)
				log.Infof("[mitmproxy] mitmdump restarted (pid %d, attempt %d)", newRunning.Cmd.Process.Pid, attempt)
				// Drain any stale exit signal queued by killed half-launched attempts.
				select {
				case <-m.restartCh:
				default:
				}
				return
			} else {
				log.Errorf("[mitmproxy] restart attempt %d: wait listen %s: %v", attempt, waitAddr, waitErr)
				if newRunning.Cmd != nil && newRunning.Cmd.Process != nil {
					_ = newRunning.Cmd.Process.Kill()
				}
			}
		} else {
			log.Errorf("[mitmproxy] restart attempt %d: launch failed: %v", attempt, launchErr)
		}

		log.Warnf("[mitmproxy] restart attempt %d failed; retrying in %s", attempt, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}
