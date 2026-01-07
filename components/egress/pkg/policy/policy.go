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
	DefaultAction string       `json:"default_action"`
}

type EgressRule struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

// ParsePolicy parses JSON from env/config into a NetworkPolicy.
// Default action falls back to "deny" to align with proposal.
func ParsePolicy(raw string) (*NetworkPolicy, error) {
	var p NetworkPolicy
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, err
	}
	if p.DefaultAction == "" {
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
