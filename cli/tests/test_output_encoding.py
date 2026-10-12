# Copyright 2026 The OpenSandbox Authors.
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

"""Regression tests for #2185: the CLI must not crash on legacy code pages.

Help text and success lines render emoji/box-drawing characters. Writing them
to a stream whose encoding cannot represent them (Windows code page 936/GBK,
cp1252, POSIX ``C`` locale, ASCII pipes) used to raise ``UnicodeEncodeError``
and abort the command with exit code 1 — after the work had already succeeded.
The CLI now degrades unencodable characters instead.
"""

from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path

from click.testing import CliRunner

from opensandbox_cli.main import cli


def test_config_help_survives_gbk_charset() -> None:
    """`config --help` renders the ⚙️ group docstring; GBK cannot encode it."""
    result = CliRunner(charset="gbk").invoke(cli, ["config", "--help"])
    assert result.exit_code == 0
    assert "Usage" in result.output


def test_root_help_survives_ascii_charset() -> None:
    """Root help (banner + command list with ⚡ docstrings) degrades on ASCII."""
    result = CliRunner(charset="ascii").invoke(cli, ["--help"])
    assert result.exit_code == 0


def test_config_init_survives_gbk_charset(tmp_path: Path) -> None:
    """`config init` must not report failure after creating the file."""
    cfg = tmp_path / "cfg.toml"
    result = CliRunner(charset="gbk").invoke(
        cli, ["--config", str(cfg), "config", "init"]
    )
    assert result.exit_code == 0
    assert cfg.exists()


def test_config_init_survives_ascii_stdout_stream(tmp_path: Path) -> None:
    """Real-process check: ASCII stdout must not turn a success into exit 1.

    Equivalent to a POSIX ``C`` locale or ``PYTHONIOENCODING=ascii`` pipe,
    independent of the host console encoding.
    """
    cfg = tmp_path / "cfg.toml"
    env = dict(os.environ, PYTHONIOENCODING="ascii")
    proc = subprocess.run(
        [
            sys.executable,
            "-m",
            "opensandbox_cli",
            "--config",
            str(cfg),
            "config",
            "init",
        ],
        capture_output=True,
        env=env,
        timeout=60,
    )
    assert proc.returncode == 0
    assert cfg.exists()
