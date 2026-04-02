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

"""Tests for PodmanSandboxService."""

import os
from unittest.mock import MagicMock, patch

import pytest

from opensandbox_server.config import (
    AppConfig,
    IngressConfig,
    PodmanConfig,
    RuntimeConfig,
    ServerConfig,
)
from opensandbox_server.services.podman import PodmanSandboxService, _check_windows_pipe


def _podman_config(**overrides) -> AppConfig:
    defaults = dict(
        server=ServerConfig(),
        runtime=RuntimeConfig(type="podman", execd_image="ghcr.io/opensandbox/platform:latest"),
        ingress=IngressConfig(mode="direct"),
    )
    defaults.update(overrides)
    return AppConfig(**defaults)


def _mock_docker():
    """Return a MagicMock that behaves like ``docker.from_env()``."""
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_client.api.get_adapter.return_value = MagicMock()
    return mock_client


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_podman_service_init_succeeds(mock_podman_docker, mock_docker):
    """PodmanSandboxService can be constructed with runtime.type='podman'."""
    mock_client = _mock_docker()
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    service = PodmanSandboxService(config=_podman_config())
    assert service is not None
    assert service.app_config.runtime.type == "podman"


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_podman_service_rejects_docker_type(mock_podman_docker, mock_docker):
    """PodmanSandboxService must reject runtime.type='docker'."""
    mock_client = _mock_docker()
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    config = AppConfig(
        server=ServerConfig(),
        runtime=RuntimeConfig(type="docker", execd_image="test:latest"),
        ingress=IngressConfig(mode="direct"),
    )
    with pytest.raises(ValueError, match="PodmanSandboxService"):
        PodmanSandboxService(config=config)


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_update_container_labels_is_noop(mock_podman_docker, mock_docker):
    """_update_container_labels must NOT call container.update on Podman."""
    mock_client = _mock_docker()
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    service = PodmanSandboxService(config=_podman_config())

    mock_container = MagicMock()
    mock_container.id = "abc123def456"
    service._update_container_labels(mock_container, {"foo": "bar"})

    mock_container.update.assert_not_called()
    mock_container.reload.assert_not_called()


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_connection_error_hint_mentions_podman(mock_podman_docker, mock_docker):
    """Error hint should reference Podman, not Docker Desktop."""
    mock_client = _mock_docker()
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    service = PodmanSandboxService(config=_podman_config())
    hint = service._connection_error_hint(FileNotFoundError("/run/podman/podman.sock"))

    assert "Podman" in hint
    assert "Docker Desktop" not in hint
    assert "podman.socket" in hint or "podman machine start" in hint


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_connection_error_hint_empty_for_generic_error(mock_podman_docker, mock_docker):
    """Non-socket errors should return an empty hint."""
    mock_client = _mock_docker()
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    service = PodmanSandboxService(config=_podman_config())
    hint = service._connection_error_hint(RuntimeError("something else"))
    assert hint == ""


@patch("sys.platform", "linux")
@patch("os.getuid", return_value=1000, create=True)
@patch("os.path.exists", return_value=True)
def test_socket_detection_linux(mock_exists, mock_getuid):
    """On Linux, detect the rootless Podman socket."""
    with patch.dict(os.environ, {"XDG_RUNTIME_DIR": "/run/user/1000"}, clear=False):
        result = PodmanSandboxService._detect_podman_socket()
    assert result is not None
    assert "podman" in result
    assert "/run/user/1000" in result
    assert result.startswith("unix://")


@patch("sys.platform", "win32")
@patch("opensandbox_server.services.podman._check_windows_pipe")
def test_socket_detection_windows(mock_check_pipe):
    """On Windows, delegate to _check_windows_pipe."""
    mock_check_pipe.return_value = "npipe:////./pipe/podman-machine-default"
    result = PodmanSandboxService._detect_podman_socket()
    assert result is not None
    assert "npipe://" in result
    assert "podman-machine-default" in result
    mock_check_pipe.assert_called_once_with("podman-machine-default")


@patch("sys.platform", "win32")
@patch("opensandbox_server.services.podman._check_windows_pipe", return_value=None)
def test_socket_detection_windows_no_podman(mock_check_pipe):
    """On Windows, return None when Podman pipe is not reachable."""
    result = PodmanSandboxService._detect_podman_socket()
    assert result is None


@patch("sys.platform", "darwin")
@patch("os.path.exists", return_value=True)
def test_socket_detection_macos(mock_exists):
    """On macOS, detect the Podman Machine socket."""
    with patch.dict(os.environ, {"HOME": "/Users/testuser"}, clear=False):
        result = PodmanSandboxService._detect_podman_socket()
    assert result is not None
    assert "podman" in result
    assert "/Users/testuser" in result
    assert result.startswith("unix://")


@patch("sys.platform", "linux")
@patch("os.getuid", return_value=1000, create=True)
@patch("os.path.exists", return_value=False)
def test_socket_detection_returns_none_when_no_socket(mock_exists, mock_getuid):
    """Return None when no Podman socket exists on disk."""
    with patch.dict(os.environ, {"XDG_RUNTIME_DIR": "/run/user/1000"}, clear=False):
        result = PodmanSandboxService._detect_podman_socket()
    assert result is None


@patch("os.close")
@patch("os.open", return_value=3)
def test_check_windows_pipe_found(mock_open, mock_close):
    """Returns pipe URL with forward slashes when the named pipe can be opened."""
    result = _check_windows_pipe("podman-machine-default")
    assert result == "npipe:////./pipe/podman-machine-default"
    # os.open should use the native Windows backslash path
    mock_open.assert_called_once_with("\\\\.\\pipe\\podman-machine-default", os.O_RDONLY)
    mock_close.assert_called_once_with(3)


@patch("os.open", side_effect=OSError("pipe not found"))
def test_check_windows_pipe_not_found(mock_open):
    """Returns None when the named pipe does not exist."""
    result = _check_windows_pipe("nonexistent-pipe")
    assert result is None


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_resolve_does_not_mutate_environ(mock_podman_docker, mock_docker):
    """_resolve_podman_url must not set os.environ['DOCKER_HOST']."""
    mock_client = _mock_docker()
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    env_before = os.environ.get("DOCKER_HOST")
    with patch.dict(os.environ, {}, clear=False):
        os.environ.pop("DOCKER_HOST", None)
        config = _podman_config(podman=PodmanConfig(socket_path="/my/podman.sock"))
        service = PodmanSandboxService(config=config)
        assert "DOCKER_HOST" not in os.environ
        assert service._podman_base_url == "unix:///my/podman.sock"


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_create_docker_client_uses_base_url(mock_podman_docker, mock_docker):
    """_create_docker_client should pass base_url to DockerClient."""
    mock_client = _mock_docker()
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    config = _podman_config(podman=PodmanConfig(socket_path="/my/podman.sock"))
    with patch.dict(os.environ, {}, clear=False):
        os.environ.pop("DOCKER_HOST", None)
        PodmanSandboxService(config=config)

    mock_podman_docker.DockerClient.assert_called_once()
    call_kwargs = mock_podman_docker.DockerClient.call_args
    assert call_kwargs.kwargs.get("base_url") == "unix:///my/podman.sock"


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_respects_existing_docker_host(mock_podman_docker, mock_docker):
    """When DOCKER_HOST is set, fall back to parent's from_env()."""
    mock_client = _mock_docker()
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    original = "unix:///custom/podman.sock"
    with patch.dict(os.environ, {"DOCKER_HOST": original}, clear=False):
        service = PodmanSandboxService(config=_podman_config())
        assert service._podman_base_url is None
        assert os.environ["DOCKER_HOST"] == original


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_retry_adapter_patches_existing_adapter(mock_podman_docker, mock_docker):
    """_configure_retry_adapter should patch max_retries on the SDK adapter."""
    mock_client = _mock_docker()
    mock_adapter = MagicMock()
    mock_client.api.get_adapter.return_value = mock_adapter
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    PodmanSandboxService(config=_podman_config())

    mock_client.api.get_adapter.assert_called_with("http+docker://")
    assert mock_adapter.max_retries is not None
    assert mock_adapter.max_retries.total == 3


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_retry_adapter_handles_missing_adapter(mock_podman_docker, mock_docker):
    """_configure_retry_adapter must not raise if get_adapter fails."""
    mock_client = _mock_docker()
    mock_client.api.get_adapter.side_effect = Exception("no adapter")
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    # Should not raise
    service = PodmanSandboxService(config=_podman_config())
    assert service is not None


@patch("opensandbox_server.services.docker.docker")
@patch("opensandbox_server.services.podman.docker_mod")
def test_factory_creates_podman_service(mock_podman_docker, mock_docker):
    """create_sandbox_service('podman') should return PodmanSandboxService."""
    mock_client = _mock_docker()
    mock_podman_docker.DockerClient.return_value = mock_client
    mock_docker.from_env.return_value = mock_client

    from opensandbox_server.services.factory import create_sandbox_service

    service = create_sandbox_service(config=_podman_config())
    assert isinstance(service, PodmanSandboxService)


def test_podman_runtime_type_accepted():
    """RuntimeConfig accepts 'podman' as a valid type."""
    cfg = RuntimeConfig(type="podman", execd_image="test:latest")
    assert cfg.type == "podman"


def test_podman_config_rejects_kubernetes_block():
    """Podman runtime must reject kubernetes config block."""
    from pydantic import ValidationError
    from opensandbox_server.config import KubernetesRuntimeConfig

    with pytest.raises(ValidationError):
        AppConfig(
            server=ServerConfig(),
            runtime=RuntimeConfig(type="podman", execd_image="test:latest"),
            kubernetes=KubernetesRuntimeConfig(),
        )


def test_podman_config_socket_path():
    """PodmanConfig should accept socket_path."""
    cfg = PodmanConfig(socket_path="/run/podman/podman.sock")
    assert cfg.socket_path == "/run/podman/podman.sock"

    cfg_default = PodmanConfig()
    assert cfg_default.socket_path is None
