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

package model

import (
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"strings"

	"github.com/alibaba/opensandbox/execd/pkg/runtime"
)

// UserIdentity represents a POSIX username or numeric UID.
// Prefer specifying exactly one of Username/UID.
type UserIdentity struct {
	Username *string `json:"name,omitempty"`
	UID      *int64  `json:"uid,omitempty"`
}

func newUserIdentityFromUsername(username string) *UserIdentity {
	return &UserIdentity{Username: &username}
}

func newUserIdentityFromUID(uid int64) *UserIdentity {
	return &UserIdentity{UID: &uid}
}

// validate ensures the identity contains either username or uid with valid values.
func (u *UserIdentity) validate() error {
	if u == nil {
		return nil
	}
	if u.Username != nil && u.UID != nil {
		return errors.New("user must not set both username and uid")
	}
	if u.Username != nil {
		if strings.TrimSpace(*u.Username) == "" {
			return errors.New("username cannot be empty")
		}
		return nil
	}
	if u.UID != nil {
		if *u.UID < 0 {
			return errors.New("uid must be non-negative")
		}
		return nil
	}
	return errors.New("user must be a username or uid")
}

// ToRuntime converts the identity to runtime.CommandUser.
func (u *UserIdentity) ToRuntime() *runtime.CommandUser {
	if u == nil {
		return nil
	}
	if u.Username != nil {
		return &runtime.CommandUser{Username: u.Username}
	}
	if u.UID != nil {
		return &runtime.CommandUser{UID: u.UID}
	}
	return nil
}

// UserIdentityFromRuntime converts runtime.CommandUser to UserIdentity.
func UserIdentityFromRuntime(user *runtime.CommandUser) *UserIdentity {
	if user == nil {
		return nil
	}
	if user.Username != nil {
		return newUserIdentityFromUsername(*user.Username)
	}
	if user.UID != nil {
		return newUserIdentityFromUID(*user.UID)
	}
	return nil
}

// validateExists ensures the referenced user/uid is present on the system.
func (u *UserIdentity) validateExists() error {
	if u == nil {
		return nil
	}
	if u.Username != nil && *u.Username != "" {
		if _, err := user.Lookup(*u.Username); err != nil {
			return fmt.Errorf("user %s not found: %w", *u.Username, err)
		}
		return nil
	}
	if u.UID != nil {
		if _, err := user.LookupId(strconv.FormatInt(*u.UID, 10)); err != nil {
			return fmt.Errorf("uid %d not found: %w", *u.UID, err)
		}
		return nil
	}
	return errors.New("user must contain name or uid")
}
