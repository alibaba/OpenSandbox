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

"""Output formatting: table (rich), JSON, YAML."""

from __future__ import annotations

import json
import sys
from typing import Any, Sequence

import click

try:
    import yaml
except ImportError:  # pragma: no cover
    yaml = None  # type: ignore[assignment]

from pydantic import BaseModel
from rich.console import Console
from rich.table import Table


class OutputFormatter:
    """Renders data in table / json / yaml format."""

    def __init__(self, fmt: str = "table", *, color: bool = True) -> None:
        self.fmt = fmt
        self.console = Console(
            stderr=False, no_color=not color, force_terminal=None
        )

    # ------------------------------------------------------------------
    # Public helpers
    # ------------------------------------------------------------------

    def print_model(self, model: BaseModel, title: str | None = None) -> None:
        """Print a single Pydantic model as key-value panel or JSON/YAML."""
        data = _model_to_dict(model)
        if self.fmt == "json":
            self._print_json(data)
        elif self.fmt == "yaml":
            self._print_yaml(data)
        else:
            self._print_kv_table(data, title=title)

    def print_models(
        self,
        models: Sequence[BaseModel],
        columns: list[str],
        *,
        title: str | None = None,
    ) -> None:
        """Print a list of Pydantic models as a table or JSON/YAML."""
        rows = [_model_to_dict(m) for m in models]
        if self.fmt == "json":
            self._print_json(rows)
        elif self.fmt == "yaml":
            self._print_yaml(rows)
        else:
            self._print_table(rows, columns, title=title)

    def print_dict(self, data: dict[str, Any], title: str | None = None) -> None:
        """Print a flat dict."""
        if self.fmt == "json":
            self._print_json(data)
        elif self.fmt == "yaml":
            self._print_yaml(data)
        else:
            self._print_kv_table(data, title=title)

    def print_text(self, text: str) -> None:
        """Print raw text (ignores format)."""
        click.echo(text)

    # ------------------------------------------------------------------
    # Internal renderers
    # ------------------------------------------------------------------

    def _print_json(self, data: Any) -> None:
        click.echo(json.dumps(data, indent=2, default=str))

    def _print_yaml(self, data: Any) -> None:
        if yaml is None:
            click.secho(
                "PyYAML is not installed. Use --output json instead.", fg="red", err=True
            )
            sys.exit(1)
        click.echo(yaml.dump(data, default_flow_style=False, allow_unicode=True).rstrip())

    def _print_kv_table(self, data: dict[str, Any], *, title: str | None = None) -> None:
        table = Table(title=title, show_header=True, header_style="bold")
        table.add_column("Key", style="cyan")
        table.add_column("Value")
        for k, v in data.items():
            table.add_row(str(k), str(v) if v is not None else "-")
        self.console.print(table)

    def _print_table(
        self,
        rows: list[dict[str, Any]],
        columns: list[str],
        *,
        title: str | None = None,
    ) -> None:
        table = Table(title=title, show_header=True, header_style="bold")
        for col in columns:
            table.add_column(col.upper())
        for row in rows:
            table.add_row(*(str(row.get(col, "-")) for col in columns))
        self.console.print(table)


# ------------------------------------------------------------------
# Helpers
# ------------------------------------------------------------------


def _model_to_dict(model: BaseModel) -> dict[str, Any]:
    return model.model_dump(mode="json")
