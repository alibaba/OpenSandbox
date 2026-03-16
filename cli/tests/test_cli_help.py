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

"""Tests that all CLI commands register correctly and --help exits cleanly."""

from __future__ import annotations

import pytest
from click.testing import CliRunner

from opensandbox_cli.main import cli


@pytest.fixture()
def runner() -> CliRunner:
    return CliRunner()


# ---------------------------------------------------------------------------
# Root
# ---------------------------------------------------------------------------


class TestRootCLI:
    def test_help(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["--help"])
        assert result.exit_code == 0
        assert "OpenSandbox CLI" in result.output

    def test_version(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["--version"])
        assert result.exit_code == 0
        assert "opensandbox" in result.output

    def test_root_lists_commands(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["--help"])
        for cmd in ("sandbox", "command", "exec", "file", "code", "config"):
            assert cmd in result.output


# ---------------------------------------------------------------------------
# Sandbox sub-commands
# ---------------------------------------------------------------------------


class TestSandboxHelp:
    def test_sandbox_help(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["sandbox", "--help"])
        assert result.exit_code == 0
        for subcmd in ("create", "list", "get", "kill", "pause", "resume", "renew", "endpoint", "health", "metrics"):
            assert subcmd in result.output

    @pytest.mark.parametrize(
        "subcmd",
        ["create", "list", "get", "kill", "pause", "resume", "renew", "endpoint", "health", "metrics"],
    )
    def test_sandbox_subcommand_help(self, runner: CliRunner, subcmd: str) -> None:
        result = runner.invoke(cli, ["sandbox", subcmd, "--help"])
        assert result.exit_code == 0
        assert subcmd in result.output.lower() or "usage" in result.output.lower()


# ---------------------------------------------------------------------------
# Command sub-commands
# ---------------------------------------------------------------------------


class TestCommandHelp:
    def test_command_help(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["command", "--help"])
        assert result.exit_code == 0
        for subcmd in ("run", "status", "logs", "interrupt"):
            assert subcmd in result.output

    @pytest.mark.parametrize("subcmd", ["run", "status", "logs", "interrupt"])
    def test_command_subcommand_help(self, runner: CliRunner, subcmd: str) -> None:
        result = runner.invoke(cli, ["command", subcmd, "--help"])
        assert result.exit_code == 0


# ---------------------------------------------------------------------------
# exec (top-level shortcut)
# ---------------------------------------------------------------------------


class TestExecHelp:
    def test_exec_help(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["exec", "--help"])
        assert result.exit_code == 0
        assert "shortcut" in result.output.lower() or "command" in result.output.lower()


# ---------------------------------------------------------------------------
# File sub-commands
# ---------------------------------------------------------------------------


class TestFileHelp:
    def test_file_help(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["file", "--help"])
        assert result.exit_code == 0
        for subcmd in ("cat", "write", "upload", "download", "rm", "mv", "mkdir", "rmdir", "search", "info", "chmod", "replace"):
            assert subcmd in result.output

    @pytest.mark.parametrize(
        "subcmd",
        ["cat", "write", "upload", "download", "rm", "mv", "mkdir", "rmdir", "search", "info", "chmod", "replace"],
    )
    def test_file_subcommand_help(self, runner: CliRunner, subcmd: str) -> None:
        result = runner.invoke(cli, ["file", subcmd, "--help"])
        assert result.exit_code == 0


# ---------------------------------------------------------------------------
# Code sub-commands
# ---------------------------------------------------------------------------


class TestCodeHelp:
    def test_code_help(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["code", "--help"])
        assert result.exit_code == 0
        for subcmd in ("run", "context", "interrupt"):
            assert subcmd in result.output

    def test_code_context_help(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["code", "context", "--help"])
        assert result.exit_code == 0
        for subcmd in ("create", "list", "delete", "delete-all"):
            assert subcmd in result.output


# ---------------------------------------------------------------------------
# Config sub-commands
# ---------------------------------------------------------------------------


class TestConfigHelp:
    def test_config_help(self, runner: CliRunner) -> None:
        result = runner.invoke(cli, ["config", "--help"])
        assert result.exit_code == 0
        for subcmd in ("init", "show", "set"):
            assert subcmd in result.output

    @pytest.mark.parametrize("subcmd", ["init", "show", "set"])
    def test_config_subcommand_help(self, runner: CliRunner, subcmd: str) -> None:
        result = runner.invoke(cli, ["config", subcmd, "--help"])
        assert result.exit_code == 0
