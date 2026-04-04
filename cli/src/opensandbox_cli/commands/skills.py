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

"""Install built-in OpenSandbox AI skills/rules for coding tools."""

from __future__ import annotations

from pathlib import Path
from typing import Literal, TypedDict, cast

import click

from opensandbox_cli.skill_registry import (
    BUILTIN_SKILLS,
    DEFAULT_SKILL,
    SkillSpec,
    extract_section,
    get_builtin_skill,
    list_builtin_skills,
    read_skill_markdown,
    render_skill_for_target,
    split_frontmatter,
)
from opensandbox_cli.utils import handle_errors


class CopyScopeConfig(TypedDict):
    strategy: Literal["copy"]
    dest_dir: Path
    preserve_frontmatter: bool
    file_suffix: str | None
    dest_file_template: str | None


class AppendScopeConfig(TypedDict):
    strategy: Literal["append"]
    dest_file: Path
    preserve_frontmatter: bool


TargetScopeConfig = CopyScopeConfig | AppendScopeConfig


class TargetConfig(TypedDict):
    label: str
    scopes: dict[str, TargetScopeConfig]


_TARGETS = cast(dict[str, TargetConfig], {
    "claude": {
        "label": "Claude Code",
        "scopes": {
            "project": {
                "strategy": "copy",
                "dest_dir": Path(".claude") / "skills",
                "preserve_frontmatter": True,
            },
            "global": {
                "strategy": "copy",
                "dest_dir": Path.home() / ".claude" / "skills",
                "preserve_frontmatter": True,
            }
        },
    },
    "cursor": {
        "label": "Cursor",
        "scopes": {
            "project": {
                "strategy": "copy",
                "dest_dir": Path(".cursor") / "rules",
                "preserve_frontmatter": False,
                "file_suffix": ".mdc",
            },
            "global": {
                "strategy": "copy",
                "dest_dir": Path.home() / ".cursor" / "rules",
                "preserve_frontmatter": False,
                "file_suffix": ".mdc",
            }
        },
    },
    "codex": {
        "label": "Codex",
        "scopes": {
            "project": {
                "strategy": "copy",
                "dest_dir": Path(".codex") / "skills",
                "dest_file_template": "{slug}/SKILL.md",
                "preserve_frontmatter": True,
            },
            "global": {
                "strategy": "copy",
                "dest_dir": Path.home() / ".codex" / "skills",
                "dest_file_template": "{slug}/SKILL.md",
                "preserve_frontmatter": True,
            },
        },
    },
    "copilot": {
        "label": "GitHub Copilot",
        "scopes": {
            "project": {
                "strategy": "append",
                "dest_file": Path(".github") / "copilot-instructions.md",
                "preserve_frontmatter": False,
            },
            "global": {
                "strategy": "append",
                "dest_file": Path.home() / ".github" / "copilot-instructions.md",
                "preserve_frontmatter": False,
            }
        },
    },
    "windsurf": {
        "label": "Windsurf",
        "scopes": {
            "project": {
                "strategy": "append",
                "dest_file": Path(".windsurfrules"),
                "preserve_frontmatter": False,
            },
            "global": {
                "strategy": "append",
                "dest_file": Path.home() / ".windsurfrules",
                "preserve_frontmatter": False,
            }
        },
    },
    "cline": {
        "label": "Cline",
        "scopes": {
            "project": {
                "strategy": "append",
                "dest_file": Path(".clinerules"),
                "preserve_frontmatter": False,
            },
            "global": {
                "strategy": "append",
                "dest_file": Path.home() / ".clinerules",
                "preserve_frontmatter": False,
            }
        },
    },
    "opencode": {
        "label": "OpenCode",
        "scopes": {
            "project": {
                "strategy": "copy",
                "dest_dir": Path(".agents") / "skills",
                "dest_file_template": "{slug}/SKILL.md",
                "preserve_frontmatter": True,
            },
            "global": {
                "strategy": "copy",
                "dest_dir": Path.home() / ".agents" / "skills",
                "dest_file_template": "{slug}/SKILL.md",
                "preserve_frontmatter": True,
            },
        },
    },
})

_ALL_TARGET_NAMES = list(_TARGETS.keys())
_ALL_SKILL_NAMES = list(BUILTIN_SKILLS.keys())
_ALL_SCOPE_NAMES = ["project", "global"]


def _marker_begin(skill: SkillSpec) -> str:
    return f"<!-- BEGIN {skill.marker_id} -->"


def _marker_end(skill: SkillSpec) -> str:
    return f"<!-- END {skill.marker_id} -->"


def _get_scope_cfg(name: str, scope: str) -> TargetScopeConfig:
    target_cfg = _TARGETS[name]
    scopes = target_cfg["scopes"]
    return scopes[scope]


def _target_layout_summary(name: str, scope: str) -> str:
    cfg = _get_scope_cfg(name, scope)
    if cfg["strategy"] == "append":
        return f"aggregate into one instructions file at {cfg['dest_file']}"

    dest_dir = cfg["dest_dir"]
    template = cfg.get("dest_file_template")
    if template:
        sample_path = dest_dir / template.format(slug="<skill-name>")
        return f"install one file per skill under {sample_path}"

    suffix = cfg.get("file_suffix") or ".md"
    sample_path = dest_dir / f"<skill-name>{suffix}"
    return f"install one file per skill under {sample_path}"


def _target_destination(name: str, scope: str, skill: SkillSpec) -> Path:
    cfg = _get_scope_cfg(name, scope)
    if cfg["strategy"] == "copy":
        dest_dir = cfg["dest_dir"]
        template = cfg.get("dest_file_template") or ""
        if template:
            return dest_dir / template.format(slug=skill.slug)
        suffix = cfg.get("file_suffix") or ".md"
        return dest_dir / f"{skill.slug}{suffix}"
    return cfg["dest_file"]


def _render_for_target(name: str, scope: str, skill: SkillSpec) -> str:
    cfg = _get_scope_cfg(name, scope)
    markdown = read_skill_markdown(skill)
    preserve_frontmatter = bool(cfg.get("preserve_frontmatter", False))
    return render_skill_for_target(
        skill,
        markdown,
        preserve_frontmatter=preserve_frontmatter,
    )


def _remove_marked_block(existing: str, skill: SkillSpec) -> str:
    begin = _marker_begin(skill)
    end = _marker_end(skill)
    if begin not in existing or end not in existing:
        return existing

    start = existing.index(begin)
    finish = existing.index(end) + len(end)
    before = existing[:start].rstrip("\n")
    after = existing[finish:].lstrip("\n")

    if before and after:
        return before + "\n\n" + after
    return before or after


def _is_installed(name: str, scope: str, skill: SkillSpec) -> bool:
    dest = _target_destination(name, scope, skill)
    if not dest.exists():
        return False

    cfg = _get_scope_cfg(name, scope)
    if cfg["strategy"] == "copy":
        return True

    content = dest.read_text(encoding="utf-8")
    return _marker_begin(skill) in content and _marker_end(skill) in content


def _install_copy(name: str, scope: str, skill: SkillSpec, content: str) -> Path:
    dest = _target_destination(name, scope, skill)
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text(content, encoding="utf-8")
    return dest


def _install_append(name: str, scope: str, skill: SkillSpec, content: str) -> Path:
    dest = _target_destination(name, scope, skill)
    dest.parent.mkdir(parents=True, exist_ok=True)

    existing = dest.read_text(encoding="utf-8") if dest.exists() else ""
    cleaned = _remove_marked_block(existing, skill).rstrip("\n")
    marked_block = (
        f"{_marker_begin(skill)}\n"
        f"{content.strip()}\n"
        f"{_marker_end(skill)}\n"
    )
    new_content = f"{cleaned}\n\n{marked_block}" if cleaned else marked_block
    dest.write_text(new_content, encoding="utf-8")
    return dest


def _install_target(name: str, scope: str, skill: SkillSpec) -> Path:
    content = _render_for_target(name, scope, skill)
    cfg = _get_scope_cfg(name, scope)
    if cfg["strategy"] == "copy":
        return _install_copy(name, scope, skill, content)
    return _install_append(name, scope, skill, content)


def _uninstall_target(name: str, scope: str, skill: SkillSpec) -> tuple[bool, Path]:
    dest = _target_destination(name, scope, skill)
    if not dest.exists():
        return False, dest

    cfg = _get_scope_cfg(name, scope)
    if cfg["strategy"] == "copy":
        dest.unlink()
        if dest.parent.exists() and not any(dest.parent.iterdir()):
            dest.parent.rmdir()
        return True, dest

    existing = dest.read_text(encoding="utf-8")
    cleaned = _remove_marked_block(existing, skill)
    if cleaned == existing:
        return False, dest
    if cleaned.strip():
        dest.write_text(cleaned.rstrip("\n") + "\n", encoding="utf-8")
    else:
        dest.unlink()
    return True, dest


def _resolve_skills(skill_name: str | None, install_all_builtins: bool) -> list[SkillSpec]:
    if install_all_builtins:
        return list_builtin_skills()
    if not skill_name:
        raise click.UsageError("A skill name is required unless --all-builtins is used.")
    return [get_builtin_skill(skill_name)]


def _print_install_guidance() -> None:
    click.echo("Install guidance:\n")
    click.echo("  Install one skill for one tool:")
    click.echo("    osb skills install <skill-name> --target <tool> --scope <scope>")
    click.echo()
    click.echo("  Install all bundled skills for one tool:")
    click.echo("    osb skills install --all-builtins --target <tool> --scope <scope>")
    click.echo()
    click.echo("  Discover skills and targets:")
    click.echo("    osb skills list")
    click.echo("    osb skills show <skill-name>")
    click.echo()
    click.echo(f"  Available skills: {', '.join(_ALL_SKILL_NAMES)}")
    click.echo(f"  Available targets: {', '.join(_ALL_TARGET_NAMES)}")
    click.echo(f"  Available scopes: {', '.join(_ALL_SCOPE_NAMES)}")


@click.group("skills", invoke_without_command=True)
@click.pass_context
def skills_group(ctx: click.Context) -> None:
    """Manage bundled OpenSandbox skills for AI coding tools."""
    if ctx.invoked_subcommand is None:
        click.echo(ctx.get_help())


@skills_group.command("install")
@click.argument(
    "skill_name",
    required=False,
    type=click.Choice(_ALL_SKILL_NAMES, case_sensitive=False),
)
@click.option(
    "--all-builtins",
    is_flag=True,
    default=False,
    help="Install all bundled skills instead of a single skill.",
)
@click.option(
    "--target",
    "-t",
    type=click.Choice(_ALL_TARGET_NAMES + ["all"], case_sensitive=False),
    default=None,
    help="Target AI tool to install the skill for.",
)
@click.option(
    "--scope",
    type=click.Choice(_ALL_SCOPE_NAMES, case_sensitive=False),
    default=None,
    help="Install scope for targets that support multiple locations.",
)
@click.option(
    "--force",
    "-f",
    is_flag=True,
    default=False,
    help="Overwrite or refresh an existing installed skill without prompting.",
)
@handle_errors
def skills_install(
    skill_name: str | None,
    all_builtins: bool,
    target: str | None,
    scope: str | None,
    force: bool,
) -> None:
    """Install one or more bundled OpenSandbox skills."""
    if all_builtins and skill_name:
        raise click.UsageError("Pass either a skill name or --all-builtins, not both.")
    if target is None or scope is None or (not all_builtins and skill_name is None):
        _print_install_guidance()
        return

    skills = _resolve_skills(skill_name, all_builtins)
    targets = _ALL_TARGET_NAMES if target == "all" else [target]

    click.echo("Install plan:\n")
    for target_name in targets:
        label = str(_TARGETS[target_name]["label"])
        click.echo(f"  {label} [{scope}]: {_target_layout_summary(target_name, scope)}")
    click.echo()

    for skill in skills:
        for target_name in targets:
            label = str(_TARGETS[target_name]["label"])
            dest = _target_destination(target_name, scope, skill)
            installed = _is_installed(target_name, scope, skill)

            if installed and not force:
                if not click.confirm(
                    f"  {label}: {skill.slug} already installed at {dest}. Refresh it?",
                    default=True,
                ):
                    click.echo(f"  ⏭  {label}: {skill.slug} skipped")
                    continue

            installed_path = _install_target(target_name, scope, skill)
            click.echo(f"  ✅ {label} [{scope}]: {skill.slug} -> {installed_path}")

    click.echo()
    click.echo("Done! Restart your AI coding tool to pick up the updated skill set.")


@skills_group.command("show")
@click.argument(
    "skill_name",
    type=click.Choice(_ALL_SKILL_NAMES, case_sensitive=False),
)
@handle_errors
def skills_show(skill_name: str) -> None:
    """Show details for a bundled skill."""
    skill = get_builtin_skill(skill_name)
    markdown = read_skill_markdown(skill)
    _, body = split_frontmatter(markdown)

    click.echo(f"Skill: {skill.slug}")
    click.echo(f"Title: {skill.title}")
    click.echo(f"Summary: {skill.summary}")
    click.echo(f"Trigger: {skill.trigger_hint}")
    click.echo()

    when_to_use = extract_section(body, "When To Use")
    if when_to_use:
        click.echo("When To Use:")
        click.echo(when_to_use)
        click.echo()

    for label, heading in (
        ("Quick Start", "Golden Paths"),
        ("Quick Start", "Core Workflow"),
        ("Quick Start", "Command Map"),
        ("Quick Start", "Common Commands"),
        ("Quick Start", "Fast Path"),
        ("Quick Start", "Inspect Current Policy"),
        ("Quick Start", "Preferred Workflow"),
    ):
        quick_start = extract_section(body, heading)
        if quick_start:
            click.echo(f"{label}:")
            click.echo(quick_start)
            click.echo()
            break

    for heading in ("Minimal Closed Loops", "Response Pattern", "Guidance"):
        section = extract_section(body, heading)
        if section:
            click.echo(f"{heading}:")
            click.echo(section)
            click.echo()

    if "```json" in body:
        click.echo("JSON Shapes:")
        start = body.find("```json")
        end = body.find("```", start + 7)
        if start != -1 and end != -1:
            click.echo(body[start + 7 : end].strip())
            click.echo()

    click.echo("Full Skill:")
    click.echo(markdown.strip())


@skills_group.command("list")
@handle_errors
def skills_list() -> None:
    """List bundled skills, supported targets, and install status."""
    click.echo("Bundled skills:\n")
    for skill in list_builtin_skills():
        click.echo(f"  {skill.slug:<24}  {skill.summary}")
        click.echo(f"  {'':<24}  {skill.trigger_hint}")

    click.echo("\nSupported targets:\n")
    for target_name, cfg in _TARGETS.items():
        label = str(cfg["label"])
        for scope_name in cfg["scopes"]:
            layout = _target_layout_summary(target_name, scope_name)
            click.echo(f"  {target_name:<10}  {scope_name:<8}  {label:<18}  {layout}")
            for skill in list_builtin_skills():
                dest = _target_destination(target_name, scope_name, skill)
                status = "installed" if _is_installed(target_name, scope_name, skill) else "not installed"
                click.echo(
                    f"  {'':<10}  {'':<8}  {'':<18}  {skill.slug:<24}  "
                    f"{status:<13}  ({dest})"
                )


@skills_group.command("uninstall")
@click.argument(
    "skill_name",
    required=False,
    default=DEFAULT_SKILL,
    type=click.Choice(_ALL_SKILL_NAMES, case_sensitive=False),
)
@click.option(
    "--target",
    "-t",
    type=click.Choice(_ALL_TARGET_NAMES + ["all"], case_sensitive=False),
    default=None,
    help="Target AI tool to remove the skill from.",
)
@click.option(
    "--scope",
    type=click.Choice(_ALL_SCOPE_NAMES, case_sensitive=False),
    default=None,
    help="Install scope to remove from.",
)
@handle_errors
def skills_uninstall(skill_name: str, target: str | None, scope: str | None) -> None:
    """Remove an installed OpenSandbox skill from one or more AI tools."""
    if target is None or scope is None:
        _print_install_guidance()
        return
    skill = get_builtin_skill(skill_name)
    targets = _ALL_TARGET_NAMES if target == "all" else [target]

    for target_name in targets:
        label = str(_TARGETS[target_name]["label"])
        removed, dest = _uninstall_target(target_name, scope, skill)
        if removed:
            click.echo(f"  🗑  {label} [{scope}]: removed {skill.slug} from {dest}")
        else:
            click.echo(f"  ⏭  {label} [{scope}]: {skill.slug} not installed")
