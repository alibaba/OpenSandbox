# Copyright 2026 Alibaba Group Holding Ltd.
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

"""Tests for CLI commands with mocked SDK calls.

Strategy: patch ``opensandbox_cli.main.ClientContext`` and ``resolve_config``
so the root ``cli`` callback creates our mock instead of a real SDK client.
"""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from click.testing import CliRunner

from opensandbox_cli.main import cli
from opensandbox_cli.output import OutputFormatter


@pytest.fixture()
def runner() -> CliRunner:
    return CliRunner()


def _build_mock_client_context(
    *,
    manager: MagicMock | None = None,
    sandbox: MagicMock | None = None,
    output_format: str = "json",
) -> MagicMock:
    ctx = MagicMock()
    ctx.resolved_config = {
        "api_key": "test-key",
        "domain": "localhost:8080",
        "protocol": "http",
        "request_timeout": 30,
        "use_server_proxy": False,
        "output_format": output_format,
        "color": False,
        "default_image": None,
        "default_timeout": None,
    }
    ctx.output = OutputFormatter(output_format, color=False)
    ctx.get_manager.return_value = manager or MagicMock()
    ctx.connect_sandbox.return_value = sandbox or MagicMock()
    ctx.resolve_sandbox_id.side_effect = lambda prefix: prefix  # passthrough
    ctx.connection_config = MagicMock()
    ctx.close = MagicMock()
    return ctx


def _invoke(
    runner: CliRunner,
    args: list[str],
    *,
    manager: MagicMock | None = None,
    sandbox: MagicMock | None = None,
    output_format: str = "json",
) -> object:
    """Invoke CLI with mocked ClientContext."""
    mock_ctx = _build_mock_client_context(
        manager=manager, sandbox=sandbox, output_format=output_format
    )

    with patch("opensandbox_cli.main.resolve_config") as mock_resolve, \
         patch("opensandbox_cli.main.ClientContext", return_value=mock_ctx), \
         patch("opensandbox_cli.main.OutputFormatter", side_effect=lambda fmt, **kw: OutputFormatter(fmt, **kw)):
        mock_resolve.return_value = mock_ctx.resolved_config
        result = runner.invoke(cli, args, catch_exceptions=False)
    return result


# ---------------------------------------------------------------------------
# Config commands (no SDK mocking needed)
# ---------------------------------------------------------------------------


class TestConfigInit:
    def test_init_creates_file(self, runner: CliRunner, tmp_path: Path) -> None:
        cfg_path = tmp_path / "config.toml"
        result = runner.invoke(cli, ["config", "init", "--path", str(cfg_path)])
        assert result.exit_code == 0
        assert "Config file created" in result.output

    def test_init_refuses_overwrite(self, runner: CliRunner, tmp_path: Path) -> None:
        cfg_path = tmp_path / "config.toml"
        cfg_path.write_text("existing")
        result = runner.invoke(cli, ["config", "init", "--path", str(cfg_path)])
        assert "already exists" in result.output

    def test_init_force_overwrites(self, runner: CliRunner, tmp_path: Path) -> None:
        cfg_path = tmp_path / "config.toml"
        cfg_path.write_text("old")
        result = runner.invoke(cli, ["config", "init", "--path", str(cfg_path), "--force"])
        assert result.exit_code == 0
        assert "Config file created" in result.output


class TestConfigShow:
    def test_show_json_output(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["-o", "json", "config", "show"])
        assert result.exit_code == 0
        data = json.loads(result.output)
        assert "api_key" in data

    def test_show_table_output(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["config", "show"])
        assert result.exit_code == 0
        assert "api_key" in result.output


class TestConfigSet:
    def test_set_updates_existing_field(self, runner: CliRunner, tmp_path: Path) -> None:
        cfg_path = tmp_path / "config.toml"
        runner.invoke(cli, ["config", "init", "--path", str(cfg_path)])
        result = runner.invoke(cli, ["config", "set", "connection.domain", "new.host", "--path", str(cfg_path)])
        assert result.exit_code == 0
        assert "Set connection.domain = new.host" in result.output

    def test_set_rejects_flat_key(self, runner: CliRunner, tmp_path: Path) -> None:
        cfg_path = tmp_path / "config.toml"
        cfg_path.write_text("[connection]\n")
        result = runner.invoke(cli, ["config", "set", "flat_key", "value", "--path", str(cfg_path)])
        assert "section.field" in result.output


# ---------------------------------------------------------------------------
# Sandbox commands
# ---------------------------------------------------------------------------


class TestSandboxList:
    def test_list_invokes_manager(self, runner: CliRunner) -> None:
        mock_mgr = MagicMock()
        mock_result = MagicMock()
        mock_result.sandbox_infos = []
        mock_mgr.list_sandbox_infos.return_value = mock_result

        result = _invoke(runner, ["-o", "json", "sandbox", "list"], manager=mock_mgr)
        assert result.exit_code == 0
        mock_mgr.list_sandbox_infos.assert_called_once()


class TestSandboxCreate:
    def test_create_uses_config_defaults(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        mock_sb.id = "sb-123"
        mock_ctx = _build_mock_client_context(sandbox=mock_sb)
        mock_ctx.resolved_config["default_image"] = "python:3.12"
        mock_ctx.resolved_config["default_timeout"] = "15m"

        with patch("opensandbox_cli.main.resolve_config") as mock_resolve, \
             patch("opensandbox_cli.main.ClientContext", return_value=mock_ctx), \
             patch("opensandbox_cli.main.OutputFormatter", side_effect=lambda fmt, **kw: OutputFormatter(fmt, **kw)), \
             patch("opensandbox.sync.sandbox.SandboxSync.create", return_value=mock_sb) as mock_create:
            mock_resolve.return_value = mock_ctx.resolved_config
            result = runner.invoke(cli, ["-o", "json", "sandbox", "create"], catch_exceptions=False)

        assert result.exit_code == 0
        mock_create.assert_called_once()
        assert mock_create.call_args.args[0] == "python:3.12"
        assert mock_create.call_args.kwargs["timeout"].total_seconds() == 900

    def test_create_requires_image_when_no_default(self, runner: CliRunner) -> None:
        result = _invoke(runner, ["sandbox", "create"])
        assert result.exit_code != 0
        assert "Sandbox image is required" in result.output

    def test_create_loads_volumes_from_file(self, runner: CliRunner, tmp_path: Path) -> None:
        mock_sb = MagicMock()
        mock_sb.id = "sb-123"
        volumes_path = tmp_path / "volumes.json"
        volumes_path.write_text(json.dumps([
            {
                "name": "workdir",
                "host": {"path": "/tmp/workdir"},
                "mountPath": "/workspace",
            }
        ]))

        mock_ctx = _build_mock_client_context(sandbox=mock_sb)
        with patch("opensandbox_cli.main.resolve_config") as mock_resolve, \
             patch("opensandbox_cli.main.ClientContext", return_value=mock_ctx), \
             patch("opensandbox_cli.main.OutputFormatter", side_effect=lambda fmt, **kw: OutputFormatter(fmt, **kw)), \
             patch("opensandbox.sync.sandbox.SandboxSync.create", return_value=mock_sb) as mock_create:
            mock_resolve.return_value = mock_ctx.resolved_config
            result = runner.invoke(
                cli,
                ["-o", "json", "sandbox", "create", "--image", "python:3.12", "--volumes-file", str(volumes_path)],
                catch_exceptions=False,
            )

        assert result.exit_code == 0
        mock_create.assert_called_once()
        volumes = mock_create.call_args.kwargs["volumes"]
        assert len(volumes) == 1
        assert volumes[0].name == "workdir"
        assert volumes[0].mount_path == "/workspace"


class TestSandboxKill:
    def test_kill_multiple(self, runner: CliRunner) -> None:
        mock_mgr = MagicMock()
        result = _invoke(runner, ["sandbox", "kill", "id1", "id2"], manager=mock_mgr)
        assert result.exit_code == 0
        assert mock_mgr.kill_sandbox.call_count == 2
        assert "Sandbox terminated: id1" in result.output
        assert "Sandbox terminated: id2" in result.output


class TestSandboxPause:
    def test_pause_calls_manager(self, runner: CliRunner) -> None:
        mock_mgr = MagicMock()
        result = _invoke(runner, ["sandbox", "pause", "sb-123"], manager=mock_mgr)
        assert result.exit_code == 0
        mock_mgr.pause_sandbox.assert_called_once_with("sb-123")
        assert "Sandbox paused: sb-123" in result.output


class TestSandboxResume:
    def test_resume_calls_manager(self, runner: CliRunner) -> None:
        mock_mgr = MagicMock()
        result = _invoke(runner, ["sandbox", "resume", "sb-123"], manager=mock_mgr)
        assert result.exit_code == 0
        mock_mgr.resume_sandbox.assert_called_once_with("sb-123")
        assert "Sandbox resumed: sb-123" in result.output


# ---------------------------------------------------------------------------
# File commands
# ---------------------------------------------------------------------------


class TestFileCat:
    def test_cat_outputs_content(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        mock_sb.files.read_file.return_value = "hello world"
        result = _invoke(runner, ["file", "cat", "sb-1", "/etc/hostname"], sandbox=mock_sb)
        assert result.exit_code == 0
        assert "hello world" in result.output
        mock_sb.files.read_file.assert_called_once_with("/etc/hostname", encoding="utf-8")


class TestFileWrite:
    def test_write_with_content_flag(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        result = _invoke(
            runner,
            ["file", "write", "sb-1", "/tmp/test.txt", "-c", "content here"],
            sandbox=mock_sb,
        )
        assert result.exit_code == 0
        assert "Written" in result.output
        mock_sb.files.write_file.assert_called_once()

    def test_write_parses_octal_mode(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        result = _invoke(
            runner,
            ["file", "write", "sb-1", "/tmp/test.txt", "-c", "content here", "--mode", "644"],
            sandbox=mock_sb,
        )
        assert result.exit_code == 0
        mock_sb.files.write_file.assert_called_once_with(
            "/tmp/test.txt", "content here", encoding="utf-8", mode=644
        )


class TestFileRm:
    def test_rm_deletes_files(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        result = _invoke(
            runner, ["file", "rm", "sb-1", "/tmp/a", "/tmp/b"], sandbox=mock_sb
        )
        assert result.exit_code == 0
        mock_sb.files.delete_files.assert_called_once_with(["/tmp/a", "/tmp/b"])


class TestFileMv:
    def test_mv_moves_file(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        result = _invoke(
            runner, ["file", "mv", "sb-1", "/tmp/old", "/tmp/new"], sandbox=mock_sb
        )
        assert result.exit_code == 0
        assert "Moved: /tmp/old" in result.output and "/tmp/new" in result.output


class TestFileMkdir:
    def test_mkdir_creates_dirs(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        result = _invoke(
            runner, ["file", "mkdir", "sb-1", "/tmp/dir1", "/tmp/dir2"], sandbox=mock_sb
        )
        assert result.exit_code == 0
        assert "Created: /tmp/dir1" in result.output
        assert "Created: /tmp/dir2" in result.output

    def test_mkdir_parses_octal_mode(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        result = _invoke(
            runner,
            ["file", "mkdir", "sb-1", "/tmp/dir1", "--mode", "755"],
            sandbox=mock_sb,
        )
        assert result.exit_code == 0
        entry = mock_sb.files.create_directories.call_args.args[0][0]
        assert entry.mode == 755


class TestFileRmdir:
    def test_rmdir_removes_dirs(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        result = _invoke(
            runner, ["file", "rmdir", "sb-1", "/workspace/old"], sandbox=mock_sb
        )
        assert result.exit_code == 0
        assert "Removed: /workspace/old" in result.output


class TestFileChmod:
    def test_chmod_parses_octal_mode(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        result = _invoke(
            runner,
            ["file", "chmod", "sb-1", "/tmp/test.txt", "--mode", "755"],
            sandbox=mock_sb,
        )
        assert result.exit_code == 0
        entry = mock_sb.files.set_permissions.call_args.args[0][0]
        assert entry.mode == 755


# ---------------------------------------------------------------------------
# Egress commands
# ---------------------------------------------------------------------------


class TestEgressCommands:
    def test_get_prints_policy(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        mock_policy = MagicMock()
        mock_policy.model_dump.return_value = {
            "defaultAction": "deny",
            "egress": [{"action": "allow", "target": "pypi.org"}],
        }
        mock_sb.get_egress_policy.return_value = mock_policy

        result = _invoke(runner, ["-o", "json", "egress", "get", "sb-1"], sandbox=mock_sb)
        assert result.exit_code == 0
        data = json.loads(result.output)
        assert data["defaultAction"] == "deny"

    def test_patch_calls_sdk(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        mock_sb.id = "sb-1"
        result = _invoke(
            runner,
            ["-o", "json", "egress", "patch", "sb-1", "--rule", "allow=pypi.org", "--rule", "deny=bad.example.com"],
            sandbox=mock_sb,
        )
        assert result.exit_code == 0
        mock_sb.patch_egress_rules.assert_called_once()
        rules = mock_sb.patch_egress_rules.call_args.args[0]
        assert len(rules) == 2
        assert rules[0].action == "allow"
        assert rules[0].target == "pypi.org"
        assert rules[1].action == "deny"
        assert rules[1].target == "bad.example.com"


# ---------------------------------------------------------------------------
# Command execution
# ---------------------------------------------------------------------------


class TestCommandRun:
    def test_background_run(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        mock_execution = MagicMock()
        mock_execution.id = "exec-123"
        mock_sb.commands.run.return_value = mock_execution

        result = _invoke(
            runner,
            ["-o", "json", "command", "run", "sb-1", "-d", "echo", "hello"],
            sandbox=mock_sb,
        )
        assert result.exit_code == 0
        data = json.loads(result.output)
        assert data["execution_id"] == "exec-123"
        assert data["mode"] == "background"


class TestExecShortcut:
    def test_exec_passes_to_run(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        mock_execution = MagicMock()
        mock_execution.id = "exec-456"
        mock_sb.commands.run.return_value = mock_execution

        result = _invoke(
            runner,
            ["-o", "json", "exec", "sb-1", "-d", "--", "ls", "-la"],
            sandbox=mock_sb,
        )
        assert result.exit_code == 0
        mock_sb.commands.run.assert_called_once()


class TestCommandInterrupt:
    def test_interrupt_calls_sdk(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        result = _invoke(
            runner, ["command", "interrupt", "sb-1", "exec-789"], sandbox=mock_sb
        )
        assert result.exit_code == 0
        mock_sb.commands.interrupt.assert_called_once_with("exec-789")
        assert "Interrupted: exec-789" in result.output


class TestCommandSession:
    def test_session_create(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        mock_sb.id = "sb-1"
        mock_sb.commands.create_session.return_value = "sess-123"
        result = _invoke(
            runner,
            ["-o", "json", "command", "session", "create", "sb-1", "--workdir", "/workspace"],
            sandbox=mock_sb,
        )
        assert result.exit_code == 0
        data = json.loads(result.output)
        assert data["session_id"] == "sess-123"
        mock_sb.commands.create_session.assert_called_once_with(working_directory="/workspace")

    def test_session_run(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        mock_execution = MagicMock()
        mock_execution.error = None
        mock_sb.commands.run_in_session.return_value = mock_execution
        result = _invoke(
            runner,
            ["command", "session", "run", "sb-1", "sess-123", "--timeout", "30s", "--", "pwd"],
            sandbox=mock_sb,
        )
        assert result.exit_code == 0
        mock_sb.commands.run_in_session.assert_called_once()
        assert mock_sb.commands.run_in_session.call_args.args[:2] == ("sess-123", "pwd")
        assert mock_sb.commands.run_in_session.call_args.kwargs["timeout"] == 30000

    def test_session_delete(self, runner: CliRunner) -> None:
        mock_sb = MagicMock()
        result = _invoke(
            runner,
            ["command", "session", "delete", "sb-1", "sess-123"],
            sandbox=mock_sb,
        )
        assert result.exit_code == 0
        mock_sb.commands.delete_session.assert_called_once_with("sess-123")
        assert "Deleted session: sess-123" in result.output


# ---------------------------------------------------------------------------
# DevOps diagnostics
# ---------------------------------------------------------------------------


class TestDevopsCommands:
    def test_logs_fetches_plain_text(self, runner: CliRunner) -> None:
        mock_client = MagicMock()
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.text = "sandbox logs"
        mock_client.get.return_value = mock_response
        mock_ctx = _build_mock_client_context()
        mock_ctx.get_devops_client.return_value = mock_client

        with patch("opensandbox_cli.main.resolve_config") as mock_resolve, \
             patch("opensandbox_cli.main.ClientContext", return_value=mock_ctx), \
             patch("opensandbox_cli.main.OutputFormatter", side_effect=lambda fmt, **kw: OutputFormatter(fmt, **kw)):
            mock_resolve.return_value = mock_ctx.resolved_config
            result = runner.invoke(cli, ["devops", "logs", "sb-1"], catch_exceptions=False)

        assert result.exit_code == 0
        assert "sandbox logs" in result.output
        mock_client.get.assert_called_once()
