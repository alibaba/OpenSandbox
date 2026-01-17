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

//go:build !windows
// +build !windows

package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alibaba/opensandbox/execd/pkg/log"
)

func (c *Controller) createBashSession(_ *CreateContextRequest) (string, error) {
	session := newBashSession(nil)
	if err := session.start(); err != nil {
		return "", fmt.Errorf("failed to start bash session: %w", err)
	}

	c.bashSessionClientMap.Store(session.config.Session, session)
	log.Info("created bash session %s", session.config.Session)
	return session.config.Session, nil
}

func (c *Controller) runBashSession(_ context.Context, request *ExecuteCodeRequest) error {
	if request.Context == "" {
		if c.getDefaultLanguageSession(request.Language) == "" {
			if err := c.createDefaultBashSession(); err != nil {
				return err
			}
		}
	}

	targetSessionID := request.Context
	if targetSessionID == "" {
		targetSessionID = c.getDefaultLanguageSession(request.Language)
	}

	session := c.getBashSession(targetSessionID)
	if session == nil {
		return ErrContextNotFound
	}

	return session.run(request.Code, request.Timeout, &request.Hooks)
}

func (c *Controller) createDefaultBashSession() error {
	session, err := c.createBashSession(&CreateContextRequest{})
	if err != nil {
		return err
	}

	c.setDefaultLanguageSession(Bash, session)
	return nil
}

func (c *Controller) getBashSession(sessionId string) *bashSession {
	if v, ok := c.bashSessionClientMap.Load(sessionId); ok {
		if s, ok := v.(*bashSession); ok {
			return s
		}
	}
	return nil
}

func (c *Controller) closeBashSession(sessionId string) error {
	session := c.getBashSession(sessionId)
	if session == nil {
		return ErrContextNotFound
	}

	err := session.close()
	if err != nil {
		return err
	}

	c.bashSessionClientMap.Delete(sessionId)
	return nil
}

// nolint:unused
func (c *Controller) listBashSessions() []string {
	sessions := make([]string, 0)
	c.bashSessionClientMap.Range(func(key, _ any) bool {
		sessionID, _ := key.(string)
		sessions = append(sessions, sessionID)
		return true
	})

	return sessions
}

// Session implementation (pipe-based, no PTY)
func newBashSession(config *bashSessionConfig) *bashSession {
	if config == nil {
		config = &bashSessionConfig{
			Session:        uuidString(),
			StartupTimeout: 5 * time.Second,
		}
	}
	return &bashSession{
		config:      config,
		stdoutLines: make(chan string, 256),
		stdoutErr:   make(chan error, 1),
	}
}

func (s *bashSession) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return errors.New("session already started")
	}

	cmd := exec.Command("bash", "--noprofile", "--norc", "-s")
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bash: %w", err)
	}

	s.cmd = cmd
	s.stdin = stdin
	s.stdout = stdout
	s.stderr = stderr
	s.started = true

	// drain stdout/stderr into channel
	go s.readStdout(stdout)
	go s.discardStderr(stderr)
	return nil
}

func (s *bashSession) readStdout(r io.Reader) {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			s.stdoutLines <- strings.TrimRight(line, "\r\n")
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.stdoutErr <- err
			}
			close(s.stdoutLines)
			return
		}
	}
}

func (s *bashSession) discardStderr(r io.Reader) {
	_, _ = io.Copy(io.Discard, r)
}

func (s *bashSession) run(command string, timeout time.Duration, hooks *ExecuteResultHook) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return errors.New("session not started")
	}

	startAt := time.Now()

	if hooks != nil && hooks.OnExecuteInit != nil {
		hooks.OnExecuteInit(s.config.Session)
	}

	waitSeconds := timeout
	if waitSeconds <= 0 {
		waitSeconds = 30 * time.Second
	}

	cleanCmd := strings.ReplaceAll(command, "\n", " ; ")

	// send command + marker
	cmdText := fmt.Sprintf("%s\nprintf \"%s$?%s\\n\"\n", cleanCmd, exitCodePrefix, exitCodeSuffix)
	if _, err := fmt.Fprint(s.stdin, cmdText); err != nil {
		return fmt.Errorf("write command: %w", err)
	}

	// collect output until marker
	timer := time.NewTimer(waitSeconds)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout after %s while running command %q", waitSeconds, command)
		case err := <-s.stdoutErr:
			if err != nil {
				return err
			}
		case line, ok := <-s.stdoutLines:
			if !ok {
				return errors.New("stdout closed unexpectedly")
			}
			if _, ok := parseExitCodeLine(line); ok {
				if hooks != nil && hooks.OnExecuteComplete != nil {
					hooks.OnExecuteComplete(time.Since(startAt))
				}
				return nil
			}
			if hooks != nil && hooks.OnExecuteStdout != nil {
				hooks.OnExecuteStdout(line)
			}
		}
	}
}

func parseExitCodeLine(line string) (int, bool) {
	p := strings.Index(line, exitCodePrefix)
	q := strings.Index(line, exitCodeSuffix)
	if p < 0 || q <= p {
		return 0, false
	}
	text := strings.TrimSpace(line[p+len(exitCodePrefix) : q])
	code, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}
	return code, true
}

func (s *bashSession) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}
	s.started = false

	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	return nil
}

func uuidString() string {
	return uuid.New().String()
}
