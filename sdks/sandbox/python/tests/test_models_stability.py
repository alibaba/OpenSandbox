#
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
#
from __future__ import annotations

from datetime import datetime, timezone

import pytest

from opensandbox.models.filesystem import MoveEntry, WriteEntry
from opensandbox.models.sandboxes import (
    Host,
    PVC,
    SandboxFilter,
    SandboxImageAuth,
    SandboxImageSpec,
    SandboxInfo,
    SandboxStatus,
    Volume,
)


def test_sandbox_image_spec_supports_positional_image() -> None:
    spec = SandboxImageSpec("python:3.11")
    assert spec.image == "python:3.11"


def test_sandbox_image_spec_rejects_blank_image() -> None:
    with pytest.raises(ValueError):
        SandboxImageSpec("   ")


def test_sandbox_image_auth_rejects_blank_username_and_password() -> None:
    with pytest.raises(ValueError):
        SandboxImageAuth(username=" ", password="x")
    with pytest.raises(ValueError):
        SandboxImageAuth(username="u", password=" ")


def test_sandbox_filter_validations() -> None:
    SandboxFilter(page=0, page_size=1)
    with pytest.raises(ValueError):
        SandboxFilter(page=-1)
    with pytest.raises(ValueError):
        SandboxFilter(page_size=0)


def test_sandbox_status_and_info_alias_dump_is_stable() -> None:
    status = SandboxStatus(state="RUNNING", last_transition_at=datetime(2025, 1, 1, tzinfo=timezone.utc))
    info = SandboxInfo(
        id=str(__import__("uuid").uuid4()),
        status=status,
        entrypoint=["/bin/sh"],
        expires_at=datetime(2025, 1, 2, tzinfo=timezone.utc),
        created_at=datetime(2025, 1, 1, tzinfo=timezone.utc),
        image=SandboxImageSpec("python:3.11"),
        metadata={"k": "v"},
    )

    dumped = info.model_dump(by_alias=True, mode="json")
    assert "expires_at" in dumped
    assert "created_at" in dumped
    assert dumped["status"]["last_transition_at"].endswith(("Z", "+00:00"))


def test_filesystem_models_aliases_and_validation() -> None:
    m = MoveEntry(source="/a", destination="/b")
    assert m.src == "/a"
    assert m.dest == "/b"

    with pytest.raises(ValueError):
        WriteEntry(path="  ", data="x")


# ============================================================================
# Volume Model Tests
# ============================================================================


def test_host_backend_requires_absolute_path() -> None:
    backend = Host(path="/data/shared")
    assert backend.path == "/data/shared"

    with pytest.raises(ValueError, match="absolute path"):
        Host(path="relative/path")


def test_pvc_backend_rejects_blank_claim_name() -> None:
    backend = PVC(claimName="my-pvc")
    assert backend.claim_name == "my-pvc"

    with pytest.raises(ValueError, match="blank"):
        PVC(claimName="   ")


def test_volume_with_host_backend() -> None:
    vol = Volume(
        name="data",
        host=Host(path="/data/shared"),
        mountPath="/mnt/data",
    )
    assert vol.name == "data"
    assert vol.host is not None
    assert vol.host.path == "/data/shared"
    assert vol.pvc is None
    assert vol.mount_path == "/mnt/data"
    assert vol.read_only is False  # default is read-write
    assert vol.sub_path is None


def test_volume_with_pvc_backend() -> None:
    vol = Volume(
        name="models",
        pvc=PVC(claimName="shared-models"),
        mountPath="/mnt/models",
        readOnly=True,
        subPath="v1",
    )
    assert vol.name == "models"
    assert vol.host is None
    assert vol.pvc is not None
    assert vol.pvc.claim_name == "shared-models"
    assert vol.mount_path == "/mnt/models"
    assert vol.read_only is True
    assert vol.sub_path == "v1"


def test_volume_rejects_blank_name() -> None:
    with pytest.raises(ValueError, match="blank"):
        Volume(
            name="   ",
            host=Host(path="/data"),
            mountPath="/mnt",
        )


def test_volume_requires_absolute_mount_path() -> None:
    with pytest.raises(ValueError, match="absolute path"):
        Volume(
            name="test",
            host=Host(path="/data"),
            mountPath="relative/path",
        )


def test_volume_serialization_uses_aliases() -> None:
    vol = Volume(
        name="test",
        pvc=PVC(claimName="my-pvc"),
        mountPath="/mnt/test",
        readOnly=True,
        subPath="sub",
    )
    dumped = vol.model_dump(by_alias=True, mode="json")
    assert "mountPath" in dumped
    assert "readOnly" in dumped
    assert "subPath" in dumped
    assert dumped["pvc"]["claimName"] == "my-pvc"
    assert dumped["readOnly"] is True


def test_volume_rejects_no_backend() -> None:
    """Volume must have exactly one backend specified."""
    with pytest.raises(ValueError, match="none was provided"):
        Volume(
            name="test",
            mountPath="/mnt/test",
        )


def test_volume_rejects_multiple_backends() -> None:
    """Volume must have exactly one backend, not multiple."""
    with pytest.raises(ValueError, match="multiple were provided"):
        Volume(
            name="test",
            host=Host(path="/data"),
            pvc=PVC(claimName="my-pvc"),
            mountPath="/mnt/test",
        )
