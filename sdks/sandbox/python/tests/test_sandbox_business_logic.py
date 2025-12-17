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

from opensandbox.config import ConnectionConfig
from opensandbox.exceptions import SandboxReadyTimeoutException
from opensandbox.sandbox import Sandbox


class _SandboxServiceStub:
    def __init__(self) -> None:
        self.renew_calls: list[tuple[object, datetime]] = []

    async def renew_sandbox_expiration(self, sandbox_id, expires_at: datetime) -> None:
        self.renew_calls.append((sandbox_id, expires_at))


class _HealthServiceStub:
    def __init__(self, *, should_raise: bool = False) -> None:
        self.should_raise = should_raise
        self.ping_calls: list[object] = []

    async def ping(self, sandbox_id) -> bool:
        self.ping_calls.append(sandbox_id)
        if self.should_raise:
            raise RuntimeError("boom")
        return True


class _Noop:
    pass


def _make_sandbox(*, health_service, sandbox_service, custom_health_check=None) -> Sandbox:
    return Sandbox(
        sandbox_id=uuid4(),
        sandbox_service=sandbox_service,
        filesystem_service=_Noop(),
        command_service=_Noop(),
        health_service=health_service,
        metrics_service=_Noop(),
        connection_config=ConnectionConfig(),
        custom_health_check=custom_health_check,
    )


@pytest.mark.asyncio
async def test_is_healthy_uses_ping_and_swallows_ping_errors() -> None:
    sbx = _make_sandbox(
        health_service=_HealthServiceStub(should_raise=True),
        sandbox_service=_SandboxServiceStub(),
    )
    assert await sbx.is_healthy() is False


@pytest.mark.asyncio
async def test_check_ready_succeeds_after_retries_without_real_sleep(monkeypatch: pytest.MonkeyPatch) -> None:
    # Avoid actual sleeping even if polling_interval > 0.
    async def _no_sleep(_: float) -> None:
        return None

    monkeypatch.setattr("opensandbox.sandbox.asyncio.sleep", _no_sleep)

    calls = {"n": 0}

    async def _custom_health(_: Sandbox) -> bool:
        calls["n"] += 1
        return calls["n"] >= 3

    sbx = _make_sandbox(
        health_service=_HealthServiceStub(),
        sandbox_service=_SandboxServiceStub(),
        custom_health_check=_custom_health,
    )

    await sbx.check_ready(timeout=timedelta(seconds=1), polling_interval=timedelta(seconds=0.01))
    assert calls["n"] == 3


@pytest.mark.asyncio
async def test_check_ready_timeout_raises() -> None:
    async def _always_false(_: Sandbox) -> bool:
        return False

    sbx = _make_sandbox(
        health_service=_HealthServiceStub(),
        sandbox_service=_SandboxServiceStub(),
        custom_health_check=_always_false,
    )

    with pytest.raises(SandboxReadyTimeoutException):
        await sbx.check_ready(timeout=timedelta(seconds=0.01), polling_interval=timedelta(seconds=0))


@pytest.mark.asyncio
async def test_renew_passes_timezone_aware_utc_datetime() -> None:
    svc = _SandboxServiceStub()
    sbx = _make_sandbox(
        health_service=_HealthServiceStub(),
        sandbox_service=svc,
    )

    before = datetime.now(timezone.utc)
    await sbx.renew(timedelta(seconds=10))
    after = datetime.now(timezone.utc)

    assert len(svc.renew_calls) == 1
    _, expires_at = svc.renew_calls[0]
    assert expires_at.tzinfo is timezone.utc
    assert before <= expires_at <= after + timedelta(seconds=12)
