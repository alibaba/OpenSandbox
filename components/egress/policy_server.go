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
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
	"github.com/alibaba/opensandbox/egress/pkg/log"
	"github.com/alibaba/opensandbox/egress/pkg/nftables"
	"github.com/alibaba/opensandbox/egress/pkg/policy"
)

type policyUpdater interface {
	CurrentPolicy() *policy.NetworkPolicy
	UpdatePolicy(*policy.NetworkPolicy)
}

// enforcementReporter reports the current enforcement mode (dns | dns+nft).
type enforcementReporter interface {
	EnforcementMode() string
}

// nftApplier applies static policy and optional dynamic DNS-learned IPs to nftables.
type nftApplier interface {
	ApplyStatic(context.Context, *policy.NetworkPolicy) error
	AddResolvedIPs(context.Context, []nftables.ResolvedIP) error
}

// startPolicyServer launches a lightweight HTTP API for updating the egress policy at runtime.
// Supported endpoints:
//   - GET  /policy : returns the currently enforced policy.
//   - POST /policy : replace the policy; empty body resets to default deny-all.
//
// nameserverIPs are merged into every applied policy so system DNS stays allowed (e.g. private DNS).
func startPolicyServer(ctx context.Context, proxy policyUpdater, nft nftApplier, enforcementMode string, addr string, token string, nameserverIPs []netip.Addr) error {
	mux := http.NewServeMux()
	handler := &policyServer{proxy: proxy, nft: nft, token: token, enforcementMode: enforcementMode, nameserverIPs: nameserverIPs}
	mux.HandleFunc("/policy", handler.handlePolicy)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	handler.server = srv

	// Shutdown listener when context ends.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warnf("policy server shutdown error: %v", err)
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(200 * time.Millisecond):
		// assume healthy start; keep logging future errors
		go func() {
			if err := <-errCh; err != nil {
				log.Errorf("policy server error: %v", err)
			}
		}()
		return nil
	}
}

type policyServer struct {
	proxy           policyUpdater
	nft             nftApplier
	server          *http.Server
	token           string
	enforcementMode string
	nameserverIPs   []netip.Addr
}

func (s *policyServer) handlePolicy(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGet(w)
	case http.MethodPost, http.MethodPut:
		s.handlePost(w, r)
	default:
		w.Header().Set("Allow", "GET, POST, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *policyServer) handleGet(w http.ResponseWriter) {
	current := s.proxy.CurrentPolicy()
	mode := modeFromPolicy(current)
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":            mode,
		"enforcementMode": s.enforcementMode,
		"policy":          current,
	})
}

func (s *policyServer) handlePost(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read body: %v", err), http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		log.Infof("policy API: reset to default deny-all")
		def := policy.DefaultDenyPolicy()
		if s.nft != nil {
			defWithNS := def.WithExtraAllowIPs(s.nameserverIPs)
			if err := s.nft.ApplyStatic(r.Context(), defWithNS); err != nil {
				log.Errorf("policy API: nftables apply failed on reset: %v", err)
				http.Error(w, fmt.Sprintf("failed to apply nftables: %v", err), http.StatusInternalServerError)
				return
			}
		}
		s.proxy.UpdatePolicy(def)
		log.Infof("policy API: proxy and nftables updated to deny_all")
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"mode":   "deny_all",
			"reason": "policy reset to default deny-all",
		})
		return
	}

	pol, err := policy.ParsePolicy(raw)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid policy: %v", err), http.StatusBadRequest)
		return
	}
	mode := modeFromPolicy(pol)
	log.Infof("policy API: updating policy to mode=%s, enforcement=%s", mode, s.enforcementMode)
	if s.nft != nil {
		polWithNS := pol.WithExtraAllowIPs(s.nameserverIPs)
		if err := s.nft.ApplyStatic(r.Context(), polWithNS); err != nil {
			log.Errorf("policy API: nftables apply failed: %v", err)
			http.Error(w, fmt.Sprintf("failed to apply nftables policy: %v", err), http.StatusInternalServerError)
			return
		}
	}
	s.proxy.UpdatePolicy(pol)
	log.Infof("policy API: proxy and nftables updated successfully")
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"mode":            mode,
		"enforcementMode": s.enforcementMode,
	})
}

func (s *policyServer) authorize(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	provided := r.Header.Get(constants.EgressAuthTokenHeader)
	if provided == "" {
		return false
	}
	if len(provided) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) == 1
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func modeFromPolicy(p *policy.NetworkPolicy) string {
	if p == nil {
		return "deny_all"
	}
	if p.DefaultAction == policy.ActionAllow && len(p.Egress) == 0 {
		return "allow_all"
	} else if p.DefaultAction == policy.ActionDeny && len(p.Egress) == 0 {
		return "deny_all"
	}

	return "enforcing"
}
