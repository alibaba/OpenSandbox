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
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
	"github.com/alibaba/opensandbox/execd/pkg/log"
)

const (
	envDumpStartMarker = "__EXECD_ENV_DUMP_START_8a3f__"
	envDumpEndMarker   = "__EXECD_ENV_DUMP_END_8a3f__"
	exitMarkerPrefix   = "__EXECD_EXIT_v1__:"
	pwdMarkerPrefix    = "__EXECD_PWD_v1__:"
)

func (c *Controller) createBashSession(req *CreateContextRequest) (string, error) {
	session := newBashSession(req.Cwd)
	if err := session.start(); err != nil {
		return "", fmt.Errorf("failed to start bash session: %w", err)
	}

	c.bashSessionClientMap.Store(session.config.Session, session)
	log.Info("created bash session %s", session.config.Session)
	return session.config.Session, nil
}

func (c *Controller) runBashSession(ctx context.Context, request *ExecuteCodeRequest) error {
	session := c.getBashSession(request.Context)
	if session == nil {
		return ErrContextNotFound
	}

	return session.run(ctx, request)
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

func (c *Controller) CreateBashSession(req *CreateContextRequest) (string, error) {
	return c.createBashSession(req)
}

func (c *Controller) RunInBashSession(ctx context.Context, req *ExecuteCodeRequest) error {
	return c.runBashSession(ctx, req)
}

func (c *Controller) DeleteBashSession(sessionID string) error {
	return c.closeBashSession(sessionID)
}

// BashSessionStatus holds observable state for a bash session.
type BashSessionStatus struct {
	SessionID    string
	Running      bool
	OutputOffset int64
}

// WriteSessionOutput appends data to the replay buffer for the named session.
// Used by the WebSocket handler to persist live output for reconnect replay.
func (c *Controller) WriteSessionOutput(sessionID string, data []byte) {
	s := c.getBashSession(sessionID)
	if s == nil {
		return
	}
	s.replay.write(data)
}

// ReplaySessionOutput returns buffered output bytes starting from offset.
// Returns (data, nextOffset). See replayBuffer.readFrom for semantics.
func (c *Controller) ReplaySessionOutput(sessionID string, offset int64) ([]byte, int64, error) {
	session := c.getBashSession(sessionID)
	if session == nil {
		return nil, 0, ErrContextNotFound
	}
	data, next := session.replay.readFrom(offset)
	return data, next, nil
}

// GetBashSessionStatus returns status info for a bash session, including replay buffer offset.
func (c *Controller) GetBashSessionStatus(sessionID string) (*BashSessionStatus, error) {
	session := c.getBashSession(sessionID)
	if session == nil {
		return nil, ErrContextNotFound
	}
	session.mu.Lock()
	running := session.wsPid != 0
	session.mu.Unlock()
	return &BashSessionStatus{
		SessionID:    sessionID,
		Running:      running,
		OutputOffset: session.replay.Total(),
	}, nil
}

// Session implementation (pipe-based, no PTY)
func newBashSession(cwd string) *bashSession {
	config := &bashSessionConfig{
		Session:        uuidString(),
		StartupTimeout: 5 * time.Second,
	}

	env := make(map[string]string)
	for _, kv := range os.Environ() {
		if k, v, ok := splitEnvPair(kv); ok {
			env[k] = v
		}
	}

	return &bashSession{
		config:       config,
		env:          env,
		cwd:          cwd,
		replay:       newReplayBuffer(defaultReplayBufSize),
		lastExitCode: -1,
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

// Start launches an interactive bash process for WebSocket stdin/stdout mode.
// It is idempotent: if the process is already running, it returns nil.
// Unlike run(), this bash process stays alive reading from stdin until closed.
func (s *bashSession) Start() error {
	s.mu.Lock()
	if s.wsPid != 0 {
		s.mu.Unlock()
		return nil // already running
	}
	if s.closing {
		s.mu.Unlock()
		return errors.New("session is closing")
	}
	s.mu.Unlock()

	cmd := exec.Command("bash", "--noprofile", "--norc")
	if s.cwd != "" {
		cmd.Dir = s.cwd
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		_ = stdinR.Close()
		_ = stdinW.Close()
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdinR.Close()
		_ = stdinW.Close()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	cmd.Stdin = stdinR
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	if err := cmd.Start(); err != nil {
		_ = stdinR.Close()
		_ = stdinW.Close()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		_ = stderrR.Close()
		_ = stderrW.Close()
		return fmt.Errorf("start bash: %w", err)
	}

	// Close child-side ends in the parent process.
	_ = stdinR.Close()
	_ = stdoutW.Close()
	_ = stderrW.Close()

	doneCh := make(chan struct{})

	s.mu.Lock()
	// Reset stale PTY state so WriteStdin targets the correct pipe on mode switch.
	s.isPTY = false
	s.ptmx = nil
	s.stdin = stdinW
	s.doneCh = doneCh
	s.wsPid = cmd.Process.Pid
	s.started = true
	s.mu.Unlock()

	// Broadcast goroutine: reads real stdout, always writes to replay buffer, and
	// fans out to the current per-connection sink when one is attached.
	// Output produced during client downtime is preserved in the replay buffer so
	// reconnecting clients can catch up via ?since=.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stdoutR.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				s.replay.write(chunk)
				s.outMu.Lock()
				w := s.stdoutW
				s.outMu.Unlock()
				if w != nil {
					_, _ = w.Write(chunk)
				}
			}
			if err != nil {
				s.outMu.Lock()
				if s.stdoutW != nil {
					_ = s.stdoutW.Close()
					s.stdoutW = nil
				}
				s.outMu.Unlock()
				return
			}
		}
	}()

	// Broadcast goroutine: reads real stderr, always writes to replay buffer, and
	// fans out to the current per-connection sink when one is attached.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stderrR.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				s.replay.write(chunk)
				s.outMu.Lock()
				w := s.stderrW
				s.outMu.Unlock()
				if w != nil {
					_, _ = w.Write(chunk)
				}
			}
			if err != nil {
				s.outMu.Lock()
				if s.stderrW != nil {
					_ = s.stderrW.Close()
					s.stderrW = nil
				}
				s.outMu.Unlock()
				return
			}
		}
	}()

	go func() {
		_ = cmd.Wait()
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		_ = stdinW.Close()
		s.mu.Lock()
		s.lastExitCode = code
		s.wsPid = 0
		s.mu.Unlock()
		close(doneCh)
	}()

	return nil
}

// StartPTY launches an interactive bash process using a PTY instead of pipes.
// stdout and stderr arrive merged on the PTY master fd.
// It is idempotent: if the process is already running, it returns nil.
func (s *bashSession) StartPTY() error {
	s.mu.Lock()
	if s.wsPid != 0 {
		s.mu.Unlock()
		return nil // already running
	}
	if s.closing {
		s.mu.Unlock()
		return errors.New("session is closing")
	}
	s.mu.Unlock()

	cmd := exec.Command("bash", "--noprofile", "--norc")
	if s.cwd != "" {
		cmd.Dir = s.cwd
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLUMNS=80", "LINES=24")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return fmt.Errorf("pty start: %w", err)
	}

	doneCh := make(chan struct{})

	s.mu.Lock()
	s.ptmx = ptmx
	s.isPTY = true
	s.doneCh = doneCh
	s.wsPid = cmd.Process.Pid
	s.started = true
	s.mu.Unlock()

	// Broadcast goroutine: reads PTY master (stdout+stderr merged), always writes to
	// replay buffer, and fans out to the current per-connection sink when one is attached.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				s.replay.write(chunk)
				s.outMu.Lock()
				w := s.stdoutW
				s.outMu.Unlock()
				if w != nil {
					_, _ = w.Write(chunk)
				}
			}
			if err != nil {
				s.outMu.Lock()
				if s.stdoutW != nil {
					_ = s.stdoutW.Close()
					s.stdoutW = nil
				}
				s.outMu.Unlock()
				return
			}
		}
	}()

	go func() {
		_ = cmd.Wait()
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		_ = ptmx.Close()
		s.mu.Lock()
		s.lastExitCode = code
		s.wsPid = 0
		// Clear PTY descriptors so a subsequent Start() in pipe mode is clean.
		s.isPTY = false
		s.ptmx = nil
		s.mu.Unlock()
		close(doneCh)
	}()

	return nil
}

// ResizePTY sends a TIOCSWINSZ ioctl to the PTY master.
// No-op if not in PTY mode.
func (s *bashSession) ResizePTY(cols, rows uint16) error {
	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	if ptmx == nil {
		return nil
	}
	return pty.Setsize(ptmx, &pty.Winsize{Rows: rows, Cols: cols})
}

// SendSignal sends a named OS signal (e.g. "SIGINT") to the session's process group.
// No-op if the session is not running or the signal name is unknown.
func (s *bashSession) SendSignal(name string) {
	s.mu.Lock()
	pid := s.wsPid
	s.mu.Unlock()
	if pid == 0 {
		return
	}
	sig := signalByName(name)
	if sig == 0 {
		return
	}
	_ = syscall.Kill(-pid, sig)
}

// signalByName maps a POSIX signal name to its syscall.Signal number.
// Returns 0 for unknown names.
func signalByName(name string) syscall.Signal {
	switch name {
	case "SIGINT":
		return syscall.SIGINT
	case "SIGTERM":
		return syscall.SIGTERM
	case "SIGKILL":
		return syscall.SIGKILL
	case "SIGQUIT":
		return syscall.SIGQUIT
	case "SIGHUP":
		return syscall.SIGHUP
	default:
		return 0
	}
}

// WriteStdin writes p to the session's stdin.
// In PTY mode it writes to the PTY master fd; in pipe mode it writes to the stdin pipe.
// Returns error if the session has not started or the pipe is closed.
func (s *bashSession) WriteStdin(p []byte) (int, error) {
	s.mu.Lock()
	isPTY := s.isPTY
	ptmx := s.ptmx
	stdin := s.stdin
	s.mu.Unlock()

	if isPTY {
		if ptmx == nil {
			return 0, errors.New("PTY not started")
		}
		return ptmx.Write(p)
	}
	if stdin == nil {
		return 0, errors.New("session not started")
	}
	return stdin.Write(p)
}

// LockWS atomically acquires exclusive WebSocket access.
// Returns false if already locked.
func (s *bashSession) LockWS() bool {
	return s.wsConnected.CompareAndSwap(false, true)
}

// UnlockWS releases the WebSocket connection lock.
func (s *bashSession) UnlockWS() {
	s.wsConnected.Store(false)
}

// IsRunning reports whether the long-lived WS bash process is currently alive.
func (s *bashSession) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wsPid != 0
}

// ExitCode returns the exit code of the most recently completed process.
// Returns -1 if the process has not yet exited.
func (s *bashSession) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastExitCode
}

// AttachOutput installs a fresh per-connection pipe pair and returns readers plus a detach func.
// The broadcast goroutine (started by Start/StartPTY) copies from the real OS pipe into the
// current PipeWriter. Calling detach() closes the PipeWriters so the returned readers return
// EOF, unblocking any scanner goroutines on this connection without affecting the underlying pipe.
func (s *bashSession) AttachOutput() (stdout io.Reader, stderr io.Reader, detach func()) {
	stdoutR, stdoutW := io.Pipe()

	s.outMu.Lock()
	// Close any previous writer (e.g. from a stale prior connection) before swapping.
	if s.stdoutW != nil {
		_ = s.stdoutW.Close()
	}
	s.stdoutW = stdoutW
	s.outMu.Unlock()

	var stderrR *io.PipeReader
	var stderrPW *io.PipeWriter

	s.mu.Lock()
	isPTY := s.isPTY
	s.mu.Unlock()

	if !isPTY {
		stderrR, stderrPW = io.Pipe()
		s.outMu.Lock()
		if s.stderrW != nil {
			_ = s.stderrW.Close()
		}
		s.stderrW = stderrPW
		s.outMu.Unlock()
	}

	detach = func() {
		s.outMu.Lock()
		// Only close if we're still the active writer (guards against double-detach).
		if s.stdoutW == stdoutW {
			_ = stdoutW.Close()
			s.stdoutW = nil
		}
		if stderrPW != nil && s.stderrW == stderrPW {
			_ = stderrPW.Close()
			s.stderrW = nil
		}
		s.outMu.Unlock()
	}

	return stdoutR, stderrR, detach
}

// Done returns a channel that is closed when the WS-mode bash process exits.
func (s *bashSession) Done() <-chan struct{} { return s.doneCh }

// IsPTY reports whether the session is running in PTY mode.
func (s *bashSession) IsPTY() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isPTY
}

func (s *bashSession) trackCurrentProcess(pid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		// close() already ran while we were in cmd.Start(); kill immediately
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		return
	}
	s.currentProcessPid = pid
}

func (s *bashSession) untrackCurrentProcess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentProcessPid = 0
}

//nolint:gocognit
func (s *bashSession) run(ctx context.Context, request *ExecuteCodeRequest) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return errors.New("session not started")
	}

	envSnapshot := copyEnvMap(s.env)

	cwd := s.cwd
	// override original cwd if specified
	if request.Cwd != "" {
		cwd = request.Cwd
	}
	sessionID := s.config.Session
	s.mu.Unlock()

	startAt := time.Now()
	if request.Hooks.OnExecuteInit != nil {
		request.Hooks.OnExecuteInit(sessionID)
	}

	wait := request.Timeout
	if wait <= 0 {
		wait = 24 * 3600 * time.Second // max to 24 hours
	}

	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	script := buildWrappedScript(request.Code, envSnapshot, cwd)
	scriptFile, err := os.CreateTemp("", "execd_bash_*.sh")
	if err != nil {
		return fmt.Errorf("create script file: %w", err)
	}
	scriptPath := scriptFile.Name()
	defer os.Remove(scriptPath) // clean up temp script regardless of outcome
	if _, err := scriptFile.WriteString(script); err != nil {
		_ = scriptFile.Close()
		return fmt.Errorf("write script file: %w", err)
	}
	if err := scriptFile.Close(); err != nil {
		return fmt.Errorf("close script file: %w", err)
	}

	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--norc", scriptPath)
	cmd.Dir = cwd // set OS-level CWD; harmless if cwd == "" (inherits daemon CWD)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Do not pass envSnapshot via cmd.Env to avoid "argument list too long" when session env is large.
	// Child inherits parent env (nil => default in Go). The script file already has "export K=V" for
	// all session vars at the top, so the session environment is applied when the script runs.
	// Use OS pipes (not io.Pipe) so we can close the parent-side write ends immediately
	// after cmd.Start() without breaking in-flight writes. The kernel buffers data
	// independently; closing the write end in the parent just signals EOF to the reader
	// once the child has exited and flushed, without any "write on closed pipe" errors.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		return fmt.Errorf("create stderr pipe: %w", err)
	}
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	if err := cmd.Start(); err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		_ = stderrR.Close()
		_ = stderrW.Close()
		log.Error("start bash session failed: %v (command: %q)", err, request.Code)
		return fmt.Errorf("start bash: %w", err)
	}
	defer s.untrackCurrentProcess()
	s.trackCurrentProcess(cmd.Process.Pid)

	// Close parent-side write ends now. The child has inherited its own copies;
	// closing ours here means the reader gets EOF as soon as the child exits,
	// without waiting for cmd.Wait() — eliminating the scan↔Wait deadlock.
	_ = stdoutW.Close()
	_ = stderrW.Close()

	// Drain stderr in a separate goroutine; fire OnExecuteStderr for each line.
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		stderrScanner := bufio.NewScanner(stderrR)
		stderrScanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for stderrScanner.Scan() {
			line := stderrScanner.Text() + "\n"
			s.replay.write([]byte(line))
			if request.Hooks.OnExecuteStderr != nil {
				request.Hooks.OnExecuteStderr(stderrScanner.Text())
			}
		}
	}()

	scanner := bufio.NewScanner(stdoutR)
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
				exitCode = &code //nolint:ineffassign
			}
		case strings.HasPrefix(line, pwdMarkerPrefix):
			pwdLine = strings.TrimPrefix(line, pwdMarkerPrefix)
		default:
			if inEnv {
				envLines = append(envLines, line)
				continue
			}
			s.replay.write([]byte(line + "\n"))
			if request.Hooks.OnExecuteStdout != nil {
				request.Hooks.OnExecuteStdout(line)
			}
		}
	}

	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	// Wait for stderr goroutine to drain.
	<-stderrDone

	if scanErr != nil {
		log.Error("read stdout failed: %v (command: %q)", scanErr, request.Code)
		return fmt.Errorf("read stdout: %w", scanErr)
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		log.Error("timeout after %s while running command: %q", wait, request.Code)
		return fmt.Errorf("timeout after %s while running command %q", wait, request.Code)
	}

	if exitCode == nil && cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode() //nolint:staticcheck
		exitCode = &code                    //nolint:ineffassign
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

	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		log.Error("command wait failed: %v (command: %q)", waitErr, request.Code)
		return waitErr
	}

	userExitCode := 0
	if exitCode != nil {
		userExitCode = *exitCode
	}

	if userExitCode != 0 {
		errMsg := fmt.Sprintf("command exited with code %d", userExitCode)
		if waitErr != nil {
			errMsg = waitErr.Error()
		}
		if request.Hooks.OnExecuteError != nil {
			request.Hooks.OnExecuteError(&execute.ErrorOutput{
				EName:     "CommandExecError",
				EValue:    strconv.Itoa(userExitCode),
				Traceback: []string{errMsg},
			})
		}
		log.Error("CommandExecError: %s (command: %q)", errMsg, request.Code)
		return nil
	}

	if request.Hooks.OnExecuteComplete != nil {
		request.Hooks.OnExecuteComplete(time.Since(startAt))
	}

	return nil
}

func buildWrappedScript(command string, env map[string]string, cwd string) string {
	var b strings.Builder

	keys := make([]string, 0, len(env))
	for k := range env {
		v := env[k]
		if isValidEnvKey(k) && !envKeysNotPersisted[k] && len(v) <= maxPersistedEnvValueSize {
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
	b.WriteString("printf \"\\n%s\\n\" \"" + envDumpStartMarker + "\"\n")
	b.WriteString("export -p\n")
	b.WriteString("printf \"%s\\n\" \"" + envDumpEndMarker + "\"\n")
	b.WriteString("printf \"" + pwdMarkerPrefix + "%s\\n\" \"$(pwd)\"\n")
	b.WriteString("printf \"" + exitMarkerPrefix + "%s\\n\" \"$__USER_EXIT_CODE__\"\n")
	b.WriteString("exit \"$__USER_EXIT_CODE__\"\n")

	return b.String()
}

// envKeysNotPersisted are not carried across runs (prompt/display vars).
var envKeysNotPersisted = map[string]bool{
	"PS1": true, "PS2": true, "PS3": true, "PS4": true,
	"PROMPT_COMMAND": true,
}

// maxPersistedEnvValueSize caps single env value length as a safeguard.
const maxPersistedEnvValueSize = 8 * 1024

func parseExportDump(lines []string) map[string]string {
	if len(lines) == 0 {
		return nil
	}
	env := make(map[string]string, len(lines))
	for _, line := range lines {
		k, v, ok := parseExportLine(line)
		if !ok || envKeysNotPersisted[k] || len(v) > maxPersistedEnvValueSize {
			continue
		}
		env[k] = v
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
	name, value := rest, ""
	if eq := strings.Index(rest, "="); eq >= 0 {
		name = rest[:eq]
		raw := rest[eq+1:]
		if unquoted, err := strconv.Unquote(raw); err == nil {
			value = unquoted
		} else {
			value = strings.Trim(raw, `"`)
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

	s.closing = true
	wsPid := s.wsPid
	runPid := s.currentProcessPid
	ptmx := s.ptmx
	s.wsPid = 0
	s.currentProcessPid = 0
	s.started = false
	s.env = nil
	s.cwd = ""

	for _, pid := range []int{wsPid, runPid} {
		if pid != 0 {
			if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
				log.Warning("kill session process group %d: %v (process may have already exited)", pid, err)
			}
		}
	}
	if ptmx != nil {
		_ = ptmx.Close()
		s.ptmx = nil
	}
	return nil
}

func uuidString() string {
	return uuid.New().String()
}
