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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"strings"

	"github.com/alibaba/opensandbox/execd/pkg/runtime"
)

// UserIdentity represents a POSIX username or numeric UID.
type UserIdentity struct {
	username *string
	uid      *int64
}

func newUserIdentityFromUsername(username string) *UserIdentity {
	return &UserIdentity{username: &username}
}

func newUserIdentityFromUID(uid int64) *UserIdentity {
	return &UserIdentity{uid: &uid}
}

func (u *UserIdentity) Username() (string, bool) {
	if u == nil || u.username == nil {
		return "", false
	}
	return *u.username, true
}

func (u *UserIdentity) UID() (int64, bool) {
	if u == nil || u.uid == nil {
		return 0, false
	}
	return *u.uid, true
}

// validate ensures the identity contains either username or uid with valid values.
func (u *UserIdentity) validate() error {
	if u == nil {
		return nil
	}
	if u.username != nil && u.uid != nil {
		return errors.New("user must not set both username and uid")
	}
	if u.username != nil {
		if strings.TrimSpace(*u.username) == "" {
			return errors.New("username cannot be empty")
		}
		return nil
	}
	if u.uid != nil {
		if *u.uid < 0 {
			return errors.New("uid must be non-negative")
		}
		return nil
	}
	return errors.New("user must be a username or uid")
}

// MarshalJSON renders the identity as either a JSON string (username) or number (uid).
func (u *UserIdentity) MarshalJSON() ([]byte, error) {
	if u == nil {
		return []byte("null"), nil
	}
	if u.username != nil {
		return json.Marshal(*u.username)
	}
	if u.uid != nil {
		return json.Marshal(*u.uid)
	}
	return []byte("null"), nil
}

// UnmarshalJSON accepts either a string username or numeric UID.
func (u *UserIdentity) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	u.username = nil
	u.uid = nil

	// Try username (string)
	if trimmed[0] == '"' {
		var username string
		if err := json.Unmarshal(trimmed, &username); err != nil {
			return err
		}
		u.username = &username
		u.uid = nil
		return nil
	}

	// Try UID (number)
	var uid int64
	if err := json.Unmarshal(trimmed, &uid); err == nil {
		u.uid = &uid
		u.username = nil
		return nil
	}

	return errors.New("user must be string username or integer uid")
}

// ToRuntime converts the identity to runtime.CommandUser.
func (u *UserIdentity) ToRuntime() *runtime.CommandUser {
	if u == nil {
		return nil
	}
	if username, ok := u.Username(); ok {
		return &runtime.CommandUser{Username: &username}
	}
	if uid, ok := u.UID(); ok {
		return &runtime.CommandUser{UID: &uid}
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
	if username, ok := u.Username(); ok {
		if _, err := user.Lookup(username); err != nil {
			return fmt.Errorf("user %s not found: %w", username, err)
		}
		return nil
	}
	if uid, ok := u.UID(); ok {
		if _, err := user.LookupId(strconv.FormatInt(uid, 10)); err != nil {
			return fmt.Errorf("uid %d not found: %w", uid, err)
		}
		return nil
	}
	return nil
}
