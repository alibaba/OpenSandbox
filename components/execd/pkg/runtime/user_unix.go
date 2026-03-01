//go:build !windows
// +build !windows

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

package runtime

import (
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"syscall"
)

// resolveUserCredential converts CommandUser to syscall.Credential and a resolved identity.
// Caller is responsible for handling permission errors when switching users.
func resolveUserCredential(u *CommandUser) (*syscall.Credential, *CommandUser, error) {
	if u == nil {
		return nil, nil, nil
	}

	var (
		usr *user.User
		err error
	)

	switch {
	case u.Username != nil:
		usr, err = user.Lookup(*u.Username)
		if err != nil {
			return nil, nil, fmt.Errorf("lookup user %s: %w", *u.Username, err)
		}
	case u.UID != nil:
		usr, err = user.LookupId(strconv.FormatInt(*u.UID, 10))
		if err != nil {
			return nil, nil, fmt.Errorf("lookup uid %d: %w", *u.UID, err)
		}
	default:
		return nil, nil, errors.New("user must provide username or uid")
	}

	uid, err := strconv.ParseUint(usr.Uid, 10, 32)
	if err != nil {
		return nil, nil, fmt.Errorf("parse uid %s: %w", usr.Uid, err)
	}
	gid, err := strconv.ParseUint(usr.Gid, 10, 32)
	if err != nil {
		return nil, nil, fmt.Errorf("parse gid %s: %w", usr.Gid, err)
	}

	// Supplementary groups: required so the process has all permissions the user is entitled to
	// (e.g. docker group for socket access, device access).
	groupIds, err := usr.GroupIds()
	if err != nil {
		return nil, nil, fmt.Errorf("lookup supplementary groups for user %s: %w", usr.Username, err)
	}
	groups := make([]uint32, 0, len(groupIds))
	for _, gidStr := range groupIds {
		g, parseErr := strconv.ParseUint(gidStr, 10, 32)
		if parseErr != nil {
			continue
		}
		// Skip primary group; it is already set in Credential.Gid.
		if g == gid {
			continue
		}
		groups = append(groups, uint32(g))
	}

	cred := &syscall.Credential{
		Uid:    uint32(uid),
		Gid:    uint32(gid),
		Groups: groups,
	}

	resolvedUID := int64(uid)
	resolved := &CommandUser{
		UID: &resolvedUID,
	}
	if usr.Username != "" {
		username := usr.Username
		resolved.Username = &username
	}

	return cred, resolved, nil
}
