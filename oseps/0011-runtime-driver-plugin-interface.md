---
title: Runtime Driver Plugin Interface
authors:
  - "@AlexandrePh"
creation-date: 2026-03-30
last-updated: 2026-03-30
status: draft
---

# OSEP-0011: Runtime Driver Plugin Interface

<!-- toc -->
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Requirements](#requirements)
- [Proposal](#proposal)
  - [Notes/Constraints/Caveats](#notesconstraintscaveats)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Design Details](#design-details)
  - [RuntimeDriver Interface](#runtimedriver-interface)
  - [Driver Registry](#driver-registry)
  - [Driver Lifecycle Hooks](#driver-lifecycle-hooks)
  - [Configuration Schema](#configuration-schema)
  - [Capability Discovery](#capability-discovery)
  - [External Driver Protocol](#external-driver-protocol)
- [Test Plan](#test-plan)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Infrastructure Needed](#infrastructure-needed)
- [Upgrade & Migration Strategy](#upgrade--migration-strategy)
<!-- /toc -->

## Summary

This proposal formalizes the top-level sandbox runtime as a pluggable driver interface, enabling OpenSandbox to support runtime backends beyond Docker and Kubernetes — including Windows VMs, cloud-managed containers (e.g., Azure Container Instances), WebAssembly runtimes, and remote bare-metal hosts. Drivers implement a standard `RuntimeDriver` interface and register via a driver registry, allowing third-party backends to be developed and deployed independently of the OpenSandbox core.

## Motivation

OpenSandbox currently supports two hardcoded runtime backends: Docker (local) and Kubernetes (cluster). The selection is made via `runtime.type` in the server config, and adding a new backend requires modifying `services/factory.py` directly. This tight coupling creates several problems:

1. **New runtimes require core changes**: Adding a backend (e.g., Firecracker standalone, Hyper-V, containerd, cloud VMs) means changing the factory, adding a new service class, and releasing a new server version. External contributors cannot ship a runtime without merging into the core repo.

2. **Windows/macOS sandbox support is blocked**: Issue #438 requests Windows-based sandboxes. The current architecture has no path for a Windows VM backend without deep changes to the lifecycle server. Similarly, Apple Containerization (macOS 26+) would need its own backend.

3. **Cloud-native backends are excluded**: Managed container services (Azure Container Instances, AWS Fargate, GCP Cloud Run) offer serverless sandbox provisioning but cannot be integrated without core modifications.

4. **OSEP-0004 solved runtime selection, not runtime pluggability**: OSEP-0004 (Pluggable Secure Container Runtime) addresses which OCI runtime class a sandbox uses (gVisor, Kata, Firecracker) within the existing Docker/K8s backends. It does not address adding entirely new backend types.

5. **OSEP-0007 hints at the need**: Fast Sandbox Runtime Support (OSEP-0007) proposes a gRPC-based controller as a new backend, but without a formal driver interface it will be another one-off integration.

### Goals

1. **Define a `RuntimeDriver` interface** that formalizes the contract between the lifecycle server and any sandbox runtime backend
2. **Introduce a driver registry** that allows drivers to be registered at startup — both built-in (Docker, Kubernetes) and external (loaded via config)
3. **Enable external drivers** to be developed, tested, and deployed independently of the OpenSandbox core release cycle
4. **Refactor existing Docker and Kubernetes backends** as the first two built-in drivers, proving the interface is sufficient
5. **Define a capability discovery mechanism** so the lifecycle server can advertise which features a given driver supports (pause/resume, snapshots, volume mounts, network policy, etc.)

### Non-Goals

1. **Implementing a Windows driver**: This OSEP defines the interface; Windows support is a separate effort that consumes this interface
2. **Changing the Sandbox Lifecycle API**: The REST API surface remains unchanged; the only addition is an optional `driver` field on `CreateSandboxRequest` for explicit driver selection
3. **Driver marketplace or distribution**: How drivers are packaged and distributed is left to individual driver authors
4. **Replacing OSEP-0004**: Secure runtime selection (gVisor/Kata) remains orthogonal — it operates within a driver, not across drivers
5. **Automatic driver inference from image**: Detecting whether an image is Linux or Windows and routing automatically is desirable but deferred to a follow-up; the initial implementation requires explicit driver selection or a configured default

## Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| R1 | Define a `RuntimeDriver` interface covering the full sandbox lifecycle (create, delete, get, list, pause, resume, renew, endpoint, status) | Must Have |
| R2 | Existing Docker and Kubernetes backends must be refactored to implement the interface with no behavior change | Must Have |
| R3 | External drivers can be registered via server configuration without modifying core code | Must Have |
| R4 | Multiple drivers can be loaded simultaneously; a default driver handles requests without explicit driver selection | Must Have |
| R5 | Drivers declare capabilities at registration time; unsupported operations return a structured error | Must Have |
| R6 | Driver startup validation: verify connectivity and prerequisites before accepting API requests | Must Have |
| R7 | `CreateSandboxRequest` accepts an optional `driver` field; omitting it uses the configured default driver | Must Have |
| R8 | `ListSandboxes` fans out across all loaded drivers and merges results | Should Have |
| R9 | The lifecycle API returns appropriate errors (501 Not Implemented) when a client requests an operation the target driver does not support | Should Have |
| R10 | External drivers communicate via a well-defined protocol (gRPC or HTTP) | Should Have |
| R11 | Driver health checks are integrated into the `/health` endpoint | Nice to Have |
| R12 | `GET /v1/drivers` endpoint lists loaded drivers and their capabilities | Nice to Have |

## Proposal

We propose extracting the implicit `SandboxService` contract into an explicit `RuntimeDriver` interface, introducing a `DriverRegistry` that supports multiple simultaneously loaded drivers, and defining a gRPC protocol for out-of-process external drivers.

```
                    Lifecycle Server
                    ┌──────────────────────────────────────┐
                    │                                      │
  POST /v1/sandboxes│   DriverRegistry                    │
  { "driver":       │   ┌──────────────────────┐          │
    "windows" }     │   │ "docker"    → Docker  │ (built-in)
  ──────────────────►   │ "kubernetes"→ K8s     │ (built-in)
                    │   │ "windows"   → gRPC ───┼──────────┼──► External Driver
                    │   │ "aci"       → gRPC ───┼──────────┼──► External Driver
                    │   └──────────────────────┘          │
                    │          │                           │
                    │    route by "driver" field           │
                    │    (or default driver if omitted)    │
                    │          │                           │
                    │          ▼                           │
                    │   target_driver.create_sandbox()     │
                    │                                      │
                    └──────────────────────────────────────┘
```

Multiple drivers are loaded at startup. Each driver has a unique name. The `CreateSandboxRequest` gains an optional `driver` field; when omitted, the configured default driver handles the request. This allows a single lifecycle server to provision both Linux and Windows sandboxes — a common requirement for platforms that support diverse worker types.

Built-in drivers (Docker, Kubernetes) are registered automatically. External drivers are registered by specifying a gRPC endpoint in the config. Cross-driver operations like `ListSandboxes` fan out to all loaded drivers and merge results, with each sandbox tagged by its driver name.

### Notes/Constraints/Caveats

1. **Interface extraction, not invention**: The `RuntimeDriver` interface is derived directly from the existing `SandboxService` ABC. This is a refactor that formalizes what already exists, not a greenfield design.

2. **Multi-driver is the default**: A platform serving diverse workers (Python on Linux, .NET on Windows, Node.js on serverless) needs multiple backends from a single API. The multi-driver design avoids forcing operators to run separate lifecycle servers per OS, which would fragment sandbox management, monitoring, and billing.

3. **Sandbox ID ownership**: When multiple drivers are loaded, the server must know which driver owns a given sandbox ID. Two approaches: (a) prefix-based IDs (e.g., `k8s-<uuid>`, `win-<uuid>`) or (b) a lightweight lookup table. The OSEP recommends prefix-based IDs for simplicity and statelessness, with the prefix being the driver name.

4. **OSEP-0004 compatibility**: Secure runtime selection (`SecureRuntimeResolver`) operates within a driver. The Docker driver and K8s driver continue to use it internally. External drivers handle their own isolation strategy.

5. **OSEP-0007 alignment**: Fast Sandbox (OSEP-0007) already proposes a gRPC-based controller. This OSEP generalizes that pattern into a standard external driver protocol, so OSEP-0007 becomes a specific driver implementation rather than a one-off integration.

6. **WorkloadProvider is untouched**: The Kubernetes `WorkloadProvider` abstraction (BatchSandbox, AgentSandbox) remains internal to the K8s driver. This OSEP operates one level above.

### Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Interface too narrow — doesn't cover future driver needs | External drivers hit limitations, fork the interface | Design interface from 3+ concrete backends (Docker, K8s, Windows prototype); include an `extensions` escape hatch for driver-specific operations |
| Interface too broad — forces simple drivers to stub many methods | High implementation burden for minimal drivers | Capability discovery: drivers declare what they support; server only routes supported operations |
| gRPC protocol adds latency for external drivers | Sandbox creation slower for external backends | gRPC is used only for the control plane (create/delete/status); data plane (execd, file ops) goes directly to the sandbox |
| Refactoring Docker/K8s breaks existing deployments | Regression in production | Refactor is internal; no config or API changes. Existing `runtime.type = "docker"` and `runtime.type = "kubernetes"` continue to work identically |
| External driver crashes or becomes unavailable | Sandbox operations fail | Health check integration (R8); driver reconnection with backoff; server returns 503 when driver is unhealthy |

## Design Details

### RuntimeDriver Interface

The interface is derived from the existing `SandboxService` ABC with minor normalization. All methods accept a context for cancellation/timeout propagation.

```python
# Illustrative — final implementation may differ

class RuntimeDriver(ABC):
    """Interface that all sandbox runtime backends must implement."""

    @abstractmethod
    async def initialize(self, config: DriverConfig) -> None:
        """Called once at server startup. Validate prerequisites, establish connections."""

    @abstractmethod
    async def shutdown(self) -> None:
        """Called at server shutdown. Release resources."""

    @abstractmethod
    async def capabilities(self) -> DriverCapabilities:
        """Return the set of operations this driver supports."""

    # --- Sandbox Lifecycle ---

    @abstractmethod
    async def create_sandbox(self, request: CreateSandboxRequest) -> SandboxInfo:
        """Provision a new sandbox and return its metadata."""

    @abstractmethod
    async def get_sandbox(self, sandbox_id: str) -> SandboxInfo:
        """Retrieve sandbox metadata by ID."""

    @abstractmethod
    async def list_sandboxes(self, options: ListOptions) -> ListSandboxesResponse:
        """List sandboxes with optional filtering and pagination."""

    @abstractmethod
    async def delete_sandbox(self, sandbox_id: str) -> None:
        """Terminate and remove a sandbox."""

    # --- Optional Operations (declared via capabilities) ---

    async def pause_sandbox(self, sandbox_id: str) -> None:
        raise NotImplementedError("pause not supported by this driver")

    async def resume_sandbox(self, sandbox_id: str) -> None:
        raise NotImplementedError("resume not supported by this driver")

    async def renew_expiration(self, sandbox_id: str, expires_at: datetime) -> datetime:
        raise NotImplementedError("renew not supported by this driver")

    # --- Endpoint Resolution ---

    @abstractmethod
    async def get_endpoint(self, sandbox_id: str, port: int) -> EndpointInfo:
        """Resolve a network endpoint for accessing a service inside the sandbox."""

    # --- Extensions ---

    async def execute_extension(self, sandbox_id: str, operation: str, payload: dict) -> dict:
        """Driver-specific operations not covered by the standard interface.
        Returns a dict response or raises NotImplementedError."""
        raise NotImplementedError(f"extension '{operation}' not supported")
```

### Driver Registry

```python
# Illustrative

class DriverRegistry:
    _builtin: dict[str, type[RuntimeDriver]] = {}
    _loaded: dict[str, RuntimeDriver] = {}
    _default: str = ""

    @classmethod
    def register_builtin(cls, name: str, driver_class: type[RuntimeDriver]) -> None:
        """Register a built-in driver class (called at import time)."""
        cls._builtin[name] = driver_class

    @classmethod
    def load_from_config(cls, config: RuntimeConfig) -> None:
        """Load all drivers from config and initialize them."""
        for driver_cfg in config.drivers:
            if driver_cfg.endpoint:
                # External driver — wrap in gRPC proxy
                driver = GrpcExternalDriver(driver_cfg)
            elif driver_cfg.name in cls._builtin:
                driver = cls._builtin[driver_cfg.name](driver_cfg)
            else:
                raise ValueError(f"Unknown driver: {driver_cfg.name}")
            await driver.initialize(driver_cfg)
            cls._loaded[driver_cfg.name] = driver
        cls._default = config.default

    @classmethod
    def get(cls, name: str | None = None) -> RuntimeDriver:
        """Get a driver by name, or the default if name is None."""
        key = name or cls._default
        if key not in cls._loaded:
            raise ValueError(f"Driver not loaded: {key}")
        return cls._loaded[key]

    @classmethod
    def all(cls) -> dict[str, RuntimeDriver]:
        """Return all loaded drivers (for fan-out operations like list)."""
        return cls._loaded


# Built-in registration
DriverRegistry.register_builtin("docker", DockerDriver)
DriverRegistry.register_builtin("kubernetes", KubernetesDriver)
```

For `CreateSandbox`, the server calls `DriverRegistry.get(request.driver)` — if the request omits `driver`, the default is used. For `ListSandboxes`, the server fans out to `DriverRegistry.all()`, merges results, and tags each sandbox with its `driver` name. For `GetSandbox`/`DeleteSandbox`, the sandbox ID encodes which driver owns it (via a prefix or a lookup table).

### Driver Lifecycle Hooks

```
Server Startup
     │
     ▼
  DriverRegistry.create(config.runtime.type)
     │
     ▼
  driver.initialize(config)     ← validate prerequisites, connect
     │
     ▼
  driver.capabilities()         ← register supported operations
     │
     ▼
  Server accepts API requests
     │
     ... (normal operation) ...
     │
     ▼
  Server Shutdown
     │
     ▼
  driver.shutdown()             ← cleanup, disconnect
```

### Configuration Schema

The `[runtime]` section is extended to support multiple drivers. The existing `type` key becomes `default` (the driver used when `CreateSandboxRequest.driver` is omitted). Each driver is configured in its own `[[runtime.drivers]]` array entry:

```toml
[runtime]
default = "kubernetes"        # driver used when request omits "driver" field

# Built-in driver — no endpoint needed
[[runtime.drivers]]
name = "kubernetes"           # matches existing runtime.type values

# Built-in driver
[[runtime.drivers]]
name = "docker"

# External driver — communicates via gRPC
[[runtime.drivers]]
name = "windows"
endpoint = "localhost:50051"   # gRPC address
tls_cert = ""                  # optional mTLS
tls_key = ""
ca_cert = ""
connect_timeout = "5s"
request_timeout = "30s"

# Another external driver
[[runtime.drivers]]
name = "aci"
endpoint = "localhost:50052"
```

**Backward compatibility**: If `[[runtime.drivers]]` is absent and `runtime.type` is set (old format), the server loads a single driver with that name as both the only driver and the default. No existing configs break.

### Capability Discovery

```python
@dataclass
class DriverCapabilities:
    pause: bool = False
    resume: bool = False
    renew: bool = False
    snapshots: bool = False
    volumes: bool = False
    network_policy: bool = False
    metrics: bool = False
    extensions: list[str] = field(default_factory=list)
```

The lifecycle server uses capabilities to:
- Return `501 Not Implemented` with a descriptive message when a client calls an unsupported operation on a specific driver
- Populate `GET /v1/drivers` (new endpoint) so SDK users can discover loaded drivers and their capabilities:

```json
{
  "default": "kubernetes",
  "drivers": [
    {
      "name": "kubernetes",
      "capabilities": { "pause": false, "resume": false, "volumes": true, "network_policy": true },
      "healthy": true
    },
    {
      "name": "windows",
      "capabilities": { "pause": true, "resume": true, "volumes": true, "network_policy": false },
      "healthy": true
    }
  ]
}
```

SDK users can call this endpoint to decide which driver to target, or rely on the default for their common case.

### External Driver Protocol

External drivers communicate via gRPC using a proto3 service definition:

```protobuf
// Illustrative — final .proto may differ

syntax = "proto3";
package opensandbox.driver.v1;

service RuntimeDriver {
  rpc Initialize(InitializeRequest) returns (InitializeResponse);
  rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
  rpc Capabilities(CapabilitiesRequest) returns (CapabilitiesResponse);

  rpc CreateSandbox(CreateSandboxRequest) returns (SandboxInfo);
  rpc GetSandbox(GetSandboxRequest) returns (SandboxInfo);
  rpc ListSandboxes(ListSandboxesRequest) returns (ListSandboxesResponse);
  rpc DeleteSandbox(DeleteSandboxRequest) returns (DeleteSandboxResponse);

  rpc PauseSandbox(PauseSandboxRequest) returns (PauseSandboxResponse);
  rpc ResumeSandbox(ResumeSandboxRequest) returns (ResumeSandboxResponse);
  rpc RenewExpiration(RenewExpirationRequest) returns (RenewExpirationResponse);

  rpc GetEndpoint(GetEndpointRequest) returns (EndpointInfo);

  rpc ExecuteExtension(ExtensionRequest) returns (ExtensionResponse);

  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}
```

The `GrpcExternalDriver` class in the lifecycle server implements `RuntimeDriver` by proxying each method to the corresponding gRPC call. This keeps the server unaware of driver internals.

## Test Plan

| Category | Scope | Approach |
|----------|-------|----------|
| Unit | `RuntimeDriver` interface contract | Test that built-in drivers satisfy the interface; mock driver for edge cases (unsupported ops, timeouts, errors) |
| Unit | `DriverRegistry` | Registration, lookup, duplicate detection, unknown driver errors |
| Unit | `GrpcExternalDriver` | Mock gRPC server; verify all methods proxy correctly, error mapping, timeout handling |
| Integration | Docker driver refactor | Run existing Docker integration tests against the refactored `DockerDriver`; diff behavior against pre-refactor baseline |
| Integration | K8s driver refactor | Run existing K8s integration tests against the refactored `KubernetesDriver` |
| Integration | External driver protocol | Spin up a test gRPC driver; verify create/delete/list/status round-trip |
| E2E | Capability-gated operations | Call pause on a driver that doesn't support it; verify 501 response |
| E2E | External driver lifecycle | Start server with external driver config; verify startup validation, operation proxying, shutdown cleanup |

## Drawbacks

1. **Abstraction cost**: Extracting the interface adds a layer of indirection to every sandbox operation. For the built-in Docker/K8s paths this is pure overhead (one extra function call) with no user-facing benefit.

2. **Interface stability pressure**: Once external drivers exist, changing the interface requires coordinating with driver authors. The interface effectively becomes a public API with backward compatibility obligations.

3. **gRPC dependency**: The external driver protocol introduces a gRPC dependency into the lifecycle server. This is a meaningful addition to the dependency tree for a feature that may have few initial consumers.

4. **Premature abstraction risk**: If only Docker and Kubernetes are used for the foreseeable future, the driver interface is YAGNI. The value only materializes when a third backend is actually built.

## Alternatives

### Alternative 1: Keep hardcoded backends, add Windows as a third

Add a `WindowsSandboxService` directly to `factory.py` alongside Docker and K8s. This is simpler and faster for the Windows case specifically, but doesn't solve the general extensibility problem and requires core changes for every new backend.

**Rejected because**: It perpetuates the pattern that OSEP-0007 (Fast Sandbox) already strains. Three hardcoded backends is manageable; five is not.

### Alternative 2: HTTP-based external driver protocol instead of gRPC

Use a REST/JSON protocol instead of gRPC for external drivers. This avoids the gRPC dependency and is easier to implement in any language.

**Not rejected — open for discussion**: gRPC was chosen for type safety, streaming support (future: log streaming), and lower latency. HTTP/JSON is a viable alternative if the community prefers simplicity over performance. The `RuntimeDriver` interface is protocol-agnostic; the transport can be swapped without changing the interface.

### Alternative 3: In-process plugin loading (shared library / Go plugin)

Load drivers as shared libraries or Go plugins at runtime. This avoids the gRPC overhead entirely.

**Rejected because**: The lifecycle server is Python. Python plugin loading (importlib, entry_points) is viable but doesn't support drivers written in other languages. Since Windows drivers may be written in Go or C#, a language-agnostic protocol is preferred.

## Infrastructure Needed

- **Proto file repository**: The `.proto` definition for the external driver protocol should live in `specs/` or a dedicated `proto/` directory
- **Test external driver**: A minimal reference driver (Go or Python) for integration testing the gRPC protocol
- **CI**: Add driver interface conformance tests to the existing CI pipeline

## Upgrade & Migration Strategy

**No breaking changes.** The refactor is internal to the lifecycle server:

1. `runtime.type = "docker"` continues to work — `DockerDriver` is registered as a built-in driver with the same name
2. `runtime.type = "kubernetes"` continues to work — `KubernetesDriver` is registered as a built-in driver with the same name
3. All existing configuration keys are preserved
4. The `SandboxService` ABC can be kept as a deprecated alias for `RuntimeDriver` during the transition period
5. New `[runtime.external]` config is purely additive — existing configs that don't use it are unaffected
