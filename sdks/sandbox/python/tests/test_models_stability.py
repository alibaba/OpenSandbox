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
    SandboxFilter,
    SandboxImageAuth,
    SandboxImageSpec,
    SandboxInfo,
    SandboxStatus,
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
        id=__import__("uuid").uuid4(),
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
