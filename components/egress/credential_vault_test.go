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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
	"github.com/alibaba/opensandbox/egress/pkg/policy"
	"github.com/stretchr/testify/require"
)

func testCredentialPolicy(t *testing.T, raw string) *policy.NetworkPolicy {
	t.Helper()
	pol, err := policy.ParsePolicy(raw)
	require.NoError(t, err)
	return pol
}

func testCredentialVaultRequest() credentialVaultCreateRequest {
	return credentialVaultCreateRequest{
		Credentials: []credential{
			{
				Name: "gitlab-token",
				Source: inlineCredentialSource{
					Type:  "inline",
					Value: "secret-token",
				},
			},
		},
		Bindings: []credentialBinding{
			{
				Name: "gitlab-api",
				Match: credentialMatch{
					Hosts:   []string{"code.alibaba-inc.com"},
					Methods: []string{"GET"},
					Paths:   []string{"/api/v8/*"},
				},
				Auth: credentialAuth{
					Type:       "customHeader",
					Name:       "PRIVATE-TOKEN",
					Credential: "gitlab-token",
				},
			},
		},
	}
}

func TestCredentialVaultCreateSanitizesAndRendersActiveSnapshot(t *testing.T) {
	store := newCredentialVaultStore(nil, func() bool { return true })
	pol := testCredentialPolicy(t, `{"defaultAction":"deny","egress":[{"action":"allow","target":"code.alibaba-inc.com"}]}`)

	state, err := store.create(testCredentialVaultRequest(), pol)
	require.NoError(t, err)
	require.Equal(t, int64(1), state.Revision)
	require.Equal(t, []credentialMetadata{{Name: "gitlab-token", SourceType: "inline", Revision: 1}}, state.Credentials)
	require.Equal(t, "customHeader", state.Bindings[0].Auth.Type)
	require.Equal(t, "Private-Token", state.Bindings[0].Auth.Name)

	payload, err := store.activeSnapshot()
	require.NoError(t, err)
	require.Equal(t, int64(1), payload.Revision)
	require.Equal(t, []injectionHeader{{Name: "Private-Token", Value: "secret-token"}}, payload.Bindings[0].Headers)
	require.Contains(t, payload.Redactions, "secret-token")
}

func TestCredentialVaultActiveRejectsProxiedPublicEgressToken(t *testing.T) {
	store := newCredentialVaultStore(nil, func() bool { return true })
	pol := testCredentialPolicy(t, `{"defaultAction":"deny","egress":[{"action":"allow","target":"code.alibaba-inc.com"}]}`)
	_, err := store.create(testCredentialVaultRequest(), pol)
	require.NoError(t, err)
	srv := &policyServer{
		token:                "public-egress-token",
		credentialProxyToken: "internal-credential-proxy-token",
		credentialVault:      store,
	}

	req := httptest.NewRequest(http.MethodGet, "/credential-vault/_active", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	req.Header.Set(constants.EgressAuthTokenHeader, "public-egress-token")
	w := httptest.NewRecorder()

	srv.handleCredentialVaultSubresource(w, req)

	require.Equal(t, http.StatusForbidden, w.Result().StatusCode)
	require.NotContains(t, w.Body.String(), "secret-token")
}

func TestCredentialVaultActiveAllowsInternalCredentialProxyToken(t *testing.T) {
	store := newCredentialVaultStore(nil, func() bool { return true })
	pol := testCredentialPolicy(t, `{"defaultAction":"deny","egress":[{"action":"allow","target":"code.alibaba-inc.com"}]}`)
	_, err := store.create(testCredentialVaultRequest(), pol)
	require.NoError(t, err)
	srv := &policyServer{
		token:                "public-egress-token",
		credentialProxyToken: "internal-credential-proxy-token",
		credentialVault:      store,
	}

	req := httptest.NewRequest(http.MethodGet, "/credential-vault/_active", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	req.Header.Set(constants.CredentialProxyAuthHeader, "internal-credential-proxy-token")
	w := httptest.NewRecorder()

	srv.handleCredentialVaultSubresource(w, req)

	require.Equal(t, http.StatusOK, w.Result().StatusCode)
	require.Contains(t, w.Body.String(), "secret-token")
	require.Contains(t, w.Body.String(), "Private-Token")
}

func TestCredentialVaultRejectsDefaultAllowWithoutExplicitCoverage(t *testing.T) {
	store := newCredentialVaultStore(nil, func() bool { return true })
	pol := testCredentialPolicy(t, `{"defaultAction":"allow","egress":[]}`)

	_, err := store.create(testCredentialVaultRequest(), pol)
	require.ErrorContains(t, err, "explicit networkPolicy.egress")
}

func TestCredentialVaultRejectsReservedAndDuplicateHeaderNamesCaseInsensitively(t *testing.T) {
	_, err := normalizeBinding(credentialBinding{
		Name:  "bad",
		Match: credentialMatch{Hosts: []string{"code.alibaba-inc.com"}},
		Auth: credentialAuth{
			Type:       "customHeader",
			Name:       "Content-Length",
			Credential: "token",
		},
	})
	require.ErrorContains(t, err, "reserved credential header name")

	_, err = normalizeBinding(credentialBinding{
		Name:  "dupe",
		Match: credentialMatch{Hosts: []string{"code.alibaba-inc.com"}},
		Auth: credentialAuth{
			Type: "customHeaders",
			Headers: []customHeaderEntry{
				{Name: "X-Access-Token", Credential: "a"},
				{Name: "x-access-token", Credential: "b"},
			},
		},
	})
	require.ErrorContains(t, err, "duplicate custom header name")
}

func TestCredentialVaultPatchRejectsDeletingReferencedCredential(t *testing.T) {
	store := newCredentialVaultStore(nil, func() bool { return true })
	pol := testCredentialPolicy(t, `{"defaultAction":"deny","egress":[{"action":"allow","target":"code.alibaba-inc.com"}]}`)
	_, err := store.create(testCredentialVaultRequest(), pol)
	require.NoError(t, err)

	_, err = store.patch(credentialVaultMutationRequest{
		Credentials: &credentialMutationSet{Delete: []string{"gitlab-token"}},
	}, pol)
	require.ErrorContains(t, err, "references unknown credential")

	state, err := store.patch(credentialVaultMutationRequest{
		Bindings:    &credentialBindingMutationSet{Delete: []string{"gitlab-api"}},
		Credentials: &credentialMutationSet{Delete: []string{"gitlab-token"}},
	}, pol)
	require.NoError(t, err)
	require.Empty(t, state.Credentials)
	require.Empty(t, state.Bindings)
}

func TestCredentialVaultActiveBindingBlocksEgressPolicyRemoval(t *testing.T) {
	initial := testCredentialPolicy(t, `{"defaultAction":"deny","egress":[{"action":"allow","target":"code.alibaba-inc.com"}]}`)
	proxy := &stubProxy{updated: initial}
	nft := &stubNft{}
	store := newCredentialVaultStore(nil, func() bool { return true })
	_, err := store.create(testCredentialVaultRequest(), initial)
	require.NoError(t, err)
	srv := &policyServer{
		proxy:           proxy,
		nft:             nft,
		enforcementMode: "dns+nft",
		credentialVault: store,
	}

	req := httptest.NewRequest(http.MethodDelete, "/policy", strings.NewReader(`["code.alibaba-inc.com"]`))
	w := httptest.NewRecorder()
	srv.handlePolicy(w, req)

	require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
	require.Len(t, proxy.updated.Egress, 1)
	require.Equal(t, 0, nft.calls)
}

func TestCredentialVaultActiveBindingBlocksPolicyReset(t *testing.T) {
	initial := testCredentialPolicy(t, `{"defaultAction":"deny","egress":[{"action":"allow","target":"code.alibaba-inc.com"}]}`)
	proxy := &stubProxy{updated: initial}
	nft := &stubNft{}
	store := newCredentialVaultStore(nil, func() bool { return true })
	_, err := store.create(testCredentialVaultRequest(), initial)
	require.NoError(t, err)
	srv := &policyServer{
		proxy:           proxy,
		nft:             nft,
		enforcementMode: "dns+nft",
		credentialVault: store,
	}

	req := httptest.NewRequest(http.MethodPost, "/policy", strings.NewReader(""))
	w := httptest.NewRecorder()
	srv.handlePolicy(w, req)

	require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
	require.Contains(t, w.Body.String(), "credential vault policy validation")
	require.Len(t, proxy.updated.Egress, 1)
	require.Equal(t, 0, nft.calls)
}

func TestCredentialVaultDeleteRequiresReady(t *testing.T) {
	t.Setenv(constants.EnvMitmproxyTransparent, "")
	srv := &policyServer{
		credentialVault: newCredentialVaultStore(nil, func() bool { return true }),
	}

	req := httptest.NewRequest(http.MethodDelete, "/credential-vault", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	w := httptest.NewRecorder()
	srv.handleCredentialVault(w, req)

	require.Equal(t, http.StatusPreconditionFailed, w.Result().StatusCode)
	require.Contains(t, w.Body.String(), "transparent mitmproxy")
}

func TestCredentialVaultWriteRequiresTLSOrLoopback(t *testing.T) {
	t.Setenv(constants.EnvMitmproxyTransparent, "true")
	initial := testCredentialPolicy(t, `{"defaultAction":"deny","egress":[{"action":"allow","target":"code.alibaba-inc.com"}]}`)
	srv := &policyServer{
		proxy:           &stubProxy{updated: initial},
		credentialVault: newCredentialVaultStore(nil, func() bool { return true }),
	}

	req := httptest.NewRequest(http.MethodPost, "/credential-vault", strings.NewReader(`{"credentials":[],"bindings":[]}`))
	req.RemoteAddr = "198.51.100.10:1234"
	w := httptest.NewRecorder()
	srv.handleCredentialVault(w, req)

	require.Equal(t, http.StatusUpgradeRequired, w.Result().StatusCode)

	req = httptest.NewRequest(http.MethodPost, "/credential-vault", strings.NewReader(`{"credentials":[],"bindings":[]}`))
	req.RemoteAddr = "127.0.0.1:4321"
	w = httptest.NewRecorder()
	srv.handleCredentialVault(w, req)

	require.Equal(t, http.StatusCreated, w.Result().StatusCode)

	req = httptest.NewRequest(http.MethodDelete, "/credential-vault", nil)
	req.RemoteAddr = "198.51.100.10:1234"
	w = httptest.NewRecorder()
	srv.handleCredentialVault(w, req)

	require.Equal(t, http.StatusUpgradeRequired, w.Result().StatusCode)

	req = httptest.NewRequest(http.MethodDelete, "/credential-vault", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	w = httptest.NewRecorder()
	srv.handleCredentialVault(w, req)

	require.Equal(t, http.StatusNoContent, w.Result().StatusCode)
}

func TestParseMitmproxyIgnoreHosts(t *testing.T) {
	require.Equal(t, []string{`^example\.com$`, `.*\.internal$`}, parseMitmproxyIgnoreHosts(`
mode:
  - transparent
ignore_hosts:
  - '^example\.com$'
  - ".*\.internal$"
listen_host: 127.0.0.1
`))

	require.Equal(t, []string{`^example\.com$`, `.*\.internal$`}, parseMitmproxyIgnoreHosts(`
ignore_hosts: ['^example\.com$', ".*\.internal$"]
`))

	require.Nil(t, parseMitmproxyIgnoreHosts("ignore_hosts: []"))
}
