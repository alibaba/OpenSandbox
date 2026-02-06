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

from src import config as config_module
from src.config import (
    AppConfig,
    GatewayConfig,
    GatewayRouteModeConfig,
    IngressConfig,
    RuntimeConfig,
    ServerConfig,
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
