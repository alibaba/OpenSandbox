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

"""Code execution commands: run, context management, interrupt."""

from __future__ import annotations

import sys

import click
from opensandbox.models.execd import OutputMessage
from opensandbox.models.execd_sync import ExecutionHandlersSync

from opensandbox_cli.client import ClientContext
from opensandbox_cli.utils import handle_errors


@click.group("code", invoke_without_command=True)
@click.pass_context
def code_group(ctx: click.Context) -> None:
    """💻 Execute code in a sandbox (via Code Interpreter)."""
    if ctx.invoked_subcommand is None:
        click.echo(ctx.get_help())


# ---- run ------------------------------------------------------------------

@code_group.command("run")
@click.argument("sandbox_id")
@click.option("--language", "-l", required=True, help="Language (python, javascript, java, go, bash, ...).")
@click.option("--code", "-c", default=None, help="Code to execute. Reads from stdin if not provided.")
@click.option("--context-id", default=None, help="Execution context ID for stateful sessions.")
@click.pass_obj
@handle_errors
def code_run(
    obj: ClientContext,
    sandbox_id: str,
    language: str,
    code: str | None,
    context_id: str | None,
) -> None:
    """Execute code in a sandbox."""
    from code_interpreter.sync.code_interpreter import CodeInterpreterSync

    if code is None:
        if sys.stdin.isatty():
            click.echo("Reading code from stdin (Ctrl+D to finish):", err=True)
        code = sys.stdin.read()

    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        interpreter = CodeInterpreterSync.create(sandbox)

        kwargs: dict = {}
        if context_id:
            ctx = interpreter.codes.get_context(context_id)
            kwargs["context"] = ctx

        def on_stdout(msg: OutputMessage) -> None:
            sys.stdout.write(msg.text)
            sys.stdout.flush()

        def on_stderr(msg: OutputMessage) -> None:
            sys.stderr.write(msg.text)
            sys.stderr.flush()

        handlers = ExecutionHandlersSync(on_stdout=on_stdout, on_stderr=on_stderr)
        execution = interpreter.codes.run(
            code, language=language, handlers=handlers, **kwargs
        )

        if execution.error:
            obj.output.error(
                f"{execution.error.name}: {execution.error.value}"
            )
            sys.exit(1)
    finally:
        sandbox.close()


# ---- context group --------------------------------------------------------

@code_group.group("context", invoke_without_command=True)
@click.pass_context
def context_group(ctx: click.Context) -> None:
    """Manage code execution contexts."""
    if ctx.invoked_subcommand is None:
        click.echo(ctx.get_help())


@context_group.command("create")
@click.argument("sandbox_id")
@click.option("--language", "-l", required=True, help="Language for the context.")
@click.pass_obj
@handle_errors
def context_create(obj: ClientContext, sandbox_id: str, language: str) -> None:
    """Create a new code execution context."""
    from code_interpreter.sync.code_interpreter import CodeInterpreterSync

    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        interpreter = CodeInterpreterSync.create(sandbox)
        ctx = interpreter.codes.create_context(language)
        obj.output.success_panel(
            {"context_id": ctx.id, "language": language},
            title="Context Created",
        )
    finally:
        sandbox.close()


@context_group.command("list")
@click.argument("sandbox_id")
@click.option("--language", "-l", required=True, help="Language to list contexts for.")
@click.pass_obj
@handle_errors
def context_list(obj: ClientContext, sandbox_id: str, language: str) -> None:
    """List code execution contexts."""
    from code_interpreter.sync.code_interpreter import CodeInterpreterSync

    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        interpreter = CodeInterpreterSync.create(sandbox)
        contexts = interpreter.codes.list_contexts(language)
        for ctx in contexts:
            click.echo(f"{ctx.id}")
    finally:
        sandbox.close()


@context_group.command("delete")
@click.argument("sandbox_id")
@click.argument("context_id")
@click.pass_obj
@handle_errors
def context_delete(obj: ClientContext, sandbox_id: str, context_id: str) -> None:
    """Delete a code execution context."""
    from code_interpreter.sync.code_interpreter import CodeInterpreterSync

    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        interpreter = CodeInterpreterSync.create(sandbox)
        interpreter.codes.delete_context(context_id)
        obj.output.success(f"Deleted context: {context_id}")
    finally:
        sandbox.close()


@context_group.command("delete-all")
@click.argument("sandbox_id")
@click.option("--language", "-l", required=True, help="Language to delete all contexts for.")
@click.pass_obj
@handle_errors
def context_delete_all(obj: ClientContext, sandbox_id: str, language: str) -> None:
    """Delete all code execution contexts for a language."""
    from code_interpreter.sync.code_interpreter import CodeInterpreterSync

    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        interpreter = CodeInterpreterSync.create(sandbox)
        interpreter.codes.delete_contexts(language)
        obj.output.success(f"Deleted all {language} contexts")
    finally:
        sandbox.close()


# ---- interrupt ------------------------------------------------------------

@code_group.command("interrupt")
@click.argument("sandbox_id")
@click.argument("execution_id")
@click.pass_obj
@handle_errors
def code_interrupt(obj: ClientContext, sandbox_id: str, execution_id: str) -> None:
    """Interrupt a running code execution."""
    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        sandbox.commands.interrupt(execution_id)
        obj.output.success(f"Interrupted: {execution_id}")
    finally:
        sandbox.close()
