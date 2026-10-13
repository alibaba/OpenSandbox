#
# Copyright 2026 The OpenSandbox Authors
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
"""OSEP-0024 idle-pause / wake-on-access E2E (fsb integration env).

Requires ``IDLE_PAUSE=1 integration-env.sh up`` plus
``OPENSANDBOX_TEST_FSB_TEMPLATE_ID``; ``integration-env.sh idle-e2e``
exports both. Skipped otherwise and excluded from ``make test``.
"""

import asyncio
import logging
import os
import statistics
import time
from datetime import timedelta

import pytest
from opensandbox.config import ConnectionConfig
from opensandbox.exceptions import SandboxApiException, SandboxException
from opensandbox.manager import SandboxManager
from opensandbox.models.sandboxes import IdlePolicy, SandboxOrigin
from opensandbox.sandbox import Sandbox

logger = logging.getLogger(__name__)

FSB_TEMPLATE_ID = os.getenv("OPENSANDBOX_TEST_FSB_TEMPLATE_ID", "")
IDLE_PAUSE_ENABLED = os.getenv("OPENSANDBOX_TEST_IDLE_PAUSE", "") == "1"
FSB_DOMAIN = os.getenv("OPENSANDBOX_TEST_DOMAIN", "127.0.0.1:18080")
FSB_PROTOCOL = os.getenv("OPENSANDBOX_TEST_PROTOCOL", "http")
FSB_API_KEY = os.getenv("OPENSANDBOX_TEST_API_KEY", "fast-sandbox-env")
# The server rejects X < 30, so 30 is the fastest workable value.
IDLE_TIMEOUT = int(os.getenv("OPENSANDBOX_TEST_IDLE_TIMEOUT", "30"))
IDLE_ROUNDS = int(os.getenv("OPENSANDBOX_TEST_IDLE_ROUNDS", "3"))

pytestmark = pytest.mark.skipif(
    not FSB_TEMPLATE_ID or not IDLE_PAUSE_ENABLED,
    reason="requires the fsb integration environment with IDLE_PAUSE=1 "
    "(OPENSANDBOX_TEST_FSB_TEMPLATE_ID + OPENSANDBOX_TEST_IDLE_PAUSE)",
)

PAUSE_BUDGET = timedelta(seconds=IDLE_TIMEOUT + 210)
# Same-node restore is fast, but a cross-node restore pulls the full
# checkpoint through DART (minutes for multi-GiB guests).
WAKE_BUDGET = timedelta(seconds=600)


def _connection_config() -> ConnectionConfig:
    return ConnectionConfig(
        domain=FSB_DOMAIN, protocol=FSB_PROTOCOL, api_key=FSB_API_KEY
    )


async def _wait_until(operation, timeout: timedelta, description: str):
    """Poll ``operation`` until it returns truthy (callable, not coroutine)."""
    deadline = time.monotonic() + timeout.total_seconds()
    start = time.monotonic()
    last = None
    while time.monotonic() < deadline:
        result = operation()
        if asyncio.iscoroutine(result):
            result = await result
        last = result
        if result:
            logger.info(
                "[wait] %s: satisfied (%.1fs)",
                description,
                time.monotonic() - start,
            )
            return result
        await asyncio.sleep(2)
    raise AssertionError(
        f"timed out after {timeout} waiting for {description}: {last!r}"
    )


async def _state_is(manager: SandboxManager, sandbox_id: str, state: str) -> bool:
    try:
        info = await manager.get_sandbox_info(sandbox_id)
    except SandboxApiException as exc:
        if exc.status_code == 404:
            return False
        raise
    return info is not None and info.status.state == state


async def _kill_and_wait_gone(
    manager: SandboxManager, sandbox_id: str
) -> None:
    try:
        await manager.kill_sandbox(sandbox_id)
    except Exception as exc:  # noqa: BLE001 - cleanup must never mask failures
        logger.warning("cleanup: kill %s failed: %s", sandbox_id, exc)
    finally:

        async def _gone() -> bool:
            try:
                await manager.get_sandbox_info(sandbox_id)
                return False
            except SandboxApiException as exc:
                return exc.status_code == 404

        await _wait_until(_gone, timedelta(minutes=2), f"sandbox {sandbox_id} deleted")


def _summarize(label: str, values: list[float]) -> str:
    return (
        f"{label}: n={len(values)} "
        f"min={min(values):.1f}s max={max(values):.1f}s "
        f"mean={statistics.mean(values):.1f}s "
        f"samples={[round(v, 1) for v in values]}"
    )


async def _wake_and_run(
    sandbox_id: str,
    connection_config: ConnectionConfig,
    marker: str,
) -> None:
    """Drive traffic at a paused sandbox until the wake serves it (no resume API call)."""
    deadline = time.monotonic() + WAKE_BUDGET.total_seconds()
    started = time.monotonic()
    last = "no attempt"
    while time.monotonic() < deadline:
        try:
            attached = await Sandbox.connect(
                sandbox_id,
                connection_config=connection_config,
                skip_health_check=True,
            )
            result = await attached.commands.run(f"echo {marker}")
            if result.error is None and marker in result.logs.stdout[0].text:
                logger.info(
                    "wake traffic served after %.1fs",
                    time.monotonic() - started,
                )
                return
            last = f"run error={result.error}"
        except SandboxException as exc:
            last = str(exc)
        await asyncio.sleep(2)
    raise AssertionError(
        f"wake traffic never succeeded within {WAKE_BUDGET}: {last}"
    )


class TestFsbIdlePauseE2E:
    """OSEP-0024 auto-pause / wake-on-access against the live fsb stack."""

    @pytest.fixture(scope="class")
    def connection_config(self):
        return _connection_config().with_transport_if_missing()

    @pytest.fixture(scope="class")
    async def manager(self, connection_config) -> SandboxManager:
        return await SandboxManager.create(connection_config)

    @pytest.mark.timeout(3600)
    async def test_01_auto_pause_then_wake_on_access(
        self, manager: SandboxManager, connection_config
    ) -> None:
        sandbox = await Sandbox.create_from_template(
            FSB_TEMPLATE_ID,
            timeout=timedelta(hours=2),
            connection_config=connection_config,
            idle_policy=IdlePolicy(
                idle_timeout_seconds=IDLE_TIMEOUT, on_idle="pause"
            ),
            metadata={"origin": "fsb-idle-pause-e2e"},
        )
        logger.info("opted-in sandbox ready: %s (rounds=%d)", sandbox.id, IDLE_ROUNDS)
        try:
            assert sandbox.origin == SandboxOrigin.TEMPLATE

            result = await sandbox.commands.run("echo pre-idle")
            assert result.error is None

            pause_times: list[float] = []
            wake_times: list[float] = []
            for rnd in range(1, IDLE_ROUNDS + 1):
                t0 = time.monotonic()
                await _wait_until(
                    lambda: _state_is(manager, sandbox.id, "Paused"),
                    PAUSE_BUDGET,
                    f"round {rnd}/{IDLE_ROUNDS}: auto-paused",
                )
                pause_times.append(time.monotonic() - t0)

                t1 = time.monotonic()
                await _wake_and_run(
                    sandbox.id, connection_config, f"wake-r{rnd}-ok"
                )
                wake_times.append(time.monotonic() - t1)
                logger.info(
                    "round %d/%d: pause=%.1fs wake=%.1fs",
                    rnd, IDLE_ROUNDS, pause_times[-1], wake_times[-1],
                )

            logger.info(_summarize("auto-pause", pause_times))
            logger.info(_summarize("wake", wake_times))
            for rnd, wake in enumerate(wake_times, start=1):
                assert wake < WAKE_BUDGET.total_seconds(), (
                    f"round {rnd} wake {wake:.1f}s exceeded budget"
                )
        finally:
            await _kill_and_wait_gone(manager, sandbox.id)
