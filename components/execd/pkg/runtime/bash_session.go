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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alibaba/opensandbox/execd/pkg/log"
)

const (
	envDumpStartMarker = "__ENV_DUMP_START__"
	envDumpEndMarker   = "__ENV_DUMP_END__"
	exitMarkerPrefix   = "__EXIT_CODE__:"
	pwdMarkerPrefix    = "__PWD__:"
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

	env := make(map[string]string)
	for _, kv := range os.Environ() {
		if k, v, ok := splitEnvPair(kv); ok {
			env[k] = v
		}
	}

	return &bashSession{
		config: config,
		env:    env,
	}
}

func (s *bashSession) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return errors.New("session already started")
	}

	s.started = true
	return nil
}

//nolint:gocognit
func (s *bashSession) run(command string, timeout time.Duration, hooks *ExecuteResultHook) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return errors.New("session not started")
	}

	envSnapshot := copyEnvMap(s.env)
	cwd := s.cwd
	sessionID := s.config.Session
	s.mu.Unlock()

	startAt := time.Now()
	if hooks != nil && hooks.OnExecuteInit != nil {
		hooks.OnExecuteInit(sessionID)
	}

	wait := timeout
	if wait <= 0 {
		wait = 24 * 3600 * time.Second // default to 24 hours
	}

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--norc", "-s")
	cmd.Env = envMapToSlice(envSnapshot)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bash: %w", err)
	}

	script := buildWrappedScript(command, envSnapshot, cwd)
	if _, err := io.WriteString(stdin, script); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return fmt.Errorf("write command: %w", err)
	}
	_ = stdin.Close()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var (
		envLines []string
		pwdLine  string
		exitCode *int
		inEnv    bool
	)

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == envDumpStartMarker:
			inEnv = true
		case line == envDumpEndMarker:
			inEnv = false
		case strings.HasPrefix(line, exitMarkerPrefix):
			if code, err := strconv.Atoi(strings.TrimPrefix(line, exitMarkerPrefix)); err == nil {
				exitCode = &code
			}
		case strings.HasPrefix(line, pwdMarkerPrefix):
			pwdLine = strings.TrimPrefix(line, pwdMarkerPrefix)
		default:
			if inEnv {
				envLines = append(envLines, line)
				continue
			}
			if hooks != nil && hooks.OnExecuteStdout != nil {
				hooks.OnExecuteStdout(line)
			}
		}
	}

	scanErr := scanner.Err()
	waitErr := cmd.Wait()

	if scanErr != nil {
		return fmt.Errorf("read stdout: %w", scanErr)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timeout after %s while running command %q", wait, command)
	}

	if exitCode == nil && cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		exitCode = &code
	}

	updatedEnv := parseExportDump(envLines)
	s.mu.Lock()
	if len(updatedEnv) > 0 {
		s.env = updatedEnv
	}
	if pwdLine != "" {
		s.cwd = pwdLine
	}
	s.mu.Unlock()

	if hooks != nil && hooks.OnExecuteComplete != nil {
		hooks.OnExecuteComplete(time.Since(startAt))
	}

	// Maintain previous behavior: non-zero exit codes do not surface as errors.
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return waitErr
	}

	return nil
}

func buildWrappedScript(command string, env map[string]string, cwd string) string {
	var b strings.Builder

	keys := make([]string, 0, len(env))
	for k := range env {
		if isValidEnvKey(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("export ")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(shellEscape(env[k]))
		b.WriteString("\n")
	}

	if cwd != "" {
		b.WriteString("cd ")
		b.WriteString(shellEscape(cwd))
		b.WriteString("\n")
	}

	b.WriteString(command)
	if !strings.HasSuffix(command, "\n") {
		b.WriteString("\n")
	}

	b.WriteString("__USER_EXIT_CODE__=$?\n")
	b.WriteString("echo \"" + envDumpStartMarker + "\"\n")
	b.WriteString("export -p\n")
	b.WriteString("echo \"" + envDumpEndMarker + "\"\n")
	b.WriteString("printf \"" + pwdMarkerPrefix + "%s\\n\" \"$(pwd)\"\n")
	b.WriteString("printf \"" + exitMarkerPrefix + "%s\\n\" \"$__USER_EXIT_CODE__\"\n")
	b.WriteString("exit \"$__USER_EXIT_CODE__\"\n")

	return b.String()
}

func parseExportDump(lines []string) map[string]string {
	if len(lines) == 0 {
		return nil
	}

	env := make(map[string]string, len(lines))
	for _, line := range lines {
		if k, v, ok := parseExportLine(line); ok {
			env[k] = v
		}
	}
	return env
}

func parseExportLine(line string) (string, string, bool) {
	const prefix = "declare -x "
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}

	rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if rest == "" {
		return "", "", false
	}

	name := rest
	value := ""
	if eq := strings.Index(rest, "="); eq >= 0 {
		name = rest[:eq]
		raw := rest[eq+1:]
		unquoted, err := strconv.Unquote(raw)
		if err != nil {
			value = strings.Trim(raw, `"`)
		} else {
			value = unquoted
		}
	}

	if !isValidEnvKey(name) {
		return "", "", false
	}

	return name, value, true
}

func shellEscape(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func isValidEnvKey(key string) bool {
	if key == "" {
		return false
	}

	for i, r := range key {
		if i == 0 {
			if (r < 'A' || (r > 'Z' && r < 'a') || r > 'z') && r != '_' {
				return false
			}
			continue
		}
		if (r < 'A' || (r > 'Z' && r < 'a') || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}

	return true
}

func copyEnvMap(src map[string]string) map[string]string {
	if src == nil {
		return map[string]string{}
	}

	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func envMapToSlice(env map[string]string) []string {
	if len(env) == 0 {
		return os.Environ()
	}

	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}

func splitEnvPair(kv string) (string, string, bool) {
	parts := strings.SplitN(kv, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	if !isValidEnvKey(parts[0]) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (s *bashSession) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}
	s.started = false
	s.env = nil
	s.cwd = ""
	return nil
}

func uuidString() string {
	return uuid.New().String()
}
