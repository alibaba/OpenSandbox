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

"""Config management commands: init, show, set."""

from __future__ import annotations

from pathlib import Path

import click

from opensandbox_cli.client import ClientContext
from opensandbox_cli.config import DEFAULT_CONFIG_PATH, init_config_file
from opensandbox_cli.utils import handle_errors


@click.group("config", invoke_without_command=True)
@click.pass_context
def config_group(ctx: click.Context) -> None:
    """Manage CLI configuration."""
    if ctx.invoked_subcommand is None:
        click.echo(ctx.get_help())


# ---- init -----------------------------------------------------------------

@config_group.command("init")
@click.option("--force", is_flag=True, default=False, help="Overwrite existing config file.")
@click.option("--path", "config_path", type=click.Path(path_type=Path), default=None, help="Config file path.")
@handle_errors
def config_init(force: bool, config_path: Path | None) -> None:
    """Create a default configuration file."""
    try:
        path = init_config_file(config_path, force=force)
        click.echo(f"Config file created: {path}")
    except FileExistsError as exc:
        click.secho(str(exc), fg="yellow", err=True)


# ---- show -----------------------------------------------------------------

@config_group.command("show")
@click.pass_obj
@handle_errors
def config_show(obj: ClientContext) -> None:
    """Show the resolved configuration."""
    obj.output.print_dict(obj.resolved_config, title="Resolved Configuration")


# ---- set ------------------------------------------------------------------

@config_group.command("set")
@click.argument("key")
@click.argument("value")
@click.option("--path", "config_path", type=click.Path(path_type=Path), default=None, help="Config file path.")
@handle_errors
def config_set(key: str, value: str, config_path: Path | None) -> None:
    """Set a configuration value (e.g. 'connection.domain' 'localhost:9090')."""
    path = config_path or DEFAULT_CONFIG_PATH
    if not path.exists():
        click.secho(f"Config file not found: {path}. Run 'osb config init' first.", fg="red", err=True)
        return

    content = path.read_text()

    # Simple key replacement in TOML
    # Supports dotted keys like connection.domain
    parts = key.split(".", 1)
    if len(parts) == 2:
        section, field = parts
        # Try to find and update existing value
        import re

        section_pattern = rf"(\[{re.escape(section)}\].*?)(?=\n\[|\Z)"
        section_match = re.search(section_pattern, content, re.DOTALL)

        # Infer TOML value type: bool > int > float > string
        def _toml_value(raw: str) -> str:
            if raw.lower() in ("true", "false"):
                return raw.lower()
            try:
                int(raw)
                return raw
            except ValueError:
                pass
            try:
                float(raw)
                return raw
            except ValueError:
                pass
            return f'"{raw}"'

        toml_val = _toml_value(value)

        if section_match:
            section_text = section_match.group(1)
            field_pattern = rf'^(#?\s*{re.escape(field)}\s*=\s*).*$'
            field_match = re.search(field_pattern, section_text, re.MULTILINE)
            if field_match:
                new_line = f'{field} = {toml_val}'
                new_section = section_text[:field_match.start()] + new_line + section_text[field_match.end():]
                content = content[:section_match.start()] + new_section + content[section_match.end():]
            else:
                # Add field to section
                insert_pos = section_match.end()
                content = content[:insert_pos] + f'\n{field} = {toml_val}' + content[insert_pos:]
        else:
            # Add new section
            content += f'\n[{section}]\n{field} = {toml_val}\n'
    else:
        click.secho("Key must be in 'section.field' format (e.g. connection.domain).", fg="red", err=True)
        return

    path.write_text(content)
    click.echo(f"Set {key} = {value}")
