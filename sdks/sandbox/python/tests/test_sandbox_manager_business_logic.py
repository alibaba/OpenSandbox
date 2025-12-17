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

import httpx
import pytest

from opensandbox.config import ConnectionConfig
from opensandbox.manager import SandboxManager


class _SandboxServiceStub:
    def __init__(self) -> None:
        self.renew_calls: list[tuple[object, datetime]] = []
        self.pause_calls: list[object] = []

    async def list_sandboxes(self, _filter):  # pragma: no cover
        raise RuntimeError("not used")

    async def get_sandbox_info(self, _sandbox_id):  # pragma: no cover
        raise RuntimeError("not used")

    async def kill_sandbox(self, _sandbox_id):  # pragma: no cover
        raise RuntimeError("not used")

    async def renew_sandbox_expiration(self, sandbox_id, new_expiration_time: datetime) -> None:
        self.renew_calls.append((sandbox_id, new_expiration_time))

    async def pause_sandbox(self, sandbox_id) -> None:
        self.pause_calls.append(sandbox_id)

    async def resume_sandbox(self, _sandbox_id):  # pragma: no cover
        raise RuntimeError("not used")


@pytest.mark.asyncio
async def test_manager_renew_uses_utc_datetime() -> None:
    svc = _SandboxServiceStub()
    mgr = SandboxManager(svc, ConnectionConfig())

    sid = uuid4()
    await mgr.renew_sandbox(sid, timedelta(seconds=5))

    assert len(svc.renew_calls) == 1
    _, dt = svc.renew_calls[0]
    assert dt.tzinfo is timezone.utc


@pytest.mark.asyncio
async def test_manager_close_does_not_close_user_transport() -> None:
    class CustomTransport(httpx.AsyncBaseTransport):
        def __init__(self) -> None:
            self.closed = False

        async def handle_async_request(self, request: httpx.Request) -> httpx.Response:  # pragma: no cover
            raise RuntimeError("not used")

        async def aclose(self) -> None:
            self.closed = True

    t = CustomTransport()
    cfg = ConnectionConfig(transport=t)

    mgr = SandboxManager(_SandboxServiceStub(), cfg)
    await mgr.close()
    assert t.closed is False
