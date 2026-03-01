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
	"os/user"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveUserCredentialWithUsername(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	u := &CommandUser{Username: &cur.Username}
	cred, resolved, err := resolveUserCredential(u)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.NotNil(t, cred) || !assert.NotNil(t, resolved) {
		return
	}
	if assert.NotNil(t, resolved.Username) {
		assert.Equal(t, cur.Username, *resolved.Username)
	}
	uid, _ := strconv.ParseUint(cur.Uid, 10, 32)
	gid, _ := strconv.ParseUint(cur.Gid, 10, 32)
	if assert.NotNil(t, resolved.UID) {
		assert.Equal(t, int64(uid), *resolved.UID)
	}
	assert.Equal(t, uint32(uid), cred.Uid)
	assert.Equal(t, uint32(gid), cred.Gid)
	expectSupplementaryGroups(t, cur, cred.Groups)
}

func TestResolveUserCredentialWithUID(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}
	uidVal, parseErr := strconv.ParseInt(cur.Uid, 10, 64)
	if parseErr != nil {
		t.Skipf("cannot parse uid: %v", parseErr)
	}

	u := &CommandUser{UID: &uidVal}
	cred, resolved, err := resolveUserCredential(u)
	if !assert.NoError(t, err) {
		return
	}
	if assert.NotNil(t, resolved.UID) {
		assert.Equal(t, uidVal, *resolved.UID)
	}
	assert.NotNil(t, resolved.Username)
	if assert.NotNil(t, cred) {
		assert.Equal(t, uint32(uidVal), cred.Uid)
		gid, _ := strconv.ParseUint(cur.Gid, 10, 32)
		assert.Equal(t, uint32(gid), cred.Gid)
		expectSupplementaryGroups(t, cur, cred.Groups)
	}
}

// expectSupplementaryGroups asserts that credGroups matches the current user's
// supplementary groups (all groups from usr.GroupIds() except the primary Gid).
func expectSupplementaryGroups(t *testing.T, usr *user.User, credGroups []uint32) {
	t.Helper()
	primaryGid, err := strconv.ParseUint(usr.Gid, 10, 32)
	if err != nil {
		t.Skipf("cannot parse primary gid: %v", err)
	}
	allGids, err := usr.GroupIds()
	if err != nil {
		t.Skipf("cannot get group ids: %v", err)
	}
	var expected []uint32
	for _, gidStr := range allGids {
		g, parseErr := strconv.ParseUint(gidStr, 10, 32)
		if parseErr != nil {
			continue
		}
		if g == primaryGid {
			continue
		}
		expected = append(expected, uint32(g))
	}
	assert.ElementsMatch(t, expected, credGroups, "supplementary groups should match user's groups excluding primary")
}
