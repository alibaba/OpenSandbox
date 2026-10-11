---
title: Best-Effort Rootfs Recovery after Abnormal Termination
authors:
  - "@lorenzozanee"
creation-date: 2026-10-12
last-updated: 2026-10-12
status: draft
---

# OSEP-0025: Best-Effort Rootfs Recovery after Abnormal Termination

<!-- toc -->
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Requirements](#requirements)
- [Proposal](#proposal)
  - [Eligibility](#eligibility)
  - [Ownership](#ownership)
  - [Consistency and failure boundaries](#consistency-and-failure-boundaries)
  - [Notes/Constraints/Caveats](#notesconstraintscaveats)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Design Details](#design-details)
  - [Recovery source record](#recovery-source-record)
  - [Manual operation and state](#manual-operation-and-state)
  - [Node-local commit and publication](#node-local-commit-and-publication)
  - [Compatibility](#compatibility)
- [Test Plan](#test-plan)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Infrastructure Needed](#infrastructure-needed)
- [Upgrade & Migration Strategy](#upgrade--migration-strategy)
<!-- /toc -->

## Summary

This proposal adds an explicitly requested, best-effort way to salvage the
root filesystem of an exact Kubernetes container attempt after abnormal
termination, when its source node and writable snapshot are still available.
It creates a normal restorable OCI rootfs snapshot only after the artifact has
been pushed successfully; it does not promise recovery or change existing
snapshot, pause, or resume behavior.

## Motivation

An explicit rootfs snapshot is durable only after its OCI image has been pushed
to the configured registry. Changes made since the last successful snapshot
remain in a containerd writable layer on one node. After an OOM, crash,
eviction, or container restart, OpenSandbox has no public operation to select
that old layer for recovery. The layer can also disappear when Kubernetes or
containerd removes the old container or when its node is lost.

The existing documentation now describes these durability boundaries and
recommends persistent volumes or an external service for data that must survive
unexpected termination. This proposal addresses the narrower remaining
question raised in [issue #2112](https://github.com/opensandbox-group/OpenSandbox/issues/2112): whether OpenSandbox
should offer an operator- or user-requested salvage operation for a retained
rootfs. It follows the earlier durability discussion in
[issue #1872](https://github.com/opensandbox-group/OpenSandbox/issues/1872).

### Goals

- Offer a manual recovery operation for an eligible, abnormally terminated
  Kubernetes container attempt.
- Bind every operation to the exact source Pod UID and container ID, and fail
  closed if the original runtime object or writable snapshot cannot be proven.
- Expose explicit progress and terminal failure or success. A snapshot is
  available for restore only after its OCI artifact has been pushed.
- Keep the current explicit snapshot, pause, resume, and restart behavior
  unchanged.
- Make the best-effort consistency and availability limits visible to callers.

### Non-Goals

- Automatically committing a rootfs after every crash, OOM, eviction, or node
  event.
- Guaranteeing recovery after source-node or disk loss, containerd garbage
  collection, Pod deletion before identity capture, or removal of the writable
  snapshot.
- Recovering process memory, in-flight application buffers, QEMU VM state, or
  mounted-volume contents.
- Restarting a Failed or Terminated sandbox, preserving its process state, or
  redefining `resume`.
- Preserving or reusing the failed sandbox ID. A restored artifact creates a new
  sandbox through the existing snapshot restore flow.
- Extending the Docker runtime or supporting runtimes that cannot identify and
  commit the exact stopped container's writable layer.

## Requirements

1. A recovery request MUST identify one recorded container attempt; sandbox ID,
   Pod name, container name, or Pod UID alone is insufficient to select a
   writable layer. The first version supports one regular rootfs container per
   recovery; a sandbox with multiple regular containers is ineligible.
2. A candidate is eligible only when the controller has durably recorded its
   source identity. At minimum it includes the sandbox ID, namespace, Pod name
   and UID, container name and runtime container ID, source node, and observed
   termination reason and time. The recovery worker MUST resolve the
   snapshotter and writable snapshot key from that exact container ID on the
   source node. If Kubernetes replaces the Pod or drops an older restart
   attempt before the controller records it, that attempt is ineligible; the
   design does not promise to observe every termination.
3. Missing, mismatched, or ambiguous identity and missing containerd metadata
   MUST produce a visible failure. The worker MUST NOT fall back to a running,
   newer, or same-name container.
4. Recovery MUST be explicitly requested by an authorized caller. Candidate
   identity recording is not a request to commit or publish an image. The route
   MUST fail closed unless API-key authentication or the tenant provider is
   configured; multi-tenant requests MUST stay within the tenant-resolved
   namespace.
5. A successful result MUST reference an OCI rootfs artifact that was fully
   pushed to the configured registry. A local image, partially uploaded image,
   or accepted operation is not a successful snapshot.
6. Recovery status and the resulting snapshot reference MUST survive server
   and controller restarts and MUST be scoped to the owning sandbox and
   container attempt.
7. Recovery MUST leave existing explicit snapshot, pause, and resume contracts
   unchanged. Mounted volumes retain their own lifecycle and backup semantics.
8. The operation MUST use the existing snapshot commit Job limits: a 10-minute
   deadline and `backoffLimit: 3`. Every Job retry remains pinned to the same
   container ID. The recovery operation itself is created once; a terminal
   failure is not retried by a new operation in the first version.
9. The first version is limited to a single-replica Kubernetes `BatchSandbox`
   using a native containerd rootfs snapshotter. Pool-backed sandboxes, multiple
   replicas, gVisor, QEMU/VM-state, Fast Sandbox, Docker, and other runtimes are
   ineligible.
10. The first version accepts only an observed `OOMKilled`, non-zero container
    exit, or Pod eviction with a recorded terminated container attempt. A
    normal zero exit is not abnormal termination.

## Proposal

Add a separate rootfs-recovery operation for Kubernetes-backed sandboxes.
Separating it from `POST /sandboxes/{sandboxId}/snapshots` keeps that existing
endpoint's Running-source requirement intact and makes the weaker consistency
and availability contract explicit.

The API exposes the recorded recovery candidates for a sandbox and lets an
authorized caller request recovery for one candidate. In single-tenant mode,
the endpoint requires a configured API key even if other server routes are
deployed without authentication; in multi-tenant mode, it uses the current
tenant's namespace mapping. The request carries an opaque `containerAttemptId`,
not a containerd ID or snapshot key. The service
returns an operation ID; callers poll its status. On success, the operation
also returns a normal snapshot ID that can be used with the existing restore
flow. The exact route and response schema are proposed for review and must be
aligned with the public lifecycle API before implementation.

The candidate list is populated from controller-observed abnormal container
termination. The controller writes the candidate into
`BatchSandbox.status.rootfsRecoveryCandidates`; recording a candidate does not
launch work. Observation is asynchronous, so the proposal does not claim that
every short-lived container attempt will be recorded. If the controller did
not persist the attempt identity before Kubernetes removed or replaced the Pod,
that attempt is not eligible and cannot be reconstructed from a same-name Pod.
Maintainers may require a node-level event recorder if controller observation
proves insufficient.

The first increment supports only a single regular rootfs container in a
single-replica, non-pooled Kubernetes `BatchSandbox` using native containerd.
Sidecars, init-container recovery, multiple regular containers, gVisor, QEMU,
Fast Sandbox, and non-Kubernetes runtimes are excluded. Eligible termination
reasons are `OOMKilled`, non-zero container exit, and Pod eviction when the
terminated attempt identity is present. Clean exit is not a recovery candidate.

### Eligibility

An attempt is eligible only when all of the following are true at request and
execution time:

- It is a Kubernetes-backed sandbox with a durably recorded abnormal
  termination attempt.
- The record binds the sandbox, Pod UID, container attempt ID, runtime container
  ID, container name, and source node. The runtime's labels must agree with the
  recorded Pod UID and container identity.
- The source node is reachable, the exact container metadata is still present,
  and it identifies a writable snapshotter and snapshot key.
- The configured snapshot registry is available to the server's existing
  credential and publication path.
- No recovery operation for the same attempt is currently committing or
  publishing.

The eligibility check is not a promise that the source will remain available:
container cleanup can race with every check. The commit worker revalidates the
identity immediately before reading the writable snapshot. If any prerequisite
disappears, it fails the operation without selecting another container.

### Ownership

| Component | Responsibility |
| --- | --- |
| Lifecycle server | Authenticate through existing API-key/tenant middleware, resolve the sandbox in the caller's namespace, create the operation, and expose status and the final snapshot ID. In single-tenant deployments, the configured API key remains operator-level authority. |
| Kubernetes controller | Persist each observed candidate in `BatchSandbox.status`; leave unobserved or already-removed attempts ineligible; schedule work only after an explicit request. |
| Node-local image committer | Run on the recorded source node, validate the exact container ID and Pod UID, read its writable snapshot, and produce the rootfs image. |
| `SandboxSnapshot` operation and snapshot catalog | Persist the requested operation and its pinned source identity; record the ordinary snapshot metadata and mark it Ready only after registry publication succeeds. |

The caller cannot provide a node, containerd ID, snapshot key, target image, or
registry credential. Those values come from the owning sandbox, the recorded
attempt, and existing server configuration. In multi-tenant mode, the operation
and candidate remain in the tenant-resolved namespace; the request must not
fall back to a cluster-wide lookup when the sandbox is absent there. The
node-local committer continues to be the only component that accesses the
containerd socket.

Kubernetes API writes are performed by the lifecycle service identity, not the
tenant caller. Grant that identity only the required `SandboxSnapshot` verbs
in the owning namespaces; do not grant those write verbs to tenant identities.
The existing tenant middleware remains the authorization boundary for the
public routes.

### Consistency and failure boundaries

The output is a filesystem image of the stopped container's writable layer.
It is at best crash-consistent: it cannot flush application buffers after an
unexpected exit, recreate process memory, or establish application-level
transaction consistency. A successful rootfs snapshot therefore does not mean
that the application state is valid.

The operation may fail before or during local commit, upload, or metadata
publication. Every terminal failure is visible and contains a bounded,
sanitized reason; no failed or incomplete artifact is exposed as restorable.
If upload succeeds but status persistence is interrupted, reconciliation uses
the same operation identity and unique target reference to verify the uploaded
artifact, rather than committing a different container. Retrying an operation
must remain bound to the same attempt and must not overwrite a prior successful
snapshot.

No fixed salvage window is guaranteed. The useful window ends when the source
container or writable snapshot is removed, the node becomes unreachable, or
the operation's configured deadline expires. That deadline is an operational
bound, not a retention promise.

### Notes/Constraints/Caveats

- The already documented contract remains authoritative: only a successfully
  published rootfs snapshot is durable; unsnapshotted changes have no recovery
  guarantee.
- A replacement container, even in the same Pod, is a different attempt and
  must never be substituted for the failed one.
- Pod UID distinguishes replacement Pods with the same name. Runtime container
  ID distinguishes restart attempts within one Pod. The containerd snapshot
  key is read from the container record selected by that exact ID.
- Mounted volumes are external to the OCI rootfs image. Their durability is
  determined by the volume and its backup policy.
- This proposal makes a concrete starting recommendation, not a claim that
  maintainers have approved a new durability guarantee. API shape, identity
  retention, and operations ownership need review before implementation.

### Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| A replacement layer is captured after a restart | Require the recorded runtime container ID and Pod UID; reject label mismatches; never use active/newest-container fallback. |
| Cleanup removes a layer during recovery | Revalidate on the source node, treat disappearance as a terminal visible failure, and make no availability promise. |
| A recovery job gains broad host-container access | Keep requests controller-generated, pin them to an authorized sandbox-owned record, limit execution to the configured node-local committer, and never expose the containerd socket to sandbox workloads. |
| A crash-consistent image is mistaken for application-consistent data | Name and document the output as rootfs-only, return explicit operation state, and retain the persistent-volume recommendation. |
| Retries create duplicate or conflicting images | Use one operation-owned target reference with provenance, verify it before reconciling status, and prohibit overwriting or retargeting. |
| Automatic salvage worsens resource pressure after OOM | Keep commit initiation manual and bound worker resources and runtime. |

## Design Details

### Recovery source record

The Kubernetes controller records one immutable candidate per observed
container attempt in `BatchSandbox.status.rootfsRecoveryCandidates`. Its opaque
ID is derived from the sandbox identity, Pod UID, container name, and runtime
container ID; it does not encode a user-selected container. The record also
stores the source node, observed termination data, and a creation timestamp.
Repeated reconciliations update neither the attempt identity nor its
termination facts.

The controller writes a candidate when it observes the terminated attempt.
Because that observation is asynchronous, it may miss an attempt that is
replaced or deleted first; no candidate is created in that case. At execution,
the node-local worker loads containerd metadata by exact runtime
container ID and checks the containerd Pod UID, namespace, Pod name, and
container name labels against the candidate. It then reads the snapshotter and
snapshot key from that same container record. Missing metadata, mismatched
labels, an absent writable snapshot, or a running/replacement container causes
failure; there is no name-based resolution fallback.

Candidate records are internal fields on the owning `BatchSandbox.status` and
do not expose containerd snapshot keys to API callers. An accepted request
copies the exact attempt identity into its `SandboxSnapshot` operation before
scheduling work, so controller and server restarts do not depend on the Pod
still existing. Candidate status retention is bounded by both age and count;
the default values must be selected before implementation. Expired candidates
are reported ineligible; the system does not retain node data solely to keep a
candidate eligible. Deleting the sandbox before a request is accepted removes
its candidates; an accepted operation remains owned by its `SandboxSnapshot`.

### Manual operation and state

The proposed API surface uses the existing API-key authentication and tenant
namespace resolution. In a single-tenant deployment, access is operator-level
because the configured API key is deployment-wide; this route is disabled if
neither an API key nor tenant provider is configured. Multi-tenant requests are
restricted to the resolved namespace, like other sandbox lifecycle requests.
The proposed routes are:

```text
GET  /v1/sandboxes/{sandboxId}/rootfs-recoveries
POST /v1/sandboxes/{sandboxId}/rootfs-recoveries
GET  /v1/rootfs-recoveries/{recoveryId}
```

`GET` lists opaque candidate IDs and eligibility state without exposing
containerd internals. `POST` requires one `containerAttemptId`; it returns
`202 Accepted` and a recovery operation ID. The ID is derived from the tenant
namespace and candidate ID, so concurrent or repeated POSTs cannot create a
second operation. `SnapshotRecord` stores that ID in a unique
`recovery_operation_id` field and stores an internal immutable
`recovery_source` value containing the candidate ID, sandbox ID, namespace, Pod
name and UID, container name, runtime container ID, source node, and observed
termination data, plus the operation's unique target reference. It does not
expose containerd snapshot keys through
the snapshot API. The `SandboxSnapshot` CR uses the same operation ID as its
deterministic name and stores the same source and target. Both records store
the public snapshot ID and tenant namespace, binding them to the same
operation.
Requesting an ineligible or unknown candidate returns a conflict or not-found
response without scheduling work. The exact error codes should follow the
repository's existing lifecycle API conventions.

The lifecycle server creates a `SnapshotRecord` in `Creating` with the unique
`recovery_operation_id` and a copy of the immutable candidate identity, then
creates a deterministically named `SandboxSnapshot` operation containing that
key and the same exact attempt identity copied from `BatchSandbox.status`.
The catalog row durably stores the pinned source and target; the CR owns
controller progress. The CR adds a `Publishing` phase, a pinned recovery
source, an operation-owned target reference allocated before the Job is
scheduled, and an immutable artifact digest. A finalizer retains the CR until
the operation is terminal. Ordinary snapshot CRs keep their existing phases
and source resolution. The recovery reconciler consumes the pinned source and
never calls the Running-Pod resolver.

The database and Kubernetes API cannot share a transaction. The unique
`recovery_operation_id` makes concurrent database inserts converge on one
snapshot record, and the deterministic CR name makes repeated Kubernetes
creates converge on one operation. The server creates the database row first;
then makes one create request for the deterministic CR name. Concurrent
requests read the existing row/CR after a uniqueness or name conflict. The
controller cannot schedule a Job before the CR exists. If the create response
is lost, the server reads that same name and returns the existing operation.
Once the row exists, a repeated POST never creates another CR or Job. If the
CR is absent, the synchronizer observes the fixed target/Job and fails closed
after the deadline rather than restarting dispatch. A key, namespace, or
snapshot-ID mismatch is a visible terminal failure and schedules no Job. The recovery status
synchronizer only updates the existing `SnapshotRecord`. If that write fails,
it records the error and retries from the CR status while the service is
running and after restart. Until both the CR and snapshot record are Ready,
the API continues to report the operation as `Publishing` and does not return
a successful snapshot result. Job retries use the existing
10-minute deadline and `backoffLimit: 3`; the operation deadline is anchored
to the durable `SnapshotRecord.created_at`. Every retry uses the same pinned
container ID and target reference. Before committing, each Job attempt checks
that reference: if a remote artifact exists, it verifies operation provenance
and adopts its digest without rebuilding or overwriting it; only an
authoritative query that confirms absence permits a new commit and publish.
If the registry cannot determine
whether the artifact exists, the attempt fails visibly for the configured Job
retry to check again. Exhausted Job retries produce a terminal failure, never
a second operation. Once the CR is `Failed`, the first version does not
requeue it or create a new operation. The finished Job's 300-second TTL is not
the retention policy for the CR or snapshot record.

The ordinary `recover_unfinished_snapshots` scan must exclude rows with a
non-null `recovery_operation_id`; those rows are owned only by a
recovery-specific synchronizer. It updates the row from CR status and does not
depend on the candidate or sandbox still existing. If the CR is forcibly
removed before terminal state, the synchronizer observes the deterministic Job
and fixed target reference but never recreates the CR or schedules a new Job,
so Kubernetes backoff cannot reset. It adopts an artifact only after
provenance verification, writes its digest-qualified restore URI and Ready
state into `SnapshotRecord`, and lets operation GET report success from that
row. If no Job or verifiable artifact remains, it marks the operation Failed
after `SnapshotRecord.created_at` plus the existing 10-minute Job deadline. An
indeterminate registry result remains Publishing until that deadline, then
fails visibly.
It must never call the ordinary snapshot runtime or submit a Running-Pod job
for a recovery record.

The recovery CR moves through `Pending`, `Committing`, `Publishing`, then
`Succeed` or `Failed`. The API maps those phases to its operation response; only
`Succeed` returns a normal `snapshotId`. A terminal response includes the
attempt ID and a sanitized reason. On acceptance, the server creates the
`SnapshotRecord` in `Creating` and stores its ID and tenant namespace on the
operation. After a verified push, the controller stores the immutable digest
and target reference on the CR. The server's recovery status synchronizer sets
`restore_config.image` to the full `registry/repository@sha256:<digest>` URI and
transitions the corresponding
`SnapshotRecord` to `Ready`; the existing restore lookup then uses the pinned
digest, not a mutable tag. The recovery path bypasses ordinary Running-source
preflight only after validating the pinned attempt. A repeated request for an
active or successful candidate returns the existing operation/result. A
terminally failed candidate cannot be retried in the first version.

The `SnapshotRecord` remains tenant-scoped and separately controls snapshot
read, restore, and delete permissions. Deleting the operation record does not
delete a Ready snapshot; registry artifact retention follows the snapshot
catalog's existing lifecycle and configured registry policy.

### Node-local commit and publication

The controller schedules the committer to the source node recorded for the
attempt. The worker receives the controller-created recovery operation and
loads the exact container by runtime ID; it does not choose a container by
Pod name or select the newest container in a matching Pod. Before committing,
it confirms the container is stopped, validates the identity labels, and
requires a non-empty writable snapshot key and snapshotter. The commit reads
that writable snapshot and assembles an OCI rootfs image using the existing
containerd image-committer path.

The image is pushed to the configured snapshot registry with the existing
server-side credential policy. The operation becomes `Succeed` only after the
remote artifact is read back by immutable digest, its manifest provenance is
verified against the operation ID and pinned source identity, and the snapshot
catalog references it as `Ready`. Each operation uses a unique target
reference that is never reused by another
operation. The uploaded image carries the operation ID as provenance. The
worker records the returned immutable digest on the operation CR before the
server marks the snapshot Ready. If a worker restarts after the push but before
that status write, reconciliation reads the operation's unique reference and
verifies its provenance before recording the digest. After an ambiguous push,
a Job retry checks the same reference first and adopts a verified artifact
instead of rebuilding it. If the registry cannot establish whether the
artifact exists, the attempt fails visibly and the bounded retry checks again;
after retry exhaustion, the operation is terminally failed and is never
exposed as restorable. A later request for the same candidate returns that
result.

### Compatibility

This is an additive Kubernetes-only API and operation. Existing calls to
`POST /sandboxes/{sandboxId}/snapshots` still require a Running sandbox, and
existing pause/resume paths retain their current state transitions. Docker and
other runtimes remain unsupported. The proposal adds candidate fields to
`BatchSandbox.status` and a recovery-source reference, operation ID,
`Publishing` phase, and artifact digest to `SandboxSnapshot`. It also adds a
nullable unique `SnapshotRecord.recovery_operation_id` and nullable internal
`SnapshotRecord.recovery_source` and
`SnapshotRecord.recovery_target_reference`; existing records remain null. The
source metadata and target are not returned by snapshot list/get APIs. These
schema changes are additive. Recovery is disabled by default. On enablement, the server checks
that the installed CRDs serve the required fields; otherwise routes return an
unsupported-capability response without creating an operation. The first
version requires server and controller to be upgraded as one release, with the
matching generated CRDs installed before the server is enabled. Independently
mixing server and controller versions is unsupported. Generated CRDs and Helm
outputs must ship with the controller.

## Test Plan

- Unit-test candidate identity creation and idempotent reconciliation across
  repeated Pod observations, bounded candidate retention, and an attempt missed
  before reconciliation.
- Test exact-ID resolution for a stopped container, matching and mismatching
  Pod UID/container labels, missing metadata, missing writable snapshot,
  multiple stopped containers, and a replacement container that is active in
  the same Pod. Include `containerd://` ID normalization.
- Verify that the committer never falls back to another container when the
  recorded container has disappeared or its snapshot key is unavailable.
- Exercise operation transitions, duplicate requests, restart recovery, and
  sanitized terminal failure reasons. Verify a failed candidate cannot be
  retried in the first version.
- In a Kubernetes integration test, terminate a source container, request
  recovery while its node and writable snapshot remain available, and verify
  that the published artifact contains the stopped attempt's files and restores
  through the existing snapshot path.
- Exercise `SnapshotRecord` creation, `SandboxSnapshot` recovery phase,
  digest propagation into `restore_config.image`, and restore lookup end to end;
  verify the ordinary startup scan skips recovery records, the recovery
  synchronizer observes a forcibly missing CR without creating a new Job, and
  no ordinary Running-Pod commit job is dispatched for recovery. Verify a
  missing CR with a verified artifact updates the catalog digest and operation
  GET result; verify a missing Job and absent artifact fails at the durable
  operation deadline without scheduling work. Inject a database status
  write failure while the server remains running and verify the operation
  stays `Publishing` until the snapshot record becomes Ready.
- Cover node unavailability, Pod replacement, containerd cleanup racing with
  commit, registry rejection/timeout, and a push that succeeds before a
  controller restart. Each must either produce one digest-pinned snapshot or
  an explicit failed/ineligible result.
- Verify requests fail closed without API-key/tenant auth, cannot cross tenant
  namespaces, and only regular single-container, single-replica native
  containerd sandboxes qualify. Verify the API server's Kubernetes identity
  has only the required namespaced `SandboxSnapshot` verbs, callers cannot
  create or mutate recovery CRs directly, and the server refuses enablement
  when the installed CRDs lack the required fields.
- Keep existing Running-sandbox snapshot, pause, and resume tests passing
  unchanged. Verify mounted-volume data is not represented as part of the OCI
  rootfs result.

## Drawbacks

- The feature adds API surface, durable attempt metadata, controller work, and
  a node-local operation even though recovery availability is inherently
  bounded by node and containerd cleanup.
- The controller must persist attempt identity early enough to survive Pod
  replacement. Kubernetes does not provide an unlimited history of terminated
  container IDs, so missed observations remain unrecoverable.
- Users may overestimate what a crash-consistent filesystem image means for
  application correctness.
- The existing committer has privileged access to node containerd. Strict
  request ownership and exact-ID validation become security-critical.

## Alternatives

1. **Extend the current snapshot endpoint to accept Failed sandboxes.** This
   looks smaller, but weakens the endpoint's existing Running-source contract
   and leaves callers unable to distinguish an ordinary point-in-time snapshot
   from a best-effort salvage operation. A separate operation makes the
   different eligibility and failure semantics explicit.
2. **Automatically commit after abnormal termination.** This could preserve
   more layers without user action, but may compete for disk and CPU after an
   OOM and can race with kubelet/containerd cleanup. It is excluded until
   identity retention and resource limits are proven.
3. **Treat `resume` as recovery.** This would conflate restoring a successfully
   persisted snapshot with restarting a failed sandbox. Existing behavior and
   documentation explicitly keep those lifecycle actions separate.
4. **Keep the documented best-effort operator procedure without a public API.**
   This avoids API and state-model cost, but leaves no supported way to bind a
   request to an exact attempt or report whether the artifact was actually
   published. This remains the correct outcome if maintainers do not accept the
   added operational contract.

## Infrastructure Needed

No new external service is required. The design reuses Kubernetes controller
work, the existing node-local image committer and containerd socket, the
existing snapshot registry and credentials, and the existing snapshot catalog.
Candidate identity is stored on `BatchSandbox.status`; accepted operation state
is stored on the `SandboxSnapshot` CR; public artifact state remains in the
existing snapshot repository. The proposal requires a bounded candidate
retention policy and node-local job resource limits; their defaults must be
selected before implementation.

## Upgrade & Migration Strategy

Apply a database migration that adds nullable unique
`SnapshotRecord.recovery_operation_id`, nullable internal
`SnapshotRecord.recovery_source`, and nullable
`SnapshotRecord.recovery_target_reference`; existing snapshots remain null. Deploy
the additive CRDs before the controller and server versions that use them.
The server advertises the capability only after both CRDs and the controller
support it. Deployments without the updated CRDs and controller continue using
current snapshot and pause/resume flows, and the recovery routes return an
unsupported-capability response. Disabling the feature stops new recovery
requests without invalidating already published snapshots.
