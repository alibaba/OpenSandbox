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

"""
Application configuration management for sandbox server.

Loads configuration from a TOML file (default: ~/.sandbox.toml) and exposes
helpers to access the parsed settings throughout the application.
"""

from __future__ import annotations

import ipaddress
import logging
import os
import re
from pathlib import Path
from typing import Any, Literal, Optional

from pydantic import BaseModel, Field, ValidationError, model_validator

try:  # Python 3.11+
    import tomllib  # type: ignore[attr-defined]
except ModuleNotFoundError:  # Python 3.10 fallback
    import tomli as tomllib  # type: ignore[import]

logger = logging.getLogger(__name__)

CONFIG_ENV_VAR = "SANDBOX_CONFIG_PATH"
DEFAULT_CONFIG_PATH = Path.home() / ".sandbox.toml"

_DOMAIN_RE = re.compile(r"^(?=.{1,253}$)(?!-)[A-Za-z0-9-]{1,63}(?:\.[A-Za-z0-9-]{1,63})+$")
_WILDCARD_DOMAIN_RE = re.compile(r"^\*\.(?!-)[A-Za-z0-9-]{1,63}(?:\.[A-Za-z0-9-]{1,63})+$")
_IPV4_WITH_PORT_RE = re.compile(r"^(?P<ip>(?:\d{1,3}\.){3}\d{1,3})(?::(?P<port>\d{1,5}))?$")

INGRESS_MODE_DIRECT = "direct"
INGRESS_MODE_GATEWAY = "gateway"
GATEWAY_ROUTE_MODE_WILDCARD = "wildcard"
GATEWAY_ROUTE_MODE_HEADER = "header"
GATEWAY_ROUTE_MODE_URI = "uri"


def _is_valid_ip(host: str) -> bool:
    try:
        ipaddress.ip_address(host)
        return True
    except ValueError:
        return False


def _is_valid_ip_or_ip_port(address: str) -> bool:
    match = _IPV4_WITH_PORT_RE.match(address)
    if not match:
        return False
    ip_str = match.group("ip")
    if not _is_valid_ip(ip_str):
        return False
    port_str = match.group("port")
    if port_str is None:
        return True
    try:
        port = int(port_str)
    except ValueError:
        return False
    return 1 <= port <= 65535


def _is_valid_domain(host: str) -> bool:
    return bool(_DOMAIN_RE.match(host))


def _is_wildcard_domain(host: str) -> bool:
    return bool(_WILDCARD_DOMAIN_RE.match(host))


class GatewayRouteModeConfig(BaseModel):
    """Routing strategy for gateway ingress exposure."""

    mode: Literal[
        GATEWAY_ROUTE_MODE_WILDCARD,
        GATEWAY_ROUTE_MODE_HEADER,
        GATEWAY_ROUTE_MODE_URI,
    ] = Field(
        ...,
        description="Routing mode used by the gateway (wildcard, header, uri).",
    )

    class Config:
        populate_by_name = True


class GatewayConfig(BaseModel):
    """Gateway mode configuration for ingress exposure."""

    address: str = Field(
        ...,
        description="Gateway host used to expose sandboxes (domain or IP, may include :port; scheme is not allowed).",
        min_length=1,
    )
    route: GatewayRouteModeConfig = Field(
        ...,
        description="Routing mode configuration used by the gateway.",
    )


class IngressConfig(BaseModel):
    """Configuration for exposing sandbox ingress."""

    mode: Literal[INGRESS_MODE_DIRECT, INGRESS_MODE_GATEWAY] = Field(
        default=INGRESS_MODE_DIRECT,
        description="Ingress exposure mode (direct or gateway).",
    )
    gateway: Optional[GatewayConfig] = Field(
        default=None,
        description="Gateway configuration required when mode = 'gateway'.",
    )

    @model_validator(mode="after")
    def validate_ingress_mode(self) -> "IngressConfig":
        if self.mode == INGRESS_MODE_GATEWAY and self.gateway is None:
            raise ValueError("gateway block must be provided when ingress.mode = 'gateway'.")
        if self.mode == INGRESS_MODE_DIRECT and self.gateway is not None:
            raise ValueError("gateway block must be omitted unless ingress.mode = 'gateway'.")

        if self.mode == INGRESS_MODE_GATEWAY and self.gateway:
            route_mode = self.gateway.route.mode
            address_raw = self.gateway.address
            hostport = address_raw
            if "://" in address_raw:
                raise ValueError("ingress.gateway.address must not include a scheme; clients choose http/https.")

            if route_mode == GATEWAY_ROUTE_MODE_WILDCARD:
                if not _is_wildcard_domain(hostport):
                    raise ValueError(
                        "ingress.gateway.address must be a wildcard domain (e.g., *.example.com) "
                        "when gateway.route.mode is wildcard."
                    )
            else:
                if "*" in hostport:
                    raise ValueError(
                        "ingress.gateway.address must not contain wildcard when gateway.route.mode is not wildcard."
                    )
                if not (_is_valid_domain(hostport) or _is_valid_ip_or_ip_port(hostport)):
                    raise ValueError(
                        "ingress.gateway.address must be a valid domain, IP, or IP:port when gateway.route.mode is not wildcard."
                    )
        return self


class ServerConfig(BaseModel):
    """FastAPI server configuration."""

    host: str = Field(
        default="0.0.0.0",
        description="Interface bound by the lifecycle API server.",
        min_length=1,
    )
    port: int = Field(
        default=8080,
        ge=1,
        le=65535,
        description="Port exposed by the lifecycle API server.",
    )
    log_level: str = Field(
        default="INFO",
        description="Python logging level for the server process.",
        min_length=3,
    )
    api_key: Optional[str] = Field(
        default=None,
        description="Global API key for authenticating incoming lifecycle API calls.",
    )


class KubernetesRuntimeConfig(BaseModel):
    """Kubernetes-specific runtime configuration."""

    kubeconfig_path: Optional[str] = Field(
        default=None,
        description="Absolute path to the kubeconfig file used for API authentication.",
    )
    informer_enabled: bool = Field(
        default=True,
        description=(
            "[Beta] Enable informer-backed cache for workload reads. "
            "Keeps a watch to reduce API pressure; set false to disable."
        ),
    )
    informer_resync_seconds: int = Field(
        default=300,
        ge=1,
        description=(
            "[Beta] Full resync interval for informer cache (seconds). "
            "Shorter intervals refresh the cache more eagerly."
        ),
    )
    informer_watch_timeout_seconds: int = Field(
        default=60,
        ge=1,
        description=(
            "[Beta] Watch timeout (seconds) before restarting the informer stream."
        ),
    )
    namespace: Optional[str] = Field(
        default=None,
        description="Namespace used for sandbox workloads.",
    )
    service_account: Optional[str] = Field(
        default=None,
        description="Service account bound to sandbox workloads.",
    )
    workload_provider: Optional[str] = Field(
        default=None,
        description="Workload provider type. If not specified, uses the first registered provider.",
    )
    batchsandbox_template_file: Optional[str] = Field(
        default=None,
        description="Path to BatchSandbox CR YAML template file. Used when workload_provider is 'batchsandbox'.",
    )


class AgentSandboxRuntimeConfig(BaseModel):
    """Agent-sandbox runtime configuration."""

    template_file: Optional[str] = Field(
        default=None,
        description="Path to Sandbox CR YAML template file for agent-sandbox.",
    )
    shutdown_policy: Literal["Delete", "Retain"] = Field(
        default="Delete",
        description="Shutdown policy applied when a sandbox expires (Delete or Retain).",
    )
    ingress_enabled: bool = Field(
        default=True,
        description="Whether ingress routing to agent-sandbox pods is expected to be enabled.",
    )


class StorageConfig(BaseModel):
    """Volume and storage configuration for sandbox mounts."""

    allowed_host_paths: list[str] = Field(
        default_factory=list,
        description=(
            "Allowlist of host path prefixes permitted for host bind mounts. "
            "If empty, all host paths are allowed (not recommended for production). "
            "Each entry must be an absolute path (e.g., '/data/opensandbox')."
        ),
    )


class EgressConfig(BaseModel):
    """Egress sidecar configuration."""

    image: Optional[str] = Field(
        default=None,
        description="Container image for the egress sidecar (used when network policy is requested).",
        min_length=1,
    )


class RuntimeConfig(BaseModel):
    """Runtime selection (docker, kubernetes, etc.)."""

    type: Literal["docker", "kubernetes"] = Field(
        ...,
        description="Active sandbox runtime implementation.",
    )
    execd_image: str = Field(
        ...,
        description="Container image that contains the execd binary for sandbox initialization.",
        min_length=1,
    )


class DockerConfig(BaseModel):
    """Docker runtime specific settings."""

    network_mode: Literal["host", "bridge"] = Field(
        default="host",
        description="Docker network mode for sandbox containers (host, bridge, ...).",
    )
    drop_capabilities: list[str] = Field(
        default_factory=lambda: [
            "AUDIT_WRITE",
            "MKNOD",
            "NET_ADMIN",
            "NET_RAW",
            "SYS_ADMIN",
            "SYS_MODULE",
            "SYS_PTRACE",
            "SYS_TIME",
            "SYS_TTY_CONFIG",
        ],
        description=(
            "Linux capabilities to drop from sandbox containers. Defaults to a conservative set to reduce host impact."
        ),
    )
    apparmor_profile: Optional[str] = Field(
        default=None,
        description=(
            "Optional AppArmor profile name applied to sandbox containers. Leave unset to let Docker choose the default."
        ),
    )
    no_new_privileges: bool = Field(
        default=True,
        description="Enable the kernel no_new_privileges flag to block privilege escalation inside the container.",
    )
    seccomp_profile: Optional[str] = Field(
        default=None,
        description=(
            "Optional seccomp profile name or path applied to sandbox containers. Leave unset to use Docker's default profile."
        ),
    )
    pids_limit: Optional[int] = Field(
        default=512,
        ge=1,
        description="Maximum number of processes allowed per sandbox container. Set to null to disable the limit.",
    )


class AppConfig(BaseModel):
    """Root application configuration model."""

    server: ServerConfig = Field(default_factory=ServerConfig)
    runtime: RuntimeConfig = Field(..., description="Sandbox runtime configuration.")
    kubernetes: Optional[KubernetesRuntimeConfig] = None
    agent_sandbox: Optional["AgentSandboxRuntimeConfig"] = None
    ingress: Optional[IngressConfig] = None
    docker: DockerConfig = Field(default_factory=DockerConfig)
    storage: StorageConfig = Field(default_factory=StorageConfig)
    egress: Optional[EgressConfig] = None

    @model_validator(mode="after")
    def validate_runtime_blocks(self) -> "AppConfig":
        if self.runtime.type == "docker":
            if self.kubernetes is not None:
                raise ValueError("Kubernetes block must be omitted when runtime.type = 'docker'.")
            if self.agent_sandbox is not None:
                raise ValueError("agent_sandbox block must be omitted when runtime.type = 'docker'.")
            if self.ingress is not None and self.ingress.mode != INGRESS_MODE_DIRECT:
                raise ValueError("ingress.mode must be 'direct' when runtime.type = 'docker'.")
        elif self.runtime.type == "kubernetes":
            if self.kubernetes is None:
                self.kubernetes = KubernetesRuntimeConfig()
            provider_type = (self.kubernetes.workload_provider or "").lower()
            if provider_type == "agent-sandbox":
                if self.agent_sandbox is None:
                    self.agent_sandbox = AgentSandboxRuntimeConfig()
            elif self.agent_sandbox is not None:
                raise ValueError(
                    "agent_sandbox block requires kubernetes.workload_provider = 'agent-sandbox'."
                )
        else:
            raise ValueError(f"Unsupported runtime type '{self.runtime.type}'.")
        return self


_config: AppConfig | None = None
_config_path: Path | None = None


def _resolve_config_path(path: str | Path | None = None) -> Path:
    """Resolve configuration file path from explicit value, env var, or default."""
    if path:
        return Path(path).expanduser()
    env_path = os.environ.get(CONFIG_ENV_VAR)
    if env_path:
        return Path(env_path).expanduser()
    return DEFAULT_CONFIG_PATH


def _load_toml_data(path: Path) -> dict[str, Any]:
    """Load TOML content from file, returning empty dict if file is missing."""
    if not path.exists():
        logger.info("Config file %s not found. Using default configuration.", path)
        return {}

    try:
        with path.open("rb") as fh:
            data = tomllib.load(fh)
            logger.info("Loaded configuration from %s", path)
            return data
    except Exception as exc:  # noqa: BLE001
        logger.error("Failed to read config file %s: %s", path, exc)
        raise


def load_config(path: str | Path | None = None) -> AppConfig:
    """
    Load configuration from TOML file and store it globally.

    Args:
        path: Optional explicit config path. Falls back to SANDBOX_CONFIG_PATH env,
              then ~/.sandbox.toml when not provided.

    Returns:
        AppConfig: Parsed application configuration.

    Raises:
        ValidationError: If the TOML contents do not match AppConfig schema.
        Exception: For any IO or parsing errors.
    """
    global _config, _config_path

    resolved_path = _resolve_config_path(path)
    raw_data = _load_toml_data(resolved_path)

    try:
        _config = AppConfig(**raw_data)
    except ValidationError as exc:
        logger.error("Invalid configuration in %s: %s", resolved_path, exc)
        raise

    _config_path = resolved_path
    return _config


def get_config() -> AppConfig:
    """
    Retrieve the currently loaded configuration, loading defaults if necessary.

    Returns:
        AppConfig: Currently active configuration.
    """
    global _config
    if _config is None:
        _config = load_config()
    return _config


def get_config_path() -> Path:
    """Return the resolved configuration path."""
    global _config_path
    if _config_path is None:
        _config_path = _resolve_config_path()
    return _config_path


__all__ = [
    "AppConfig",
    "ServerConfig",
    "RuntimeConfig",
    "IngressConfig",
    "GatewayConfig",
    "GatewayRouteModeConfig",
    "INGRESS_MODE_DIRECT",
    "INGRESS_MODE_GATEWAY",
    "DockerConfig",
    "StorageConfig",
    "KubernetesRuntimeConfig",
    "EgressConfig",
    "DEFAULT_CONFIG_PATH",
    "CONFIG_ENV_VAR",
    "get_config",
    "get_config_path",
    "load_config",
]
