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
Comprehensive Sync E2E tests for SandboxSync functionality.

This mirrors `test_sandbox_e2e.py` but uses the synchronous SDK.
"""

import logging
import time
from concurrent.futures import ThreadPoolExecutor
from datetime import timedelta
from io import BytesIO

import pytest
from opensandbox import SandboxSync
from opensandbox.models.execd import (
    ExecutionComplete,
    ExecutionError,
    ExecutionInit,
    OutputMessage,
    RunCommandOpts,
)
from opensandbox.models.execd_sync import ExecutionHandlersSync
from opensandbox.models.filesystem import (
    ContentReplaceEntry,
    MoveEntry,
    SearchEntry,
    SetPermissionEntry,
    WriteEntry,
)
from opensandbox.models.sandboxes import SandboxImageSpec

from tests.base_e2e_test import create_connection_config_sync, get_sandbox_image

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


class TestSandboxE2ESync:
    """Comprehensive E2E tests for SandboxSync functionality (ordered)."""

    sandbox = None
    connection_config = None
    _setup_done = False

    @pytest.fixture(scope="class", autouse=True)
    def _sandbox_lifecycle(self, request):
        """Create sandbox once and ALWAYS cleanup to avoid resource leaks."""
        request.cls._ensure_sandbox_created()
        try:
            yield
        finally:
            sandbox = request.cls.sandbox
            if sandbox is not None:
                try:
                    sandbox.kill()
                except Exception as e:
                    logger.warning("Teardown: sandbox.kill() failed: %s", e, exc_info=True)
                try:
                    sandbox.close()
                except Exception as e:
                    logger.warning("Teardown: sandbox.close() failed: %s", e, exc_info=True)

            cfg = request.cls.connection_config
            if cfg is not None:
                try:
                    cfg.transport.close()
                except Exception:
                    pass

    @classmethod
    def _ensure_sandbox_created(cls) -> None:
        if cls._setup_done:
            return

        logger.info("=" * 100)
        logger.info("SETUP: Creating sandbox (sync)")
        logger.info("=" * 100)

        cls.connection_config = create_connection_config_sync()

        cls.sandbox = SandboxSync.create(
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
                "PYTHON_VERSION": "3.12",
            },
            health_check_polling_interval=timedelta(milliseconds=500),
        )

        logger.info("✓ Sandbox created: %s", cls.sandbox.id)
        cls._setup_done = True

    @pytest.mark.timeout(120)
    @pytest.mark.order(1)
    def test_01_sandbox_lifecycle_and_health(self) -> None:
        """Test sandbox lifecycle and health monitoring."""
        TestSandboxE2ESync._ensure_sandbox_created()
        sandbox = TestSandboxE2ESync.sandbox
        assert sandbox is not None

        logger.info("=" * 80)
        logger.info("TEST 1: Testing sandbox lifecycle and health monitoring (sync)")
        logger.info("=" * 80)

        assert isinstance(sandbox.id, str)
        assert sandbox.is_healthy() is True

        info = sandbox.get_info()
        assert info.id == sandbox.id
        assert info.status.state == "Running"
        assert info.created_at is not None
        assert info.expires_at is not None
        assert info.expires_at > info.created_at
        assert info.entrypoint == ["tail", "-f", "/dev/null"]

        duration = info.expires_at - info.created_at
        min_duration = timedelta(minutes=1)
        max_duration = timedelta(minutes=3)
        assert min_duration <= duration <= max_duration, (
            f"Duration {duration} should be between 1 and 3 minutes"
        )

        assert info.metadata is not None
        assert info.metadata.get("tag") == "e2e-test"

        endpoint = sandbox.get_endpoint(44772)
        assert endpoint is not None
        assert endpoint.endpoint is not None
        _assert_endpoint_has_port(endpoint.endpoint, 44772)

        metrics = sandbox.get_metrics()
        assert metrics is not None
        assert metrics.cpu_count > 0
        assert 0.0 <= metrics.cpu_used_percentage <= 100.0
        assert metrics.memory_total_in_mib > 0
        assert 0.0 <= metrics.memory_used_in_mib <= metrics.memory_total_in_mib
        _assert_recent_timestamp_ms(metrics.timestamp, tolerance_ms=120_000)

        await_renew = timedelta(minutes=20)
        renew_response = sandbox.renew(await_renew)
        assert renew_response is not None
        assert renew_response.expires_at > info.expires_at

        renewed_info = sandbox.get_info()
        assert renewed_info.expires_at > info.expires_at
        assert abs((renewed_info.expires_at - renew_response.expires_at).total_seconds()) < 10

        now = renewed_info.expires_at.__class__.now(tz=renewed_info.expires_at.tzinfo)
        remaining = renewed_info.expires_at - now
        assert remaining > timedelta(minutes=18), f"Remaining TTL too small: {remaining}"
        assert remaining < timedelta(minutes=22), f"Remaining TTL too large: {remaining}"

        assert sandbox.files is not None
        assert sandbox.commands is not None
        assert sandbox.metrics is not None
        assert sandbox.connection_config is not None

        # Connect to existing sandbox by ID and run a basic command.
        sandbox2 = SandboxSync.connect(
            sandbox.id, connection_config=TestSandboxE2ESync.connection_config
        )
        try:
            assert sandbox2.id == sandbox.id
            assert sandbox2.is_healthy() is True
            connect_result = sandbox2.commands.run(
                "echo connect-ok",
            )
            assert connect_result.error is None
            assert len(connect_result.logs.stdout) == 1
            assert connect_result.logs.stdout[0].text == "connect-ok"
        finally:
            sandbox2.close()

    @pytest.mark.timeout(120)
    @pytest.mark.order(2)
    def test_02_basic_command_execution(self) -> None:
        """Test basic command execution."""
        TestSandboxE2ESync._ensure_sandbox_created()
        sandbox = TestSandboxE2ESync.sandbox
        assert sandbox is not None

        logger.info("=" * 80)
        logger.info("TEST 2: Testing basic command execution (sync)")
        logger.info("=" * 80)

        stdout_messages: list[OutputMessage] = []
        stderr_messages: list[OutputMessage] = []
        results = []
        completed_events: list[ExecutionComplete] = []
        errors: list[ExecutionError] = []
        init_events: list[ExecutionInit] = []

        def on_stdout(msg):
            stdout_messages.append(msg)

        def on_stderr(msg):
            stderr_messages.append(msg)

        def on_result(result):
            results.append(result)

        def on_execution_complete(complete):
            completed_events.append(complete)

        def on_error(error):
            errors.append(error)

        def on_init(init):
            init_events.append(init)

        handlers = ExecutionHandlersSync(
            on_stdout=on_stdout,
            on_stderr=on_stderr,
            on_result=on_result,
            on_execution_complete=on_execution_complete,
            on_error=on_error,
            on_init=on_init,
        )

        echo_result = sandbox.commands.run(
            "echo 'Hello OpenSandbox E2E'",
            handlers=handlers,
        )

        assert echo_result is not None
        assert echo_result.id is not None and echo_result.id.strip()
        assert echo_result.error is None
        assert len(echo_result.logs.stdout) == 1
        assert echo_result.logs.stdout[0].text == "Hello OpenSandbox E2E"
        assert echo_result.logs.stdout[0].is_error is False
        _assert_recent_timestamp_ms(echo_result.logs.stdout[0].timestamp)
        assert len(echo_result.logs.stderr) == 0

        assert len(init_events) == 1
        assert len(completed_events) == 1
        assert init_events[0].id == echo_result.id
        _assert_recent_timestamp_ms(init_events[0].timestamp)
        _assert_recent_timestamp_ms(completed_events[0].timestamp)
        assert completed_events[0].execution_time_in_millis >= 0

        assert len(stdout_messages) == 1
        assert stdout_messages[0].text == "Hello OpenSandbox E2E"
        assert stdout_messages[0].is_error is False
        _assert_recent_timestamp_ms(stdout_messages[0].timestamp)
        assert len(errors) == 0

        pwd_result = sandbox.commands.run(
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

        start_time = time.time()
        sandbox.commands.run(
            "sleep 30",
            opts=RunCommandOpts(background=True),
        )
        end_time = time.time()
        execution_time_ms = (end_time - start_time) * 1000
        assert execution_time_ms < 10000

        stdout_messages.clear()
        stderr_messages.clear()
        errors.clear()
        completed_events.clear()
        init_events.clear()

        fail_result = sandbox.commands.run(
            "nonexistent-command-that-does-not-exist",
            handlers=handlers,
        )

        assert fail_result.error is not None
        assert fail_result.error.name == "CommandExecError"
        assert len(fail_result.logs.stderr) > 0
        assert any(
            "nonexistent-command-that-does-not-exist" in m.text for m in fail_result.logs.stderr
        )
        assert all(m.is_error is True for m in fail_result.logs.stderr)
        _assert_recent_timestamp_ms(fail_result.logs.stderr[0].timestamp)

        assert len(init_events) == 1
        assert init_events[0].id == fail_result.id
        _assert_recent_timestamp_ms(init_events[0].timestamp)
        # Contract: error and complete are mutually exclusive; failing command should emit error only.
        assert len(errors) >= 1
        assert len(completed_events) == 0

        assert errors[0].name == "CommandExecError"
        assert len(stderr_messages) > 0
        assert "nonexistent-command-that-does-not-exist" in stderr_messages[0].text

    @pytest.mark.timeout(120)
    @pytest.mark.order(3)
    def test_03_basic_filesystem_operations(self) -> None:
        """Test basic filesystem operations."""
        TestSandboxE2ESync._ensure_sandbox_created()
        sandbox = TestSandboxE2ESync.sandbox
        assert sandbox is not None

        logger.info("=" * 80)
        logger.info("TEST 3: Testing basic filesystem operations (sync)")
        logger.info("=" * 80)

        test_dir1 = f"/tmp/fs_test1_{int(time.time() * 1000)}"
        test_dir2 = f"/tmp/fs_test2_{int(time.time() * 1000)}"

        dir_entry1 = WriteEntry(path=test_dir1, mode=755)
        dir_entry2 = WriteEntry(path=test_dir2, mode=644)
        sandbox.files.create_directories([dir_entry1, dir_entry2])

        dir_info_map = sandbox.files.get_file_info([test_dir1, test_dir2])
        assert test_dir1 in dir_info_map
        assert test_dir2 in dir_info_map
        assert dir_info_map[test_dir1].path == test_dir1
        assert dir_info_map[test_dir2].path == test_dir2
        assert dir_info_map[test_dir1].mode == 755
        assert dir_info_map[test_dir2].mode == 644
        assert dir_info_map[test_dir1].owner
        assert dir_info_map[test_dir1].group
        _assert_times_close(dir_info_map[test_dir1].created_at, dir_info_map[test_dir1].modified_at)

        ls_result = sandbox.commands.run(
            "ls -la | grep fs_test",
            opts=RunCommandOpts(working_directory="/tmp"),
        )
        assert len(ls_result.logs.stdout) == 2

        test_file1 = f"{test_dir1}/test_file1.txt"
        test_file2 = f"{test_dir1}/test_file2.txt"
        test_file3 = f"{test_dir1}/test_file3.txt"
        test_content = "Hello Filesystem!\nLine 2 with special chars: åäö\nLine 3"

        write_entry1 = WriteEntry(path=test_file1, data=test_content, mode=644)
        write_entry2 = WriteEntry(path=test_file2, data=test_content.encode("utf-8"), mode=755)
        write_entry3 = WriteEntry(
            path=test_file3,
            data=BytesIO(test_content.encode("utf-8")),
            group="nogroup",
            owner="nobody",
            mode=755,
        )
        sandbox.files.write_files([write_entry1, write_entry2, write_entry3])

        read_content1 = sandbox.files.read_file(test_file1, encoding="utf-8")
        read_content1_partial = sandbox.files.read_file(
            test_file1,
            encoding="utf-8",
            range_header="bytes=0-9",
        )
        read_bytes2 = sandbox.files.read_bytes(test_file2)
        read_content2 = read_bytes2.decode("utf-8")

        stream3 = sandbox.files.read_bytes_stream(test_file3)
        read_content3_bytes = b""
        for chunk in stream3:
            read_content3_bytes += chunk
        read_content3 = read_content3_bytes.decode("utf-8")

        expected_size = len(test_content.encode("utf-8"))
        assert read_content1 == test_content
        assert read_content2 == test_content
        assert read_content3 == test_content
        assert read_content1_partial == test_content[:10]

        file_info_map = sandbox.files.get_file_info([test_file1, test_file2, test_file3])
        file_info1 = file_info_map[test_file1]
        assert file_info1.path == test_file1
        assert file_info1.size == expected_size
        assert file_info1.mode == 644
        assert file_info1.owner is not None
        assert file_info1.group is not None
        _assert_times_close(file_info1.created_at, file_info1.modified_at)

        file_info2 = file_info_map[test_file2]
        assert file_info2.path == test_file2
        assert file_info2.size == expected_size
        assert file_info2.mode == 755
        assert file_info2.owner is not None
        assert file_info2.group is not None
        _assert_times_close(file_info2.created_at, file_info2.modified_at)

        file_info3 = file_info_map[test_file3]
        assert file_info3.path == test_file3
        assert file_info3.size == expected_size
        assert file_info3.mode == 755
        assert file_info3.owner == "nobody"
        assert file_info3.group == "nogroup"
        _assert_times_close(file_info3.created_at, file_info3.modified_at)

        search_all_entry = SearchEntry(path=test_dir1, pattern="*")
        all_files_list = sandbox.files.search(search_all_entry)
        all_files = {entry.path: entry for entry in all_files_list}
        assert len(all_files) == 3
        assert test_file1 in all_files
        assert test_file2 in all_files
        assert test_file3 in all_files
        assert all_files[test_file1].size == expected_size
        _assert_times_close(all_files[test_file1].created_at, all_files[test_file1].modified_at)

        perm_entry1 = SetPermissionEntry(path=test_file1, mode=755, owner="nobody", group="nogroup")
        perm_entry2 = SetPermissionEntry(path=test_file2, mode=600, owner="nobody", group="nogroup")
        sandbox.files.set_permissions([perm_entry1, perm_entry2])

        updated_info_map = sandbox.files.get_file_info([test_file1, test_file2])
        updated_info1 = updated_info_map[test_file1]
        updated_info2 = updated_info_map[test_file2]
        assert updated_info1.mode == 755
        assert updated_info1.owner == "nobody"
        assert updated_info1.group == "nogroup"
        assert updated_info2.mode == 600
        assert updated_info2.owner == "nobody"
        assert updated_info2.group == "nogroup"

        before_update_info = sandbox.files.get_file_info([test_file1])[test_file1]
        updated_content1 = test_content + "\nAppended line to file1"
        updated_content2 = test_content + "\nAppended line to file2"
        time.sleep(0.05)
        sandbox.files.write_files(
            [
                WriteEntry(path=test_file1, data=updated_content1, mode=644),
                WriteEntry(path=test_file2, data=updated_content2, mode=755),
            ]
        )

        new_content1 = sandbox.files.read_file(test_file1, encoding="utf-8")
        new_content2 = sandbox.files.read_file(test_file2, encoding="utf-8")
        assert new_content1 == updated_content1
        assert new_content2 == updated_content2

        after_update_info = sandbox.files.get_file_info([test_file1])[test_file1]
        assert after_update_info.size == len(updated_content1.encode("utf-8"))
        _assert_modified_updated(before_update_info.modified_at, after_update_info.modified_at, min_delta_ms=1)

        # Replace file contents via API (replace_contents)
        before_replace_info = after_update_info
        time.sleep(0.05)
        sandbox.files.replace_contents(
            [
                ContentReplaceEntry(
                    path=test_file1,
                    old_content="Appended line to file1",
                    new_content="Replaced line in file1",
                )
            ]
        )
        replaced_content1 = sandbox.files.read_file(test_file1, encoding="utf-8")
        assert "Replaced line in file1" in replaced_content1
        assert "Appended line to file1" not in replaced_content1
        after_replace_info = sandbox.files.get_file_info([test_file1])[test_file1]
        _assert_modified_updated(before_replace_info.modified_at, after_replace_info.modified_at, min_delta_ms=1)

        # Move/rename a file via API (move_files)
        moved_path = f"{test_dir2}/moved_file3.txt"
        sandbox.files.move_files([MoveEntry(src=test_file3, dest=moved_path)])
        moved_bytes = sandbox.files.read_bytes(moved_path)
        assert moved_bytes.decode("utf-8") == test_content
        with pytest.raises(Exception):
            sandbox.files.read_bytes(test_file3)

        # Delete file via API (delete_files)
        sandbox.files.delete_files([test_file2])
        with pytest.raises(Exception):
            sandbox.files.read_file(test_file2, encoding="utf-8")

        files_after = sandbox.files.search(SearchEntry(path=test_dir1, pattern="*"))
        assert {e.path for e in files_after} == {test_file1}

        # Delete directories recursively (delete_directories)
        sandbox.files.delete_directories([test_dir1, test_dir2])
        verify_dirs_deleted = sandbox.commands.run(
            f"test ! -d {test_dir1} && test ! -d {test_dir2} && echo OK",
            opts=RunCommandOpts(working_directory="/tmp"),
        )
        assert verify_dirs_deleted.error is None
        assert len(verify_dirs_deleted.logs.stdout) == 1
        assert verify_dirs_deleted.logs.stdout[0].text == "OK"

    @pytest.mark.timeout(360)
    @pytest.mark.order(4)
    def test_04_interrupt_command(self) -> None:
        """Test interrupting a long-running command."""
        TestSandboxE2ESync._ensure_sandbox_created()
        sandbox = TestSandboxE2ESync.sandbox
        assert sandbox is not None

        logger.info("=" * 80)
        logger.info("TEST 4: Testing command interrupt (sync)")
        logger.info("=" * 80)

        init_events: list[ExecutionInit] = []
        completed_events: list[ExecutionComplete] = []
        errors: list[ExecutionError] = []

        def on_init(init: ExecutionInit):
            init_events.append(init)

        def on_complete(complete: ExecutionComplete):
            completed_events.append(complete)

        def on_error(error: ExecutionError):
            errors.append(error)

        handlers = ExecutionHandlersSync(
            on_init=on_init,
            on_execution_complete=on_complete,
            on_error=on_error,
        )

        start = time.time()
        with ThreadPoolExecutor(max_workers=1) as ex:
            future = ex.submit(
                sandbox.commands.run,
                "sleep 30",
                handlers=handlers,
            )
            deadline = time.time() + 15
            while len(init_events) == 0 and time.time() < deadline:
                time.sleep(0.1)
            assert len(init_events) == 1
            assert init_events[0].id is not None and init_events[0].id.strip()
            _assert_recent_timestamp_ms(init_events[0].timestamp)

            sandbox.commands.interrupt(init_events[0].id)
            execution = future.result(timeout=30)

        elapsed = time.time() - start
        assert execution is not None
        assert execution.id == init_events[0].id
        assert elapsed < 20, f"Interrupted command took too long: {elapsed:.2f}s"
        assert (len(completed_events) > 0) or (len(errors) > 0), (
            f"expected exactly one of complete/error, got complete={len(completed_events)} "
            f"error={len(errors)}"
        )
        if len(completed_events) > 0:
            assert len(completed_events) == 1
            _assert_recent_timestamp_ms(completed_events[0].timestamp, tolerance_ms=180_000)
        assert execution.error is not None or len(execution.logs.stderr) > 0
        if execution.error is not None:
            assert execution.error.name
            assert execution.error.value
            _assert_recent_timestamp_ms(execution.error.timestamp, tolerance_ms=180_000)

    @pytest.mark.timeout(600)
    @pytest.mark.order(5)
    def test_05_sandbox_pause(self) -> None:
        """Test sandbox pause operation."""
        TestSandboxE2ESync._ensure_sandbox_created()
        sandbox = TestSandboxE2ESync.sandbox
        assert sandbox is not None

        logger.info("=" * 80)
        logger.info("TEST 5: Testing sandbox pause operation (sync)")
        logger.info("=" * 80)

        logger.info("Waiting 20 seconds before pausing to ensure sandbox is stable...")
        time.sleep(20)

        sandbox.pause()

        poll_count = 0
        final_status = None
        while poll_count < 300:
            time.sleep(1)
            poll_count += 1
            info = sandbox.get_info()
            current_status = info.status
            logger.info("Poll %s: Status = %s", poll_count, current_status.state)
            if current_status.state == "Pausing":
                continue
            final_status = current_status
            break

        assert final_status is not None
        assert final_status.state == "Paused"

        # Confirm pause semantics: execd becomes unhealthy/unreachable after pause.
        healthy = True
        for _ in range(10):
            healthy = sandbox.is_healthy()
            if not healthy:
                break
            time.sleep(0.5)
        assert healthy is False, "Sandbox should be unhealthy after pause"

    @pytest.mark.timeout(120)
    @pytest.mark.order(6)
    def test_06_sandbox_resume(self) -> None:
        """Test sandbox resume operation."""
        TestSandboxE2ESync._ensure_sandbox_created()
        sandbox = TestSandboxE2ESync.sandbox
        assert sandbox is not None

        logger.info("=" * 80)
        logger.info("TEST 6: Testing sandbox resume operation (sync)")
        logger.info("=" * 80)

        resumed = SandboxSync.resume(
            sandbox_id=sandbox.id,
            connection_config=TestSandboxE2ESync.connection_config,
        )
        TestSandboxE2ESync.sandbox = resumed
        sandbox = resumed

        poll_count = 0
        final_status = None
        while poll_count < 60:
            time.sleep(1)
            poll_count += 1
            info = sandbox.get_info()
            current_status = info.status
            logger.info("Poll %s: Status = %s", poll_count, current_status.state)
            if current_status.state == "Running":
                final_status = current_status
                break

        assert final_status is not None
        assert final_status.state == "Running"
        healthy = False
        for _ in range(30):
            healthy = sandbox.is_healthy()
            if healthy:
                break
            time.sleep(1)
        assert healthy is True, "Sandbox should be healthy after resume"

        # Minimal smoke check: after resume, the existing SandboxSync instance should still be usable.
        echo = sandbox.commands.run("echo resume-ok")
        assert echo.error is None
        assert len(echo.logs.stdout) == 1
        assert echo.logs.stdout[0].text == "resume-ok"
