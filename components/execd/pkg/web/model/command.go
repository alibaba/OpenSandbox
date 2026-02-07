// Copyright 2025 Alibaba Group Holding Ltd.
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

package model

import (
	"time"

	"github.com/go-playground/validator/v10"
)

// CommandStatusResponse represents command status for REST APIs.
type CommandStatusResponse struct {
	ID         string        `json:"id"`
	Content    string        `json:"content,omitempty"`
	User       *UserIdentity `json:"user,omitempty"`
	Running    bool          `json:"running"`
	ExitCode   *int          `json:"exit_code,omitempty"`
	Error      string        `json:"error,omitempty"`
	StartedAt  time.Time     `json:"started_at,omitempty"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
}

// RunCommandRequest represents a shell command execution request.
type RunCommandRequest struct {
	Command    string `json:"command" validate:"required"`
	Cwd        string `json:"cwd,omitempty"`
	Background bool   `json:"background,omitempty"`
	// User specifies the username or UID to run the command as.
	// Effective switching requires root or CAP_SETUID/CAP_SETGID (and valid UID/GID
	// mappings when using user namespaces); otherwise it will fail with a permission error.
	User *UserIdentity `json:"user,omitempty"`
}

func (r *RunCommandRequest) Validate() error {
	validate := validator.New()
	if err := validate.Struct(r); err != nil {
		return err
	}
	if err := r.User.validate(); err != nil {
		return err
	}
	if err := r.User.validateExists(); err != nil {
		return err
	}
	return nil
}
