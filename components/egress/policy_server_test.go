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

package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alibaba/opensandbox/egress/pkg/policy"
)

type stubProxy struct {
	updated *policy.NetworkPolicy
}

func (s *stubProxy) CurrentPolicy() *policy.NetworkPolicy {
	return s.updated
}

func (s *stubProxy) UpdatePolicy(p *policy.NetworkPolicy) {
	s.updated = p
}

type stubNft struct {
	err     error
	calls   int
	applied *policy.NetworkPolicy
}

func (s *stubNft) ApplyStatic(_ context.Context, p *policy.NetworkPolicy) error {
	s.calls++
	s.applied = p
	return s.err
}

func TestHandlePolicy_AppliesNftAndUpdatesProxy(t *testing.T) {
	proxy := &stubProxy{}
	nft := &stubNft{}
	srv := &policyServer{proxy: proxy, nft: nft, enforcementMode: "dns+nft"}

	body := `{"defaultAction":"deny","egress":[{"action":"allow","target":"1.1.1.1"}]}`
	req := httptest.NewRequest(http.MethodPost, "/policy", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.handlePolicy(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if hdr := resp.Header.Get("Content-Type"); !strings.Contains(hdr, "application/json") {
		t.Fatalf("expected json response, got %s", hdr)
	}
	if nft.calls != 1 {
		t.Fatalf("expected nft ApplyStatic called once, got %d", nft.calls)
	}
	if proxy.updated == nil {
		t.Fatalf("expected proxy policy to be updated")
	}
	if proxy.updated.DefaultAction != policy.ActionDeny {
		t.Fatalf("unexpected defaultAction: %s", proxy.updated.DefaultAction)
	}
}

func TestHandlePolicy_NftFailureReturns500(t *testing.T) {
	proxy := &stubProxy{}
	nft := &stubNft{err: errors.New("boom")}
	srv := &policyServer{proxy: proxy, nft: nft, enforcementMode: "dns+nft"}

	body := `{"defaultAction":"deny","egress":[{"action":"allow","target":"1.1.1.1"}]}`
	req := httptest.NewRequest(http.MethodPost, "/policy", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.handlePolicy(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	if nft.calls != 1 {
		t.Fatalf("expected nft ApplyStatic called once, got %d", nft.calls)
	}
	if proxy.updated != nil {
		t.Fatalf("expected proxy policy not updated on nft failure")
	}
}

func TestHandleGet_ReturnsEnforcementMode(t *testing.T) {
	proxy := &stubProxy{updated: policy.DefaultDenyPolicy()}
	srv := &policyServer{proxy: proxy, nft: nil, enforcementMode: "dns"}

	req := httptest.NewRequest(http.MethodGet, "/policy", nil)
	w := httptest.NewRecorder()

	srv.handlePolicy(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"enforcementMode":"dns"`) {
		t.Fatalf("expected enforcementMode dns in response, got: %s", string(body))
	}
}
