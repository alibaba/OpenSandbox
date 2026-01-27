# Egress Sidecar TODO (Linux MVP → Full OSEP-0001)

## Gaps vs OSEP-0001
- No Layer 2 yet: no nftables full isolation, no DoH/DoT blocking, no IP/CIDR rules.
- Policy surface is minimal: only domain allow + defaultAction; missing deny rules, IP/CIDR, `require_full_isolation`.
- Observability missing: no enforcement mode/status exposure, no violation logs.
- Capability probing missing: no CAP_NET_ADMIN/nftables detection; no hostNetwork rejection.
- Platform integration missing: server/SDK/spec not updated; sidecar not wired into server flow.
- No IPv6; startup ordering not enforced (relies on container start order).

## Short-term priorities (suggested order)
1) Capability probing & mode exposure  
   - Detect CAP_NET_ADMIN and nftables; set `dns-only` vs `dns+nftables`; surface in logs/status.  
   - Fast-fail on hostNetwork.
2) Layer 2 via nftables  
   - Allow-set + default DROP; add DNS-learned IPs dynamically.  
   - Static IP/CIDR rules; block DoH/DoT ports.
3) Policy expansion  
   - Support deny rules, IP/CIDR, `require_full_isolation`.  
   - Validation and clear errors.
4) Observability & logging  
   - Violation logs (domain/action/upstream IP); expose current enforcement mode.  
   - Optional lightweight health/status endpoint.
5) Platform & SDK alignment  
   - Update `specs/sandbox-lifecycle.yml`; add `network_policy` to Python/Kotlin SDKs.  
   - Server (Docker/K8s) integrates sidecar injection; NET_ADMIN only on sidecar.
6) Security hardening  
   - Whitelist/validate upstream DNS to avoid arbitrary 53 egress abuse.  
   - Document bypass/limits (dns-only can be bypassed via direct IP/DoH).
7) IPv6 & tests  
   - Handle IPv6 support or explicit non-support.  
   - Unit/integration tests: interception, graceful degrade, nftables, DoH blocking, hostNetwork rejection.

## Dev notes
- Current behavior: SO_MARK=0x1 bypass for proxy’s own upstream DNS; iptables only redirects port 53, no other DROP rules.  
- Runtime deps: Linux, `CAP_NET_ADMIN`, `iptables` binary; upstream DNS must be reachable and recursive.

