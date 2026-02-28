#!/bin/bash

# Copyright 2026 Alibaba Group Holding Ltd.
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#     http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Simple smoke test using local image.
# Requires Docker with --cap-add=NET_ADMIN available.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# tests/ is two levels under repo root: components/egress/tests -> climb 3 levels.
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

IMG="opensandbox/egress:local"
containerName="egress-smoke-nft"
POLICY_PORT=18080

info() { echo "[$(date +%H:%M:%S)] $*"; }

cleanup() {
  docker rm -f "${containerName}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

info "Building image ${IMG}"
docker build -t "${IMG}" -f "${REPO_ROOT}/components/egress/Dockerfile" "${REPO_ROOT}"

info "Starting containerName"
docker run -d --name "${containerName}" \
  --cap-add=NET_ADMIN \
  --sysctl net.ipv6.conf.all.disable_ipv6=1 \
  --sysctl net.ipv6.conf.default.disable_ipv6=1 \
  -e OPENSANDBOX_EGRESS_MODE=dns+nft \
  -p ${POLICY_PORT}:18080 \
  "${IMG}"

info "Waiting for policy server..."
for i in {1..50}; do
  if curl -sf "http://127.0.0.1:${POLICY_PORT}/healthz" >/dev/null; then
    break
  fi
  sleep 0.5
done

info "Pushing policy (allow by default; deny github.com & 10.0.0.0/8)"
curl -sSf -XPOST "http://127.0.0.1:${POLICY_PORT}/policy" \
  -d '{"defaultAction":"allow","egress":[{"action":"deny","target":"*.github.com"},{"action":"deny","target":"10.0.0.0/8"}]}'

run_in_app() {
  docker run --rm --network container:"${containerName}" curlimages/curl "$@"
}

pass() { info "PASS: $*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

info "Test: allowed domain should succeed (google.com)"
run_in_app -I https://google.com --max-time 10 >/dev/null 2>&1 || fail "google.com should succeed"
pass "google.com allowed"

info "Test: denied domain should fail (api.github.com)"
if run_in_app -I https://api.github.com --max-time 8 >/dev/null 2>&1; then
  fail "api.github.com should be blocked"
else
  pass "api.github.com blocked"
fi

info "Test: allowed IP should succeed (1.1.1.1)"
run_in_app -I https://1.1.1.1 --max-time 10 >/dev/null 2>&1 || fail "1.1.1.1 should succeed"
pass "1.1.1.1 allowed"

info "Test: denied CIDR should fail (10.0.0.1)"
if run_in_app -I http://10.0.0.1 --max-time 5 >/dev/null 2>&1; then
  fail "10.0.0.1 should be blocked"
else
  pass "10.0.0.1 blocked"
fi

info "Test: DoT (853) should be blocked"
if run_in_app -k https://1.1.1.1:853 --max-time 5 >/dev/null 2>&1; then
  fail "DoT 853 should be blocked"
else
  pass "DoT 853 blocked"
fi

info "All smoke tests passed."