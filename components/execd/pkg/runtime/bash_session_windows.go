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

//go:build windows
// +build windows

package runtime

import (
	"context"
	"errors"
	"time"
)

var errBashSessionNotSupported = errors.New("bash session is not supported on windows")

func (c *Controller) createBashSession(_ *CreateContextRequest) (string, error) {
	return "", errBashSessionNotSupported
}

func (c *Controller) runBashSession(_ context.Context, _ *ExecuteCodeRequest) error { //nolint:revive
	return errBashSessionNotSupported
}

func (c *Controller) createDefaultBashSession() error { //nolint:revive
	return errBashSessionNotSupported
}

func (c *Controller) getBashSession(_ string) (*bashSession, error) { //nolint:revive
	return nil, errBashSessionNotSupported
}

func (c *Controller) closeBashSession(_ string) error { //nolint:revive
	return errBashSessionNotSupported
}

func (c *Controller) listBashSessions() []string { //nolint:revive
	return nil
}

// Stub methods on bashSession to satisfy interfaces on non-Linux platforms.
func newBashSession(config *bashSessionConfig) *bashSession {
	return &bashSession{config: config}
}

func (s *bashSession) start() error {
	return errBashSessionNotSupported
}

func (s *bashSession) run(_ string, _ time.Duration, _ *ExecuteResultHook) error {
	return errBashSessionNotSupported
}

func (s *bashSession) close() error {
	return nil
}
