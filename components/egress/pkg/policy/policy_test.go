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

package policy

import "testing"

func TestParsePolicy_EmptyOrNullDefaultsDeny(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"null",
		"{}\n",
	}
	for _, raw := range cases {
		p, err := ParsePolicy(raw)
		if err != nil {
			t.Fatalf("raw %q returned error: %v", raw, err)
		}
		if p == nil {
			t.Fatalf("raw %q expected default deny policy, got nil", raw)
		}
		if p.DefaultAction != ActionDeny {
			t.Fatalf("raw %q expected defaultAction deny, got %+v", raw, p)
		}
		if got := p.Evaluate("example.com."); got != ActionDeny {
			t.Fatalf("raw %q expected deny evaluation, got %s", raw, got)
		}
	}
}

func TestParsePolicy_DefaultActionFallback(t *testing.T) {
	p, err := ParsePolicy(`{"egress":[{"action":"allow","target":"example.com"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatalf("expected policy object, got nil")
	}
	if p.DefaultAction != ActionDeny {
		t.Fatalf("expected defaultAction fallback to deny, got %+v", p)
	}
}

func TestParsePolicy_EmptyEgressDefaultsDeny(t *testing.T) {
	p, err := ParsePolicy(`{"defaultAction":""}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.DefaultAction != ActionDeny {
		t.Fatalf("expected default deny when defaultAction missing, got %+v", p)
	}
	if got := p.Evaluate("anything.com."); got != ActionDeny {
		t.Fatalf("expected evaluation deny for empty egress, got %s", got)
	}
}

func TestParsePolicy_IPAndCIDRSupported(t *testing.T) {
	raw := `{
		"defaultAction":"deny",
		"egress":[
			{"action":"allow","target":"1.1.1.1"},
			{"action":"allow","target":"2.2.0.0/16"},
			{"action":"deny","target":"2001:db8::/32"},
			{"action":"deny","target":"2001:db8::1"}
		]
	}`
	p, err := ParsePolicy(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	allowV4, allowV6, denyV4, denyV6 := p.StaticIPSets()
	if len(allowV4) != 2 || allowV4[0] != "1.1.1.1" || allowV4[1] != "2.2.0.0/16" {
		t.Fatalf("allowV4 unexpected: %+v", allowV4)
	}
	if len(denyV6) != 2 {
		t.Fatalf("expected 2 denyV6 entries, got %+v", denyV6)
	}
	if len(allowV6) != 0 || len(denyV4) != 0 {
		t.Fatalf("allowV6/denyV4 should be empty, got %v / %v", allowV6, denyV4)
	}
}

func TestParsePolicy_InvalidAction(t *testing.T) {
	if _, err := ParsePolicy(`{"egress":[{"action":"foo","target":"example.com"}]}`); err == nil {
		t.Fatalf("expected error for invalid action")
	}
}

func TestParsePolicy_EmptyTargetError(t *testing.T) {
	if _, err := ParsePolicy(`{"egress":[{"action":"allow","target":""}]}`); err == nil {
		t.Fatalf("expected error for empty target")
	}
}
