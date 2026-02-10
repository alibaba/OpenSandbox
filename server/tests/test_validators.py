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

import pytest
from fastapi import HTTPException

from src.api.schema import Host, PVC, Volume
from src.services.constants import SandboxErrorCodes
from src.services.validators import (
    ensure_metadata_labels,
    ensure_valid_host_path,
    ensure_valid_mount_path,
    ensure_valid_pvc_name,
    ensure_valid_sub_path,
    ensure_valid_volume_name,
    ensure_volumes_valid,
)


def test_ensure_metadata_labels_accepts_common_k8s_forms():
    # Various valid label shapes: with/without prefix, mixed chars, empty value allowed.
    valid_metadata = {
        "app": "web",
        "opensandbox.io/hello": "world",
        "k8s.io/name": "app-1",
        "example.com/label": "a.b_c-1",
        "team": "A1_b-2.c",
        "empty": "",
    }

    # Should not raise
    ensure_metadata_labels(valid_metadata)


def test_ensure_metadata_labels_allows_none_or_empty():
    ensure_metadata_labels(None)
    ensure_metadata_labels({})


# ============================================================================
# Volume Name Validation Tests
# ============================================================================


class TestEnsureValidVolumeName:
    """Tests for ensure_valid_volume_name function."""

    def test_valid_simple_name(self):
        """Simple lowercase names should be valid."""
        ensure_valid_volume_name("workdir")
        ensure_valid_volume_name("data")
        ensure_valid_volume_name("models")

    def test_valid_name_with_numbers(self):
        """Names with numbers should be valid."""
        ensure_valid_volume_name("data1")
        ensure_valid_volume_name("vol2")
        ensure_valid_volume_name("123")

    def test_valid_name_with_hyphens(self):
        """Names with hyphens should be valid."""
        ensure_valid_volume_name("my-volume")
        ensure_valid_volume_name("data-cache-1")
        ensure_valid_volume_name("a-b-c")

    def test_empty_name_raises(self):
        """Empty name should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_volume_name("")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_VOLUME_NAME

    def test_name_too_long_raises(self):
        """Name exceeding 63 characters should raise HTTPException."""
        long_name = "a" * 64
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_volume_name(long_name)
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_VOLUME_NAME

    def test_uppercase_name_raises(self):
        """Uppercase letters should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_volume_name("MyVolume")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_VOLUME_NAME

    def test_underscore_name_raises(self):
        """Underscores should raise HTTPException (not valid DNS label)."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_volume_name("my_volume")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_VOLUME_NAME

    def test_name_starting_with_hyphen_raises(self):
        """Names starting with hyphen should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_volume_name("-volume")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_VOLUME_NAME

    def test_name_ending_with_hyphen_raises(self):
        """Names ending with hyphen should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_volume_name("volume-")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_VOLUME_NAME


# ============================================================================
# Mount Path Validation Tests
# ============================================================================


class TestEnsureValidMountPath:
    """Tests for ensure_valid_mount_path function."""

    def test_valid_absolute_path(self):
        """Absolute paths should be valid."""
        ensure_valid_mount_path("/mnt/data")
        ensure_valid_mount_path("/")
        ensure_valid_mount_path("/home/user/work")

    def test_empty_path_raises(self):
        """Empty path should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_mount_path("")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_MOUNT_PATH

    def test_relative_path_raises(self):
        """Relative paths should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_mount_path("data/files")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_MOUNT_PATH

    def test_path_not_starting_with_slash_raises(self):
        """Paths not starting with '/' should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_mount_path("mnt/data")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_MOUNT_PATH


# ============================================================================
# SubPath Validation Tests
# ============================================================================


class TestEnsureValidSubPath:
    """Tests for ensure_valid_sub_path function."""

    def test_none_subpath_valid(self):
        """None subpath should be valid."""
        ensure_valid_sub_path(None)

    def test_empty_subpath_valid(self):
        """Empty string subpath should be valid."""
        ensure_valid_sub_path("")

    def test_relative_subpath_valid(self):
        """Relative paths should be valid."""
        ensure_valid_sub_path("task-001")
        ensure_valid_sub_path("user/data")
        ensure_valid_sub_path("a/b/c")

    def test_absolute_subpath_raises(self):
        """Absolute paths should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_sub_path("/absolute/path")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_SUB_PATH

    def test_path_traversal_raises(self):
        """Path traversal (..) should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_sub_path("../parent")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_SUB_PATH

    def test_embedded_path_traversal_raises(self):
        """Embedded path traversal should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_sub_path("a/../b")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_SUB_PATH


# ============================================================================
# Host Path Validation Tests
# ============================================================================


class TestEnsureValidHostPath:
    """Tests for ensure_valid_host_path function."""

    def test_valid_absolute_path(self):
        """Absolute paths should be valid."""
        ensure_valid_host_path("/data/opensandbox")
        ensure_valid_host_path("/tmp")

    def test_empty_path_raises(self):
        """Empty path should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_host_path("")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_HOST_PATH

    def test_relative_path_raises(self):
        """Relative paths should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_host_path("data/files")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_HOST_PATH

    def test_path_with_traversal_raises(self):
        """Paths with traversal should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_host_path("/data/../etc/passwd")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_HOST_PATH

    def test_path_with_double_slash_raises(self):
        """Paths with double slashes should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_host_path("/data//files")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_HOST_PATH

    def test_allowed_prefix_match(self):
        """Paths under allowed prefixes should be valid."""
        allowed = ["/data/opensandbox", "/tmp/sandbox"]
        ensure_valid_host_path("/data/opensandbox/user-a", allowed)
        ensure_valid_host_path("/tmp/sandbox/task-1", allowed)

    def test_allowed_prefix_exact_match(self):
        """Exact prefix match should be valid."""
        allowed = ["/data/opensandbox"]
        ensure_valid_host_path("/data/opensandbox", allowed)

    def test_path_not_in_allowed_prefix_raises(self):
        """Paths not under allowed prefixes should raise HTTPException."""
        allowed = ["/data/opensandbox"]
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_host_path("/etc/passwd", allowed)
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.HOST_PATH_NOT_ALLOWED

    def test_partial_prefix_match_raises(self):
        """Partial prefix matches should not be allowed."""
        allowed = ["/data/opensandbox"]
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_host_path("/data/opensandbox-evil", allowed)
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.HOST_PATH_NOT_ALLOWED


# ============================================================================
# PVC Name Validation Tests
# ============================================================================


class TestEnsureValidPvcName:
    """Tests for ensure_valid_pvc_name function."""

    def test_valid_simple_name(self):
        """Simple lowercase names should be valid."""
        ensure_valid_pvc_name("my-pvc")
        ensure_valid_pvc_name("data-volume")
        ensure_valid_pvc_name("pvc1")

    def test_empty_name_raises(self):
        """Empty name should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_pvc_name("")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_PVC_NAME

    def test_name_too_long_raises(self):
        """Name exceeding 253 characters should raise HTTPException."""
        long_name = "a" * 254
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_pvc_name(long_name)
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_PVC_NAME

    def test_uppercase_name_raises(self):
        """Uppercase letters should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_pvc_name("MyPVC")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_PVC_NAME

    def test_underscore_name_raises(self):
        """Underscores should raise HTTPException."""
        with pytest.raises(HTTPException) as exc_info:
            ensure_valid_pvc_name("my_pvc")
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_PVC_NAME


# ============================================================================
# Volumes List Validation Tests
# ============================================================================


class TestEnsureVolumesValid:
    """Tests for ensure_volumes_valid function."""

    def test_none_volumes_valid(self):
        """None volumes should be valid."""
        ensure_volumes_valid(None)

    def test_empty_volumes_valid(self):
        """Empty volumes list should be valid."""
        ensure_volumes_valid([])

    def test_valid_host_volume(self):
        """Valid host volume should pass validation."""
        volume = Volume(
            name="workdir",
            host=Host(path="/data/opensandbox"),
            mount_path="/mnt/work",
            read_only=False,
        )
        ensure_volumes_valid([volume])

    def test_valid_pvc_volume(self):
        """Valid PVC volume should pass validation."""
        volume = Volume(
            name="models",
            pvc=PVC(claim_name="shared-models-pvc"),
            mount_path="/mnt/models",
            read_only=True,
        )
        ensure_volumes_valid([volume])

    def test_valid_volume_with_subpath(self):
        """Valid volume with subPath should pass validation."""
        volume = Volume(
            name="workdir",
            host=Host(path="/data/opensandbox"),
            mount_path="/mnt/work",
            read_only=False,
            sub_path="task-001",
        )
        ensure_volumes_valid([volume])

    def test_multiple_valid_volumes(self):
        """Multiple valid volumes should pass validation."""
        volumes = [
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
        ]
        ensure_volumes_valid(volumes)

    def test_duplicate_volume_name_raises(self):
        """Duplicate volume names should raise HTTPException."""
        volumes = [
            Volume(
                name="workdir",
                host=Host(path="/data/a"),
                mount_path="/mnt/a",
                read_only=False,
            ),
            Volume(
                name="workdir",  # Duplicate name
                host=Host(path="/data/b"),
                mount_path="/mnt/b",
                read_only=False,
            ),
        ]
        with pytest.raises(HTTPException) as exc_info:
            ensure_volumes_valid(volumes)
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.DUPLICATE_VOLUME_NAME

    def test_invalid_volume_name_rejected_by_pydantic(self):
        """Invalid volume name should be rejected by Pydantic pattern validation."""
        from pydantic import ValidationError

        # Pydantic validates the pattern before our validators run
        with pytest.raises(ValidationError) as exc_info:
            Volume(
                name="Invalid_Name",  # Invalid: uppercase and underscore
                host=Host(path="/data/opensandbox"),
                mount_path="/mnt/work",
                read_only=False,
            )
        assert "name" in str(exc_info.value)

    def test_invalid_mount_path_rejected_by_pydantic(self):
        """Invalid mount path should be rejected by Pydantic pattern validation."""
        from pydantic import ValidationError

        # Pydantic validates the pattern before our validators run
        with pytest.raises(ValidationError) as exc_info:
            Volume(
                name="workdir",
                host=Host(path="/data/opensandbox"),
                mount_path="relative/path",  # Invalid: not absolute
                read_only=False,
            )
        assert "mount_path" in str(exc_info.value)

    def test_invalid_subpath_raises(self):
        """Invalid subPath should raise HTTPException."""
        volume = Volume(
            name="workdir",
            host=Host(path="/data/opensandbox"),
            mount_path="/mnt/work",
            read_only=False,
            sub_path="../escape",  # Invalid: path traversal
        )
        with pytest.raises(HTTPException) as exc_info:
            ensure_volumes_valid([volume])
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_SUB_PATH

    def test_host_path_allowlist_enforced(self):
        """Host path allowlist should be enforced."""
        volume = Volume(
            name="workdir",
            host=Host(path="/etc/passwd"),  # Not in allowed list
            mount_path="/mnt/work",
            read_only=False,
        )
        with pytest.raises(HTTPException) as exc_info:
            ensure_volumes_valid([volume], allowed_host_prefixes=["/data/opensandbox"])
        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.HOST_PATH_NOT_ALLOWED

    def test_invalid_pvc_name_rejected_by_pydantic(self):
        """Invalid PVC name should be rejected by Pydantic pattern validation."""
        from pydantic import ValidationError

        # Pydantic validates the pattern before our validators run
        with pytest.raises(ValidationError) as exc_info:
            PVC(claim_name="Invalid_PVC")  # Invalid: uppercase and underscore
        assert "claim_name" in str(exc_info.value)
