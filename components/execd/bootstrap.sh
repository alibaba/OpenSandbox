#!/bin/sh

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

set -e

_forward_signal() {
	sig="$1"
	pid="$2"
	kill "-$sig" "$pid" 2>/dev/null || true
	wait "$pid" 2>/dev/null || true
	exit 0
}

# Returns 0 if the value looks like a boolean "true" (1, true, yes, on).
is_truthy() {
	case "$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]')" in
	1 | true | yes | on) return 0 ;;
	*) return 1 ;;
	esac
}

_sudo() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
	elif command -v sudo >/dev/null 2>&1; then
		sudo -n "$@"
	else
		"$@"
	fi
}

# Install mitm egress CA into the system trust store (no extra env vars).
# - Debian/Ubuntu/Alpine: update-ca-certificates + /usr/local/share/ca-certificates/
# - RHEL/CentOS/Fedora/Alma/Rocky: update-ca-trust + /etc/pki/ca-trust/source/anchors/
trust_mitm_ca() {
	cert="$1"
	if command -v update-ca-certificates >/dev/null 2>&1; then
		_sudo mkdir -p /usr/local/share/ca-certificates
		_sudo cp "$cert" /usr/local/share/ca-certificates/opensandbox-mitmproxy-ca.crt
		_sudo update-ca-certificates
		return 0
	fi
	if command -v update-ca-trust >/dev/null 2>&1; then
		_sudo mkdir -p /etc/pki/ca-trust/source/anchors
		_sudo cp "$cert" /etc/pki/ca-trust/source/anchors/opensandbox-mitmproxy-ca.pem
		if ! _sudo update-ca-trust extract; then
			_sudo update-ca-trust
		fi
		return 0
	fi

	echo "warning: cannot install mitm CA (need update-ca-certificates or update-ca-trust)" >&2
	return 0
}

# Chromium/Chrome on Linux do not use only the system trust store: they also honor the per-user
# NSS database at $HOME/.pki/nssdb. Import the same mitm CA there so the browser trusts it.
# Requires certutil (e.g. Alpine: nss-tools, Debian/Ubuntu: libnss3-tools).
trust_mitm_ca_nss() {
	cert="$1"
	[ -f "$cert" ] || return 0
	[ -n "${HOME:-}" ] && [ -d "$HOME" ] || return 0
	if ! command -v certutil >/dev/null 2>&1; then
		return 0
	fi
	pki="${HOME}/.pki/nssdb"
	if ! mkdir -p "$pki" 2>/dev/null; then
		return 0
	fi
	if [ -f "$pki/cert9.db" ]; then
		nssdb="sql:$pki"
	elif [ -f "$pki/cert8.db" ]; then
		nssdb="dbm:$pki"
	else
		nssdb="sql:$pki"
		if ! certutil -N -d "$nssdb" --empty-password 2>/dev/null; then
			[ -f "$pki/cert9.db" ] || return 0
		fi
	fi
	nick="opensandbox-mitmproxy"
	certutil -D -d "$nssdb" -n "$nick" 2>/dev/null || true
	if ! certutil -A -d "$nssdb" -n "$nick" -t "C,," -i "$cert"; then
		echo "warning: failed to import mitm CA into NSS at $pki (Chrome may still distrust); need certutil" >&2
		return 0
	fi
	return 0
}

MITM_CA="/opt/opensandbox/mitmproxy-ca-cert.pem"
if is_truthy "${OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT:-}"; then
	i=0
	while [ "$i" -lt 30 ]; do
		if [ -f "$MITM_CA" ] && [ -s "$MITM_CA" ]; then
			break
		fi
		sleep 1
		i=$((i + 1))
	done
	if [ ! -f "$MITM_CA" ] || [ ! -s "$MITM_CA" ]; then
		echo "warning: timed out after 30s waiting for $MITM_CA (egress mitm CA export); continuing without system CA trust" >&2
	elif ! trust_mitm_ca "$MITM_CA"; then
		echo "warning: failed to install mitm CA into system trust store; TLS interception may not work for system libraries" >&2
	fi

	if [ -f "$MITM_CA" ] && [ -s "$MITM_CA" ]; then
		trust_mitm_ca_nss "$MITM_CA" || true
		export NODE_EXTRA_CA_CERTS="$MITM_CA"
		export REQUESTS_CA_BUNDLE="$MITM_CA"
	fi
fi

EXECD="${EXECD:=/opt/opensandbox/execd}"

if [ -z "${EXECD_ENVS:-}" ]; then
	EXECD_ENVS="/opt/opensandbox/.env"
fi
if ! mkdir -p "$(dirname "$EXECD_ENVS")" 2>/dev/null; then
	echo "warning: failed to create dir for EXECD_ENVS=$EXECD_ENVS" >&2
fi
if ! touch "$EXECD_ENVS" 2>/dev/null; then
	echo "warning: failed to touch EXECD_ENVS=$EXECD_ENVS" >&2
fi
export EXECD_ENVS

echo "starting OpenSandbox Execd daemon at $EXECD."
$EXECD &

# Allow chained shell commands (e.g., /test1.sh && /test2.sh)
# Usage:
#   bootstrap.sh -c "/test1.sh && /test2.sh"
# Or set BOOTSTRAP_CMD="/test1.sh && /test2.sh"
CMD=""
if [ "${BOOTSTRAP_CMD:-}" != "" ]; then
	CMD="$BOOTSTRAP_CMD"
elif [ $# -ge 1 ] && [ "$1" = "-c" ]; then
	shift
	CMD="$*"
fi

SHELL_BIN="${BOOTSTRAP_SHELL:-}"
if [ -z "$SHELL_BIN" ]; then
	if command -v bash >/dev/null 2>&1; then
		SHELL_BIN="$(command -v bash)"
	elif command -v sh >/dev/null 2>&1; then
		SHELL_BIN="$(command -v sh)"
	else
		echo "error: neither bash nor sh found in PATH" >&2
		exit 1
	fi
fi

if [ "$CMD" != "" ]; then
	"$SHELL_BIN" -c "$CMD" &
	CMD_PID=$!
elif [ $# -eq 0 ]; then
	"$SHELL_BIN" &
	CMD_PID=$!
else
	"$@" &
	CMD_PID=$!
fi

trap '_forward_signal TERM "$CMD_PID"' TERM

wait "$CMD_PID" 2>/dev/null
exit $?
