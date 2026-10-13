---
title: Egress
description: Per-sandbox outbound control — FQDN allowlists enforced at the DNS and network layers, transparent credential injection, and fail-closed startup.
---

# Egress

Egress is how a sandbox gets a network policy instead of open internet access. It runs as a sidecar sharing the sandbox's network namespace, so every outbound packet passes it — no application configuration, and no way around it from inside the sandbox. Policy is declared at create time (`networkPolicy`) or by the platform, and enforced continuously.

![Egress control flow](../../public/images/egress-control-flow.svg)

## Capabilities at a glance

| Capability | What you get |
|---|---|
| FQDN rules | Allow or deny by domain, wildcard (`*.pypi.org`), IP, or CIDR |
| DNS filtering (default) | Denied domains fail resolution; allowed ones resolve normally |
| Network enforcement (`dns+nft`) | Strict default-deny at the packet layer, fed by DNS-resolved addresses with bounded leases |
| Platform overlays | Operator-set `deny.always` / `allow.always` floors that user policy cannot override |
| Credential Vault | Secrets stay in the sidecar; credentials are injected into matching outbound requests |
| Transparent TLS interception (L7) | See, control, and authenticate outbound HTTPS — production-usable mechanism, experimental configuration surface |
| Fail-closed startup | If enforcement cannot be installed, the sidecar exits — it never runs as a no-op |

## How interception works

**Layer 1 — DNS.** All port-53 traffic is redirected into the sidecar's DNS proxy. A denied domain answers `NXDOMAIN`; an allowed domain resolves through to the real upstream, and its resolved addresses are recorded.

**Layer 2 — Network (`dns+nft` mode).** nftables drops everything not explicitly allowed. Allowed traffic passes by static rule or by DNS-learned address sets: a resolved IP receives a bounded lease (with a grace window for active TCP connections), so "allowed" never silently becomes "allowed forever". UDP and QUIC flows rely on DNS lease timing alone.

**Precedence.** First matching rule wins, in this order: platform `deny.always`, platform `allow.always`, then your policy — so the platform's deny always beats your allow. The overlays live in files inside the sidecar image and hot-reload every minute. A reload publishes the parsed pair only after the corresponding nftables static policy is accepted; the experimental revision runtime additionally requires exact receiver confirmation. Parse or apply failures keep the last accepted in-memory rules. Failures remain eligible for retry unless the experimental revision runtime requires recovery, as described below. Your policy is set per sandbox and can be mutated at runtime (add, replace, remove by target) through the API the SDKs expose.

## Design decisions

**Enforcement lives in the data path, not beside it.** The sidecar shares the sandbox's network namespace and installs redirect rules there, so every packet passes the policy engine without application cooperation — and without the option to bypass it, because the workload holds no `NET_ADMIN` in that namespace.

**Two layers, two costs.** DNS filtering alone is cheap and universal, but it only gates names: an application that obtains an address by other means is not covered. The `dns+nft` layer adds packet-level truth — default-deny with DNS-learned exceptions that carry bounded leases (renewed while a TCP connection is active), so "allowed" never quietly becomes "allowed forever".

**Encrypted DNS is treated as an escape hatch.** Filtering plaintext DNS is void if the workload can resolve over HTTPS: DoT (853) is therefore always dropped, and blocking DoH over 443 is a one-switch option.

**Fail-closed over best-effort.** If the redirect rules cannot be installed, the sidecar exits instead of running as a silent pass-through; a supervisor restarts it with backoff and a crash-loop breaker. "No enforcement" must never look like enforcement.

**Portable by fallback, honest about limits.** Redirects install through iptables where available and fall back to native nftables on kernels lacking the required extensions (Firecracker-style guest kernels). gVisor's netstack cannot support this mechanism — Kata is the supported secure runtime — and transparent service-mesh sidecars conflict by construction, since both rewrite the same namespace's outbound traffic. These boundaries are explicit, not best-effort.

## Transparent TLS interception (L7)

Layers 1 and 2 decide *whether* a connection may happen; the transparent MITM layer adds control and visibility over *what goes through it*. When enabled, outbound HTTP and HTTPS is redirected into a mitmproxy listener inside the sidecar, which recovers the true destination from connection state and terminates TLS on the sandbox's behalf.

Status: **experimental but production-usable**. The interception, credential-injection, and CA-delivery mechanism is complete and runs in production settings; what remains experimental is the configuration surface — extra ports and diagnostic switches may still change between releases.

::: warning Internal OSEP-0023 revision runtime
`OPENSANDBOX_EGRESS_EXPERIMENTAL_REVISION_RUNTIME=true` is a development-only,
sidecar-profile gate for the OSEP-0023 revision protocol. The default is off.
Each mitmdump launch or child restart receives a fresh authenticated process
session and must acknowledge the current in-memory Vault snapshot (or the
authoritative initial empty state) with its committed Policy epoch before
health becomes ready.

The live receiver jointly installs the immutable snapshot and its
credential-free TLS selector view for connection and request admission. The
internal sidecar experiment supports bound HTTPS/443 HTTP/1.1 traffic; it does
not expose a supported public `interceptionMode` or enable Fast Sandbox or
HTTP/2 support. See the [live admission boundary](/guides/credential-vault#experimental-live-credential-bound-admission-osep-0023).

Vault writes, existing `POST`/`PUT`/`PATCH`/`DELETE /policy` handlers and automatic
always-rule reloads share the Policy/Vault mutation barrier and an exclusive
live-session lease. Policy candidates validate all active bindings before any
external effect and preserve the public Vault revision and already-rendered
credentials. The Policy commit order is optional user-policy file persistence,
nft application, exact receiver acknowledgement or reconciled readback, then
Go proxy/base publication and API success. Reload skips user-policy persistence;
the loader's accepted rules and metadata publish only after receiver confirmation.
Failed reloads never rewrite operator-owned always files.

These stages are not one atomic visibility boundary. In particular, nft rules
can affect traffic before receiver acknowledgement. Once nft succeeded, even a
definitive receiver rejection requires sticky recovery, quarantine and cleanup
of the exact child/session. Reapplying the old nft policy is not proof that
connections or requests created under the new rules were undone. A confirmed
receiver update is finalized locally even if the HTTP request is then canceled;
a lost API response does not roll back a committed update.

Unavailable sessions, unconfirmed receiver outcomes and recovery-blocked writes
return sanitized 503 responses. Existing initial file/nft failures may remain
500; input validation stays 400 and the rule limit stays 413. A no-match DELETE
or unchanged reload does not advance the internal Policy epoch, but cannot
bypass recovery checks. Other committed writes may advance the epoch even when
the content is equal. Later Vault mutations and bootstrap preserve that epoch.
No epoch field, idempotency key or SDK method is added. `GET /policy` remains the
last Go-published user policy, without always overlays, kernel state or an
in-progress transaction indicator.

Receiver acknowledgement covers publication and registration of old-request
deadlines, not waiting for drain completion. The default is 30 seconds; later
updates cannot extend the earliest old-request deadline. Expiry closes the
whole tracked connection, potentially interrupting newer requests on it, while
still-covered idle connections with no old requests remain usable. HTTP 101
upgrades retain request admission and also drain on Policy-only changes. Actual
closure follows the sweep and event loop, with failed closes retried. This does
not promise termination of opaque or direct-IP established connections.

The experimental lifecycle also keeps an authoritative policy base and a
private bootstrap ticket under the shared policy/Vault barrier. Initial startup
and the revision layer's child-restart path use the same final check: the captured policy base,
Store and Vault mutation identity must still be current, the exact child/session
must still belong to the pending attempt, and recovery must not be required.
Snapshot installation or listener availability alone cannot make health ready.
Policy or always-rule publication, including replacement with identical rules,
and Vault changes invalidate an older capture. A rejected attempt stops and reaps
only its own child before closing its session. A rejected launch can capture
fresh state only while recovery has not been required. Initial startup also
retries a stale publication with a fresh
capture, up to three total launch attempts, so an immediate always-rule reload
can settle without forcing the sidecar to exit. Redirect and CA preparation run
once. Other startup errors, cancellation, shutdown, and recovery-required state
stop startup; exhausting the stale-attempt limit leaves health not-ready.

If experimental policy processing attempts a policy-file or nft update and then
fails with an uncertain outcome, or session cleanup cannot be confirmed, the Go
owner latches `recovery-required`. Health stays not-ready, and internal candidate
preparation, Vault mutation ownership and new bootstrap publication are blocked.
Parsing or validation errors before external effects do not themselves set this
latch. Shutdown and teardown remain available subject to the fault-containment
boundary below. The first
recovery reason remains sticky for that Go process incarnation; successful
readback, listener checks and ordinary child restarts cannot clear it. There is
no reset API in this increment.

When this gate is enabled and `OPENSANDBOX_EGRESS_POLICY_FILE` is configured,
policy persistence requires an **egress-owned writable directory mount**. Mount
the directory containing the policy, rather than binding the policy file itself.
The directory must be owned by the egress process's effective UID, owner-writable
and searchable, and not group- or world-writable. Only that egress instance may
write the directory or policy file. Ancestors must be root- or effective-UID-owned
and protected from other writers (shared sticky ancestors such as `/tmp` are
permitted). The parent directory must already exist; egress does not create it,
change host permissions, or migrate existing mounts.

The experimental store requires Linux mount identities from `statx`, or from
`/proc/self/fdinfo` when `statx` mount IDs are unavailable. It rejects single-file bind mounts,
read-only projections, symlinks in the path, non-regular files and hard-linked
files before replacing the original. Unsupported directory/file layouts detected by validation fail startup and are
revalidated for each update. Construction and the pre-update snapshot probe
directory synchronization before replacing the policy file; a later I/O failure
can still require recovery after rename.

Kernel-managed `security.selinux` labels, mutable IMA digest attributes
(`security.ima` digest/digest-ng formats), and EVM HMAC attributes (`security.evm`)
are permitted on existing and newly created files. Egress never copies these
attributes: the kernel assigns or regenerates them for each replacement inode.
For an existing file, the replacement's SELinux label must exactly match the
snapshot label, and any previously present IMA digest/EVM HMAC must be present
again before rename. Signature formats, unknown integrity formats, user xattrs,
file capabilities, other unknown attributes, and file/parent-directory ACLs are
rejected before replacement. Attribute format checks classify this storage
contract; they do not authenticate digests/HMACs or replace kernel appraisal.

Use a dedicated directory whose directory/process SELinux creation policy gives
all randomized sibling temporary files the intended policy label. For an
initially absent policy, that kernel-assigned temporary-inode label becomes the
policy label. Egress does not look up labels for the final basename, run
`restorecon`, or implement filename-specific transitions. A pre-existing custom
or filename-dependent label that differs from the temporary inode is rejected;
Egress never silently relabels it. IMA/EVM deployments must allow the kernel to
regenerate mutable integrity attributes for randomized sibling files under the
same appraisal rules as the final policy path; policies requiring externally
signed replacement files are unsupported. No host security policy is changed.

The operator must use a filesystem that supports
same-directory rename and file/directory synchronization. For example, mount a
prepared host directory at `/var/egress/policy` and configure the file path as
`/var/egress/policy/policy.json`. Keep platform `allow.always`/`deny.always` inputs
outside this exclusively written directory.

An update writes a new temporary inode in that same directory, preserves the
existing file's owner and permission bits (new files use `0600`), synchronizes and
closes the writer, then reopens the same inode read-only with no-follow and
identity checks. This allows last-writer-close IMA updates to finish before final
attribute inspection and another file sync. Only then does it rename over the
policy and synchronize the directory. A known
failure before rename does not write or replace the original: no restore or nft apply is
attempted, and that failure alone does not require recovery or freeze writers.
Once rename is attempted, a failure is conservatively treated as an uncertain
outcome. The owner enters sticky recovery before best-effort restoration. Restore
uses atomic replacement of the exact original bytes, owner and permission bits
with the snapshot SELinux label check. Integrity attributes are regenerated, not
restored byte-for-byte. It uses synchronized
removal when the file did not previously exist. Even successful restoration does
not clear recovery. Temporary files are cleaned up on ordinary error returns;
a process crash may leave a temporary file for operator inspection.

This protocol does not make policy files and nft state one crash-consistent
transaction. The policy-file primitive provides no durable recovery journal, kernel rollback or packet
fence. Existing open file descriptors continue to see the previous inode; readers
must reopen the policy path to observe replacements. Default deployments with the
experimental gate off, including DNS-only mode, retain legacy in-place persistence.
Fast Sandbox does not use this sidecar policy-file store.

Static nft applies classify their kernel effect as `Unchanged`, `Committed`, or
`Unknown`. Local ruleset construction errors, cancellation before starting nft,
and process-start failures are `Unchanged`: no kernel operation was executed.
A successful command is `Committed`. After the process starts, timeout, signal,
nonzero exit, wait or output-copy errors are `Unknown`, as are errors from a
runner without a trusted classification. Diagnostic text and exit status cannot
prove that the kernel stayed unchanged. The existing single missing-table
fallback remains bounded; if both attempts fail, both diagnostics are retained
and an `Unknown` first attempt cannot be downgraded by an `Unchanged` retry.

An experimental runtime apply that is `Unchanged` preserves the previous policy
base and DNS/TCP tracking without adding recovery or freezing a healthy Manager.
When the policy file was already saved, this requires successful durable restore
of its exact previous bytes, metadata, or absence. Any restore failure requires
recovery, including a failure classified as leaving the *new* file unchanged.
Without a configured policy file, the old base remains retryable directly.
`Unknown` requires recovery before best-effort file restore; successful restore
cannot clear that latch. Failed always-rule reloads keep the previous loader,
proxy and policy base. Bootstrap invalidation and existing recovery or quiescence
are never reversed. Initial enforcement failure still terminates startup,
including an `Unchanged` failure.

In the ordinary sidecar's experimental revision runtime, entering recovery also
synchronously freezes runtime writes in the same nft Manager. A terminal static
apply error with an `Unknown` effect freezes that Manager before releasing its
lock, starting with the first startup apply; an `Unchanged` error or successful
missing-table fallback does not add a freeze. Uncertain policy-file persistence,
failed restoration after a saved file, and session-cleanup failures freeze
it through the recovery owner. The owner publishes the health and bootstrap
restrictions before waiting for an already admitted nft writer, so `/healthz`
can return 503 while that writer drains. The write-admission boundary is the
atomic closure of admission, before waiting for the Manager lock. Queued and
new writers check that gate after acquiring the lock and cannot enter while
recovery waits. An operation admitted before the gate closes may finish its nft
update and associated state publication; the freeze call returns only after
that operation has drained.

The freeze covers six runtime paths: static policy replacement (`ApplyStatic`),
direct dynamic IP additions (`AddResolvedIPs`), DNS answer
publication (`AddResolvedDomain`), late domain-refresh results
(`applyDomainRefresh`), active TCP lease renewal, and upstream proxy address
updates (`AddUpstreamProxyIPs`). DNS queries already in flight may complete,
but their results cannot write rules or republish old authorization state.
Stopping lease renewal can reduce workload availability; DNS-learned upstream
proxy addresses can expire and cause later proxy connections to fail. Existing
connections are not necessarily disconnected by the freeze.

There is no unfreeze operation. The Manager API `RemoveEnforcement` can still
delete the nft table without clearing the Manager's frozen state. Legacy
shutdown uses that API; the experimental runtime owner retains enforcement at
shutdown as described below. The Manager freeze alone does not provide a packet
fence: existing rules, leased addresses and established connections may still
allow traffic. The freeze does not survive process restart,
roll back unknown external effects, or add a hard shutdown deadline. Legacy
sidecar and Fast Sandbox behavior are unchanged; dns-only recovery keeps its
existing readiness behavior without requiring an nft Manager.

Kernel write-admission validation uses
`TestNftQuiescenceAfterCommittedStaticError` in a separate Linux network
namespace. It commits a new ruleset through the real nft runner before injecting
an error, then checks that all six runtime paths leave the new static policy and
empty dynamic/upstream sets unchanged. It requires `nft`, `unshare`, and
permissions to create a network namespace and operate nftables. The privileged
egress CI explicitly selects it alongside `TestDynamicElementRenewal`,
`TestNftStartFailurePreservesKernelAndRetry`, `TestNftRealMissingTableFallback`,
and the owner-level `TestRevisionNftEffectsRealFileAndKernel` with
`OPENSANDBOX_NFT_TEST=1`. The owner test uses real atomic Save/Restore with both
prior bytes and prior absence, alongside a real production start failure or a
lost result after an actual kernel commit. The dedicated nft runner checks exact
test discovery and each test's RUN/PASS evidence and rejects any SKIP;
missing prerequisites fail enabled tests. Ordinary
Go tests skip kernel validation when that variable is unset, so their success
does not establish kernel behavior or packet isolation.

This state is in memory only. Readiness is not a network-traffic fence, and this
increment provides no atomic policy-file/nft rollback, dynamic DNS-state recovery
or whole-process crash durability. Restarting the entire sidecar loses the latch
and is not a verified safe recovery procedure. Internal Policy epochs advance
only on committed Policy revisions and are retained by Vault writes and
bootstrap. With the gate off, legacy policy success responses retain their
existing meaning.
:::

### Experimental runtime fault containment

A separate fault-containment owner supplements the process-local recovery latch
and nft Manager freeze described above. For the Linux `dns+nft` sidecar, it uses the existing experimental
revision-runtime gate and adds no lifecycle-server wiring, deployment topology
or configuration surface. Normal policy, always-rule and internal Vault updates
do not install a whole-IP fence. The independent fence is attempted through the
recovery-required path, including an unknown external effect. A known
`prepareRejected` or definitive `committed=false` result does not by itself add
permanent isolation when no local effect requires recovery. A Policy update
that has already committed nft rules requires recovery and quarantine even
with that definitive receiver rejection. Successful operations retain their
existing behavior unless recovery was already required.

An unexpected child death enters recovery only when the observation still
matches the exact currently owned, running child generation. Stale, already
detached or normally stopping generations do not trigger a new fence. Normal
shutdown likewise does not add a fence. In this experimental path, shutdown
stops services but retains existing policy and redirect rules, even before a
fault, so a late failure during drain or session cleanup cannot be followed by
enforcement removal. Legacy shutdown cleanup is unchanged.

Containment targets the original network namespace pinned for the current Go
process's lifetime. `QuarantineConfirmed` requires confirmation of that target
and exact kernel readback of the independent `inet opensandbox_quarantine`
table. Failure to validate the target, install the fence or confirm its readback
leaves packet isolation `QuarantineUnknown`. Readiness, writer quiescence and
packet isolation remain separate facts: a 503 response or process exit does not
prove that traffic has stopped.

When confirmed in that target, the fence drops IPv4 and IPv6 at input, output
and forward hooks, including loopback, established and new connections, UDP,
marked traffic, the MITM UID, DNS and upstream-proxy exceptions. This also
interrupts health/control HTTP and execd IP connections. The boundary assumes
no workload NET_ADMIN/NET_RAW, hostile privileged namespace writer, or L2 or
offloaded bypass of the supported inet path. Detection is not instantaneous;
there is no zero-leakage guarantee before detection and fence confirmation.
An installation failure reports unknown isolation and does not guarantee that
the workload is killed or otherwise stopped.

Ordinary policy-table and redirect cleanup do not delete this independent
fence. After a runtime fault, the owner does not automatically remove it or
restore service. A table left in the kernel is not a complete restart protocol.
The namespace pin lasts only for this Go process: this increment makes no
containment or recovery guarantee across Go-process or container restart and
provides no durable intent, replay, pause/resume recovery or safe automatic
restart. Legacy prestart may remove ordinary redirects while leaving a residual
fence, so a restart can fail to become ready; destroy and recreate the sandbox
instead of treating restart as verified recovery. Durable lifecycle
ownership and recovery remain separate future work.

**Trust is delivered, not disabled.** The sidecar exports its CA, and the sandbox bootstrap installs it into the system, NSS, and JDK trust stores on a best-effort basis — clients keep certificate verification on (`curl` without `-k`), and traffic stays encrypted end-to-end from the sandbox's point of view. Images that run Chromium-family browsers should ship the native `certutil` package so the per-user NSS store can be updated.

**What it enables:**

- **Credential Vault** — injection happens when request headers are read, so it applies regardless of body size, including fully streamed uploads.
- **TLS-level observability** — optional request-level outcome metrics (decrypt / passthrough) for targeted diagnostic windows.

**Extendable at L7: custom mitmproxy addons.** The interception layer is not closed. The sidecar always loads its bundled system addon first, then any additional mitmproxy addons listed — comma-separated — in `OPENSANDBOX_EGRESS_MITMPROXY_SCRIPT`, in the order given. Addons are standard mitmproxy scripts with access to the full L7 hook surface: hostname, HTTP method, URI path, headers, and bodies of every intercepted request and response. Operators can therefore build a custom egress image that adds arbitrary L7 policies on top of the built-in FQDN and credential machinery — fine-grained hostname, method, URI, or header filtering, custom header injection, request rewriting — as ordinary Python addons. The system addon keeps its position ahead of user addons, so the built-in behavior is fully in place before any custom logic runs.

**Chained upstream proxy.** Setting `OPENSANDBOX_EGRESS_UPSTREAM_PROXY` to `http://host[:port]` or `https://host[:port]` on the egress container chains all mitmproxy-handled egress through that proxy via `CONNECT`, optionally with `OPENSANDBOX_EGRESS_UPSTREAM_PROXY_AUTH` (a complete `Proxy-Authorization` header value). The endpoint accepts only a host and optional port (or a single `/` path); credentials, query, fragment, and other paths are rejected, and validation errors do not include the supplied URL. The host must be a literal IP or a dotted domain name — a dotless name expands differently through the pod resolver's DNS search list than through the egress's direct query, so the containment sets could miss the address actually dialed. The `CONNECT` authority is the SNI/Host-derived FQDN when known. This is fail closed: pass-through flows that cannot be chained (no-SNI, `ignore_hosts`/`tcp_hosts`/`udp_hosts` matches, UDP) are refused rather than sent direct. Chaining requires transparent MITM; outside the fast-sandbox profile it also requires `dns+nft` enforcement, and invalid combinations fail startup before the proxy hostname is registered for infrastructure DNS. Under `dns+nft`, the proxy endpoint is reachable only from the mitmproxy process (UID-, IP-, and port-scoped nft rules), never through the sandbox allow sets; hostname endpoints resolve through an infrastructure DNS path that bypasses sandbox policy evaluation. Literal proxy IPs remain permanently seeded in those scoped sets; DNS-learned addresses use explicit bounded TTLs. On Docker and Kubernetes, administrators configure it via the server's `[egress.upstream_proxy]` section — see [server configuration](https://github.com/opensandbox-group/OpenSandbox/blob/main/server/configuration.md#egress). The same section can deliver an extra CA bundle (a Docker host PEM path or a Kubernetes Secret mounted read-only into the egress sidecar only) that mitmproxy trusts in addition to — never instead of — the system roots, for every upstream TLS verification. For configuration, runtime, rotation, and troubleshooting procedures, see [Chained Upstream Proxy Operations](/guides/egress-upstream-proxy).

Under the fast-sandbox profile the same chaining applies to the shared mitmproxy, with containment adapted to the source-IP enforcement model: the proxy endpoint is dropped profile-wide in the shared dispatch chain (forward and input paths, ahead of the per-subject rules), so no subject policy — default-allow included — can CONNECT the proxy directly, while the locally generated mitmproxy dial is unaffected. A hostname endpoint is registered as an infrastructure domain on the shared DNS proxy so its answers never feed the dynamic allow sets; the shared mitmdump resolves the name through the fastlet pod's own resolver, so it must be resolvable via cluster DNS. Because the shared DNS proxy's forward upstreams and the pod resolver can return different address sets (split-horizon or operator-configured DNS), the egress queries both authorities concurrently and seeds the containment drop sets with the union. Drop elements are permanent (no kernel timeout): containment persists through egress downtime. An address is pruned only after it is absent from two successful refreshes of both authorities; a sandbox DNS lookup renews its retention. Partial resolver failures are logged and any returned addresses are added without pruning existing elements. Startup resolves and seeds the in-memory addresses with bounded retries **before resetting the nft table**. Both authorities must complete successfully and their union must be nonempty; otherwise startup fails and the previous kernel table is left intact. The reset installs the seeded addresses atomically, and nft updates have a separate timeout from DNS resolution. On Docker and Kubernetes sidecar deployments administrators configure the server's `[egress.upstream_proxy]` section; fast-sandbox deployments set the two environment variables on the fastlet pod's egress container directly.

**Scope notes:**

- Ports 80 and 443 are always intercepted; a bounded list of extra ports can be added. Vault binding matching currently fires on 80/443 only — extra ports are decrypted and logged but not credentialed.
- IPv6 destinations are intercepted alongside IPv4 (listeners and redirect rules cover both address families).
- A not-yet-initialized proxy is visible in health: the sidecar answers `503 mitmproxy not ready` rather than passing traffic unexamined.
- One known upstream behavior: an SSE body larger than ~1 MB can be truncated when the origin closes the connection with a TCP RST mid-body — standard TCP semantics, not an interception bug. See [Egress SSE Truncation](/guides/egress-sse-truncation).

## Credentials without secrets in the sandbox

With transparent TLS interception enabled, the sidecar can inject credentials into outbound requests that match a vault binding — bearer tokens, basic auth, API keys, or custom headers. The vault lives in the sidecar's memory only: the sandbox workload sees authenticated traffic but never holds the secret itself. See [Credential Vault](/guides/credential-vault).

## Where it runs

| Deployment | Shape |
|---|---|
| Docker | A separate sidecar container; the sandbox joins the sidecar's network namespace |
| Kubernetes | The sidecar is appended to the sandbox pod and holds the pod's only `NET_ADMIN` |
| Pooled sandboxes | Pre-warmed pods cannot gain a sidecar per request — put required controls in the Pool template (per-request `networkPolicy` + pool references are rejected) |
| Fast Sandbox | A shared egress process per Fastlet pod, one subject per sandbox — see [Fast Sandbox: Networking](/architecture/fast-sandbox/networking) |

## Service Mesh Compatibility

::: warning Not supported with transparent mesh sidecars
Egress is designed to be the only transparent outbound interception layer in the sandbox's network namespace. Deployments that inject a service-mesh sidecar (Istio/Envoy and similar) into the same pod are not currently supported for egress features.
:::

Both layers rewrite outbound traffic in the same namespace, so per-sandbox policy, transparent MITM, and Credential Vault cannot be relied on alongside mesh injection. Prefer excluding sandbox pods from mesh injection, or enforce outbound policy with a CNI-level mechanism instead. The same constraint applies to gVisor: its netstack does not implement the redirect mechanism egress requires — use a Kata runtime, which provides comparable isolation and full egress support (see the [compatibility matrix](/guides/secure-container#compatibility-matrix)).

## Shutdown

On shutdown, the sidecar keeps DNS and network rules working while in-flight deliveries finish (a bounded window), then removes its network rules and flushes telemetry in the legacy path. The experimental runtime-containment owner instead retains ordinary policy/redirect rules at shutdown, without adding a normal-shutdown fence. Delivery is best effort — forced termination can drop events. Under Docker, deletion gives the sidecar a 9-second stop budget before forced termination. A lightweight supervisor restarts the sidecar on crash with exponential backoff and a crash-loop breaker, and cleans stale redirect state before a fresh start.

## Observability (OpenTelemetry)

When the platform configures an OTLP endpoint, the sidecar exports per-sandbox metrics: denied request counts, DNS query outcomes (denied-by-policy and resolver-failure are deliberately separate counters — alerting on the wrong one inverts the diagnosis), nftables update failures, and process resource usage. Cardinality is bounded: no queried domain or destination ever becomes a label. See [telemetry](/guides/telemetry#egress-sidecar) for the metric list and export configuration.
