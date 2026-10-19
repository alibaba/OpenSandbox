---
title: Experimental HTTP/2 admission and drain
description: OSEP-0023 HTTP/2 admission, connection retirement, failure containment, and current verification limits.
---

# Experimental HTTP/2 admission and drain

::: warning Experimental implementation
This is an internal, default-off extension of the [OSEP-0023 revision runtime](/architecture/network/egress#transparent-tls-interception-l7). Client-migration acceptance and the full runtime verification matrix remain incomplete. It adds no supported public configuration surface or Fast Sandbox support. HTTP/3, CONNECT and cross-SNI coalescing are outside its scope.
:::

## Compatibility boundary

The adapter supports CPython 3.10 and 3.11 with the shipped direct/lazy connection strategy and exactly `mitmproxy==11.0.2`, `h2==4.1.0` and `hyperframe==6.0.1`. The image pins these dependencies and runs the adapter's version check during its build; missing or mismatched versions fail closed at runtime. Changing the pins requires compatibility verification of the adapter, rather than widening the gate alone. Other interpreters and Python versions, including Python 3.12 and later, are rejected before H2 adapter installation and request admission. The TLS slot, already acquired before ALPN selection, is reclaimed by normal disconnect. Their task scheduling and transport-registration ordering are not covered by this compatibility boundary. Configured chained upstream proxies are rejected for this H2 path because their tunnel and transport ownership is not covered. Existing H1 and legacy behavior is unchanged. Genuine untracked, non-TLS cleartext HTTP/2 remains outside credential admission; rewriting the visible scheme of tracked or decrypted HTTP/2 does not bypass raw-identity validation.

## Admission and identity

The existing `LiveReceiver` → `RevisionPublisher` → registry/receiver path remains the only publication owner, with Registry → Receiver lock order and unchanged authenticated IPC.

A request is admitted when the system `requestheaders` hook obtains its unique handle and immutable snapshot, not when HEADERS are parsed. The registry records the actual inbound client stream ID and a monotonic connection high-water mark. The first host removal freezes that bound, including zero if nothing was admitted. Finished streams, repeated publication and remove/re-add cannot reopen or raise it.

Before credential injection, the adapter validates the original pseudo-headers: HTTPS, one ASCII authority on port 443, actual destination port 443, an optional consistent single Host, and agreement with connection/token SNI. Rewritten scheme or host cannot substitute for original identity. Missing, malformed or unassociated identity fails closed.

## Retirement and drain

1. Before mutation acknowledgement, publication atomically installs the snapshot, new-request fence, frozen stream bound and first retirement deadlines. Acknowledgement does not wait for drain.
2. Valid requests refused at `requestheaders` receive REFUSED_STREAM and do not reach the origin. Previously admitted requests retain their own snapshots.
3. Once old requests and their local downstream/upstream output obligations are terminal, ordinary h2 APIs send one NO_ERROR GOAWAY with the frozen bound, followed by close.
4. Deadline expiry force-closes the whole connection and can interrupt other streams after rotation of a still-covered host. This is abnormal closure, not graceful completion.

Maintenance polling uses the exact connection handler's registered wakeup task. No-op housekeeping is filtered before dependency event dispatch, so it does not continually refresh the network inactivity watchdog. Actionable drain and cleanup work still runs without requiring new client traffic. The integration does not replace dependency classes or globally patch task scheduling.

Response/error hooks alone do not release a handle: DATA, trailers, queued upstream requests and unfinished uploads remain obligations. The adapter observes actual upstream children and client/upstream stream mappings without rewriting their queues or forcing H1 upstreams. Reset-stranded trailers wait only for an existing first deadline, or close abnormally if none exists. An early response followed by reset during an unfinished upload can orphan upstream work; the client and verified owned upstreams then close immediately.

Only the marked kill/NO_RESPONSE path represents normal refusal. Pre-hook 400/413 responses, parser errors and missing identity must not become retryable refusals. Pending identity storage is bounded by the request budget. Exhausting that association budget closes the connection abnormally; ordinary registry capacity denial uses the `requestheaders` refusal path.

### Client retry limitation

Delayed GOAWAY does not guarantee migration while an old response is unfinished. In the recorded Go 1.27.2 `net/http` synthetic GET configuration, retries after REFUSED_STREAM repeatedly selected the same connection and exhausted an eight-second deadline without reaching the origin. The held-old acceptance case remains **FAIL**.

A separate bounded-old case passed only when the harness released the old response early. The new GET completed once on a fresh connection with **HTTP/1.1 200**, while the old HTTP/2 response completed intact. This does not prove H2-to-H2 migration, migration while old remains unfinished, or success for arbitrary finite response durations. Go-peer frames were not captured, so GOAWAY causality and wire ordering remain unproven. Neither outcome should be generalized to other client versions or configurations.

The proxy does not replay requests or change error codes to influence client pooling. The narrower passing case does not replace the failed acceptance gate.

## Failure containment

Client disconnect fences admission immediately, but cleanup retains exact ownership until related upstreams are verified terminal. A disconnected hook, a CLOSED flag or an exit code alone does not establish transport closure. Cleanup observes actual task/IO identities and writer-close confirmation; callbacks must remain effective through canceled wakeups while handler registration remains valid.

There are two distinct fatal recovery conditions. Neither releases accounting for resources whose terminal state is unproven.

### Completed-task transport defect

An exceptional once-only `os._exit(86)` of the current mitmdump is permitted only when all of these observations agree on its owner loop:

- The exact live runtime, client/token, adapter, registered handler and actual HttpLayer-owned upstream/task identities match.
- That task was captured before requesting normal close, and the ordinary close command or the same handler's synchronous close was consumed.
- The task has ended, the connection is CLOSED, and the current transport entry still identifies that task and an actual TCP StreamWriter that is not closing. An old pre-connect IO entry is insufficient.
- The runtime's escalation flag has not previously been set.

The flag is set before exit, without logging, IPC, network operations or cleanup hooks in between. The adapter does not signal the Go owner. Normal RST/refusal, pending tasks, canceled pre-connect tasks without IO, closing writers, stale identities and unknown ownership do not satisfy this predicate. Failure to satisfy it is not proof that a handle is safe to release.

### Unverifiable cleanup ownership

If lifecycle observation cannot establish required ownership or prove completed cleanup, the runtime exits the current mitmdump with code 87 instead of leaving it running indefinitely with retained admission budgets. This is a separate condition from the completed-task/current-writer defect above: it does not claim that an open writer was proven. The runtime sets its once-only exit flag before exiting, without releasing unproven resources or logging first. Ordinary stale callbacks after admission replacement or release still use their identity guards. A completed task whose admission is still exactly retained but whose handler/IO ownership no longer matches triggers code 87; an unrelated late callback does not.

### Owner recovery and availability

The existing Go generation owner reaps an unexpected owned-child exit and retains session, readiness and recovery responsibilities. An exit can interrupt other requests served by the same mitmdump process. For a current, running, non-stopping owned child, the experimental runtime latches recovery-required when its quarantine controller is configured (nft-based modes). It attempts namespace quarantine, which can stop all sandbox networking, including loopback, DNS and control/health connections. Recovery from that state requires sandbox destruction and recreation. Without the nft quarantine controller, the MITM readiness gate becomes false on exit, but ordinary child restart may succeed; the same sticky-quarantine recovery behavior does not apply. Neither path guarantees an instantaneous atomic readiness response or automatic restoration. `QuarantineUnknown`, an exit code or a 503 does not establish packet isolation. The boundary excludes arbitrary addon descendants or inherited FDs, origin business completion, replay safety and a strict total reap deadline. Changed addon connection shapes require new ownership verification. See the [revision-runtime warning](/architecture/network/egress#transparent-tls-interception-l7).

## Verification boundaries

Socket-free tests cover registry/publication, the fixed-version adapter, lifecycle, hooks and H1 regressions. They do not prove socket, TLS or IPC behavior.

The opt-in runtime runner uses the actual system addon, authenticated Unix IPC, and SOCKS5 followed by TLS to a real `127.0.0.1:443` destination. Its 15 manual-peer cases cover attachment, removal/drain, rotation, remove/re-add, reset and disconnect with H1/H2 upstreams, short/default expiry and the isolated exceptional-exit case. These passed on an earlier tested tree; they were **NOT_RUN** in the later two-case Go run. Historical results are not fresh verification of subsequent revisions.

Outstanding acceptance includes:

- H2-A5: successful library-driven migration with the old response preserved; the held-old Go case remains failed.
- P2: pre-ACK/paused-hook races, fragmented HEADERS and flow-control variants, upstream queues/unfinished uploads/stranded trailers, and path-only or policy-only rotation.
- P3: Linux transparent-443/original-destination behavior, the restricted production user, applicable IPv4/IPv6 paths, actual Go owner/quarantine readback and the complete relevant H1/policy/credential regression matrix.

Loopback testing, including loopback on Linux, does not establish these production networking properties. Process reap and origin socket closure do not establish kernel quarantine or origin business completion.

The runner reports PASS, FAIL, BLOCKED and NOT_RUN separately, with exact commit/tree and dependency identities. A selected subset passing is not full acceptance. Go observations require a separately built fixture and an explicit exact `--require-go-version`; the runner must not infer that requirement from the binary under test or silently select another toolchain. See the [runtime runner](https://github.com/opensandbox-group/OpenSandbox/blob/main/components/egress/tests/test_credential_bound_h2_runtime.py) and [Go fixture instructions](https://github.com/opensandbox-group/OpenSandbox/blob/main/components/egress/tests/fixtures/h2_retry_client_README.md) for execution requirements.
