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
	"encoding/json"
	"os/user"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunCommand_requestValidateWithUser(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	req := RunCommandRequest{
		Command: "ls",
		User:    newUserIdentityFromUsername(cur.Username),
	}
	assert.NoError(t, req.Validate(), "expected validation success with uid user")

	if uid, parseErr := strconv.ParseInt(cur.Uid, 10, 64); parseErr == nil {
		req.User = newUserIdentityFromUID(uid)
		assert.NoError(t, req.Validate(), "expected validation success with existing uid")
	}

	req.User = newUserIdentityFromUID(-1)
	assert.Error(t, req.Validate(), "expected validation error for negative uid")

	req.User = newUserIdentityFromUsername("")
	assert.Error(t, req.Validate(), "expected validation error for empty username")
}

func TestRunCommand_requestValidateUserExists(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	req := RunCommandRequest{
		Command: "echo ok",
		User:    newUserIdentityFromUsername(cur.Username),
	}
	assert.NoError(t, req.Validate(), "expected validation success with existing username")

	if uid, parseErr := strconv.ParseInt(cur.Uid, 10, 64); parseErr == nil {
		req.User = newUserIdentityFromUID(uid)
		assert.NoError(t, req.Validate(), "expected validation success with existing uid")
	}

	req.User = newUserIdentityFromUsername("user-does-not-exist-123456789")
	assert.Error(t, req.Validate(), "expected validation error for missing username")
}

func TestUserIdentity_jsonRoundTrip(t *testing.T) {
	var user UserIdentity
	assert.NoError(t, json.Unmarshal([]byte(`"sandbox"`), &user))
	name, ok := user.Username()
	assert.True(t, ok)
	assert.Equal(t, "sandbox", name)
	_, ok = user.UID()
	assert.False(t, ok)
	assert.NoError(t, user.validate())
	b, err := json.Marshal(&user)
	assert.NoError(t, err)
	assert.Equal(t, `"sandbox"`, string(b))

	var uidUser UserIdentity
	assert.NoError(t, json.Unmarshal([]byte(`1001`), &uidUser))
	uid, ok := uidUser.UID()
	assert.True(t, ok)
	assert.Equal(t, int64(1001), uid)
	_, ok = uidUser.Username()
	assert.False(t, ok)
	assert.NoError(t, uidUser.validate())
	b, err = json.Marshal(&uidUser)
	assert.NoError(t, err)
	assert.Equal(t, "1001", string(b))
}

func TestUserIdentity_unmarshalInvalid(t *testing.T) {
	var user UserIdentity
	assert.Error(t, json.Unmarshal([]byte(`{"name":"bad"}`), &user))
}
