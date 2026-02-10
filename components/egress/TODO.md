# Egress Sidecar TODO (Linux MVP → Full OSEP-0001)

- Layer 2 still partial: static IP/CIDR now pushed to nftables, DoH/DoT blocking added (853 + optional 443 blocklist). DNS-learned IPs/dynamic isolation intentionally NOT targeted (see No goals).
- Policy surface: IP/CIDR parsing/validation done; `require_full_isolation` and richer validation messages are out of scope (see No goals). Dynamic IP learn/apply is out of scope (see No goals).
- Observability missing: no violation logs.
- Capability probing missing: no CAP_NET_ADMIN/nftables detection; hostNetwork 已由 server 侧阻断。 Capability detection + mode exposure moved to No goals.
- Platform integration completed: specs/SDK/server wiring done; NET_ADMIN only on sidecar.
- No IPv6; startup ordering not enforced (relies on container start order).

## Short-term priorities (suggested order)
1) Layer 2 via nftables  
   - Tune DoH/DoT rules (ordering, allow-list exceptions, counters).
4) Observability & logging  
   - Violation logs (domain/action/upstream IP); expose current enforcement mode.  
   - Optional lightweight health/status endpoint.
6) Security hardening  
   - Whitelist/validate upstream DNS to avoid arbitrary 53 egress abuse.  
   - Document bypass/limits (dns-only can be bypassed via direct IP/DoH).
7) IPv6 & tests  
   - Handle IPv6 support or explicit non-support.  
   - Unit/integration tests: interception, graceful degrade, nftables, DoH blocking, hostNetwork rejection.
8) Dynamic IP insertion from DNS answers (TTL-learned) and dynamic L2 isolation.

## No goals (explicitly excluded)
- Capability probing & mode exposure (CAP_NET_ADMIN/nft detection, mode surfacing).
- Policy expansion: `require_full_isolation` and richer validation errors.

## Dev notes
- Current behavior: default deny-all baseline even when no policy is provided; POST /policy empty resets to deny-all; env bootstrap defaults to deny-all.  
- DNS proxy always runs; SO_MARK=0x1 bypass for proxy’s own upstream DNS; iptables only redirects port 53, no other DROP rules.  
- nftables: static IP/CIDR applied on start and policy update; retry without delete-table if table absent; failures fall back to DNS-only.  
- Runtime deps: Linux, `CAP_NET_ADMIN`, `iptables`/`nft` binaries; upstream DNS must be reachable and recursive.

