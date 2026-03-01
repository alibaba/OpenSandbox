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
	"os/user"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunCommandRequestValidate(t *testing.T) {
	req := RunCommandRequest{Command: "ls"}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected command validation success: %v", err)
	}

	req.TimeoutMs = -100
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error when timeout is negative")
	}

	req.TimeoutMs = 0
	req.Command = "ls"
	if err := req.Validate(); err != nil {
		t.Fatalf("expected success when timeout is omitted/zero: %v", err)
	}

	req.TimeoutMs = 10
	req.Command = ""
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error when command is empty")
	}
}

func TestRunCommandRequestValidate_UserObject(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	req := RunCommandRequest{
		Command: "ls",
		User: &UserIdentity{
			Username: &cur.Username,
		},
	}
	assert.NoError(t, req.Validate())

	if uid, parseErr := strconv.ParseInt(cur.Uid, 10, 64); parseErr == nil {
		req.User = &UserIdentity{
			UID: &uid,
		}
		assert.NoError(t, req.Validate())
	}

	req.User = &UserIdentity{
		Username: ptrString("sandbox"),
		UID:      ptrInt64(1001),
	}
	assert.Error(t, req.Validate())
}

func ptrString(s string) *string { return &s }
func ptrInt64(i int64) *int64    { return &i }
