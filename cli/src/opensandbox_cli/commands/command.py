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

"""Command execution commands: run, status, logs, interrupt + top-level exec alias."""

from __future__ import annotations

import shlex
import sys
from datetime import timedelta

import click
from opensandbox.models.execd import OutputMessage, RunCommandOpts
from opensandbox.models.execd_sync import ExecutionHandlersSync

from opensandbox_cli.client import ClientContext
from opensandbox_cli.utils import DURATION, handle_errors


@click.group("command", invoke_without_command=True)
@click.pass_context
def command_group(ctx: click.Context) -> None:
    """⚡ Execute commands in a sandbox."""
    if ctx.invoked_subcommand is None:
        click.echo(ctx.get_help())


# ---- run ------------------------------------------------------------------

def _run_command(
    obj: ClientContext,
    sandbox_id: str,
    command: tuple[str, ...],
    background: bool,
    workdir: str | None,
    timeout: timedelta | None,
) -> None:
    """Shared implementation for 'command run' and top-level 'exec'."""
    cmd_str = " ".join(shlex.quote(arg) for arg in command)
    sandbox = obj.connect_sandbox(sandbox_id)

    try:
        opts = RunCommandOpts(
            background=background,
            working_directory=workdir,
            timeout=timeout,
        )

        if background:
            execution = sandbox.commands.run(cmd_str, opts=opts)
            obj.output.success_panel(
                {
                    "execution_id": execution.id,
                    "sandbox_id": sandbox_id,
                    "mode": "background",
                },
                title="Background Command Started",
            )
            return

        # Foreground: stream stdout/stderr to terminal
        last_text = ""

        def on_stdout(msg: OutputMessage) -> None:
            nonlocal last_text
            last_text = msg.text
            sys.stdout.write(msg.text)
            sys.stdout.flush()

        def on_stderr(msg: OutputMessage) -> None:
            nonlocal last_text
            last_text = msg.text
            sys.stderr.write(msg.text)
            sys.stderr.flush()

        handlers = ExecutionHandlersSync(on_stdout=on_stdout, on_stderr=on_stderr)
        execution = sandbox.commands.run(cmd_str, opts=opts, handlers=handlers)

        # Ensure terminal prompt starts on a new line
        if last_text and not last_text.endswith("\n"):
            sys.stdout.write("\n")
            sys.stdout.flush()

        if execution.error:
            obj.output.error_panel(
                f"{execution.error.name}: {execution.error.value}",
                title="Execution Error",
            )
            sys.exit(1)
    finally:
        sandbox.close()


@command_group.command("run")
@click.argument("sandbox_id")
@click.argument("command", nargs=-1, required=True)
@click.option("-d", "--background", is_flag=True, default=False, help="Run in background.")
@click.option("-w", "--workdir", default=None, help="Working directory.")
@click.option("-t", "--timeout", type=DURATION, default=None, help="Command timeout (e.g. 30s, 5m).")
@click.pass_obj
@handle_errors
def command_run(
    obj: ClientContext,
    sandbox_id: str,
    command: tuple[str, ...],
    background: bool,
    workdir: str | None,
    timeout: timedelta | None,
) -> None:
    """Run a command in a sandbox."""
    _run_command(obj, sandbox_id, command, background, workdir, timeout)


# ---- status ---------------------------------------------------------------

@command_group.command("status")
@click.argument("sandbox_id")
@click.argument("execution_id")
@click.pass_obj
@handle_errors
def command_status(obj: ClientContext, sandbox_id: str, execution_id: str) -> None:
    """Get command execution status."""
    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        status = sandbox.commands.get_command_status(execution_id)
        obj.output.print_model(status, title="Command Status")
    finally:
        sandbox.close()


# ---- logs -----------------------------------------------------------------

@command_group.command("logs")
@click.argument("sandbox_id")
@click.argument("execution_id")
@click.option("--cursor", type=int, default=None, help="Cursor for incremental reads.")
@click.pass_obj
@handle_errors
def command_logs(
    obj: ClientContext, sandbox_id: str, execution_id: str, cursor: int | None
) -> None:
    """Get background command logs."""
    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        logs = sandbox.commands.get_background_command_logs(execution_id, cursor=cursor)
        if obj.output.fmt in ("json", "yaml"):
            obj.output.print_model(logs, title="Command Logs")
        else:
            click.echo(logs.content)
    finally:
        sandbox.close()


# ---- interrupt ------------------------------------------------------------

@command_group.command("interrupt")
@click.argument("sandbox_id")
@click.argument("execution_id")
@click.pass_obj
@handle_errors
def command_interrupt(obj: ClientContext, sandbox_id: str, execution_id: str) -> None:
    """Interrupt a running command."""
    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        sandbox.commands.interrupt(execution_id)
        obj.output.success(f"Interrupted: {execution_id}")
    finally:
        sandbox.close()


# ---- top-level exec alias ------------------------------------------------

@click.command("exec")
@click.argument("sandbox_id")
@click.argument("command", nargs=-1, required=True)
@click.option("-d", "--background", is_flag=True, default=False, help="Run in background.")
@click.option("-w", "--workdir", default=None, help="Working directory.")
@click.option("-t", "--timeout", type=DURATION, default=None, help="Command timeout (e.g. 30s, 5m).")
@click.pass_obj
@handle_errors
def exec_cmd(
    obj: ClientContext,
    sandbox_id: str,
    command: tuple[str, ...],
    background: bool,
    workdir: str | None,
    timeout: timedelta | None,
) -> None:
    """🚀 Execute a command in a sandbox (shortcut for 'command run')."""
    _run_command(obj, sandbox_id, command, background, workdir, timeout)
