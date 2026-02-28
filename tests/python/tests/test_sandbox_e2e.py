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
"""
Comprehensive E2E tests for Sandbox functionality.
"""

import asyncio
import logging
import time
from datetime import timedelta
from io import BytesIO

import pytest
from opensandbox import Sandbox
from opensandbox.models.execd import (
    ExecutionComplete,
    ExecutionError,
    ExecutionHandlers,
    ExecutionInit,
    ExecutionResult,
    OutputMessage,
    RunCommandOpts,
)
from opensandbox.models.filesystem import (
    ContentReplaceEntry,
    MoveEntry,
    SearchEntry,
    SetPermissionEntry,
    WriteEntry,
)
from opensandbox.models.sandboxes import Host, NetworkPolicy, NetworkRule, PVC, SandboxImageSpec, Volume

from tests.base_e2e_test import create_connection_config, get_sandbox_image

logger = logging.getLogger(__name__)


def _now_ms() -> int:
    return int(time.time() * 1000)


def _assert_recent_timestamp_ms(ts: int, *, tolerance_ms: int = 60_000) -> None:
    assert isinstance(ts, int)
    assert ts > 0
    delta = abs(_now_ms() - ts)
    assert delta <= tolerance_ms, f"timestamp too far from now: delta={delta}ms (ts={ts})"


def _assert_endpoint_has_port(endpoint: str, expected_port: int) -> None:
    assert endpoint
    # In some deployments lifecycle returns direct "host:port".
    # In others it returns a reverse-proxy route like "domain/route/{id}/{port}".
    # In both cases, we expect NO scheme, and the port to be present deterministically.
    assert "://" not in endpoint, f"unexpected scheme in endpoint: {endpoint}"

    if "/" in endpoint:
        assert endpoint.endswith(f"/{expected_port}"), (
            f"endpoint route must end with /{expected_port}: {endpoint}"
        )
        # Keep this strict: the route must contain a non-empty domain prefix.
        assert endpoint.split("/", 1)[0], f"missing domain in endpoint: {endpoint}"
        return

    host, port = endpoint.rsplit(":", 1)
    assert host, f"missing host in endpoint: {endpoint}"
    assert port.isdigit(), f"non-numeric port in endpoint: {endpoint}"
    assert int(port) == expected_port, f"endpoint port mismatch: {endpoint} != :{expected_port}"


def _assert_times_close(created_at, modified_at, *, tolerance_seconds: float = 2.0) -> None:
    """
    Some filesystems / implementations may report created/modified with slight reordering.
    We only assert they're close, and rely on explicit update operations to validate mtime.
    """
    delta = abs((modified_at - created_at).total_seconds())
    assert delta <= tolerance_seconds, f"created/modified skew too large: {delta}s"


def _assert_modified_updated(before, after, *, min_delta_ms: int = 0, allow_skew_ms: int = 1000) -> None:
    """
    Validate modified_at moved forward after a mutating operation, allowing small clock jitter.
    """
    delta_ms = int((after - before).total_seconds() * 1000)
    assert delta_ms >= min_delta_ms - allow_skew_ms, (
        f"modified_at did not update as expected: delta_ms={delta_ms} "
        f"(min_delta_ms={min_delta_ms}, allow_skew_ms={allow_skew_ms})"
    )


@pytest.mark.asyncio
class TestSandboxE2E:
    """Comprehensive E2E tests for Sandbox functionality."""

    sandbox = None
    connection_config = None
    _setup_done = False

    @pytest.fixture(scope="class", autouse=True)
    async def _sandbox_lifecycle(self, request):
        """Create sandbox once and ALWAYS cleanup to avoid resource leaks."""
        await request.cls._ensure_sandbox_created()
        try:
            yield
        finally:
            sandbox = request.cls.sandbox
            if sandbox is not None:
                try:
                    # await sandbox.kill()
                    pass
                except Exception as e:
                    logger.warning("Teardown: sandbox.kill() failed: %s", e, exc_info=True)
                try:
                    await sandbox.close()
                except Exception as e:
                    logger.warning("Teardown: sandbox.close() failed: %s", e, exc_info=True)

    @classmethod
    async def _ensure_sandbox_created(cls):
        """Ensure sandbox is created before running tests."""
        if cls._setup_done:
            return

        logger.info("=" * 100)
        logger.info("SETUP: Creating sandbox")
        logger.info("=" * 100)

        cls.connection_config = create_connection_config()

        cls.sandbox = await Sandbox.create(
            image=SandboxImageSpec(get_sandbox_image()),
            connection_config=cls.connection_config,
            timeout=timedelta(minutes=2),
            ready_timeout=timedelta(seconds=30),
            metadata={"tag": "e2e-test"},
            env={
                "E2E_TEST": "true",
                "GO_VERSION": "1.25",
                "JAVA_VERSION": "21",
                "NODE_VERSION": "22",
                "PYTHON_VERSION": "3.12"
            },
            health_check_polling_interval=timedelta(milliseconds=500),
        )

        logger.info(f"✓ Sandbox created: {cls.sandbox.id}")
        logger.info("=" * 100)

        cls._setup_done = True

    @pytest.mark.timeout(120)
    @pytest.mark.order(1)
    async def test_01_sandbox_lifecycle_and_health(self):
        """Test sandbox lifecycle and health monitoring."""
        await self._ensure_sandbox_created()
        sandbox = TestSandboxE2E.sandbox

        logger.info("=" * 80)
        logger.info("TEST 1: Testing sandbox lifecycle and health monitoring")
        logger.info("=" * 80)

        logger.info("Step 1: Verify basic sandbox properties")
        assert sandbox is not None
        assert isinstance(sandbox.id, str)
        assert await sandbox.is_healthy() is True
        logger.info(f"✓ Sandbox ID: {sandbox.id}")
        logger.info("✓ Sandbox is healthy")

        logger.info("Step 2: Get sandbox information")
        info = await sandbox.get_info()
        assert info.id == sandbox.id
        assert info.status.state == "Running"
        assert info.created_at is not None
        assert info.expires_at is not None
        assert info.expires_at > info.created_at
        assert info.entrypoint == ["tail", "-f", "/dev/null"]

        duration = info.expires_at - info.created_at
        min_duration = timedelta(minutes=1)
        max_duration = timedelta(minutes=3)
        assert min_duration <= duration <= max_duration, \
            f"Duration {duration} should be between 1 and 3 minutes"

        assert info.metadata is not None
        assert info.metadata.get("tag") == "e2e-test"
        logger.info(
            "✓ Sandbox info: state=%s, created=%s, expires=%s",
            info.status.state,
            info.created_at,
            info.expires_at,
        )

        logger.info("Step 3: Get sandbox endpoint for default execd port")
        endpoint = await sandbox.get_endpoint(44772)
        assert endpoint is not None
        assert endpoint.endpoint is not None
        _assert_endpoint_has_port(endpoint.endpoint, 44772)
        logger.info(f"✓ Sandbox endpoint: {endpoint.endpoint}")

        logger.info("Step 4: Get and verify metrics")
        metrics = await sandbox.get_metrics()
        assert metrics is not None
        assert metrics.cpu_count > 0
        assert 0.0 <= metrics.cpu_used_percentage <= 100.0
        assert metrics.memory_total_in_mib > 0
        assert 0.0 <= metrics.memory_used_in_mib <= metrics.memory_total_in_mib
        _assert_recent_timestamp_ms(metrics.timestamp, tolerance_ms=120_000)
        logger.info(
            "✓ CPU: %s cores, %.2f%% used",
            metrics.cpu_count,
            metrics.cpu_used_percentage,
        )
        logger.info(
            "✓ Memory: %s/%s MiB",
            int(metrics.memory_used_in_mib),
            int(metrics.memory_total_in_mib),
        )

        logger.info("Step 5: Test sandbox renewal (extend expiration time)")
        renew_response = await sandbox.renew(timedelta(minutes=20))
        assert renew_response is not None
        assert renew_response.expires_at > info.expires_at
        logger.info("✓ Sandbox expiration renewed to %s", renew_response.expires_at)

        renewed_info = await sandbox.get_info()
        assert renewed_info.expires_at > info.expires_at
        assert renewed_info.id == sandbox.id
        assert renewed_info.status.state == "Running"

        # The renew API should return the new expiration time. Allow small backend-side skew.
        assert abs((renewed_info.expires_at - renew_response.expires_at).total_seconds()) < 10

        # Renewal is "now + timeout" (SDK behavior). Validate remaining TTL is close to 5 minutes.
        now = renewed_info.expires_at.__class__.now(tz=renewed_info.expires_at.tzinfo)
        remaining = renewed_info.expires_at - now
        assert remaining > timedelta(minutes=18), f"Remaining TTL too small: {remaining}"
        assert remaining < timedelta(minutes=22), f"Remaining TTL too large: {remaining}"

        logger.info(
            "✓ Sandbox expiration updated from %s to %s",
            info.expires_at,
            renewed_info.expires_at,
        )

        logger.info("Step 6: Test access to service components")

        assert sandbox.files is not None
        assert sandbox.commands is not None
        assert sandbox.metrics is not None
        assert sandbox.connection_config is not None
        logger.info("✓ All sandbox service components are accessible")

        logger.info("Step 7: Connect to existing sandbox by ID")
        sandbox2 = await Sandbox.connect(
            sandbox_id=sandbox.id,
            connection_config=TestSandboxE2E.connection_config,
        )
        try:
            assert sandbox2.id == sandbox.id
            assert await sandbox2.is_healthy() is True
            connect_result = await sandbox2.commands.run("echo connect-ok")
            assert connect_result.error is None
            assert len(connect_result.logs.stdout) == 1
            assert connect_result.logs.stdout[0].text == "connect-ok"
        finally:
            await sandbox2.close()

        logger.info("TEST 1 PASSED: Sandbox lifecycle and health test completed successfully")


    @pytest.mark.timeout(120)
    @pytest.mark.order(1)
    async def test_01a_network_policy_create(self):
        logger.info("=" * 80)
        logger.info("TEST 1a: Creating sandbox with networkPolicy (async)")
        logger.info("=" * 80)

        cfg = create_connection_config()
        sandbox = await Sandbox.create(
            image=SandboxImageSpec(get_sandbox_image()),
            connection_config=cfg,
            timeout=timedelta(minutes=2),
            ready_timeout=timedelta(seconds=30),
            network_policy=NetworkPolicy(
                defaultAction="deny",
                egress=[NetworkRule(action="allow", target="pypi.org")],
            ),
        )
        try:
            await asyncio.sleep(5)
            result = await sandbox.commands.run("curl -I https://www.github.com")
            assert result.error is not None
            result = await sandbox.commands.run("curl -I https://pypi.org")
            assert result.error is None
        finally:
            try:
                await sandbox.kill()
            except Exception:
                pass
            await sandbox.close()

    @pytest.mark.timeout(120)
    @pytest.mark.order(1)
    async def test_01b_host_volume_mount(self):
        """Test creating a sandbox with a host volume mount."""
        logger.info("=" * 80)
        logger.info("TEST 1b: Creating sandbox with host volume mount (async)")
        logger.info("=" * 80)

        host_dir = "/tmp/opensandbox-e2e/host-volume-test"
        container_mount_path = "/mnt/host-data"

        cfg = create_connection_config()
        sandbox = await Sandbox.create(
            image=SandboxImageSpec(get_sandbox_image()),
            connection_config=cfg,
            timeout=timedelta(minutes=2),
            ready_timeout=timedelta(seconds=30),
            volumes=[
                Volume(
                    name="test-host-vol",
                    host=Host(path=host_dir),
                    mountPath=container_mount_path,
                    readOnly=False,
                ),
            ],
        )
        try:
            logger.info(f"✓ Sandbox with volume created: {sandbox.id}")

            # Step 1: Verify the host marker file is visible inside the sandbox
            logger.info("Step 1: Verify host marker file is readable inside the sandbox")
            result = await sandbox.commands.run(f"cat {container_mount_path}/marker.txt")
            assert result.error is None, f"Failed to read marker file: {result.error}"
            assert len(result.logs.stdout) == 1
            assert result.logs.stdout[0].text == "opensandbox-e2e-marker"
            logger.info("✓ Host marker file read successfully inside sandbox")

            # Step 2: Write a file from inside the sandbox to the mounted path (read-write)
            logger.info("Step 2: Write a file from inside the sandbox to the mount path")
            result = await sandbox.commands.run(
                f"echo 'written-from-sandbox' > {container_mount_path}/sandbox-output.txt"
            )
            assert result.error is None, f"Failed to write file: {result.error}"

            # Step 3: Verify the written file is readable
            result = await sandbox.commands.run(f"cat {container_mount_path}/sandbox-output.txt")
            assert result.error is None
            assert len(result.logs.stdout) == 1
            assert result.logs.stdout[0].text == "written-from-sandbox"
            logger.info("✓ File written and verified inside sandbox")

            # Step 4: Verify the mount path is a proper directory
            logger.info("Step 3: Verify mount path is a directory")
            result = await sandbox.commands.run(f"test -d {container_mount_path} && echo OK")
            assert result.error is None
            assert len(result.logs.stdout) == 1
            assert result.logs.stdout[0].text == "OK"
            logger.info("✓ Mount path is a valid directory")

        finally:
            try:
                await sandbox.kill()
            except Exception:
                pass
            await sandbox.close()

        logger.info("TEST 1b PASSED: Host volume mount test completed successfully")

    @pytest.mark.timeout(120)
    @pytest.mark.order(1)
    async def test_01c_host_volume_mount_readonly(self):
        """Test creating a sandbox with a read-only host volume mount."""
        logger.info("=" * 80)
        logger.info("TEST 1c: Creating sandbox with read-only host volume mount (async)")
        logger.info("=" * 80)

        host_dir = "/tmp/opensandbox-e2e/host-volume-test"
        container_mount_path = "/mnt/host-data-ro"

        cfg = create_connection_config()
        sandbox = await Sandbox.create(
            image=SandboxImageSpec(get_sandbox_image()),
            connection_config=cfg,
            timeout=timedelta(minutes=2),
            ready_timeout=timedelta(seconds=30),
            volumes=[
                Volume(
                    name="test-host-vol-ro",
                    host=Host(path=host_dir),
                    mountPath=container_mount_path,
                    readOnly=True,
                ),
            ],
        )
        try:
            logger.info(f"✓ Sandbox with read-only volume created: {sandbox.id}")

            # Step 1: Verify the host marker file is readable
            result = await sandbox.commands.run(f"cat {container_mount_path}/marker.txt")
            assert result.error is None, f"Failed to read marker file: {result.error}"
            assert len(result.logs.stdout) == 1
            assert result.logs.stdout[0].text == "opensandbox-e2e-marker"
            logger.info("✓ Host marker file read successfully in read-only mount")

            # Step 2: Verify writing is denied on read-only mount
            result = await sandbox.commands.run(
                f"touch {container_mount_path}/should-fail.txt"
            )
            assert result.error is not None, "Write should fail on read-only mount"
            logger.info("✓ Write correctly denied on read-only mount")

        finally:
            try:
                await sandbox.kill()
            except Exception:
                pass
            await sandbox.close()

        logger.info("TEST 1c PASSED: Read-only host volume mount test completed successfully")

    @pytest.mark.timeout(120)
    @pytest.mark.order(1)
    async def test_01d_pvc_named_volume_mount(self):
        """Test creating a sandbox with a PVC (Docker named volume) mount."""
        logger.info("=" * 80)
        logger.info("TEST 1d: Creating sandbox with PVC named volume mount (async)")
        logger.info("=" * 80)

        pvc_volume_name = "opensandbox-e2e-pvc-test"
        container_mount_path = "/mnt/pvc-data"

        cfg = create_connection_config()
        sandbox = await Sandbox.create(
            image=SandboxImageSpec(get_sandbox_image()),
            connection_config=cfg,
            timeout=timedelta(minutes=2),
            ready_timeout=timedelta(seconds=30),
            volumes=[
                Volume(
                    name="test-pvc-vol",
                    pvc=PVC(claimName=pvc_volume_name),
                    mountPath=container_mount_path,
                    readOnly=False,
                ),
            ],
        )
        try:
            logger.info(f"✓ Sandbox with PVC volume created: {sandbox.id}")

            # Step 1: Verify the marker file seeded into the named volume is readable
            logger.info("Step 1: Verify PVC marker file is readable inside the sandbox")
            result = await sandbox.commands.run(f"cat {container_mount_path}/marker.txt")
            assert result.error is None, f"Failed to read marker file: {result.error}"
            assert len(result.logs.stdout) == 1
            assert result.logs.stdout[0].text == "pvc-marker-data"
            logger.info("✓ PVC marker file read successfully inside sandbox")

            # Step 2: Write a file from inside the sandbox to the named volume
            logger.info("Step 2: Write a file from inside the sandbox to the PVC mount")
            result = await sandbox.commands.run(
                f"echo 'written-to-pvc' > {container_mount_path}/pvc-output.txt"
            )
            assert result.error is None, f"Failed to write file: {result.error}"

            # Step 3: Verify the written file is readable
            result = await sandbox.commands.run(f"cat {container_mount_path}/pvc-output.txt")
            assert result.error is None
            assert len(result.logs.stdout) == 1
            assert result.logs.stdout[0].text == "written-to-pvc"
            logger.info("✓ File written and verified inside sandbox via PVC mount")

            # Step 4: Verify the mount path is a proper directory
            result = await sandbox.commands.run(f"test -d {container_mount_path} && echo OK")
            assert result.error is None
            assert len(result.logs.stdout) == 1
            assert result.logs.stdout[0].text == "OK"
            logger.info("✓ PVC mount path is a valid directory")

        finally:
            try:
                await sandbox.kill()
            except Exception:
                pass
            await sandbox.close()

        logger.info("TEST 1d PASSED: PVC named volume mount test completed successfully")

    @pytest.mark.timeout(120)
    @pytest.mark.order(1)
    async def test_01e_pvc_named_volume_mount_readonly(self):
        """Test creating a sandbox with a read-only PVC (Docker named volume) mount."""
        logger.info("=" * 80)
        logger.info("TEST 1e: Creating sandbox with read-only PVC named volume mount (async)")
        logger.info("=" * 80)

        pvc_volume_name = "opensandbox-e2e-pvc-test"
        container_mount_path = "/mnt/pvc-data-ro"

        cfg = create_connection_config()
        sandbox = await Sandbox.create(
            image=SandboxImageSpec(get_sandbox_image()),
            connection_config=cfg,
            timeout=timedelta(minutes=2),
            ready_timeout=timedelta(seconds=30),
            volumes=[
                Volume(
                    name="test-pvc-vol-ro",
                    pvc=PVC(claimName=pvc_volume_name),
                    mountPath=container_mount_path,
                    readOnly=True,
                ),
            ],
        )
        try:
            logger.info(f"✓ Sandbox with read-only PVC volume created: {sandbox.id}")

            # Step 1: Verify the marker file is readable on read-only mount
            result = await sandbox.commands.run(f"cat {container_mount_path}/marker.txt")
            assert result.error is None, f"Failed to read marker file: {result.error}"
            assert len(result.logs.stdout) == 1
            assert result.logs.stdout[0].text == "pvc-marker-data"
            logger.info("✓ PVC marker file read successfully in read-only mount")

            # Step 2: Verify writing is denied on read-only mount
            result = await sandbox.commands.run(
                f"touch {container_mount_path}/should-fail.txt"
            )
            assert result.error is not None, "Write should fail on read-only PVC mount"
            logger.info("✓ Write correctly denied on read-only PVC mount")

        finally:
            try:
                await sandbox.kill()
            except Exception:
                pass
            await sandbox.close()

        logger.info("TEST 1e PASSED: Read-only PVC named volume mount test completed successfully")

    @pytest.mark.timeout(120)
    @pytest.mark.order(1)
    async def test_01f_pvc_named_volume_subpath_mount(self):
        """Test creating a sandbox with a PVC named volume mount using subPath."""
        logger.info("=" * 80)
        logger.info("TEST 1f: Creating sandbox with PVC named volume subPath mount (async)")
        logger.info("=" * 80)

        pvc_volume_name = "opensandbox-e2e-pvc-test"
        container_mount_path = "/mnt/train"

        cfg = create_connection_config()
        sandbox = await Sandbox.create(
            image=SandboxImageSpec(get_sandbox_image()),
            connection_config=cfg,
            timeout=timedelta(minutes=2),
            ready_timeout=timedelta(seconds=30),
            volumes=[
                Volume(
                    name="test-pvc-subpath",
                    pvc=PVC(claimName=pvc_volume_name),
                    mountPath=container_mount_path,
                    readOnly=False,
                    subPath="datasets/train",
                ),
            ],
        )
        try:
            logger.info(f"✓ Sandbox with PVC subPath volume created: {sandbox.id}")

            # Step 1: Verify the subpath marker file is readable
            logger.info("Step 1: Verify subPath marker file is readable")
            result = await sandbox.commands.run(f"cat {container_mount_path}/marker.txt")
            assert result.error is None, f"Failed to read subpath marker file: {result.error}"
            assert len(result.logs.stdout) == 1
            assert result.logs.stdout[0].text == "pvc-subpath-marker"
            logger.info("✓ SubPath marker file read successfully")

            # Step 2: Verify we only see the subpath contents (not the full volume)
            logger.info("Step 2: Verify only subPath contents are visible")
            result = await sandbox.commands.run(f"ls {container_mount_path}/")
            assert result.error is None
            # Should contain marker.txt but NOT 'datasets' directory (we are inside it)
            stdout_text = "\n".join(msg.text for msg in result.logs.stdout)
            assert "marker.txt" in stdout_text
            assert "datasets" not in stdout_text
            logger.info("✓ Only subPath contents are visible inside the sandbox")

            # Step 3: Write a file and verify (retry read-back for transient SSE drops)
            logger.info("Step 3: Write and verify a file inside subPath mount")
            result = await sandbox.commands.run(
                f"echo 'subpath-write-test' > {container_mount_path}/output.txt"
            )
            assert result.error is None
            for _attempt in range(3):
                result = await sandbox.commands.run(f"cat {container_mount_path}/output.txt")
                if result.logs.stdout:
                    break
                await asyncio.sleep(1)
            assert result.error is None
            assert len(result.logs.stdout) == 1
            assert result.logs.stdout[0].text == "subpath-write-test"
            logger.info("✓ File written and verified inside subPath mount")

        finally:
            try:
                await sandbox.kill()
            except Exception:
                pass
            await sandbox.close()

        logger.info("TEST 1f PASSED: PVC subPath named volume mount test completed successfully")

    @pytest.mark.timeout(120)
    @pytest.mark.order(2)
    async def test_02_basic_command_execution(self):
        """Test basic command execution."""
        await self._ensure_sandbox_created()
        sandbox = TestSandboxE2E.sandbox

        logger.info("=" * 80)
        logger.info("TEST 2: Testing basic command execution")
        logger.info("=" * 80)

        logger.info("Step 1: Simple echo command with handlers to capture events")
        stdout_messages = []
        stderr_messages = []
        results = []
        completed_events = []
        errors = []
        init_events = []

        async def on_stdout(msg: OutputMessage):
            stdout_messages.append(msg)
            logger.info(f"Stdout: {msg.text}")

        async def on_stderr(msg: OutputMessage):
            stderr_messages.append(msg)
            logger.warning(f"Stderr: {msg.text}")

        async def on_result(result: ExecutionResult):
            results.append(result)
            logger.info(f"Result: {result.text}")

        async def on_execution_complete(complete: ExecutionComplete):
            completed_events.append(complete)
            logger.info(f"Execution completed in {complete.execution_time_in_millis} ms")

        async def on_error(error: ExecutionError):
            errors.append(error)
            logger.error(f"Error: {error.name} - {error.value}")

        async def on_init(init: ExecutionInit):
            init_events.append(init)
            logger.info(f"Execution initialized with ID: {init.id}")

        handlers = ExecutionHandlers(
            on_stdout=on_stdout,
            on_stderr=on_stderr,
            on_result=on_result,
            on_execution_complete=on_execution_complete,
            on_error=on_error,
            on_init=on_init
        )

        echo_result = await sandbox.commands.run(
            "echo 'Hello OpenSandbox E2E'",
            handlers=handlers,
        )

        # Verify result
        assert echo_result is not None
        assert echo_result.id is not None and echo_result.id.strip()
        assert echo_result.error is None
        assert len(echo_result.logs.stdout) == 1
        assert echo_result.logs.stdout[0].text == "Hello OpenSandbox E2E"
        assert echo_result.logs.stdout[0].is_error is False
        _assert_recent_timestamp_ms(echo_result.logs.stdout[0].timestamp)
        assert len(echo_result.logs.stderr) == 0

        # Verify handlers captured events
        assert len(init_events) == 1, "Execution should have exactly one init event"
        assert len(completed_events) == 1, "Execution should have exactly one completion event"
        assert init_events[0].id == echo_result.id
        _assert_recent_timestamp_ms(init_events[0].timestamp)
        _assert_recent_timestamp_ms(completed_events[0].timestamp)
        assert completed_events[0].execution_time_in_millis >= 0

        assert len(stdout_messages) == 1, "Should have captured exactly one stdout message"
        assert stdout_messages[0].text == "Hello OpenSandbox E2E"
        assert stdout_messages[0].is_error is False
        _assert_recent_timestamp_ms(stdout_messages[0].timestamp)

        assert len(errors) == 0, "Should have no errors for successful command"

        logger.info(
            "✓ Captured %s stdout, %s stderr, %s results, %s errors, %s completions, %s inits",
            len(stdout_messages),
            len(stderr_messages),
            len(results),
            len(errors),
            len(completed_events),
            len(init_events),
        )

        logger.info("Step 2: Command with working directory")
        pwd_result = await sandbox.commands.run(
            "pwd",
            opts=RunCommandOpts(working_directory="/tmp"),
        )
        assert pwd_result is not None
        assert pwd_result.id is not None and pwd_result.id.strip()
        assert pwd_result.error is None
        assert len(pwd_result.logs.stdout) == 1
        assert pwd_result.logs.stdout[0].text == "/tmp"
        assert pwd_result.logs.stdout[0].is_error is False
        _assert_recent_timestamp_ms(pwd_result.logs.stdout[0].timestamp)
        logger.info(f"✓ PWD command executed: {pwd_result}")

        logger.info("Step 3: Background command")
        start_time = time.time()
        await sandbox.commands.run(
            "sleep 30",
            opts=RunCommandOpts(background=True),
        )
        end_time = time.time()

        execution_time = (end_time - start_time) * 1000
        assert execution_time < 10000, \
            f"Background command should return quickly, but took {execution_time} ms"
        logger.info(f"✓ Background command returned in {execution_time:.2f} ms")

        logger.info("Step 4: Test failing command")
        # Clear event lists for fail test
        stdout_messages.clear()
        stderr_messages.clear()
        errors.clear()
        completed_events.clear()
        init_events.clear()

        fail_result = await sandbox.commands.run(
            "nonexistent-command-that-does-not-exist",
            handlers=handlers,
        )

        # Verify error result
        assert fail_result is not None
        assert fail_result.id is not None and fail_result.id.strip()
        assert fail_result.error is not None
        assert fail_result.error.name == "CommandExecError"
        assert len(fail_result.logs.stderr) > 0
        assert any(
            "nonexistent-command-that-does-not-exist" in m.text for m in fail_result.logs.stderr
        )
        assert all(m.is_error is True for m in fail_result.logs.stderr)
        _assert_recent_timestamp_ms(fail_result.logs.stderr[0].timestamp)

        # Verify handlers captured error events
        assert len(init_events) == 1, "Execution should have exactly one init event"
        assert init_events[0].id == fail_result.id
        _assert_recent_timestamp_ms(init_events[0].timestamp)
        # Contract: error and complete are mutually exclusive; failing command should emit error only.
        assert len(errors) >= 1, "Should have captured error events"
        assert len(completed_events) == 0, "Failing command should not emit completion event"

        assert errors[0].name == "CommandExecError", "Error name should match"
        assert len(stderr_messages) > 0, "Should have captured stderr messages"
        assert "nonexistent-command-that-does-not-exist" in stderr_messages[0].text, (
            "Stderr should contain command name"
        )

        logger.info(f"✓ Failed command result: {fail_result}")

        logger.info("TEST 2 PASSED: Basic command execution test completed successfully")

    @pytest.mark.timeout(120)
    @pytest.mark.order(3)
    async def test_02a_command_status_and_logs(self):
        """Test command status + background logs."""
        await self._ensure_sandbox_created()
        sandbox = TestSandboxE2E.sandbox

        exec_result = await sandbox.commands.run(
            "sh -c 'echo log-line-1; echo log-line-2; sleep 2'",
            opts=RunCommandOpts(background=True),
        )
        assert exec_result.id is not None
        command_id = exec_result.id

        status = await sandbox.commands.get_command_status(command_id)
        assert status.id == command_id
        assert isinstance(status.running, bool)

        logs_text = ""
        cursor = None
        for _ in range(20):
            logs = await sandbox.commands.get_background_command_logs(command_id, cursor=cursor)
            logs_text += logs.content
            cursor = logs.cursor if logs.cursor is not None else cursor
            if "log-line-2" in logs_text:
                break
            await asyncio.sleep(1.0)

        assert "log-line-1" in logs_text
        assert "log-line-2" in logs_text

    @pytest.mark.timeout(120)
    @pytest.mark.order(4)
    async def test_03_basic_filesystem_operations(self):
        """Test basic filesystem operations."""
        await self._ensure_sandbox_created()
        sandbox = TestSandboxE2E.sandbox

        logger.info("=" * 80)
        logger.info("TEST 3: Testing basic filesystem operations")
        logger.info("=" * 80)

        test_dir1 = f"/tmp/fs_test1_{int(time.time() * 1000)}"
        test_dir2 = f"/tmp/fs_test2_{int(time.time() * 1000)}"

        logger.info("Step 1: Create directories")
        dir_entry1 = WriteEntry(path=test_dir1, mode=755)
        dir_entry2 = WriteEntry(path=test_dir2, mode=644)
        await sandbox.files.create_directories([dir_entry1, dir_entry2])
        logger.info(f"✓ Created directories: {test_dir1}, {test_dir2}")

        dir_info_map = await sandbox.files.get_file_info([test_dir1, test_dir2])
        assert test_dir1 in dir_info_map
        assert test_dir2 in dir_info_map
        assert dir_info_map[test_dir1].path == test_dir1
        assert dir_info_map[test_dir2].path == test_dir2
        assert dir_info_map[test_dir1].mode == 755
        assert dir_info_map[test_dir2].mode == 644
        assert dir_info_map[test_dir1].owner
        assert dir_info_map[test_dir1].group
        _assert_times_close(dir_info_map[test_dir1].created_at, dir_info_map[test_dir1].modified_at)

        ls_result = await sandbox.commands.run(
            "ls -la | grep fs_test",
            opts=RunCommandOpts(working_directory="/tmp"),
        )
        assert len(ls_result.logs.stdout) == 2, "Should find exactly 2 directories"
        logger.info(f"✓ Directory verification: {ls_result}")

        logger.info("Step 2: Create and write files")
        test_file1 = f"{test_dir1}/test_file1.txt"
        test_file2 = f"{test_dir1}/test_file2.txt"
        test_file3 = f"{test_dir1}/test_file3.txt"
        test_content = "Hello Filesystem!\\nLine 2 with special chars: åäö\\nLine 3"

        write_entry1 = WriteEntry(path=test_file1, data=test_content, mode=644)
        write_entry2 = WriteEntry(path=test_file2, data=test_content.encode('utf-8'), mode=755)
        write_entry3 = WriteEntry(
            path=test_file3,
            data=BytesIO(test_content.encode('utf-8')),
            group="nogroup",
            owner="nobody",
            mode=755
        )
        await sandbox.files.write_files([write_entry1, write_entry2, write_entry3])
        logger.info("✓ Created 3 test files")

        logger.info("Step 3: Read and verify file content using different methods")
        read_content1 = await sandbox.files.read_file(test_file1, encoding='utf-8')
        read_content1_partial = await sandbox.files.read_file(
            test_file1, encoding='utf-8', range_header="bytes=0-9"
        )

        read_bytes2 = await sandbox.files.read_bytes(test_file2)
        read_content2 = read_bytes2.decode('utf-8')

        stream3 = await sandbox.files.read_bytes_stream(test_file3)
        read_content3_bytes = b""
        async for chunk in stream3:
            read_content3_bytes += chunk
        read_content3 = read_content3_bytes.decode("utf-8")

        expected_size = len(test_content.encode("utf-8"))
        assert read_content1 == test_content
        assert read_content2 == test_content
        assert read_content3 == test_content
        assert read_content1_partial == test_content[:10]
        logger.info("✓ All file reads successful and content verified")

        logger.info("Step 4: Get and verify file info")
        all_test_files = [test_file1, test_file2, test_file3]
        file_info_map = await sandbox.files.get_file_info(all_test_files)

        file_info1 = file_info_map[test_file1]
        assert file_info1 is not None
        assert file_info1.path == test_file1
        assert file_info1.size == expected_size
        assert file_info1.mode == 644
        assert file_info1.owner is not None
        assert file_info1.group is not None
        _assert_times_close(file_info1.created_at, file_info1.modified_at)

        file_info2 = file_info_map[test_file2]
        assert file_info2 is not None
        assert file_info2.path == test_file2
        assert file_info2.size == expected_size
        assert file_info2.mode == 755
        assert file_info2.owner is not None
        assert file_info2.group is not None
        _assert_times_close(file_info2.created_at, file_info2.modified_at)

        file_info3 = file_info_map[test_file3]
        assert file_info3 is not None
        assert file_info3.path == test_file3
        assert file_info3.size == expected_size
        assert file_info3.mode == 755
        assert file_info3.owner == "nobody"
        assert file_info3.group == "nogroup"
        _assert_times_close(file_info3.created_at, file_info3.modified_at)
        logger.info(f"✓ File info verified: size={file_info1.size}, mode={oct(file_info1.mode)}")

        logger.info("Step 5: Test search functionality")
        search_all_entry = SearchEntry(path=test_dir1, pattern="*")
        all_files_list = await sandbox.files.search(search_all_entry)
        all_files = {entry.path: entry for entry in all_files_list}

        assert len(all_files) == 3
        assert test_file1 in all_files
        assert test_file2 in all_files
        assert test_file3 in all_files
        assert all_files[test_file1].size == expected_size
        _assert_times_close(all_files[test_file1].created_at, all_files[test_file1].modified_at)
        logger.info("✓ Search found all 3 files")

        logger.info("Step 6: Test permission changes")
        perm_entry1 = SetPermissionEntry(
            path=test_file1,
            mode=755,
            owner="nobody",
            group="nogroup"
        )
        perm_entry2 = SetPermissionEntry(
            path=test_file2,
            mode=600,
            owner="nobody",
            group="nogroup"
        )
        await sandbox.files.set_permissions([perm_entry1, perm_entry2])

        updated_info_map = await sandbox.files.get_file_info([test_file1, test_file2])
        updated_info1 = updated_info_map[test_file1]
        updated_info2 = updated_info_map[test_file2]

        assert updated_info1.mode == 755
        assert updated_info1.owner == "nobody"
        assert updated_info1.group == "nogroup"

        assert updated_info2.mode == 600
        assert updated_info2.owner == "nobody"
        assert updated_info2.group == "nogroup"
        logger.info("✓ Permissions updated successfully")

        logger.info("Step 7: Update file content")
        before_update_info = (await sandbox.files.get_file_info([test_file1]))[test_file1]
        updated_content1 = test_content + "\\nAppended line to file1"
        updated_content2 = test_content + "\\nAppended line to file2"

        # Ensure server-visible mtime delta is measurable.
        await asyncio.sleep(0.05)

        update_entry1 = WriteEntry(path=test_file1, data=updated_content1, mode=644)
        update_entry2 = WriteEntry(path=test_file2, data=updated_content2, mode=755)
        await sandbox.files.write_files([update_entry1, update_entry2])

        new_content1 = await sandbox.files.read_file(test_file1, encoding="utf-8")
        new_content2 = await sandbox.files.read_file(test_file2, encoding="utf-8")

        assert new_content1 == updated_content1
        assert new_content2 == updated_content2
        logger.info("✓ File content updated successfully")

        after_update_info = (await sandbox.files.get_file_info([test_file1]))[test_file1]
        assert after_update_info.size == len(updated_content1.encode("utf-8"))
        _assert_modified_updated(before_update_info.modified_at, after_update_info.modified_at, min_delta_ms=1)

        logger.info("Step 8: Replace file contents via API (replace_contents)")
        before_replace_info = after_update_info
        await asyncio.sleep(0.05)
        replace_entry = ContentReplaceEntry(
            path=test_file1,
            old_content="Appended line to file1",
            new_content="Replaced line in file1",
        )
        await sandbox.files.replace_contents([replace_entry])
        replaced_content1 = await sandbox.files.read_file(test_file1, encoding="utf-8")
        assert "Replaced line in file1" in replaced_content1
        assert "Appended line to file1" not in replaced_content1

        after_replace_info = (await sandbox.files.get_file_info([test_file1]))[test_file1]
        _assert_modified_updated(before_replace_info.modified_at, after_replace_info.modified_at, min_delta_ms=1)

        logger.info("Step 9: Move/rename a file via API (move_files)")
        moved_path = f"{test_dir2}/moved_file3.txt"
        await sandbox.files.move_files([MoveEntry(src=test_file3, dest=moved_path)])
        moved_bytes = await sandbox.files.read_bytes(moved_path)
        assert moved_bytes.decode("utf-8") == test_content
        with pytest.raises(Exception):
            await sandbox.files.read_bytes(test_file3)

        logger.info("Step 10: Delete file via API (delete_files)")
        await sandbox.files.delete_files([test_file2])
        with pytest.raises(Exception):
            await sandbox.files.read_file(test_file2, encoding="utf-8")

        # After move+delete, search should reflect the updated view.
        files_after = await sandbox.files.search(SearchEntry(path=test_dir1, pattern="*"))
        assert {e.path for e in files_after} == {test_file1}

        logger.info("Step 11: Delete directories recursively (delete_directories)")
        await sandbox.files.delete_directories([test_dir1, test_dir2])
        verify_dirs_deleted = await sandbox.commands.run(
            f"test ! -d {test_dir1} && test ! -d {test_dir2} && echo OK",
            opts=RunCommandOpts(working_directory="/tmp"),
        )
        assert verify_dirs_deleted.error is None
        assert len(verify_dirs_deleted.logs.stdout) == 1
        assert verify_dirs_deleted.logs.stdout[0].text == "OK"

        logger.info("TEST 3 PASSED: Basic filesystem operations test completed successfully")

    @pytest.mark.timeout(120)
    @pytest.mark.order(5)
    async def test_04_interrupt_command(self):
        """Test interrupting a long-running command."""
        await self._ensure_sandbox_created()
        sandbox = TestSandboxE2E.sandbox

        logger.info("=" * 80)
        logger.info("TEST 4: Testing command interrupt")
        logger.info("=" * 80)

        init_events: list[ExecutionInit] = []
        completed_events: list[ExecutionComplete] = []
        errors: list[ExecutionError] = []
        init_received = asyncio.Event()

        async def on_init(init: ExecutionInit):
            init_events.append(init)
            init_received.set()

        async def on_execution_complete(complete: ExecutionComplete):
            completed_events.append(complete)

        async def on_error(error: ExecutionError):
            errors.append(error)

        handlers = ExecutionHandlers(
            on_init=on_init,
            on_execution_complete=on_execution_complete,
            on_error=on_error,
        )

        start = time.time()
        task = asyncio.create_task(
            sandbox.commands.run(
                "sleep 30",
                handlers=handlers,
            )
        )

        await asyncio.wait_for(init_received.wait(), timeout=15)
        assert len(init_events) == 1
        assert init_events[0].id is not None and init_events[0].id.strip()
        _assert_recent_timestamp_ms(init_events[0].timestamp)

        await sandbox.commands.interrupt(init_events[0].id)

        execution = await asyncio.wait_for(task, timeout=30)
        elapsed = time.time() - start

        assert execution is not None
        assert execution.id == init_events[0].id
        assert elapsed < 20, f"Interrupted command took too long: {elapsed:.2f}s"
        # Contract: error and complete are mutually exclusive.
        assert (len(completed_events) > 0) or (len(errors) > 0), (
            f"expected exactly one of complete/error, got complete={len(completed_events)} "
            f"error={len(errors)}"
        )
        if len(completed_events) > 0:
            assert len(completed_events) == 1
            _assert_recent_timestamp_ms(completed_events[0].timestamp, tolerance_ms=180_000)

        # Interrupt should stop the process early; most implementations surface an error and/or stderr.
        assert execution.error is not None or len(execution.logs.stderr) > 0
        if execution.error is not None:
            assert execution.error.name
            assert execution.error.value
            _assert_recent_timestamp_ms(execution.error.timestamp, tolerance_ms=180_000)

    @pytest.mark.timeout(120)
    @pytest.mark.order(6)
    async def test_05_sandbox_pause(self):
        """Test sandbox pause operation."""
        await self._ensure_sandbox_created()
        sandbox = TestSandboxE2E.sandbox

        logger.info("=" * 80)
        logger.info("TEST 5: Testing sandbox pause operation")
        logger.info("=" * 80)

        # Sandbox has been exercised through tests 01-04; a brief settle is sufficient.
        await asyncio.sleep(2)
        assert await sandbox.is_healthy(), "Sandbox should be healthy before pause"

        logger.info("Requesting sandbox pause...")
        await sandbox.pause()

        start_time = time.time()
        poll_count = 0
        final_status = None

        logger.info("Polling for status change (timeout: 30s)...")
        while poll_count < 30:
            await asyncio.sleep(1)
            poll_count += 1

            info = await sandbox.get_info()
            current_status = info.status
            logger.info(f"Poll {poll_count}: Status = {current_status.state}")

            if current_status.state == "Pausing":
                continue
            else:
                final_status = current_status
                break

        assert final_status is not None, "Failed to get final status after pause operation"
        assert final_status.state == "Paused", "Sandbox should be in Paused state"

        # Verify pause semantics: execd should be unreachable.
        # The global HTTP request_timeout is 3 min, so we wrap the single
        # is_healthy() call in a short asyncio timeout.  A paused container's
        # frozen process will never reply, causing either a timeout (good) or
        # an immediate connection refusal (also good).
        try:
            healthy = await asyncio.wait_for(sandbox.is_healthy(), timeout=15)
        except asyncio.TimeoutError:
            healthy = False
        assert healthy is False, "Sandbox should be unhealthy after pause"

        elapsed_time = (time.time() - start_time) * 1000
        logger.info(f"✓ Sandbox pause confirmed in {elapsed_time:.2f} ms")

    @pytest.mark.timeout(120)
    @pytest.mark.order(7)
    async def test_06_sandbox_resume(self):
        """Test sandbox resume operation."""
        await self._ensure_sandbox_created()
        sandbox = TestSandboxE2E.sandbox

        logger.info("=" * 80)
        logger.info("TEST 6: Testing sandbox resume operation")
        logger.info("=" * 80)

        logger.info("Requesting sandbox resume...")
        resumed = await Sandbox.resume(
            sandbox_id=sandbox.id,
            connection_config=TestSandboxE2E.connection_config,
        )
        # Replace the class-held instance so subsequent operations/teardown use the resumed instance.
        TestSandboxE2E.sandbox = resumed
        sandbox = resumed

        start_time = time.time()
        poll_count = 0
        final_status = None

        logger.info("Polling for status change (timeout: 1 minute)...")
        while poll_count < 60:
            await asyncio.sleep(1)
            poll_count += 1

            info = await sandbox.get_info()
            current_status = info.status
            logger.info(f"Poll {poll_count}: Status = {current_status.state}")

            if current_status.state == "Running":
                final_status = current_status
                break

        assert final_status is not None, "Failed to get final status after resume operation"
        assert final_status.state == "Running", "Sandbox should be in Running state after resume"

        logger.info("Verifying sandbox health after resume...")
        healthy = False
        for _ in range(30):
            healthy = await sandbox.is_healthy()
            if healthy:
                break
            await asyncio.sleep(1)
        assert healthy is True, "Sandbox should be healthy after resume"

        # Minimal smoke check: after resume, the existing Sandbox instance should still be usable.
        # This helps validate that SDK re-bound its execd adapters (endpoint may change across resume).
        echo = await sandbox.commands.run("echo resume-ok")
        assert echo.error is None
        assert len(echo.logs.stdout) == 1
        assert echo.logs.stdout[0].text == "resume-ok"

        elapsed_time = (time.time() - start_time) * 1000
        logger.info(f"✓ Sandbox resume completed in {elapsed_time:.2f} ms")
        logger.info("TEST 5 PASSED: Sandbox resume operation test completed successfully")
