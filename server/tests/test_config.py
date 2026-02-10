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
from src.config import AppConfig, RouterConfig, RuntimeConfig, ServerConfig, StorageConfig


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
        type = "docker"
        execd_image = "ghcr.io/opensandbox/platform:test"

        [router]
        domain = "opensandbox.io"
        """
    )
    config_path = tmp_path / "config.toml"
    config_path.write_text(toml)

    loaded = config_module.load_config(config_path)
    assert loaded.server.host == "127.0.0.1"
    assert loaded.server.port == 9000
    assert loaded.server.log_level == "DEBUG"
    assert loaded.server.api_key == "secret"
    assert loaded.runtime.type == "docker"
    assert loaded.runtime.execd_image == "ghcr.io/opensandbox/platform:test"
    assert loaded.router is not None
    assert loaded.router.domain == "opensandbox.io"
    assert loaded.docker.network_mode == "host"


def test_docker_runtime_disallows_kubernetes_block():
    server_cfg = ServerConfig()
    runtime_cfg = RuntimeConfig(type="docker", execd_image="busybox:latest")
    kubernetes_cfg = config_module.KubernetesRuntimeConfig(namespace="sandbox")
    with pytest.raises(ValueError):
        AppConfig(server=server_cfg, runtime=runtime_cfg, kubernetes=kubernetes_cfg)


def test_kubernetes_runtime_fills_missing_block():
    server_cfg = ServerConfig()
    runtime_cfg = RuntimeConfig(type="kubernetes", execd_image="ghcr.io/opensandbox/platform:latest")
    app_cfg = AppConfig(server=server_cfg, runtime=runtime_cfg)
    assert app_cfg.kubernetes is not None


def test_router_requires_exactly_one_domain():
    with pytest.raises(ValueError):
        RouterConfig(domain=None, wildcard_domain=None)
    with pytest.raises(ValueError):
        RouterConfig(domain="opensandbox.io", wildcard_domain="*.opensandbox.io")
    cfg = RouterConfig(domain="opensandbox.io")
    assert cfg.domain == "opensandbox.io"


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
