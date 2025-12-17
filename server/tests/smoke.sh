#!/bin/bash
# Copyright 2025 Alibaba Group Holding Ltd.
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


set -euo pipefail

color() {
  if [[ -t 1 ]] && command -v tput >/dev/null 2>&1; then
    tput setaf "$1"
  fi
}

reset_color() {
  if [[ -t 1 ]] && command -v tput >/dev/null 2>&1; then
    tput sgr0
  fi
}

STEP_COLOR=6   # cyan
INFO_COLOR=2   # green
WARN_COLOR=3   # yellow
ERR_COLOR=1    # red

step() {
  printf "\n%s==== %s ====%s\n" "$(color ${STEP_COLOR})" "$1" "$(reset_color)"
}

info() {
  printf "%s%s%s\n" "$(color ${INFO_COLOR})" "$1" "$(reset_color)"
}

warn() {
  printf "%s%s%s\n" "$(color ${WARN_COLOR})" "$1" "$(reset_color)" >&2
}

error() {
  printf "%s%s%s\n" "$(color ${ERR_COLOR})" "$1" "$(reset_color)" >&2
}

BASE_URL="${BASE_URL:-http://localhost:32888}"
API_KEY_HEADER=()
if [[ -n "${OPEN_SANDBOX_API_KEY:-}" ]]; then
  API_KEY_HEADER=(-H "OPEN-SANDBOX-API-KEY: ${OPEN_SANDBOX_API_KEY}")
fi

curl_json() {
  curl -sfSL "$@" "${API_KEY_HEADER[@]}"
}

curl_json_status() {
  # Returns body + trailing status code line to allow non-2xx handling.
  curl -sSL -w "\n%{http_code}" "$@" "${API_KEY_HEADER[@]}"
}

wait_for_running() {
  local deadline=$((SECONDS + 10))
  while true; do
    local resp
    resp=$(curl_json "${BASE_URL}/sandboxes/${SANDBOX_ID}")
    local state
    state=$(python - <<'PY' "${resp}"
import json,sys
body=json.loads(sys.argv[1])
print(body.get("status", {}).get("state", ""))
PY
)
    if [[ "${state}" == "Running" ]]; then
      printf '%s' "${resp}"
      return 0
    fi
    if [[ "${state}" == "Failed" || "${state}" == "Terminated" ]]; then
      error "Sandbox ${SANDBOX_ID} entered terminal state '${state}' before running."
      return 1
    fi
    if (( SECONDS >= deadline )); then
      error "Sandbox ${SANDBOX_ID} did not reach Running state within 10s (last state: ${state})."
      return 1
    fi
    sleep 1
  done
}

wait_for_expired() {
  local sandbox_id=$1
  local deadline=$((SECONDS + 90))
  while true; do
    local resp body status
    resp=$(curl_json_status "${BASE_URL}/sandboxes/${sandbox_id}")
    status="${resp##*$'\n'}"
    body="${resp%$'\n'*}"
    if [[ "${status}" == "404" ]]; then
      info "Sandbox ${sandbox_id} expired as expected."
      return 0
    fi
    if (( SECONDS >= deadline )); then
      error "Sandbox ${sandbox_id} did not expire within expected window (last status ${status})."
      echo "${body}"
      return 1
    fi
    sleep 2
  done
}

docker pull ubuntu:latest

create_payload='{
  "image": { "uri": "ubuntu" },
  "env": { "HELLO": "WORLD" },
  "metadata": { "hello": "world" },
  "entrypoint": ["tail", "-f", "/dev/null"],
  "resourceLimits": { "cpu": "500m", "memory": "512Mi" },
  "timeout": 60
}'

step "Create sandbox (60s TTL)"
create_resp=$(curl_json \
  -H 'Content-Type: application/json' \
  -d "${create_payload}" \
  "${BASE_URL}/sandboxes")

SANDBOX_ID=$(python - <<'PY' "${create_resp}"
import json,sys
data=json.loads(sys.argv[1])
sid=str(data.get("id","")).strip()
if not sid:
    raise SystemExit("Failed to parse sandbox id from response")
print(sid,end="")
PY
)

echo "Sandbox created: id=${SANDBOX_ID}"

step "Wait for sandbox to reach Running"
get_resp=$(wait_for_running)
state=$(python - <<'PY' "${get_resp}"
import json,sys
body=json.loads(sys.argv[1])
print(body.get("status",{}).get("state"))
PY
)
echo "Sandbox state: ${state}"

python - <<'PY' "${get_resp}" "${SANDBOX_ID}"
import json,sys
body=json.loads(sys.argv[1])
expected=sys.argv[2]
assert str(body.get("id"))==expected, "Sandbox ID mismatch in GET response"
assert body.get("status",{}).get("state") in {"Pending","Running","Unknown","Paused","Terminated","Failed"}, "Unexpected state"
PY

step "List sandboxes (metadata filter)"
list_resp=$(curl_json \
  -G \
  --data-urlencode "metadata=hello=world" \
  --data-urlencode "page=1" \
  --data-urlencode "pageSize=10" \
  "${BASE_URL}/sandboxes")

python - <<'PY' "${list_resp}" "${SANDBOX_ID}"
import json,sys
body=json.loads(sys.argv[1])
sid=sys.argv[2]
ids=[item.get("id") for item in body.get("items",[])]
assert sid in ids, "Sandbox ID not found in list response"
assert body.get("pagination",{}).get("page") == 1, "Unexpected pagination page"
PY
echo "List check passed (found sandbox, pagination ok)"

step "Renew sandbox expiration (+10m)"
new_expiration=$(python - <<'PY'
from datetime import datetime, timedelta, timezone
print((datetime.now(timezone.utc) + timedelta(minutes=10)).isoformat())
PY
)

renew_payload=$(cat <<JSON
{
  "expiresAt": "${new_expiration}"
}
JSON
)

renew_resp=$(curl_json \
  -X POST \
  -H 'Content-Type: application/json' \
  -d "${renew_payload}" \
  "${BASE_URL}/sandboxes/${SANDBOX_ID}/renew-expiration")
renewed=$(python - <<'PY' "${renew_resp}"
import json,sys
body=json.loads(sys.argv[1])
print(body.get("expiresAt"))
PY
)
echo "Expiration renewed to: ${renewed}"

step "Request endpoint on port 8080"
endpoint_resp=$(curl_json "${BASE_URL}/sandboxes/${SANDBOX_ID}/endpoints/8080")
endpoint=$(python - <<'PY' "${endpoint_resp}"
import json,sys
body=json.loads(sys.argv[1])
print(body.get("endpoint"))
PY
)
echo "Endpoint: ${endpoint}"

step "Delete sandbox"
curl_json -X DELETE "${BASE_URL}/sandboxes/${SANDBOX_ID}"
echo "Sandbox ${SANDBOX_ID} deleted."

step "Create short-lived sandbox (60s TTL) for auto-expiration"
create_payload_short='{
  "image": { "uri": "ubuntu" },
  "env": {},
  "metadata": { "lifecycle": "short" },
  "entrypoint": ["tail", "-f", "/dev/null"],
  "resourceLimits": { "cpu": "1", "memory": "2Gi" },
  "timeout": 60
}'

create_resp_short=$(curl_json \
  -H 'Content-Type: application/json' \
  -d "${create_payload_short}" \
  "${BASE_URL}/sandboxes")

SANDBOX_ID=$(python - <<'PY' "${create_resp_short}"
import json,sys
data=json.loads(sys.argv[1])
sid=str(data.get("id","")).strip()
if not sid:
    raise SystemExit("Failed to parse sandbox id from response")
print(sid,end="")
PY
)

echo "Short-lived sandbox created: id=${SANDBOX_ID}"

step "Wait for short-lived sandbox to reach Running"
get_resp_short=$(wait_for_running)
state_short=$(python - <<'PY' "${get_resp_short}"
import json,sys
body=json.loads(sys.argv[1])
print(body.get("status",{}).get("state"))
PY
)
echo "Sandbox state: ${state_short}"

step "Wait for sandbox ${SANDBOX_ID} to auto-expire (expect 404)"
wait_for_expired "${SANDBOX_ID}"

step "Lifecycle API smoke test completed successfully"
