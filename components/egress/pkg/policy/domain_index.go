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

import "strings"

// compiledDomainRule stores the original egress order and effective action.
// The smallest index wins to preserve "first match wins" semantics.
type compiledDomainRule struct {
	index  int
	action string
}

// compiledDomainIndex accelerates domain evaluate while preserving rule order semantics.
// - exact holds exact-domain patterns (e.g. "api.example.com")
// - wildcard holds wildcard suffix patterns (e.g. ".example.com" for "*.example.com")
type compiledDomainIndex struct {
	exact    map[string]compiledDomainRule
	wildcard map[string]compiledDomainRule
}

func compileDomainIndex(egress []EgressRule) *compiledDomainIndex {
	idx := &compiledDomainIndex{
		exact:    make(map[string]compiledDomainRule),
		wildcard: make(map[string]compiledDomainRule),
	}
	for i, r := range egress {
		if r.targetKind != targetDomain {
			continue
		}
		pattern := strings.ToLower(strings.TrimSpace(r.Target))
		if pattern == "" {
			continue
		}
		cr := compiledDomainRule{
			index:  i,
			action: r.Action,
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*")
			if _, exists := idx.wildcard[suffix]; !exists {
				idx.wildcard[suffix] = cr
			}
			continue
		}
		if _, exists := idx.exact[pattern]; !exists {
			idx.exact[pattern] = cr
		}
	}
	return idx
}

func (idx *compiledDomainIndex) match(domain string) (string, bool) {
	if idx == nil || domain == "" {
		return "", false
	}

	var best compiledDomainRule
	matched := false

	if rule, ok := idx.exact[domain]; ok {
		if rule.index == 0 {
			return rule.action, true
		}
		best = rule
		matched = true
	}

	for cursor := domain; ; {
		dot := strings.IndexByte(cursor, '.')
		if dot < 0 {
			break
		}
		suffix := cursor[dot:]
		if rule, ok := idx.wildcard[suffix]; ok {
			if rule.index == 0 {
				return rule.action, true
			}
			if !matched || rule.index < best.index {
				best = rule
				matched = true
			}
		}
		cursor = cursor[dot+1:]
	}

	if !matched {
		return "", false
	}
	return best.action, true
}
