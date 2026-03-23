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

import textwrap

import pytest
from pydantic import ValidationError

from src import config as config_module
from src.config import (
    AppConfig,
    RenewIntentRedisConfig,
    EGRESS_MODE_DNS,
    EGRESS_MODE_DNS_NFT,
    EgressConfig,
    GatewayConfig,
    GatewayRouteModeConfig,
    IngressConfig,
    RuntimeConfig,
    ServerConfig,
    StorageConfig,
)


def _reset_config(monkeypatch):
    monkeypatch.setattr(config_module, "_config", None, raising=False)
    monkeypatch.setattr(config_module, "_config_path", None, raising=False)


def test_load_config_from_file(tmp_path, monkeypatch):
    _reset_config(monkeypatch)
    toml = textwrap.dedent(
        """
        [server]
        host = "127.0.0.1"
        port = 9000
        log_level = "DEBUG"
        api_key = "secret"
        max_sandbox_timeout_seconds = 172800

        [runtime]
        type = "kubernetes"
        execd_image = "opensandbox/execd:test"

        [ingress]
        mode = "gateway"
        gateway.address = "*.opensandbox.io"
        gateway.route.mode = "wildcard"
        """
    )
    config_path = tmp_path / "config.toml"
    config_path.write_text(toml)

    loaded = config_module.load_config(config_path)
    assert loaded.server.host == "127.0.0.1"
    assert loaded.server.port == 9000
    assert loaded.server.log_level == "DEBUG"
    assert loaded.server.api_key == "secret"
    assert loaded.server.max_sandbox_timeout_seconds == 172800
    assert loaded.runtime.type == "kubernetes"
    assert loaded.runtime.execd_image == "opensandbox/execd:test"
    assert loaded.ingress is not None
    assert loaded.ingress.mode == "gateway"
    assert loaded.ingress.gateway is not None
    assert loaded.ingress.gateway.address == "*.opensandbox.io"
    assert loaded.ingress.gateway.route.mode == "wildcard"
    assert loaded.kubernetes is not None


def test_docker_runtime_disallows_kubernetes_block():
    server_cfg = ServerConfig()
    runtime_cfg = RuntimeConfig(type="docker", execd_image="busybox:latest")
    kubernetes_cfg = config_module.KubernetesRuntimeConfig(namespace="sandbox")
    with pytest.raises(ValueError):
        AppConfig(server=server_cfg, runtime=runtime_cfg, kubernetes=kubernetes_cfg)


def test_server_config_defaults_include_max_sandbox_timeout():
    server_cfg = ServerConfig()
    assert server_cfg.max_sandbox_timeout_seconds is None


def test_renew_intent_defaults():
    cfg = AppConfig(runtime=RuntimeConfig(type="docker", execd_image="opensandbox/execd:latest"))
    ar = cfg.renew_intent
    assert ar.enabled is False
    assert ar.min_interval_seconds == 60
    assert ar.redis.enabled is False
    assert ar.redis.dsn is None
    assert ar.redis.queue_key == "opensandbox:renew:intent"
    assert ar.redis.consumer_concurrency == 8


def test_renew_intent_redis_requires_dsn_when_enabled():
    with pytest.raises(ValidationError):
        RenewIntentRedisConfig(enabled=True, dsn=None)
    with pytest.raises(ValidationError):
        RenewIntentRedisConfig(enabled=True, dsn="   ")
    cfg = RenewIntentRedisConfig(enabled=True, dsn="redis://127.0.0.1:6379/0")
    assert cfg.dsn == "redis://127.0.0.1:6379/0"


def test_load_config_renew_intent_nested(tmp_path, monkeypatch):
    _reset_config(monkeypatch)
    toml = textwrap.dedent(
        """
        [server]
        host = "127.0.0.1"
        port = 9000

        [renew_intent]
        enabled = true
        min_interval_seconds = 30

        [renew_intent.redis]
        enabled = true
        dsn = "redis://example:6379/1"
        queue_key = "custom:renew"
        consumer_concurrency = 4

        [runtime]
        type = "docker"
        execd_image = "opensandbox/execd:test"
        """
    )
    config_path = tmp_path / "config.toml"
    config_path.write_text(toml)

    loaded = config_module.load_config(config_path)
    ar = loaded.renew_intent
    assert ar.enabled is True
    assert ar.min_interval_seconds == 30
    assert ar.redis.enabled is True
    assert ar.redis.dsn == "redis://example:6379/1"
    assert ar.redis.queue_key == "custom:renew"
    assert ar.redis.consumer_concurrency == 4


def test_kubernetes_runtime_fills_missing_block():
    server_cfg = ServerConfig()
    runtime_cfg = RuntimeConfig(type="kubernetes", execd_image="opensandbox/execd:latest")
    app_cfg = AppConfig(server=server_cfg, runtime=runtime_cfg)
    assert app_cfg.kubernetes is not None


def test_ingress_gateway_requires_gateway_block():
    with pytest.raises(ValueError):
        IngressConfig(mode="gateway")
    cfg = IngressConfig(
        mode="gateway",
        gateway=GatewayConfig(
            address="gateway.opensandbox.io",
            route=GatewayRouteModeConfig(mode="uri"),
        ),
    )
    assert cfg.gateway.route.mode == "uri"


def test_gateway_address_validation_for_wildcard_mode():
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="gateway.opensandbox.io",
                route=GatewayRouteModeConfig(mode="wildcard"),
            ),
        )
    cfg = IngressConfig(
        mode="gateway",
        gateway=GatewayConfig(
            address="*.opensandbox.io",
            route=GatewayRouteModeConfig(mode="wildcard"),
        ),
    )
    assert cfg.gateway.address == "*.opensandbox.io"
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="10.0.0.1",
                route=GatewayRouteModeConfig(mode="wildcard"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="http://10.0.0.1:8080",
                route=GatewayRouteModeConfig(mode="wildcard"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="10.0.0.1:8080",
                route=GatewayRouteModeConfig(mode="wildcard"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="https://*.opensandbox.io",
                route=GatewayRouteModeConfig(mode="wildcard"),
            ),
        )


def test_gateway_route_mode_allows_wildcard_alias():
    cfg = IngressConfig(
        mode="gateway",
        gateway=GatewayConfig(
            address="*.opensandbox.io",
            route=GatewayRouteModeConfig(mode="wildcard"),
        ),
    )
    assert cfg.gateway.route.mode == "wildcard"


def test_gateway_address_validation_for_non_wildcard_mode():
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="*.opensandbox.io",
                route=GatewayRouteModeConfig(mode="header"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="not a host",
                route=GatewayRouteModeConfig(mode="uri"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="gateway.opensandbox.io:8080",
                route=GatewayRouteModeConfig(mode="header"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="10.0.0.1:70000",
                route=GatewayRouteModeConfig(mode="header"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="ftp://gateway.opensandbox.io",
                route=GatewayRouteModeConfig(mode="header"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="http://",
                route=GatewayRouteModeConfig(mode="header"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="http://user:pass@gateway.opensandbox.io",
                route=GatewayRouteModeConfig(mode="header"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="http://gateway.opensandbox.io:8080",
                route=GatewayRouteModeConfig(mode="header"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="10.0.0.1:0",
                route=GatewayRouteModeConfig(mode="uri"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="10.0.0.1:abc",
                route=GatewayRouteModeConfig(mode="uri"),
            ),
        )
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="http://[::1]",
                route=GatewayRouteModeConfig(mode="header"),
            ),
        )
    cfg = IngressConfig(
        mode="gateway",
        gateway=GatewayConfig(
            address="gateway.opensandbox.io",
            route=GatewayRouteModeConfig(mode="uri"),
        ),
    )
    assert cfg.gateway.address == "gateway.opensandbox.io"
    cfg_ip = IngressConfig(
        mode="gateway",
        gateway=GatewayConfig(
            address="10.0.0.1",
            route=GatewayRouteModeConfig(mode="header"),
        ),
    )
    assert cfg_ip.gateway.address == "10.0.0.1"
    cfg_ip_port = IngressConfig(
        mode="gateway",
        gateway=GatewayConfig(
            address="10.0.0.1:8080",
            route=GatewayRouteModeConfig(mode="header"),
        ),
    )
    assert cfg_ip_port.gateway.address == "10.0.0.1:8080"


def test_gateway_address_allows_scheme_less_defaults():
    cfg = IngressConfig(
        mode="gateway",
        gateway=GatewayConfig(
            address="*.example.com",
            route=GatewayRouteModeConfig(mode="wildcard"),
        ),
    )
    assert cfg.gateway.address == "*.example.com"
    with pytest.raises(ValueError):
        IngressConfig(
            mode="gateway",
            gateway=GatewayConfig(
                address="https://*.example.com",
                route=GatewayRouteModeConfig(mode="wildcard"),
            ),
        )


def test_direct_mode_rejects_gateway_block():
    with pytest.raises(ValueError):
        IngressConfig(
            mode="direct",
            gateway=GatewayConfig(
                address="gateway.opensandbox.io",
                route=GatewayRouteModeConfig(mode="header"),
            ),
        )


def test_docker_runtime_rejects_gateway_ingress():
    server_cfg = ServerConfig()
    runtime_cfg = RuntimeConfig(type="docker", execd_image="busybox:latest")
    with pytest.raises(ValueError):
        AppConfig(
            server=server_cfg,
            runtime=runtime_cfg,
            ingress=IngressConfig(
                mode="gateway",
                gateway=GatewayConfig(
                    address="gateway.opensandbox.io",
                    route=GatewayRouteModeConfig(mode="header"),
                ),
            ),
        )
    # direct remains valid
    app_cfg = AppConfig(
        server=server_cfg,
        runtime=runtime_cfg,
        ingress=IngressConfig(mode="direct"),
    )
    assert app_cfg.ingress.mode == "direct"


# ============================================================================
# StorageConfig Tests
# ============================================================================


def test_storage_config_defaults():
    """StorageConfig should default to empty allowed_host_paths list."""
    cfg = StorageConfig()
    assert cfg.allowed_host_paths == []


def test_storage_config_with_paths():
    """StorageConfig should accept explicit allowed_host_paths."""
    cfg = StorageConfig(allowed_host_paths=["/data/opensandbox", "/tmp/sandbox"])
    assert cfg.allowed_host_paths == ["/data/opensandbox", "/tmp/sandbox"]


def test_app_config_default_storage():
    """AppConfig should include default StorageConfig when not specified."""
    server_cfg = ServerConfig()
    runtime_cfg = RuntimeConfig(type="docker", execd_image="busybox:latest")
    app_cfg = AppConfig(server=server_cfg, runtime=runtime_cfg)
    assert app_cfg.storage is not None
    assert app_cfg.storage.allowed_host_paths == []


def test_load_config_with_storage_block(tmp_path, monkeypatch):
    """StorageConfig should be loaded from [storage] TOML block."""
    _reset_config(monkeypatch)
    toml = textwrap.dedent(
        """
        [server]
        host = "127.0.0.1"
        port = 9000

        [runtime]
        type = "docker"
        execd_image = "ghcr.io/opensandbox/platform:test"

        [router]
        domain = "opensandbox.io"

        [storage]
        allowed_host_paths = ["/data/opensandbox", "/tmp/sandbox"]
        """
    )
    config_path = tmp_path / "config.toml"
    config_path.write_text(toml)

    loaded = config_module.load_config(config_path)
    assert loaded.storage is not None
    assert loaded.storage.allowed_host_paths == ["/data/opensandbox", "/tmp/sandbox"]


def test_load_config_without_storage_block_uses_defaults(tmp_path, monkeypatch):
    """AppConfig should use default StorageConfig when [storage] is not in TOML."""
    _reset_config(monkeypatch)
    toml = textwrap.dedent(
        """
        [server]
        host = "127.0.0.1"
        port = 9000

        [runtime]
        type = "docker"
        execd_image = "ghcr.io/opensandbox/platform:test"

        [router]
        domain = "opensandbox.io"
        """
    )
    config_path = tmp_path / "config.toml"
    config_path.write_text(toml)

    loaded = config_module.load_config(config_path)
    assert loaded.storage is not None
    assert loaded.storage.allowed_host_paths == []


# ============================================================================
# SecureRuntimeConfig Tests
# ============================================================================


def test_secure_runtime_empty_type_is_valid():
    """Empty type (default runc) should be valid."""
    cfg = config_module.SecureRuntimeConfig(type="")
    assert cfg.type == ""
    assert cfg.docker_runtime is None
    assert cfg.k8s_runtime_class is None


def test_secure_runtime_gvisor_with_docker_runtime_is_valid():
    """gVisor with docker_runtime should be valid."""
    cfg = config_module.SecureRuntimeConfig(
        type="gvisor",
        docker_runtime="runsc",
        k8s_runtime_class="gvisor",
    )
    assert cfg.type == "gvisor"
    assert cfg.docker_runtime == "runsc"
    assert cfg.k8s_runtime_class == "gvisor"


def test_secure_runtime_gvisor_with_k8s_runtime_class_is_valid():
    """gVisor with only k8s_runtime_class should be valid."""
    cfg = config_module.SecureRuntimeConfig(
        type="gvisor",
        docker_runtime=None,
        k8s_runtime_class="gvisor",
    )
    assert cfg.type == "gvisor"
    assert cfg.docker_runtime is None
    assert cfg.k8s_runtime_class == "gvisor"


def test_secure_runtime_kata_with_runtimes_is_valid():
    """Kata with both runtimes should be valid."""
    cfg = config_module.SecureRuntimeConfig(
        type="kata",
        docker_runtime="kata-runtime",
        k8s_runtime_class="kata-qemu",
    )
    assert cfg.type == "kata"
    assert cfg.docker_runtime == "kata-runtime"
    assert cfg.k8s_runtime_class == "kata-qemu"


def test_secure_runtime_firecracker_with_k8s_runtime_is_valid():
    """Firecracker with k8s_runtime_class should be valid."""
    cfg = config_module.SecureRuntimeConfig(
        type="firecracker",
        docker_runtime="",
        k8s_runtime_class="kata-fc",
    )
    assert cfg.type == "firecracker"
    assert cfg.docker_runtime == ""
    assert cfg.k8s_runtime_class == "kata-fc"


def test_secure_runtime_firecracker_without_k8s_runtime_raises_error():
    """Firecracker without k8s_runtime_class should raise error."""
    with pytest.raises(ValueError) as exc:
        config_module.SecureRuntimeConfig(
            type="firecracker",
            docker_runtime="",
            k8s_runtime_class=None,
        )
    assert "k8s_runtime_class" in str(exc.value).lower()


def test_secure_runtime_gvisor_without_any_runtime_raises_error():
    """gVisor without any runtime configured should raise error."""
    with pytest.raises(ValueError) as exc:
        config_module.SecureRuntimeConfig(
            type="gvisor",
            docker_runtime=None,
            k8s_runtime_class=None,
        )
    assert "docker_runtime" in str(exc.value).lower() or "k8s_runtime_class" in str(exc.value).lower()


def test_secure_runtime_kata_without_any_runtime_raises_error():
    """Kata without any runtime configured should raise error."""
    with pytest.raises(ValueError) as exc:
        config_module.SecureRuntimeConfig(
            type="kata",
            docker_runtime=None,
            k8s_runtime_class=None,
        )
    assert "docker_runtime" in str(exc.value).lower() or "k8s_runtime_class" in str(exc.value).lower()


def test_secure_runtime_invalid_type_raises_error():
    """Invalid type should raise ValidationError."""
    with pytest.raises(Exception):
        config_module.SecureRuntimeConfig(type="invalid_runtime")


def test_app_config_with_secure_runtime():
    """AppConfig should parse secure_runtime section."""
    cfg = AppConfig(
        runtime={"type": "docker", "execd_image": "execd:v1"},
        secure_runtime={
            "type": "gvisor",
            "docker_runtime": "runsc",
            "k8s_runtime_class": "gvisor",
        },
    )
    assert cfg.secure_runtime is not None
    assert cfg.secure_runtime.type == "gvisor"
    assert cfg.secure_runtime.docker_runtime == "runsc"


def test_app_config_without_secure_runtime():
    """AppConfig without secure_runtime should have None."""
    cfg = AppConfig(
        runtime={"type": "docker", "execd_image": "execd:v1"},
    )
    assert cfg.secure_runtime is None


def test_load_config_with_secure_runtime(tmp_path, monkeypatch):
    """SecureRuntimeConfig should be loaded from [secure_runtime] TOML block."""
    _reset_config(monkeypatch)
    toml = textwrap.dedent(
        """
        [server]
        host = "127.0.0.1"
        port = 9000

        [runtime]
        type = "docker"
        execd_image = "ghcr.io/opensandbox/platform:test"

        [secure_runtime]
        type = "gvisor"
        docker_runtime = "runsc"
        k8s_runtime_class = "gvisor"
        """
    )
    config_path = tmp_path / "config.toml"
    config_path.write_text(toml)

    loaded = config_module.load_config(config_path)
    assert loaded.secure_runtime is not None
    assert loaded.secure_runtime.type == "gvisor"
    assert loaded.secure_runtime.docker_runtime == "runsc"
    assert loaded.secure_runtime.k8s_runtime_class == "gvisor"


def test_docker_runtime_with_firecracker_raises_error():
    """Docker runtime with Firecracker secure runtime should raise error.

    Firecracker (kata-fc) is only available as a Kubernetes RuntimeClass,
    not as a Docker OCI runtime. This test prevents the silent fallback
    to runc which would bypass the intended microVM isolation.
    """
    with pytest.raises(ValueError) as exc:
        AppConfig(
            runtime={"type": "docker", "execd_image": "execd:v1"},
            secure_runtime={
                "type": "firecracker",
                "k8s_runtime_class": "kata-fc",
            },
        )
    assert "firecracker" in str(exc.value).lower()
    assert "kubernetes" in str(exc.value).lower()


def test_kubernetes_runtime_with_firecracker_is_valid():
    """Kubernetes runtime with Firecracker should be valid."""
    cfg = AppConfig(
        runtime={"type": "kubernetes", "execd_image": "execd:v1"},
        kubernetes={"namespace": "default"},
        secure_runtime={
            "type": "firecracker",
            "k8s_runtime_class": "kata-fc",
        },
    )
    assert cfg.runtime.type == "kubernetes"
    assert cfg.secure_runtime is not None
    assert cfg.secure_runtime.type == "firecracker"
    assert cfg.secure_runtime.k8s_runtime_class == "kata-fc"


def test_egress_config_mode_literal():
    assert EgressConfig(image="opensandbox/egress:v1").mode == EGRESS_MODE_DNS
    cfg = EgressConfig(image="opensandbox/egress:v1", mode=EGRESS_MODE_DNS_NFT)
    assert cfg.mode == EGRESS_MODE_DNS_NFT
