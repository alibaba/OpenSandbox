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

package dnsproxy

import (
	"testing"

	"github.com/alibaba/opensandbox/egress/pkg/policy"
)

func TestProxyUpdatePolicy(t *testing.T) {
	proxy, err := New(nil, "127.0.0.1:15353")
	if err != nil {
		t.Fatalf("init proxy: %v", err)
	}

	if proxy.CurrentPolicy() != nil {
		t.Fatalf("expected initial allow-all (nil policy)")
	}

	pol, err := policy.ParsePolicy(`{"defaultAction":"deny","egress":[{"action":"allow","target":"example.com"}]}`)
	if err != nil {
		t.Fatalf("parse policy: %v", err)
	}

	proxy.UpdatePolicy(pol)
	if proxy.CurrentPolicy() == nil {
		t.Fatalf("expected policy after update")
	}
	if got := proxy.CurrentPolicy().Evaluate("example.com."); got != policy.ActionAllow {
		t.Fatalf("policy evaluation mismatch, want allow got %s", got)
	}

	proxy.UpdatePolicy(nil)
	if proxy.CurrentPolicy() != nil {
		t.Fatalf("expected allow-all after clearing policy")
	}
}

func TestLoadPolicyFromEnvVar(t *testing.T) {
	const envName = "TEST_EGRESS_POLICY"
	t.Setenv(envName, `{"defaultAction":"deny","egress":[{"action":"allow","target":"example.com"}]}`)

	pol, err := LoadPolicyFromEnvVar(envName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pol == nil || pol.Evaluate("example.com.") != policy.ActionAllow {
		t.Fatalf("expected parsed policy to allow example.com")
	}

	t.Setenv(envName, "")
	pol, err = LoadPolicyFromEnvVar(envName)
	if err != nil {
		t.Fatalf("unexpected error on empty env: %v", err)
	}
	if pol != nil {
		t.Fatalf("expected nil policy when env is empty")
	}
}
