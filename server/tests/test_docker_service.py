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

from datetime import datetime, timezone
from unittest.mock import MagicMock, patch

import pytest
from fastapi import HTTPException, status

from src.config import AppConfig, IngressConfig, RuntimeConfig, ServerConfig
from src.services.constants import SANDBOX_ID_LABEL, SandboxErrorCodes
from src.services.docker import DockerSandboxService, PendingSandbox
from src.services.helpers import parse_memory_limit, parse_nano_cpus, parse_timestamp
from src.api.schema import (
    CreateSandboxRequest,
    CreateSandboxResponse,
    ImageSpec,
    NetworkPolicy,
    ListSandboxesRequest,
    ResourceLimits,
    Sandbox,
    SandboxFilter,
    SandboxStatus,
)


def _app_config() -> AppConfig:
    return AppConfig(
        server=ServerConfig(),
        runtime=RuntimeConfig(type="docker", execd_image="ghcr.io/opensandbox/platform:latest"),
        ingress=IngressConfig(mode="direct"),
    )


def test_parse_memory_limit_handles_units():
    assert parse_memory_limit("512Mi") == 512 * 1024 * 1024
    assert parse_memory_limit("1G") == 1_000_000_000
    assert parse_memory_limit("2gi") == 2 * 1024 ** 3
    assert parse_memory_limit("invalid") is None


def test_parse_nano_cpus():
    assert parse_nano_cpus("500m") == 500_000_000
    assert parse_nano_cpus("2") == 2_000_000_000
    assert parse_nano_cpus("bad") is None


def test_parse_timestamp_defaults_on_invalid():
    ts = parse_timestamp("0001-01-01T00:00:00Z")
    assert ts.tzinfo is not None
    future = parse_timestamp("2024-01-01T00:00:00Z")
    assert future.year == 2024


def test_env_allows_empty_string_and_skips_none():
    # Use base config helper
    DockerSandboxService(config=_app_config())
    # Build request with mixed env values
    req = CreateSandboxRequest(
        image=ImageSpec(uri="python:3.11"),
        timeout=120,
        resourceLimits=ResourceLimits(root={}),
        env={"FOO": "bar", "EMPTY": "", "NONE": None},
        metadata={},
        entrypoint=["python"],
    )
    # Validate env handling
    env_dict = req.env or {}
    environment = []
    for key, value in env_dict.items():
        if value is None:
            continue
        environment.append(f"{key}={value}")

    assert "FOO=bar" in environment
    assert "EMPTY=" in environment  # empty string preserved
    # None should be skipped
    assert all(not item.startswith("NONE=") for item in environment)


@patch("src.services.docker.docker")
def test_create_sandbox_applies_security_defaults(mock_docker):
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_client.api.create_host_config.return_value = {
        "security_opt": ["no-new-privileges:true"],
        "cap_drop": _app_config().docker.drop_capabilities,
        "pids_limit": _app_config().docker.pids_limit,
    }
    mock_client.api.create_container.return_value = {"Id": "cid"}
    mock_client.containers.get.return_value = MagicMock()
    mock_docker.from_env.return_value = mock_client

    service = DockerSandboxService(config=_app_config())
    request = CreateSandboxRequest(
        image=ImageSpec(uri="python:3.11"),
        timeout=120,
        resourceLimits=ResourceLimits(root={}),
        env={},
        metadata={},
        entrypoint=["python"],
    )

    with patch.object(service, "_ensure_image_available"), patch.object(
        service, "_prepare_sandbox_runtime"
    ):
        service.create_sandbox(request)

    host_config = mock_client.api.create_container.call_args.kwargs["host_config"]
    assert "no-new-privileges:true" in host_config.get("security_opt", [])
    assert host_config.get("cap_drop") == service.app_config.docker.drop_capabilities
    assert host_config.get("pids_limit") == service.app_config.docker.pids_limit


@patch("src.services.docker.docker")
def test_create_sandbox_rejects_invalid_metadata(mock_docker):
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.from_env.return_value = mock_client

    service = DockerSandboxService(config=_app_config())

    request = CreateSandboxRequest(
        image=ImageSpec(uri="python:3.11"),
        timeout=120,
        resourceLimits=ResourceLimits(root={}),
        env={},
        metadata={"Bad Key": "ok"},  # space is invalid for label key
        entrypoint=["python"],
    )

    with pytest.raises(HTTPException) as exc:
        service.create_sandbox(request)

    assert exc.value.status_code == status.HTTP_400_BAD_REQUEST
    assert exc.value.detail["code"] == SandboxErrorCodes.INVALID_METADATA_LABEL
    mock_client.containers.create.assert_not_called()


@patch("src.services.docker.docker")
def test_create_sandbox_requires_entrypoint(mock_docker):
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.from_env.return_value = mock_client

    service = DockerSandboxService(config=_app_config())

    request = CreateSandboxRequest(
        image=ImageSpec(uri="python:3.11"),
        timeout=120,
        resourceLimits=ResourceLimits(root={}),
        env={},
        metadata={},
        entrypoint=["python"],
    )
    request.entrypoint = []

    with pytest.raises(HTTPException) as exc:
        service.create_sandbox(request)

    assert exc.value.status_code == status.HTTP_400_BAD_REQUEST
    assert exc.value.detail["code"] == SandboxErrorCodes.INVALID_ENTRYPOINT
    mock_client.containers.create.assert_not_called()


@patch("src.services.docker.docker")
def test_network_policy_rejected_on_host_mode(mock_docker):
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.from_env.return_value = mock_client

    cfg = _app_config()
    cfg.docker.network_mode = "host"
    cfg.runtime.egress_image = "egress:latest"
    service = DockerSandboxService(config=cfg)

    request = CreateSandboxRequest(
        image=ImageSpec(uri="python:3.11"),
        timeout=120,
        resourceLimits=ResourceLimits(root={}),
        env={},
        metadata={},
        entrypoint=["python"],
        networkPolicy=NetworkPolicy(default_action="deny", egress=[]),
    )

    with pytest.raises(HTTPException) as exc:
        service.create_sandbox(request)

    assert exc.value.status_code == status.HTTP_400_BAD_REQUEST
    assert exc.value.detail["code"] == SandboxErrorCodes.INVALID_PARAMETER


@patch("src.services.docker.docker")
def test_network_policy_requires_egress_image(mock_docker):
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.from_env.return_value = mock_client

    cfg = _app_config()
    cfg.docker.network_mode = "bridge"
    cfg.runtime.egress_image = None
    service = DockerSandboxService(config=cfg)

    request = CreateSandboxRequest(
        image=ImageSpec(uri="python:3.11"),
        timeout=120,
        resourceLimits=ResourceLimits(root={}),
        env={},
        metadata={},
        entrypoint=["python"],
        networkPolicy=NetworkPolicy(default_action="deny", egress=[]),
    )

    with pytest.raises(HTTPException) as exc:
        service.create_sandbox(request)

    assert exc.value.status_code == status.HTTP_400_BAD_REQUEST
    assert exc.value.detail["code"] == SandboxErrorCodes.INVALID_PARAMETER


@patch("src.services.docker.docker")
def test_egress_sidecar_injection_and_capabilities(mock_docker):
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []

    def host_cfg_side_effect(**kwargs):
        return kwargs

    mock_client.api.create_host_config.side_effect = host_cfg_side_effect
    mock_client.api.create_container.side_effect = [
        {"Id": "sidecar-id"},
        {"Id": "main-id"},
    ]
    mock_client.containers.get.side_effect = [MagicMock(id="sidecar-id"), MagicMock(id="main-id")]
    mock_docker.from_env.return_value = mock_client

    cfg = _app_config()
    cfg.docker.network_mode = "bridge"
    cfg.runtime.egress_image = "egress:latest"
    service = DockerSandboxService(config=cfg)

    req = CreateSandboxRequest(
        image=ImageSpec(uri="python:3.11"),
        timeout=120,
        resourceLimits=ResourceLimits(root={}),
        env={},
        metadata={},
        entrypoint=["python"],
        networkPolicy=NetworkPolicy(default_action="deny", egress=[]),
    )

    with patch.object(service, "_ensure_image_available"), patch.object(service, "_prepare_sandbox_runtime"):
        service.create_sandbox(req)

    assert len(mock_client.api.create_container.call_args_list) == 2
    sidecar_call = mock_client.api.create_container.call_args_list[0]
    main_call = mock_client.api.create_container.call_args_list[1]
    sidecar_kwargs = sidecar_call.kwargs
    main_kwargs = main_call.kwargs

    # Sidecar host config should have NET_ADMIN and port bindings
    assert "NET_ADMIN" in sidecar_kwargs["host_config"]["cap_add"]
    assert "44772" in sidecar_kwargs["host_config"]["port_bindings"]
    assert "8080" in sidecar_kwargs["host_config"]["port_bindings"]

    # Main container should share sidecar netns, drop NET_ADMIN, and have no port bindings
    assert main_kwargs["host_config"]["network_mode"] == "container:sidecar-id"
    assert "NET_ADMIN" in set(main_kwargs["host_config"].get("cap_drop") or [])
    assert "port_bindings" not in main_kwargs["host_config"]

    # Main container labels should carry host port info
    labels = main_kwargs["labels"]
    assert labels.get("opensandbox.io/embedding-proxy-port")
    assert labels.get("opensandbox.io/http-port")


def test_expire_cleans_sidecar():
    service = DockerSandboxService(config=_app_config())
    mock_container = MagicMock()
    mock_container.attrs = {"State": {"Running": False}, "Config": {"Labels": {}}}
    mock_container.kill = MagicMock()
    mock_container.remove = MagicMock()

    with patch.object(service, "_get_container_by_sandbox_id", return_value=mock_container), patch.object(
        service, "_remove_expiration_tracking"
    ) as mock_remove, patch.object(service, "_cleanup_egress_sidecar") as mock_cleanup, patch.object(
        service, "_docker_operation"
    ) as mock_op:
        mock_op.return_value.__enter__.return_value = None
        mock_op.return_value.__exit__.return_value = None
        service._expire_sandbox("sandbox-id")

    mock_cleanup.assert_called_once_with("sandbox-id")
    mock_remove.assert_called_once()


def test_restore_cleans_orphan_sidecar():
    cfg = _app_config()
    service = DockerSandboxService(config=cfg)

    orphan_sidecar = MagicMock()
    orphan_sidecar.attrs = {"Config": {"Labels": {"opensandbox.io/egress-sidecar-for": "orphan-id"}}}

    with patch.object(service.docker_client.containers, "list", return_value=[orphan_sidecar]), patch.object(
        service, "_get_container_by_sandbox_id"
    ) as mock_get, patch.object(service, "_cleanup_egress_sidecar") as mock_cleanup:
        mock_get.side_effect = HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail={})
        service._restore_existing_sandboxes()

    mock_cleanup.assert_called_once_with("orphan-id")
@patch("src.services.docker.docker")
def test_create_sandbox_async_returns_provisioning(mock_docker):
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.from_env.return_value = mock_client

    service = DockerSandboxService(config=_app_config())

    request = CreateSandboxRequest(
        image=ImageSpec(uri="python:3.11"),
        timeout=120,
        resourceLimits=ResourceLimits(root={}),
        env={},
        metadata={"team": "async"},
        entrypoint=["python", "app.py"],
    )

    with patch.object(service, "create_sandbox") as mock_sync:
        mock_sync.return_value = CreateSandboxResponse(
            id="sandbox-sync",
            status=SandboxStatus(
                state="Running",
                reason="CONTAINER_RUNNING",
                message="started",
                last_transition_at=datetime.now(timezone.utc),
            ),
            metadata={"team": "async"},
            expiresAt=datetime.now(timezone.utc),
            createdAt=datetime.now(timezone.utc),
            entrypoint=["python", "app.py"],
        )
        response = service.create_sandbox(request)

    assert response.status.state == "Running"
    assert response.metadata == {"team": "async"}
    mock_sync.assert_called_once()


@patch("src.services.docker.docker")
def test_get_sandbox_returns_pending_state(mock_docker):
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.from_env.return_value = mock_client

    service = DockerSandboxService(config=_app_config())

    request = CreateSandboxRequest(
        image=ImageSpec(uri="python:3.11"),
        timeout=120,
        resourceLimits=ResourceLimits(root={}),
        env={},
        metadata={},
        entrypoint=["python", "app.py"],
    )

    with patch.object(service, "create_sandbox") as mock_sync:
        mock_sync.return_value = CreateSandboxResponse(
            id="sandbox-sync",
            status=SandboxStatus(
                state="Running",
                reason="CONTAINER_RUNNING",
                message="started",
                last_transition_at=datetime.now(timezone.utc),
            ),
            metadata={},
            expiresAt=datetime.now(timezone.utc),
            createdAt=datetime.now(timezone.utc),
            entrypoint=["python", "app.py"],
        )
        response = service.create_sandbox(request)

    assert response.status.state == "Running"
    assert response.entrypoint == ["python", "app.py"]


@patch("src.services.docker.docker")
def test_list_sandboxes_deduplicates_container_and_pending(mock_docker):
    # Build a realistic container mock to avoid parse_timestamp errors.
    container = MagicMock()
    container.attrs = {
        "Config": {"Labels": {SANDBOX_ID_LABEL: "sandbox-123"}},
        "Created": "2025-01-01T00:00:00Z",
        "State": {
            "Status": "running",
            "Running": True,
            "FinishedAt": "0001-01-01T00:00:00Z",
            "ExitCode": 0,
        },
    }
    container.image = MagicMock(tags=["image:latest"], short_id="sha-image")

    mock_client = MagicMock()
    mock_client.containers.list.return_value = [container]
    mock_docker.from_env.return_value = mock_client

    service = DockerSandboxService(config=_app_config())
    sandbox_id = "sandbox-123"

    # Prepare container and pending representations
    container_sandbox = Sandbox(
        id=sandbox_id,
        image=ImageSpec(uri="image:latest"),
        status=SandboxStatus(
            state="Running",
            reason="CONTAINER_RUNNING",
            message="running",
            last_transition_at=datetime.now(timezone.utc),
        ),
        metadata={"team": "c"},
        entrypoint=["/bin/sh"],
        expiresAt=datetime.now(timezone.utc),
        createdAt=datetime.now(timezone.utc),
    )
    # Force container state to be returned
    service._container_to_sandbox = MagicMock(return_value=container_sandbox)

    response = service.list_sandboxes(ListSandboxesRequest(filter=SandboxFilter(), pagination=None))

    assert len(response.items) == 1
    assert response.items[0].status.state == "Running"
    assert response.items[0].metadata == {"team": "c"}


@patch("src.services.docker.docker")
def test_get_sandbox_prefers_container_over_pending(mock_docker):
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.from_env.return_value = mock_client

    service = DockerSandboxService(config=_app_config())
    sandbox_id = "sandbox-abc"

    pending_status = SandboxStatus(
        state="Pending",
        reason="SANDBOX_SCHEDULED",
        message="pending",
        last_transition_at=datetime.now(timezone.utc),
    )
    service._pending_sandboxes[sandbox_id] = PendingSandbox(
        request=MagicMock(metadata={}, entrypoint=["/bin/sh"], image=ImageSpec(uri="image:latest")),
        created_at=datetime.now(timezone.utc),
        expires_at=datetime.now(timezone.utc),
        status=pending_status,
    )

    container_sandbox = Sandbox(
        id=sandbox_id,
        image=ImageSpec(uri="image:latest"),
        status=SandboxStatus(
            state="Running",
            reason="CONTAINER_RUNNING",
            message="running",
            last_transition_at=datetime.now(timezone.utc),
        ),
        metadata={},
        entrypoint=["/bin/sh"],
        expiresAt=datetime.now(timezone.utc),
        createdAt=datetime.now(timezone.utc),
    )

    service._get_container_by_sandbox_id = MagicMock(return_value=MagicMock())
    service._container_to_sandbox = MagicMock(return_value=container_sandbox)

    sandbox = service.get_sandbox(sandbox_id)
    assert sandbox.status.state == "Running"
    assert sandbox.entrypoint == ["/bin/sh"]


@patch("src.services.docker.docker")
def test_async_worker_cleans_up_leftover_container_on_failure(mock_docker):
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.from_env.return_value = mock_client

    service = DockerSandboxService(config=_app_config())
    sandbox_id = "sandbox-fail"
    created_at = datetime.now(timezone.utc)
    expires_at = created_at

    pending_status = SandboxStatus(
        state="Pending",
        reason="SANDBOX_SCHEDULED",
        message="pending",
        last_transition_at=created_at,
    )
    service._pending_sandboxes[sandbox_id] = PendingSandbox(
        request=MagicMock(metadata={}, entrypoint=["/bin/sh"], image=ImageSpec(uri="image:latest")),
        created_at=created_at,
        expires_at=expires_at,
        status=pending_status,
    )

    service._provision_sandbox = MagicMock(
        side_effect=HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail={"message": "boom"},
        )
    )
    service._cleanup_failed_containers = MagicMock()

    service._async_provision_worker(
        sandbox_id,
        MagicMock(),
        created_at,
        expires_at,
    )

    service._cleanup_failed_containers.assert_called_once_with(sandbox_id)
    assert service._pending_sandboxes[sandbox_id].status.state == "Failed"
