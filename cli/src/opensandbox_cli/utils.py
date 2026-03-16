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

"""Shared CLI utilities: duration parsing, error handling, key-value parsing."""

from __future__ import annotations

import functools
import re
import sys
from datetime import timedelta

import click


# ---------------------------------------------------------------------------
# Duration parsing  (e.g. "10m", "1h30m", "90s", "2h")
# ---------------------------------------------------------------------------

_DURATION_RE = re.compile(
    r"^(?:(?P<hours>\d+)h)?(?:(?P<minutes>\d+)m)?(?:(?P<seconds>\d+)s)?$"
)


def parse_duration(value: str) -> timedelta:
    """Parse a human-friendly duration string into a ``timedelta``.

    Supported formats: ``10m``, ``1h30m``, ``90s``, ``2h``, ``1h30m45s``.
    A plain integer is treated as seconds.
    """
    value = value.strip()
    if not value:
        raise click.BadParameter("Duration cannot be empty")

    # Plain integer → seconds
    if value.isdigit():
        return timedelta(seconds=int(value))

    m = _DURATION_RE.match(value)
    if not m or not m.group(0):
        raise click.BadParameter(
            f"Invalid duration '{value}'. Use format like 10m, 1h30m, 90s."
        )

    hours = int(m.group("hours") or 0)
    minutes = int(m.group("minutes") or 0)
    seconds = int(m.group("seconds") or 0)
    return timedelta(hours=hours, minutes=minutes, seconds=seconds)


class DurationType(click.ParamType):
    """Click parameter type for duration strings."""

    name = "duration"

    def convert(
        self, value: str, param: click.Parameter | None, ctx: click.Context | None
    ) -> timedelta:
        if isinstance(value, timedelta):
            return value
        try:
            return parse_duration(value)
        except click.BadParameter:
            self.fail(
                f"Invalid duration '{value}'. Use format like 10m, 1h30m, 90s.",
                param,
                ctx,
            )


DURATION = DurationType()


# ---------------------------------------------------------------------------
# Key=Value parsing  (e.g. --env FOO=bar)
# ---------------------------------------------------------------------------


class KeyValueType(click.ParamType):
    """Click parameter type that parses ``KEY=VALUE`` strings into a tuple."""

    name = "KEY=VALUE"

    def convert(
        self, value: str, param: click.Parameter | None, ctx: click.Context | None
    ) -> tuple[str, str]:
        if isinstance(value, tuple):
            return value
        if "=" not in value:
            self.fail(f"Expected KEY=VALUE format, got '{value}'", param, ctx)
        key, _, val = value.partition("=")
        return (key, val)


KEY_VALUE = KeyValueType()


# ---------------------------------------------------------------------------
# Error handling decorator
# ---------------------------------------------------------------------------


def handle_errors(fn):  # type: ignore[no-untyped-def]
    """Decorator that catches SDK / HTTP exceptions and prints a friendly message."""

    @functools.wraps(fn)
    def wrapper(*args, **kwargs):  # type: ignore[no-untyped-def]
        try:
            return fn(*args, **kwargs)
        except click.exceptions.Exit:
            raise
        except click.ClickException:
            raise
        except Exception as exc:
            # Import here to avoid circular imports at module level
            from opensandbox.exceptions import SandboxException

            if isinstance(exc, SandboxException):
                click.secho(f"Error: {exc}", fg="red", err=True)
            else:
                click.secho(f"Error: {exc}", fg="red", err=True)
            sys.exit(1)

    return wrapper
