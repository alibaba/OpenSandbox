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

import (
	"encoding/json"
	"strings"
)

const (
	ActionAllow = "allow"
	ActionDeny  = "deny"
)

// NetworkPolicy is the minimal MVP shape for egress control.
// Only domain/wildcard targets are honored in this MVP.
type NetworkPolicy struct {
	Egress        []EgressRule `json:"egress"`
	DefaultAction string       `json:"defaultAction"`
}

type EgressRule struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

// ParsePolicy parses JSON from env/config into a NetworkPolicy.
// Default action falls back to "deny" to align with proposal.
func ParsePolicy(raw string) (*NetworkPolicy, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return nil, nil // treat empty/null payload as allow-all
	}

	var p NetworkPolicy
	if err := json.Unmarshal([]byte(trimmed), &p); err != nil {
		return nil, err
	}
	if p.DefaultAction == "" {
		if len(p.Egress) == 0 {
			return nil, nil // empty object -> allow-all
		}
		p.DefaultAction = ActionDeny
	}
	return &p, nil
}

// Evaluate returns allow/deny for a given domain (lowercased).
func (p *NetworkPolicy) Evaluate(domain string) string {
	if p == nil {
		return ActionAllow
	}
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	for _, r := range p.Egress {
		if r.matchesDomain(domain) {
			if r.Action == "" {
				return ActionDeny
			}
			return r.Action
		}
	}
	if p.DefaultAction == "" {
		return ActionDeny
	}
	return p.DefaultAction
}

func (r *EgressRule) matchesDomain(domain string) bool {
	pattern := strings.ToLower(strings.TrimSpace(r.Target))
	domain = strings.ToLower(domain)

	if pattern == "" {
		return false
	}
	if pattern == domain {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		// "*.example.com" matches "a.example.com" but not "example.com"
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(domain, suffix) && domain != strings.TrimPrefix(pattern, "*.")
	}
	return false
}
