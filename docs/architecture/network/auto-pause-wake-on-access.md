---
title: Auto-Pause and Wake-on-Access
description: The ingress-driven lifecycle loop for Fast Sandbox — idle observation, automatic checkpoint pause, and wake-on-access with bounded request parking (OSEP-0024).
---

# Auto-Pause and Wake-on-Access

Fast Sandbox sandboxes are created in milliseconds but then hold their full CPU and memory allocation for as long as they live — including the idle tail. Agent workloads are idle most of the time: seconds of activity followed by minutes of silence. Auto-pause and wake-on-access close that gap with one lifecycle loop: when an opted-in sandbox sees no ingress traffic for a continuous idle threshold **X**, the server pauses it through fast-sandbox's checkpoint machinery (memory and CPU drop to zero, the Sandbox CR is retained); when a request later arrives for a paused sandbox, the ingress gateway triggers the resume and **parks** the request for up to a wait budget **Y** instead of failing it with a permanent `503`.

Both halves are off by default and per-sandbox opt-in. The full proposal, including alternatives and the test plan, is [OSEP-0024](https://github.com/opensandbox-group/OpenSandbox/blob/main/oseps/0024-auto-pause-and-wake-on-access-for-fast-sandbox.md). For deployment and usage, see the [Auto-Pause & Wake-on-Access guide](/guides/auto-pause-wake).

## The loop at a glance

```text
t0        last ingress request observed (activity key written)
t0 + X    idle gates pass → PauseSandbox (desired state)
t0 + X+Δ  checkpoint durable, runtime released   [Pausing → Paused]
later     request arrives → resume triggered, request parks ≤ Y
          ├─ Ready within Y   → forward, serve normally
          └─ Y exhausted      → 503 + Retry-After (restore continues)
```

Three properties make the loop cheap: the observation point already exists (every client request transits the ingress), the pause/resume primitives already exist (FastPath desired-state RPCs), and the wake path adds no new synchronous dependency (no Redis, no queue — the gateway resumes directly over the gRPC connection it already holds).

![Auto-pause and wake-on-access sequence](../../public/images/auto-pause-wake-sequence.png)

👉 <a href="/architecture/network/auto-pause-wake.html" target="_blank" rel="noopener">Interactive sequence diagram</a> — walk the activity-recording, auto-pause, and wake-on-access phases with guided views, theme switching, and relationship tracing.

## Design philosophy

### One observation point

Sandboxes have private IPs and are reachable only through the gateway proxy chain, so exec calls, file transfers, and every user port transit the ingress — one vantage covers all client-driven activity. The ingress is therefore the **only activity producer** and the **only wake authority** in the system; the server decides *when* to pause, but only the data plane sees *whether traffic is flowing*.

The mirror image is deliberate too: what the ingress cannot see does not keep the sandbox alive. Activity that originates inside the sandbox with no inbound traffic (a background job, a long computation started by an earlier request) will be paused — unless the application declares an [`activeProbe`](#the-activity-probe-pull-mode-observation) and answers busy while its work runs.

### Desired state replaces coordination

`PauseSandbox` and `ResumeSandbox` are compare-and-set desired-state patches on the Sandbox CR (`spec.state`), idempotent on replay. That single property removes most of the distributed-systems machinery the loop would otherwise need:

- **Pause side** uses a per-sandbox distributed lock (`SET NX EX`) only because a checkpoint has real cost and must happen once per idle episode. Even there, correctness never depends on the lock — a lost race converges to one pause anyway, because replaying the patch on an already-paused sandbox is an idempotent success.
- **Wake side** uses no lock and no leader at all. Every ingress replica runs its own singleflight registry, so R replicas produce at most R resume RPCs, of which exactly one performs the state patch — and the expensive step, the restore, executes once because it is driven by `spec.state`, not by the number of callers. **Dedup comes from convergence, not coordination.**

### Opt-in, three-party activation

Mirroring OSEP-0009's renew-on-access, the feature activates only when three independent parties agree: the server (`[automation.idle_pause]` config), the ingress (`--activity-enabled` / `--wake-enabled`), and the sandbox itself (the `lifecycle.idlePolicy` create field, persisted as the `sandbox.opensandbox.io/idle-policy` CR annotation). A sandbox without a declared policy is **never auto-paused and never auto-woken** — deployment flags can never change the behavior of sandboxes that predate them. A policy that is missing, unreadable, or malformed on the consumer side is treated exactly as absent, never as an implicit opt-in.

### Fail-safe by construction

Every dependency failure resolves toward *leave the sandbox running* or *answer 503* — never toward data loss or an unintended pause:

- Redis unreachable → the sweep is skipped entirely; wake works because it does not touch Redis.
- Activity key missing (dropped write, Redis restart/eviction/flush) → **unknown, never proof of idleness**. The sweeper records a tombstone and measures inactivity from the first *observation* of unknown state, so data loss can only delay a pause by one window, never accelerate one.
- Pause fails for any reason → the sandbox stays `Running`; the worst case of the pause subsystem is a missed pause.
- `ResumeSandbox` rejected (`FailedPrecondition`, `Aborted` fence) → re-evaluated on the next poll or next sweep.

### Bounded wake path

The wake path bounds every resource before doing work: the parking lot (`--wake-park-max`) bounds held requests, restore permits (`--wake-restore-max`) bound concurrent VM restores independently of request lifetimes, and per-sandbox singleflight collapses a burst of N requests into one resume flight. Overflow is shed with `503` **before** any `ResumeSandbox` is issued. Budget exhaustion never cancels an in-flight restore — the resume is desired state that converges regardless, so the next request typically finds the sandbox `Ready` and is served on the fast path.

### Provenance and replay authority

A reproducible body proves a request *can* be re-sent; it does not prove the sandbox *never executed it*. The gateway therefore splits replay authority by the one fact it can prove — whether the request bytes were ever sent upstream:

| | Pre-forward (wake parking) | Post-forward (stale-route re-entry) |
|---|---|---|
| When | Parking happens before any upstream dial | Stale-route signal after bytes reached upstream |
| Execution proof | Provably never executed | "Executed then disconnected" is indistinguishable from "forwarded but not executed" |
| Replay rule | Any reproducible request parks and forwards after wake | Retry **once**, only idempotent methods (`GET`/`HEAD`/`PUT`/`DELETE`) with reproducible bodies; everything else keeps `503` + `Retry-After` |

The gateway-owned `OpenSandbox-Wake: expired` marker rides only on parked-timeout responses — the proxy strips the header from every upstream response, so it is the sole provenance proof that a request was never forwarded. SDKs auto-retry only the marked response, which is what makes retry safe for any method, including exec starts.

## How it works

### Activity tracking (ingress → Redis)

Each proxied request produces one observation, written as a side effect — never inline on the forwarding path:

- **Key**: `opensandbox:activity:{sandbox_id}`, value = Unix milliseconds. The prefix is fixed in code on both the ingress (writer) and server (reader) — it is not a deployment knob.
- **Write**: monotonic max update (a Lua script overwrites only when the new timestamp is greater), `EX activity_ttl_seconds`, issued asynchronously through a buffered channel + background pipeline. Failures are logged and dropped. Coalescing bounds writes to at most one per sandbox per `--activity-min-interval` (default 5 s) per replica, regardless of request rate.
- **TTL semantics**: `X ≤ activity_ttl_seconds` is enforced at create time, so a key that is present but stale by ≥ X is already idle *by value*. A missing key means nothing on its own — it enters the [tombstone](#fail-safe-by-construction) path above.
- **Lifecycle writes**: create and every successful resume write an immediate observation, so the idle clock starts from the latest lifecycle transition rather than from nothing.
- **Coverage**: every proxied request counts — exec and file calls on raw port 44772, user ports, health pings. Traffic through the server proxy path (`/sandboxes/{id}/proxy/{port}`) bypasses the ingress and is invisible in this revision.

### The activity probe (pull-mode observation)

The probe closes the one blind spot of ingress-only observation: work the sandbox is doing without generating traffic. A sandbox that declares `idlePolicy.activeProbe` adds a pull-mode producer — when the sweeper is otherwise ready to pause, it issues `GET {path}` on the declared port **through the ingress gateway** (same routing and TLS as client traffic), carrying the internal sentinel header `OpenSandbox-Skip-Activity: skip` so the probe never resets the idle clock it is measuring.

The verdict maps to activity, not to a second decision path: `200 {"active": true}` is recorded as an ordinary observation (a busy answer simply resets the idle clock — a continuously busy sandbox sees one probe per ~X); `{"active": false}` proceeds toward pause, with the optional `message` carried into the structured pause audit log; any non-200, malformed body, timeout, or connection error records nothing and drops the candidate for this sweep — a timeout may be an app too busy to answer, so only an explicit "not active" lets a pause through. Because the probe spans seconds, the sweeper re-verifies the activity key under the lock after the probe returns, closing the race with a request that landed meanwhile.

### Idle sweeper and pause gates (server)

A background task on each server replica runs every `sweep_interval_seconds` (default 15 s). It enumerates `Running` candidates per tenant namespace via FastPath `ListSandboxes` (the sandbox list, not a Redis key scan — a sandbox whose activity key is absent is exactly the one that needs tombstone handling), batch-reads the activity keys (`MGET`), and evaluates the gates **in order**; the first failure drops the candidate silently for this sweep:

1. **Opt-in** — the `idle-policy` CR annotation carries a valid policy (Kubernetes API `GET`, `resourceVersion: "0"`, only for candidates that already look idle).
2. **State** — `Running`; `Pausing`/`Paused` need no action, `Resuming` is protected.
3. **Idle** — key present and aged ≥ X, or tombstone aged ≥ X; a missing key with no/fresh tombstone is *unknown, skip*.
4. **Expiry** — `expires_at > now`; an expired sandbox belongs to the expiry path.
5. **Lock** — `SET opensandbox:idlep:lock:{id} NX EX`; failure means another replica is handling it.
6. **Re-verify under lock** — a newer observation drops the candidate.
7. **Activity probe** (only when declared) and a second re-verify after it.

Surviving candidates get `PauseSandbox` with an idempotent `request_id` (`idlepause-{sandbox_id}`). Multi-replica sweep needs no leader election: the lock admits one replica per idle episode, FastPath idempotency absorbs lost races, and a crashed replica's lock expires by TTL. The cost is N× duplicated candidate scanning across replicas, kept cheap by Redis-first filtering.

### Pause flow and state mapping

| Transition | Trigger | Meaning |
|---|---|---|
| `Running → Pausing` | gates pass, `PauseSandbox` accepted | Checkpoint in progress; cancelable — a wake arriving during the checkpoint cancels the pause intent |
| `Pausing → Paused` | checkpoint durable, runtime released | CRD retained, memory/CPU footprint zero, artifact set (`vmstate` + rootfs manifest) in the store |
| `Pausing → Running` | wake arrives mid-checkpoint | Resume cancels the pause before the checkpoint releases |
| `Paused → Resuming → Running` | wake-on-access or manual resume | Runtime + dataplane Ready again |
| `Paused → Terminated` | `expires_at` passes | Pausing never stops the absolute expiry clock; the retained CR flips to `Terminated` and wake then fails fast with `404` |

The mapping to public states is exactly the existing lifecycle table — `GET /sandboxes/{id}` already renders `Pausing`/`Paused`/`Resuming`; this loop only makes those states reachable automatically.

### Wake-on-access: detection, singleflight, parking

The wake path lives in the ingress FastPath provider, next to route resolution:

| State at resolution | Behavior |
|---|---|
| `Paused` | Start/join a resume flight, park |
| `Pausing` | Start/join a flight — fast-sandbox cancels the pause intent (no wait for the checkpoint) |
| `Resuming` | Join the in-flight flight, park |
| Terminal / expired | Fail fast `404` — never resume |
| No policy or `wakeOnAccess: false` | Exempt from every row above: today's permanent `503`, no `ResumeSandbox` |

**Resume flight (singleflight)** — keyed `(namespace, sandbox_id)` in a per-replica registry. The first request acquires a parking slot **and** a restore permit, then calls `ResumeSandbox` (fenced on `expected_checkpoint_id` — the only race that matters, a re-pause slipping in) and polls `GetSandbox` with exponential backoff (50 ms × 1.3 + jitter) until Runtime and DataPlane are Ready. Concurrent requests join the flight and share its remaining budget: N parked requests, one resume RPC, one poll loop. A flight that observes `Ready` writes the shared activity key, so the post-resume idle window starts cleanly regardless of which replica won.

**Parking** — the parked handler simply does not invoke the reverse proxy yet: it blocks on the flight's completion channel, the client's disconnect, or the remaining-budget timer. Parking happens before any upstream dial, so a parked request holds no upstream connection — its cost is one goroutine plus the held client connection. The flight runs on a background context: budget exhaustion or client disconnect never cancels the resume. WebSocket upgrades park too — the upgrade completes after wake against the restored upstream.

**Stale-route re-entry** — a request forwarded on a cached route can surface a pause later as a stale upstream response. On that error the provider invalidates the cache, calls `GetSandbox`, and re-enters the wake path when the state is wakeable, retrying the request once under the [replay rules](#provenance-and-replay-authority).

### Wake-timeout contract

Budget exhaustion answers `503` with `Retry-After: 1` and the gateway-owned marker `OpenSandbox-Wake: expired` (body code `sandbox_wake_timeout`). The marker is reserved and stripped from upstream responses, so a bare `503` stays byte-for-byte today's paused-sandbox answer while the marked response is provably safe to retry for any method. Official SDKs apply a dedicated retry contract to this response only — wait per `Retry-After`, resend, bound the total wait with `overall_deadline` (default 60 s) and `max_attempts` (default 5), and surface a typed `WakeTimeoutExceeded` error on exhaustion. See the [guide](/guides/auto-pause-wake#wake-behavior-and-sdk-retry) for the client-side behavior.

## Failure modes

| Failure | Behavior |
|---|---|
| Redis unreachable | Sweeps skipped (no pauses); wake unaffected (no Redis on that path) |
| Redis data loss (restart / eviction / flush) | Missing key ⇒ tombstone ⇒ pause delayed by ≥ X from first observation, never accelerated |
| Ingress replica partitioned from Redis | At most one spurious pause per stale window, self-healed by wake; detect via per-replica `activity.writes_dropped` |
| Two replicas race to pause | `NX` lock + idempotent CAS — exactly one pause per idle episode |
| Resume racing a re-pause | `expected_checkpoint_id` fence returns `Aborted`; the flight restarts within the remaining budget |
| Slow (cross-node) restore | Budget Y bounds client wait; `503` + `Retry-After` while the restore continues; next request served fast |
| Burst to many paused sandboxes | Lot + permits bound; overflow shed before any RPC |
| Paused + expired sandbox accessed | `404` once the reconciler flips the retained CR (one-sync window is an accepted race) |

## Security boundary

The ingress gains exactly two FastPath calls on the channel it already holds — `GetSandbox` (read) and `ResumeSandbox` (write) — plus one read-only Kubernetes grant: `get` on the Sandbox CR, namespaced to tenants it already serves, solely to read the idle-policy annotation. It never calls `PauseSandbox`: pause stays a server-side authority. Wake is not an auth bypass — parking happens after the existing host/route-scope/signature verification, so a request that would be rejected today is rejected before any resume is triggered; a client can only wake sandboxes it can already address.

## Observability

The loop ships metrics aligned with OSEP-0010 — wake flights (triggered/joined), parked requests and wait durations, shed count, active restores, activity writes dropped, policy-read outcomes, pause requests (accepted/gated/failed), probe outcomes, and the **flap ratio** (pauses followed by a wake within a window — the single most useful tuning signal for X). Dashboards and alert guidance: wake P95 < 500 ms same-node, shed rate ≈ 0, flap ratio, paused-sandbox count and artifact-store usage per pool. See the [Telemetry guide](/guides/telemetry).

## Where to next

- [Auto-Pause & Wake-on-Access guide](/guides/auto-pause-wake) — enablement, `idlePolicy` reference, SDK retry behavior, tuning.
- [Ingress](/architecture/network/ingress) — the observation point and wake authority; renew-intent signaling shares its patterns.
- [Fast Sandbox: Pause, Resume, and Snapshots](/architecture/fast-sandbox/checkpoints) — the checkpoint machinery this loop automates.
- [OSEP-0024](https://github.com/opensandbox-group/OpenSandbox/blob/main/oseps/0024-auto-pause-and-wake-on-access-for-fast-sandbox.md) — full proposal, alternatives, and test plan.
