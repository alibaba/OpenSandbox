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

"""Tests for bundled skill install/list/show/uninstall flows."""

from __future__ import annotations

import importlib.resources
from pathlib import Path

import pytest
from click.testing import CliRunner

from opensandbox_cli.commands.skills import _TARGETS, skills_group
from opensandbox_cli.skill_registry import list_builtin_skills


@pytest.fixture()
def isolated_skill_targets(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    patched = {
        "claude": {
            **_TARGETS["claude"],
            "scopes": {
                "project": {
                    **_TARGETS["claude"]["scopes"]["project"],
                    "dest_dir": tmp_path / ".claude" / "skills",
                },
                "global": {
                    **_TARGETS["claude"]["scopes"]["global"],
                    "dest_dir": tmp_path / "home" / ".claude" / "skills",
                },
            },
        },
        "cursor": {
            **_TARGETS["cursor"],
            "scopes": {
                "project": {
                    **_TARGETS["cursor"]["scopes"]["project"],
                    "dest_dir": tmp_path / ".cursor" / "rules",
                },
                "global": {
                    **_TARGETS["cursor"]["scopes"]["global"],
                    "dest_dir": tmp_path / "home" / ".cursor" / "rules",
                },
            },
        },
        "codex": {
            **_TARGETS["codex"],
            "scopes": {
                "project": {
                    **_TARGETS["codex"]["scopes"]["project"],
                    "dest_dir": tmp_path / ".codex" / "skills",
                },
                "global": {
                    **_TARGETS["codex"]["scopes"]["global"],
                    "dest_dir": tmp_path / "home" / ".codex" / "skills",
                },
            },
        },
        "copilot": {
            **_TARGETS["copilot"],
            "scopes": {
                "project": {
                    **_TARGETS["copilot"]["scopes"]["project"],
                    "dest_file": tmp_path / ".github" / "copilot-instructions.md",
                },
                "global": {
                    **_TARGETS["copilot"]["scopes"]["global"],
                    "dest_file": tmp_path / "home" / ".github" / "copilot-instructions.md",
                },
            },
        },
        "windsurf": {
            **_TARGETS["windsurf"],
            "scopes": {
                "project": {
                    **_TARGETS["windsurf"]["scopes"]["project"],
                    "dest_file": tmp_path / ".windsurfrules",
                },
                "global": {
                    **_TARGETS["windsurf"]["scopes"]["global"],
                    "dest_file": tmp_path / "home" / ".windsurfrules",
                },
            },
        },
        "cline": {
            **_TARGETS["cline"],
            "scopes": {
                "project": {
                    **_TARGETS["cline"]["scopes"]["project"],
                    "dest_file": tmp_path / ".clinerules",
                },
                "global": {
                    **_TARGETS["cline"]["scopes"]["global"],
                    "dest_file": tmp_path / "home" / ".clinerules",
                },
            },
        },
        "opencode": {
            **_TARGETS["opencode"],
            "scopes": {
                "project": {
                    **_TARGETS["opencode"]["scopes"]["project"],
                    "dest_dir": tmp_path / ".agents" / "skills",
                },
                "global": {
                    **_TARGETS["opencode"]["scopes"]["global"],
                    "dest_dir": tmp_path / "home" / ".agents" / "skills",
                },
            },
        },
    }
    monkeypatch.setattr("opensandbox_cli.commands.skills._TARGETS", patched)


class TestSkillsCommands:
    def test_install_without_args_prints_guidance_and_does_not_install(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
        tmp_path: Path,
    ) -> None:
        result = runner.invoke(skills_group, ["install"])

        assert result.exit_code == 0
        assert "Install guidance:" in result.output
        assert "osb skills install <skill-name> --target <tool> --scope <scope>" in result.output
        assert not (tmp_path / ".claude" / "skills" / "troubleshoot-sandbox.md").exists()

    def test_install_with_skill_but_without_target_prints_guidance(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
    ) -> None:
        result = runner.invoke(skills_group, ["install", "troubleshoot-sandbox"])

        assert result.exit_code == 0
        assert "Install guidance:" in result.output

    def test_install_with_all_builtins_but_without_target_prints_guidance(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
    ) -> None:
        result = runner.invoke(skills_group, ["install", "--all-builtins"])

        assert result.exit_code == 0
        assert "Install guidance:" in result.output

    def test_install_copy_target_creates_named_skill_file(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
        tmp_path: Path,
    ) -> None:
        result = runner.invoke(
            skills_group,
            ["install", "troubleshoot-sandbox", "--target", "claude", "--scope", "project"],
        )

        assert result.exit_code == 0
        dest = tmp_path / ".claude" / "skills" / "troubleshoot-sandbox.md"
        assert dest.exists()
        content = dest.read_text(encoding="utf-8")
        assert content.startswith("---\nname: troubleshoot-sandbox")

    def test_install_codex_creates_skill_directory_with_frontmatter(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
        tmp_path: Path,
    ) -> None:
        result = runner.invoke(
            skills_group,
            ["install", "troubleshoot-sandbox", "--target", "codex", "--scope", "project"],
        )

        assert result.exit_code == 0
        dest = tmp_path / ".codex" / "skills" / "troubleshoot-sandbox" / "SKILL.md"
        content = dest.read_text(encoding="utf-8")
        assert content.startswith("---\nname: troubleshoot-sandbox")
        assert "# OpenSandbox Troubleshooting" in content

    def test_install_all_builtins_to_codex_creates_skill_directories(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
        tmp_path: Path,
    ) -> None:
        result = runner.invoke(
            skills_group,
            ["install", "--all-builtins", "--target", "codex", "--scope", "project"],
        )

        assert result.exit_code == 0
        assert "Install plan:" in result.output
        assert "install one file per skill" in result.output
        for skill in list_builtin_skills():
            dest = tmp_path / ".codex" / "skills" / skill.slug / "SKILL.md"
            assert dest.exists()

    def test_install_rejects_skill_name_and_all_builtins_together(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
    ) -> None:
        result = runner.invoke(
            skills_group,
            ["install", "troubleshoot-sandbox", "--all-builtins"],
        )

        assert result.exit_code != 0
        assert "either a skill name or --all-builtins" in result.output

    def test_install_to_global_codex_uses_global_path(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
        tmp_path: Path,
    ) -> None:
        result = runner.invoke(
            skills_group,
            ["install", "troubleshoot-sandbox", "--target", "codex", "--scope", "global"],
        )

        assert result.exit_code == 0
        assert (tmp_path / "home" / ".codex" / "skills" / "troubleshoot-sandbox" / "SKILL.md").exists()

    def test_install_to_project_opencode_creates_skill_directory(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
        tmp_path: Path,
    ) -> None:
        result = runner.invoke(
            skills_group,
            ["install", "network-egress", "--target", "opencode", "--scope", "project"],
        )

        assert result.exit_code == 0
        dest = tmp_path / ".agents" / "skills" / "network-egress" / "SKILL.md"
        assert dest.exists()
        assert dest.read_text(encoding="utf-8").startswith("---\nname: network-egress")

    def test_show_prints_skill_metadata_and_content(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
    ) -> None:
        result = runner.invoke(skills_group, ["show", "file-operations"])

        assert result.exit_code == 0
        assert "Skill: file-operations" in result.output
        assert "Title: OpenSandbox File Operations" in result.output
        assert "When To Use:" in result.output
        assert "Quick Start:" in result.output
        assert "Minimal Closed Loops:" in result.output
        assert "Full Skill:" in result.output
        assert "osb file cat" in result.output

    def test_show_supports_new_network_egress_skill(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
    ) -> None:
        result = runner.invoke(skills_group, ["show", "network-egress"])

        assert result.exit_code == 0
        assert "Skill: network-egress" in result.output
        assert "Quick Start:" in result.output
        assert "osb egress patch" in result.output

    def test_show_surfaces_json_shapes_for_lifecycle_skill(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
    ) -> None:
        result = runner.invoke(skills_group, ["show", "sandbox-lifecycle"])

        assert result.exit_code == 0
        assert "JSON Shapes:" in result.output
        assert '"defaultAction": "deny"' in result.output
        assert '"mountPath": "/workspace/data"' in result.output

    def test_list_reports_all_builtins(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
    ) -> None:
        result = runner.invoke(skills_group, ["list"])

        assert result.exit_code == 0
        assert "aggregate into one instructions file" in result.output
        assert "install one file per skill" in result.output
        for skill in list_builtin_skills():
            assert skill.slug in result.output

    def test_list_reports_not_installed_when_append_target_file_exists_without_marker(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
        tmp_path: Path,
    ) -> None:
        dest = tmp_path / ".github" / "copilot-instructions.md"
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text("user custom instructions\n", encoding="utf-8")

        result = runner.invoke(skills_group, ["list"])

        assert result.exit_code == 0
        assert "copilot" in result.output
        assert "not installed" in result.output

    def test_uninstall_append_target_preserves_non_skill_content(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
        tmp_path: Path,
    ) -> None:
        install_result = runner.invoke(
            skills_group,
            ["install", "troubleshoot-sandbox", "--target", "copilot", "--scope", "project"],
        )
        assert install_result.exit_code == 0

        dest = tmp_path / ".github" / "copilot-instructions.md"
        original = dest.read_text(encoding="utf-8")
        dest.write_text("team rules\n\n" + original, encoding="utf-8")

        uninstall_result = runner.invoke(
            skills_group,
            ["uninstall", "troubleshoot-sandbox", "--target", "copilot", "--scope", "project"],
        )

        assert uninstall_result.exit_code == 0
        assert dest.read_text(encoding="utf-8") == "team rules\n"

    def test_reinstall_append_target_does_not_duplicate_skill_block(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
        tmp_path: Path,
    ) -> None:
        first = runner.invoke(
            skills_group,
            ["install", "troubleshoot-sandbox", "--target", "copilot", "--scope", "project"],
        )
        assert first.exit_code == 0

        second = runner.invoke(
            skills_group,
            ["install", "troubleshoot-sandbox", "--target", "copilot", "--scope", "project", "--force"],
        )

        assert second.exit_code == 0
        dest = tmp_path / ".github" / "copilot-instructions.md"
        content = dest.read_text(encoding="utf-8")
        assert content.count("<!-- BEGIN opensandbox-troubleshoot-sandbox -->") == 1

    def test_install_all_builtins_to_copy_target_creates_new_skill_files(
        self,
        runner: CliRunner,
        isolated_skill_targets: None,
        tmp_path: Path,
    ) -> None:
        result = runner.invoke(
            skills_group,
            ["install", "--all-builtins", "--target", "claude", "--scope", "project"],
        )

        assert result.exit_code == 0
        assert "Install plan:" in result.output
        assert "install one file per skill" in result.output
        assert (tmp_path / ".claude" / "skills" / "network-egress.md").exists()
        assert (tmp_path / ".claude" / "skills" / "devops-diagnostics.md").exists()


class TestSkillContentQuality:
    def _read_builtin_skill(self, package_file: str) -> str:
        resource = importlib.resources.files("opensandbox_cli") / "skills" / package_file
        return Path(str(resource)).read_text(encoding="utf-8")

    def test_file_operations_examples_match_real_cli_flags(self) -> None:
        content = self._read_builtin_skill("opensandbox-file-operations.md")

        assert "## Operation Modes" in content
        assert 'osb file search <sandbox-id> /workspace --pattern "*.py"' in content
        assert "osb file replace <sandbox-id> /path/to/file --old old --new new" in content
        assert "osb file chmod <sandbox-id> /path/to/script --mode 0755" in content
        assert "If the content should come from stdin, omit `-c`" in content
        assert "do not suggest `rm` or `rmdir` until the target has been verified" in content
        assert "osb file upload <sandbox-id> ./local.txt /workspace/local.txt" in content
        assert "osb file info <sandbox-id> /workspace/tmp.txt" in content

    def test_command_execution_covers_execution_modes_and_session_workdir_rules(self) -> None:
        content = self._read_builtin_skill("opensandbox-command-execution.md")

        assert "## Execution Modes" in content
        assert "osb command run <sandbox-id> --background -- <command>" in content
        assert "osb command logs <sandbox-id> <execution-id> --cursor 0" in content
        assert "session create --workdir" in content
        assert "session run --workdir" in content
        assert "Do not suggest `command logs` for foreground commands" in content

    def test_sandbox_lifecycle_includes_copy_pasteable_json_shapes(self) -> None:
        content = self._read_builtin_skill("opensandbox-sandbox-lifecycle.md")

        assert "## Configuration Resolution" in content
        assert "osb config show" in content
        assert "keep using the current configuration, temporarily override it for one command, or persist new values" in content
        assert "## Golden Path" in content
        assert "osb sandbox health <sandbox-id>" in content
        assert "osb sandbox endpoint <sandbox-id> --port 8080" in content
        assert "osb sandbox metrics <sandbox-id> --watch" in content
        assert "--skip-health-check" in content
        assert '"defaultAction": "deny"' in content
        assert '"mountPath": "/workspace/data"' in content
        assert '"claimName": "shared-models-pvc"' in content
        assert '"readOnly": false' in content

    def test_troubleshoot_skill_includes_authenticated_http_examples(self) -> None:
        content = self._read_builtin_skill("opensandbox-troubleshoot.md")

        assert "## Configuration Resolution" in content
        assert "tell the user which server and protocol the CLI is currently pointed at" in content
        assert "use raw HTTP only after domain, protocol, and API key expectations are explicit" in content
        assert 'OPEN-SANDBOX-API-KEY: <api-key>' in content
        assert "/diagnostics/summary" in content
        assert "## Triage Model" in content
        assert "osb sandbox health <sandbox-id>" in content
        assert "## Symptom To Command Mapping" in content
        assert "outbound network access failure" in content
        assert "do not paper over the issue with `--skip-health-check`" in content
        assert "/diagnostics/logs?tail=500" in content

    def test_network_egress_covers_policy_model_and_behavior_verification(self) -> None:
        content = self._read_builtin_skill("opensandbox-network-egress.md")

        assert "## Policy Model" in content
        assert "`defaultAction`" in content
        assert "merge semantics" in content
        assert "Use `osb egress patch` for already-created sandboxes" in content
        assert "osb exec <sandbox-id> -- curl -I https://pypi.org" in content
        assert "osb egress patch <sandbox-id> --rule allow=*.example.com" in content

    def test_devops_diagnostics_covers_plain_text_model_and_symptom_mapping(self) -> None:
        content = self._read_builtin_skill("opensandbox-devops-diagnostics.md")

        assert "## Configuration Resolution" in content
        assert "Use the resolved configuration as the source of truth" in content
        assert "## Diagnostics Model" in content
        assert "plain-text output" in content
        assert "## Command Selection By Symptom" in content
        assert "suspected OOM or exit code issue" in content
        assert "start with `summary`, then `inspect`" in content
        assert "use `troubleshoot-sandbox` when the user wants root cause analysis" in content

    def test_network_egress_resolves_effective_configuration_before_policy_changes(self) -> None:
        content = self._read_builtin_skill("opensandbox-network-egress.md")

        assert "## Configuration Resolution" in content
        assert "osb config show" in content
        assert "Do not assume the user configures OpenSandbox only through environment variables." in content
        assert "temporarily override them, or persist a new configuration" in content
