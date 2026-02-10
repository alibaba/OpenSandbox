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

"""Tests for Pydantic schema models."""

import pytest
from pydantic import ValidationError

from src.api.schema import (
    CreateSandboxRequest,
    Host,
    ImageSpec,
    PVC,
    ResourceLimits,
    Volume,
)


# ============================================================================
# Host Tests
# ============================================================================


class TestHost:
    """Tests for Host model."""

    def test_valid_path(self):
        """Valid absolute path should be accepted."""
        backend = Host(path="/data/opensandbox")
        assert backend.path == "/data/opensandbox"

    def test_path_required(self):
        """Path field should be required."""
        with pytest.raises(ValidationError) as exc_info:
            Host()  # type: ignore
        errors = exc_info.value.errors()
        assert any(e["loc"] == ("path",) for e in errors)

    def test_serialization(self):
        """Model should serialize correctly."""
        backend = Host(path="/data/opensandbox")
        data = backend.model_dump()
        assert data == {"path": "/data/opensandbox"}

    def test_deserialization(self):
        """Model should deserialize correctly."""
        data = {"path": "/data/opensandbox"}
        backend = Host.model_validate(data)
        assert backend.path == "/data/opensandbox"


# ============================================================================
# PVC Tests
# ============================================================================


class TestPVC:
    """Tests for PVC model."""

    def test_valid_claim_name(self):
        """Valid claim name should be accepted."""
        backend = PVC(claim_name="my-pvc")
        assert backend.claim_name == "my-pvc"

    def test_claim_name_alias(self):
        """claimName alias should work."""
        data = {"claimName": "my-pvc"}
        backend = PVC.model_validate(data)
        assert backend.claim_name == "my-pvc"

    def test_serialization_uses_alias(self):
        """Serialization should use camelCase alias."""
        backend = PVC(claim_name="my-pvc")
        data = backend.model_dump(by_alias=True)
        assert data == {"claimName": "my-pvc"}

    def test_claim_name_required(self):
        """claim_name field should be required."""
        with pytest.raises(ValidationError) as exc_info:
            PVC()  # type: ignore
        errors = exc_info.value.errors()
        assert any("claim_name" in str(e["loc"]) or "claimName" in str(e["loc"]) for e in errors)


# ============================================================================
# Volume Tests
# ============================================================================


class TestVolume:
    """Tests for Volume model."""

    def test_valid_host_volume(self):
        """Valid host volume should be accepted."""
        volume = Volume(
            name="workdir",
            host=Host(path="/data/opensandbox"),
            mount_path="/mnt/work",
            read_only=False,
        )
        assert volume.name == "workdir"
        assert volume.host is not None
        assert volume.host.path == "/data/opensandbox"
        assert volume.mount_path == "/mnt/work"
        assert volume.read_only is False
        assert volume.pvc is None
        assert volume.sub_path is None

    def test_valid_pvc_volume(self):
        """Valid PVC volume should be accepted."""
        volume = Volume(
            name="models",
            pvc=PVC(claim_name="shared-models-pvc"),
            mount_path="/mnt/models",
            read_only=True,
        )
        assert volume.name == "models"
        assert volume.pvc is not None
        assert volume.pvc.claim_name == "shared-models-pvc"
        assert volume.mount_path == "/mnt/models"
        assert volume.read_only is True
        assert volume.host is None

    def test_valid_volume_with_subpath(self):
        """Volume with subPath should be accepted."""
        volume = Volume(
            name="workdir",
            host=Host(path="/data/opensandbox"),
            mount_path="/mnt/work",
            read_only=False,
            sub_path="task-001",
        )
        assert volume.sub_path == "task-001"

    def test_no_backend_raises(self):
        """Volume without any backend should raise ValidationError."""
        with pytest.raises(ValidationError) as exc_info:
            Volume(
                name="workdir",
                mount_path="/mnt/work",
                read_only=False,
            )
        # Check that validation error mentions backend
        error_message = str(exc_info.value)
        assert "backend" in error_message.lower()

    def test_multiple_backends_raises(self):
        """Volume with multiple backends should raise ValidationError."""
        with pytest.raises(ValidationError) as exc_info:
            Volume(
                name="workdir",
                host=Host(path="/data/opensandbox"),
                pvc=PVC(claim_name="my-pvc"),
                mount_path="/mnt/work",
                read_only=False,
            )
        # Check that validation error mentions backend
        error_message = str(exc_info.value)
        assert "backend" in error_message.lower()

    def test_serialization_host_volume(self):
        """Host volume should serialize correctly with camelCase aliases."""
        volume = Volume(
            name="workdir",
            host=Host(path="/data/opensandbox"),
            mount_path="/mnt/work",
            read_only=False,
            sub_path="task-001",
        )
        data = volume.model_dump(by_alias=True, exclude_none=True)
        assert data == {
            "name": "workdir",
            "host": {"path": "/data/opensandbox"},
            "mountPath": "/mnt/work",
            "readOnly": False,
            "subPath": "task-001",
        }

    def test_serialization_pvc_volume(self):
        """PVC volume should serialize correctly with camelCase aliases."""
        volume = Volume(
            name="models",
            pvc=PVC(claim_name="shared-models-pvc"),
            mount_path="/mnt/models",
            read_only=True,
        )
        data = volume.model_dump(by_alias=True, exclude_none=True)
        assert data == {
            "name": "models",
            "pvc": {"claimName": "shared-models-pvc"},
            "mountPath": "/mnt/models",
            "readOnly": True,
        }

    def test_deserialization_host_volume(self):
        """Host volume should deserialize correctly from camelCase."""
        data = {
            "name": "workdir",
            "host": {"path": "/data/opensandbox"},
            "mountPath": "/mnt/work",
            "readOnly": False,
            "subPath": "task-001",
        }
        volume = Volume.model_validate(data)
        assert volume.name == "workdir"
        assert volume.host is not None
        assert volume.host.path == "/data/opensandbox"
        assert volume.mount_path == "/mnt/work"
        assert volume.read_only is False
        assert volume.sub_path == "task-001"

    def test_deserialization_pvc_volume(self):
        """PVC volume should deserialize correctly from camelCase."""
        data = {
            "name": "models",
            "pvc": {"claimName": "shared-models-pvc"},
            "mountPath": "/mnt/models",
            "readOnly": True,
        }
        volume = Volume.model_validate(data)
        assert volume.name == "models"
        assert volume.pvc is not None
        assert volume.pvc.claim_name == "shared-models-pvc"
        assert volume.mount_path == "/mnt/models"
        assert volume.read_only is True


# ============================================================================
# CreateSandboxRequest with Volumes Tests
# ============================================================================


class TestCreateSandboxRequestWithVolumes:
    """Tests for CreateSandboxRequest with volumes field."""

    def test_request_without_volumes(self):
        """Request without volumes should be valid."""
        request = CreateSandboxRequest(
            image=ImageSpec(uri="python:3.11"),
            timeout=3600,
            resource_limits=ResourceLimits({"cpu": "500m", "memory": "512Mi"}),
            entrypoint=["python", "-c", "print('hello')"],
        )
        assert request.volumes is None

    def test_request_with_empty_volumes(self):
        """Request with empty volumes list should be valid."""
        request = CreateSandboxRequest(
            image=ImageSpec(uri="python:3.11"),
            timeout=3600,
            resource_limits=ResourceLimits({"cpu": "500m", "memory": "512Mi"}),
            entrypoint=["python", "-c", "print('hello')"],
            volumes=[],
        )
        assert request.volumes == []

    def test_request_with_host_volume(self):
        """Request with host volume should be valid."""
        request = CreateSandboxRequest(
            image=ImageSpec(uri="python:3.11"),
            timeout=3600,
            resource_limits=ResourceLimits({"cpu": "500m", "memory": "512Mi"}),
            entrypoint=["python", "-c", "print('hello')"],
            volumes=[
                Volume(
                    name="workdir",
                    host=Host(path="/data/opensandbox"),
                    mount_path="/mnt/work",
                    read_only=False,
                )
            ],
        )
        assert request.volumes is not None
        assert len(request.volumes) == 1
        assert request.volumes[0].name == "workdir"

    def test_request_with_pvc_volume(self):
        """Request with PVC volume should be valid."""
        request = CreateSandboxRequest(
            image=ImageSpec(uri="python:3.11"),
            timeout=3600,
            resource_limits=ResourceLimits({"cpu": "500m", "memory": "512Mi"}),
            entrypoint=["python", "-c", "print('hello')"],
            volumes=[
                Volume(
                    name="models",
                    pvc=PVC(claim_name="shared-models-pvc"),
                    mount_path="/mnt/models",
                    read_only=True,
                )
            ],
        )
        assert request.volumes is not None
        assert len(request.volumes) == 1
        assert request.volumes[0].pvc is not None
        assert request.volumes[0].pvc.claim_name == "shared-models-pvc"

    def test_request_with_multiple_volumes(self):
        """Request with multiple volumes should be valid."""
        request = CreateSandboxRequest(
            image=ImageSpec(uri="python:3.11"),
            timeout=3600,
            resource_limits=ResourceLimits({"cpu": "500m", "memory": "512Mi"}),
            entrypoint=["python", "-c", "print('hello')"],
            volumes=[
                Volume(
                    name="workdir",
                    host=Host(path="/data/opensandbox"),
                    mount_path="/mnt/work",
                    read_only=False,
                ),
                Volume(
                    name="models",
                    pvc=PVC(claim_name="shared-models-pvc"),
                    mount_path="/mnt/models",
                    read_only=True,
                ),
            ],
        )
        assert request.volumes is not None
        assert len(request.volumes) == 2

    def test_serialization_with_volumes(self):
        """Request with volumes should serialize correctly."""
        request = CreateSandboxRequest(
            image=ImageSpec(uri="python:3.11"),
            timeout=3600,
            resource_limits=ResourceLimits({"cpu": "500m", "memory": "512Mi"}),
            entrypoint=["python", "-c", "print('hello')"],
            volumes=[
                Volume(
                    name="workdir",
                    host=Host(path="/data/opensandbox"),
                    mount_path="/mnt/work",
                    read_only=False,
                    sub_path="task-001",
                )
            ],
        )
        data = request.model_dump(by_alias=True, exclude_none=True)
        assert "volumes" in data
        assert len(data["volumes"]) == 1
        assert data["volumes"][0]["name"] == "workdir"
        assert data["volumes"][0]["mountPath"] == "/mnt/work"
        assert data["volumes"][0]["readOnly"] is False
        assert data["volumes"][0]["subPath"] == "task-001"

    def test_deserialization_with_volumes(self):
        """Request with volumes should deserialize correctly."""
        data = {
            "image": {"uri": "python:3.11"},
            "timeout": 3600,
            "resourceLimits": {"cpu": "500m", "memory": "512Mi"},
            "entrypoint": ["python", "-c", "print('hello')"],
            "volumes": [
                {
                    "name": "workdir",
                    "host": {"path": "/data/opensandbox"},
                    "mountPath": "/mnt/work",
                    "readOnly": False,
                    "subPath": "task-001",
                },
                {
                    "name": "models",
                    "pvc": {"claimName": "shared-models-pvc"},
                    "mountPath": "/mnt/models",
                    "readOnly": True,
                },
            ],
        }
        request = CreateSandboxRequest.model_validate(data)
        assert request.volumes is not None
        assert len(request.volumes) == 2

        # Check host volume
        assert request.volumes[0].name == "workdir"
        assert request.volumes[0].host is not None
        assert request.volumes[0].host.path == "/data/opensandbox"
        assert request.volumes[0].mount_path == "/mnt/work"
        assert request.volumes[0].read_only is False
        assert request.volumes[0].sub_path == "task-001"

        # Check PVC volume
        assert request.volumes[1].name == "models"
        assert request.volumes[1].pvc is not None
        assert request.volumes[1].pvc.claim_name == "shared-models-pvc"
        assert request.volumes[1].mount_path == "/mnt/models"
        assert request.volumes[1].read_only is True
