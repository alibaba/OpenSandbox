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
	"net/http"
	"net/netip"
	"strings"
	"sync"
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
	ApplyStatic(ctx context.Context, p *policy.NetworkPolicy, normalizeIntervalSets bool) error
	AddResolvedIPs(context.Context, []nftables.ResolvedIP) error
}

// startPolicyServer launches a lightweight HTTP API for updating the egress policy at runtime.
//
// nameserverIPs are merged into every applied policy so system DNS stays allowed (e.g. private DNS).
func startPolicyServer(ctx context.Context, proxy policyUpdater, nft nftApplier, enforcementMode string, addr string, token string, nameserverIPs []netip.Addr, policyFile string) error {
	maxEgressRules := maxEgressRulesFromEnv()
	if maxEgressRules > 0 {
		log.Infof("policy API: max egress rules per policy (POST/PATCH) = %d (set %s=0 to disable)", maxEgressRules, constants.EnvMaxEgressRules)
	}

	mux := http.NewServeMux()
	handler := &policyServer{
		proxy:           proxy,
		nft:             nft,
		token:           token,
		enforcementMode: enforcementMode,
		nameserverIPs:   nameserverIPs,
		policyFile:      strings.TrimSpace(policyFile),
		maxEgressRules:  maxEgressRules,
	}

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
	policyFile      string     // if set, successful policy changes are persisted here
	maxEgressRules  int        // 0 = unlimited; >0 = max len(Egress) for POST/PATCH
	mu              sync.Mutex // serializes read-merge-apply to avoid lost updates across POST/PATCH
}

type policyStatusResponse struct {
	Status          string `json:"status,omitempty"`
	Mode            string `json:"mode,omitempty"`
	EnforcementMode string `json:"enforcementMode,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Policy          any    `json:"policy,omitempty"`
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
	case http.MethodPatch:
		s.handlePatch(w, r)
	default:
		w.Header().Set("Allow", "GET, POST, PUT, PATCH")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *policyServer) handleGet(w http.ResponseWriter) {
	current := s.proxy.CurrentPolicy()
	mode := modeFromPolicy(current)
	writeJSON(w, http.StatusOK, policyStatusResponse{
		Status:          "ok",
		Mode:            mode,
		EnforcementMode: s.enforcementMode,
		Policy:          current,
	})
}

func (s *policyServer) handlePost(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := readPolicyRequestBody(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read body: %v", err), http.StatusBadRequest)
		return
	}

	if raw == "" {
		log.Infof("policy API: reset to default deny-all")
		def := policy.DefaultDenyPolicy()
		if !s.commitPolicy(r.Context(), w, def, "reset") {
			return
		}
		log.Infof("policy API: proxy and nftables updated to deny_all")
		writeJSON(w, http.StatusOK, policyStatusResponse{
			Status: "ok",
			Mode:   "deny_all",
			Reason: "policy reset to default deny-all",
		})
		return
	}

	pol, err := policy.ParsePolicy(raw)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid policy: %v", err), http.StatusBadRequest)
		return
	}
	if !s.enforceEgressRuleLimit(w, len(pol.Egress)) {
		return
	}

	mode := modeFromPolicy(pol)
	log.Infof("policy API: updating policy to mode=%s, enforcement=%s", mode, s.enforcementMode)
	if !s.commitPolicy(r.Context(), w, pol, "post") {
		return
	}
	log.Infof("policy API: proxy and nftables updated successfully")
	writeJSON(w, http.StatusOK, policyStatusResponse{
		Status:          "ok",
		Mode:            mode,
		EnforcementMode: s.enforcementMode,
	})
}

// handlePatch merges JSON egress rules into the current policy (PATCH body = array of rules).
func (s *policyServer) handlePatch(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := readPolicyRequestBody(r)
	if err != nil || raw == "" {
		http.Error(w, fmt.Sprintf("failed to read body: %v", err), http.StatusBadRequest)
		return
	}

	var patchRules []policy.EgressRule
	if err := json.Unmarshal([]byte(raw), &patchRules); err != nil || len(patchRules) == 0 {
		http.Error(w, fmt.Sprintf("invalid patch rules: %v", err), http.StatusBadRequest)
		return
	}

	newPolicy, err := patchMergedPolicy(s.proxy.CurrentPolicy(), patchRules)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid merged policy: %v", err), http.StatusBadRequest)
		return
	}
	if !s.enforceEgressRuleLimit(w, len(newPolicy.Egress)) {
		return
	}

	mode := modeFromPolicy(newPolicy)
	log.Infof("policy API: patching policy with %d new rule(s), mode=%s, enforcement=%s", len(patchRules), mode, s.enforcementMode)
	if !s.commitPolicy(r.Context(), w, newPolicy, "patch") {
		return
	}
	log.Infof("policy API: patch applied successfully")
	writeJSON(w, http.StatusOK, policyStatusResponse{
		Status:          "ok",
		Mode:            mode,
		EnforcementMode: s.enforcementMode,
	})
}

// commitPolicy applies one logical update: persist (if configured) → nft → in-memory policy.
func (s *policyServer) commitPolicy(ctx context.Context, w http.ResponseWriter, pol *policy.NetworkPolicy, op string) bool {
	if err := s.persistPolicy(pol); err != nil {
		log.Errorf("policy API: persist policy failed: %v", err)
		http.Error(w, fmt.Sprintf("failed to persist policy: %v", err), http.StatusInternalServerError)
		return false
	}
	if s.nft != nil {
		if err := s.nft.ApplyStatic(ctx, pol.WithExtraAllowIPs(s.nameserverIPs), false); err != nil {
			log.Errorf("policy API: nftables apply failed (%s): %v", op, err)
			http.Error(w, fmt.Sprintf("failed to apply nftables policy: %v", err), http.StatusInternalServerError)
			return false
		}
	}
	s.proxy.UpdatePolicy(pol)
	return true
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

func (s *policyServer) enforceEgressRuleLimit(w http.ResponseWriter, egressCount int) bool {
	if s.maxEgressRules <= 0 {
		return true
	}
	if egressCount > s.maxEgressRules {
		http.Error(w, fmt.Sprintf("egress rule total count %d exceeds limit %d", egressCount, s.maxEgressRules), http.StatusRequestEntityTooLarge)
		return false
	}
	return true
}

func (s *policyServer) persistPolicy(p *policy.NetworkPolicy) error {
	if s.policyFile == "" {
		return nil
	}
	return policy.SavePolicyFile(s.policyFile, p)
}
