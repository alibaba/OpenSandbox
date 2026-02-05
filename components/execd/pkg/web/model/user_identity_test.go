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
	if err := req.Validate(); err != nil {
		t.Fatalf("expected validation success with uid user: %v", err)
	}

	if uid, parseErr := strconv.ParseInt(cur.Uid, 10, 64); parseErr == nil {
		req.User = newUserIdentityFromUID(uid)
		if err := req.Validate(); err != nil {
			t.Fatalf("expected validation success with existing uid: %v", err)
		}
	}

	req.User = newUserIdentityFromUID(-1)
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error for negative uid")
	}

	req.User = newUserIdentityFromUsername("")
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error for empty username")
	}
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
	if err := req.Validate(); err != nil {
		t.Fatalf("expected validation success with existing username: %v", err)
	}

	if uid, parseErr := strconv.ParseInt(cur.Uid, 10, 64); parseErr == nil {
		req.User = newUserIdentityFromUID(uid)
		if err := req.Validate(); err != nil {
			t.Fatalf("expected validation success with existing uid: %v", err)
		}
	}

	req.User = newUserIdentityFromUsername("user-does-not-exist-123456789")
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error for missing username")
	}
}

func TestUserIdentity_jsonRoundTrip(t *testing.T) {
	var user UserIdentity
	if err := json.Unmarshal([]byte(`"sandbox"`), &user); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if name, ok := user.Username(); !ok || name != "sandbox" {
		t.Fatalf("expected username=sandbox, got %q", name)
	}
	if _, ok := user.UID(); ok {
		t.Fatalf("expected uid to be unset")
	}
	if err := user.validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	b, err := json.Marshal(&user)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if string(b) != `"sandbox"` {
		t.Fatalf("expected marshaled username, got %s", string(b))
	}

	var uidUser UserIdentity
	if err := json.Unmarshal([]byte(`1001`), &uidUser); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if uid, ok := uidUser.UID(); !ok || uid != 1001 {
		t.Fatalf("expected uid=1001, got %d", uid)
	}
	if _, ok := uidUser.Username(); ok {
		t.Fatalf("expected username to be unset")
	}
	if err := uidUser.validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	b, err = json.Marshal(&uidUser)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if string(b) != "1001" {
		t.Fatalf("expected marshaled uid, got %s", string(b))
	}
}

func TestUserIdentity_unmarshalInvalid(t *testing.T) {
	var user UserIdentity
	if err := json.Unmarshal([]byte(`{"name":"bad"}`), &user); err == nil {
		t.Fatalf("expected error for object input")
	}
}
