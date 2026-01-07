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

from datetime import datetime, timedelta, timezone
from uuid import uuid4

import pytest

from opensandbox.adapters.sandboxes_adapter import SandboxesAdapter
from opensandbox.config import ConnectionConfig
from opensandbox.exceptions import SandboxApiException
from opensandbox.models.sandboxes import SandboxFilter, SandboxImageSpec


class _Resp:
    def __init__(self, *, status_code: int, parsed) -> None:
        self.status_code = status_code
        self.parsed = parsed


def _api_create_sandbox_response(sandbox_id: str):
    from opensandbox.api.lifecycle.models.create_sandbox_response import (
        CreateSandboxResponse,
    )
    from opensandbox.api.lifecycle.models.sandbox_status import SandboxStatus

    return CreateSandboxResponse(
        id=sandbox_id,
        status=SandboxStatus(state="Running"),
        expires_at=datetime(2025, 1, 2, tzinfo=timezone.utc),
        created_at=datetime(2025, 1, 1, tzinfo=timezone.utc),
        entrypoint=["/bin/sh"],
    )


def _api_list_sandboxes_response():
    from opensandbox.api.lifecycle.models.image_spec import ImageSpec
    from opensandbox.api.lifecycle.models.list_sandboxes_response import (
        ListSandboxesResponse,
    )
    from opensandbox.api.lifecycle.models.pagination_info import PaginationInfo
    from opensandbox.api.lifecycle.models.sandbox import Sandbox
    from opensandbox.api.lifecycle.models.sandbox_status import SandboxStatus

    sbx = Sandbox(
        id=str(uuid4()),
        image=ImageSpec(uri="python:3.11"),
        status=SandboxStatus(state="Running"),
        entrypoint=["/bin/sh"],
        expires_at=datetime(2025, 1, 2, tzinfo=timezone.utc),
        created_at=datetime(2025, 1, 1, tzinfo=timezone.utc),
    )
    return ListSandboxesResponse(
        items=[sbx],
        pagination=PaginationInfo(
            page=0,
            page_size=10,
            total_items=1,
            total_pages=1,
            has_next_page=False,
        ),
    )


@pytest.mark.asyncio
async def test_create_sandbox_success(monkeypatch: pytest.MonkeyPatch) -> None:
    called = {}

    async def _fake_asyncio_detailed(*, client, body):
        called["body"] = body
        return _Resp(status_code=200, parsed=_api_create_sandbox_response(str(uuid4())))

    monkeypatch.setattr(
        "opensandbox.api.lifecycle.api.sandboxes.post_sandboxes.asyncio_detailed",
        _fake_asyncio_detailed,
    )

    cfg = ConnectionConfig(domain="example.com:8080", api_key="k")
    adapter = SandboxesAdapter(cfg)

    out = await adapter.create_sandbox(
        spec=SandboxImageSpec("python:3.11"),
        entrypoint=["/bin/sh"],
        env={},
        metadata={},
        timeout=timedelta(seconds=3),
        resource={"cpu": "100m"},
        extensions={"storage.id": "abc123", "debug": "true"},
    )
    assert isinstance(out.id, str)
    assert "image" in called["body"].to_dict()
    assert called["body"].to_dict()["extensions"] == {"storage.id": "abc123", "debug": "true"}


@pytest.mark.asyncio
async def test_create_sandbox_empty_response_raises(monkeypatch: pytest.MonkeyPatch) -> None:
    async def _fake_asyncio_detailed(*, client, body):
        return _Resp(status_code=200, parsed=None)

    monkeypatch.setattr(
        "opensandbox.api.lifecycle.api.sandboxes.post_sandboxes.asyncio_detailed",
        _fake_asyncio_detailed,
    )

    adapter = SandboxesAdapter(ConnectionConfig())
    with pytest.raises(SandboxApiException):
        await adapter.create_sandbox(
            spec=SandboxImageSpec("python:3.11"),
            entrypoint=["/bin/sh"],
            env={},
            metadata={},
            timeout=timedelta(seconds=1),
            resource={"cpu": "100m"},
            extensions={"debug": "true"},
        )


@pytest.mark.asyncio
async def test_list_sandboxes_metadata_double_encoded(monkeypatch: pytest.MonkeyPatch) -> None:
    from opensandbox.api.lifecycle.types import UNSET as API_UNSET

    captured = {}

    async def _fake_asyncio_detailed(*, client, state, metadata, page, page_size):
        captured.update(
            {"state": state, "metadata": metadata, "page": page, "page_size": page_size}
        )
        return _Resp(status_code=200, parsed=_api_list_sandboxes_response())

    monkeypatch.setattr(
        "opensandbox.api.lifecycle.api.sandboxes.get_sandboxes.asyncio_detailed",
        _fake_asyncio_detailed,
    )

    adapter = SandboxesAdapter(ConnectionConfig())
    f = SandboxFilter(metadata={"k k": "v/v"})
    await adapter.list_sandboxes(f)

    assert captured["metadata"] == "k k=v/v"
    assert captured["state"] is API_UNSET


@pytest.mark.asyncio
async def test_pause_resume_kill_call_openapi(monkeypatch: pytest.MonkeyPatch) -> None:
    sbx_id = str(uuid4())
    calls: list[tuple[str, str]] = []

    async def _ok_pause(*, client, sandbox_id):
        calls.append(("pause", sandbox_id))
        return _Resp(status_code=204, parsed=None)

    async def _ok_resume(*, client, sandbox_id):
        calls.append(("resume", sandbox_id))
        return _Resp(status_code=204, parsed=None)

    async def _ok_kill(*, client, sandbox_id):
        calls.append(("kill", sandbox_id))
        return _Resp(status_code=204, parsed=None)

    monkeypatch.setattr(
        "opensandbox.api.lifecycle.api.sandboxes.post_sandboxes_sandbox_id_pause.asyncio_detailed",
        _ok_pause,
    )
    monkeypatch.setattr(
        "opensandbox.api.lifecycle.api.sandboxes.post_sandboxes_sandbox_id_resume.asyncio_detailed",
        _ok_resume,
    )
    monkeypatch.setattr(
        "opensandbox.api.lifecycle.api.sandboxes.delete_sandboxes_sandbox_id.asyncio_detailed",
        _ok_kill,
    )

    adapter = SandboxesAdapter(ConnectionConfig())
    await adapter.pause_sandbox(sbx_id)
    await adapter.resume_sandbox(sbx_id)
    await adapter.kill_sandbox(sbx_id)

    assert calls == [("pause", sbx_id), ("resume", sbx_id), ("kill", sbx_id)]


@pytest.mark.asyncio
async def test_renew_sandbox_expiration_sends_timezone_aware(monkeypatch: pytest.MonkeyPatch) -> None:
    captured = {}

    async def _fake_asyncio_detailed(*, client, sandbox_id, body):
        from opensandbox.api.lifecycle.models.renew_sandbox_expiration_response import (
            RenewSandboxExpirationResponse,
        )

        captured["expires_at"] = body.expires_at
        return _Resp(
            status_code=200,
            parsed=RenewSandboxExpirationResponse(expires_at=body.expires_at),
        )

    monkeypatch.setattr(
        "opensandbox.api.lifecycle.api.sandboxes.post_sandboxes_sandbox_id_renew_expiration.asyncio_detailed",
        _fake_asyncio_detailed,
    )

    adapter = SandboxesAdapter(ConnectionConfig())
    await adapter.renew_sandbox_expiration(str(uuid4()), datetime(2025, 1, 1))  # naive

    assert captured["expires_at"].tzinfo is timezone.utc
