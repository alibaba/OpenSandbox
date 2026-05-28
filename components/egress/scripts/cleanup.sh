#!/bin/sh
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
#
# Best-effort cleanup of egress sidecar state. Designed to be used as the
# opensandbox-supervisor --post-exit (after worker death) and/or --pre-start
# (before next launch) hook. Safe to run repeatedly; safe to run when egress
# was never up.
#
# Hard contract: this script MUST NOT exit non-zero. Each step swallows its
# own errors. A poststop that crashes is worse than dirty state that the
# next startup will tolerate.

# Intentionally no `set -e`. `set -u` for typo safety on env names only.
set -u

log() { printf '[egress-cleanup] %s\n' "$*" >&2; }

# Wraps a command so non-zero exit is silently absorbed. Output goes to
# stderr so it shows up in container logs without polluting the event log.
try() { "$@" 2>&1 | sed 's/^/  /' >&2; return 0; }

# ─── iptables DNS redirect (pkg/iptables/redirect.go) ────────────────
remove_dns_redirect() {
  command -v iptables >/dev/null 2>&1 || { log "iptables not present; skipping DNS redirect cleanup"; return 0; }

  DNS_PORT=15353
  MARK_HEX=0x1

  # Remove in reverse install order. The same rule may not exist (clean
  # boot), or may exist multiple times (accumulated across crash restarts);
  # loop -D until it returns non-zero, then move on. Cap iterations so a
  # broken iptables doesn't spin forever.
  delete_until_gone() {
    i=0
    while [ $i -lt 32 ]; do
      "$@" 2>/dev/null || break
      i=$((i + 1))
    done
  }

  for fam in iptables ip6tables; do
    command -v "$fam" >/dev/null 2>&1 || continue
    delete_until_gone "$fam" -t nat -D OUTPUT -p tcp --dport 53 -j REDIRECT --to-port "$DNS_PORT"
    delete_until_gone "$fam" -t nat -D OUTPUT -p udp --dport 53 -j REDIRECT --to-port "$DNS_PORT"
    delete_until_gone "$fam" -t nat -D OUTPUT -p tcp --dport 53 -m mark --mark "$MARK_HEX" -j RETURN
    delete_until_gone "$fam" -t nat -D OUTPUT -p udp --dport 53 -m mark --mark "$MARK_HEX" -j RETURN
  done

  # Per-exempt-dst RETURN rules (OPENSANDBOX_EGRESS_NAMESERVER_EXEMPT is a
  # comma-separated IP list; matches dnsproxy.ParseNameserverExemptList).
  exempt="${OPENSANDBOX_EGRESS_NAMESERVER_EXEMPT:-}"
  if [ -n "$exempt" ]; then
    OLD_IFS="$IFS"
    IFS=','
    for d in $exempt; do
      d=$(printf '%s' "$d" | tr -d ' ')
      [ -z "$d" ] && continue
      case "$d" in
        *:*) fam=ip6tables ;;
        *)   fam=iptables ;;
      esac
      command -v "$fam" >/dev/null 2>&1 || continue
      delete_until_gone "$fam" -t nat -D OUTPUT -p tcp --dport 53 -d "$d" -j RETURN
      delete_until_gone "$fam" -t nat -D OUTPUT -p udp --dport 53 -d "$d" -j RETURN
    done
    IFS="$OLD_IFS"
  fi

  log "iptables DNS redirect rules removed (best-effort)"
}

# ─── iptables transparent HTTP (pkg/iptables/transparent.go) ─────────
remove_transparent_http() {
  enabled="${OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT:-}"
  case "$enabled" in
    1|true|TRUE|True|yes|YES|Yes|y|Y|on|ON|On) ;;
    *) return 0 ;;
  esac
  command -v iptables >/dev/null 2>&1 || return 0

  MITM_PORT="${OPENSANDBOX_EGRESS_MITMPROXY_PORT:-18081}"
  MITM_UID="${OPENSANDBOX_EGRESS_MITMPROXY_UID:-10042}"

  delete_until_gone() {
    i=0
    while [ $i -lt 32 ]; do
      "$@" 2>/dev/null || break
      i=$((i + 1))
    done
  }

  delete_until_gone iptables -t nat -D OUTPUT -p tcp \
    -m owner ! --uid-owner "$MITM_UID" \
    -m multiport --dports 80,443 \
    -j REDIRECT --to-ports "$MITM_PORT"
  delete_until_gone iptables -t nat -D OUTPUT -p tcp -d 127.0.0.0/8 -j RETURN

  log "iptables transparent HTTP rules removed (best-effort)"
}

# ─── nftables table `opensandbox` (pkg/nftables/manager.go:31) ───────
remove_nft_table() {
  command -v nft >/dev/null 2>&1 || { log "nft not present; skipping nftables cleanup"; return 0; }
  # `delete table` is atomic: either it exists and is removed, or returns
  # non-zero (which we swallow). Family is `inet` (matches manager.go).
  try nft delete table inet opensandbox
  log "nftables 'opensandbox' table removed (best-effort)"
}

# ─── stray mitmdump (orphaned after hard crash) ──────────────────────
kill_stray_mitmdump() {
  command -v pkill >/dev/null 2>&1 || { log "pkill not present; skipping mitmdump reap"; return 0; }
  # mitmdump runs as the `mitmproxy` user (uid 10042 per egress Dockerfile).
  # SIGTERM first; give it a moment; SIGKILL anything that ignored TERM.
  try pkill -TERM -u mitmproxy -f mitmdump
  # Short sleep, but bounded so this hook still finishes inside the
  # supervisor's PostExitTimeout (default 30s) with headroom.
  sleep 1
  try pkill -KILL -u mitmproxy -f mitmdump
  log "stray mitmdump processes reaped (best-effort)"
}

main() {
  log "starting (worker_exit_code=${WORKER_EXIT_CODE:-?} signal=${WORKER_SIGNAL:-?} attempt=${WORKER_ATTEMPT:-?})"
  remove_dns_redirect
  remove_transparent_http
  remove_nft_table
  kill_stray_mitmdump
  log "done"
  exit 0
}

# Trap unexpected interpreter errors so we still exit 0.
trap 'log "cleanup hit shell error on line $LINENO; exiting 0 anyway"; exit 0' HUP INT TERM
main "$@" || true
exit 0
