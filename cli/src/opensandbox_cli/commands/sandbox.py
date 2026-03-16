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

"""Sandbox lifecycle commands: create, list, get, kill, pause, resume, renew, endpoint, health, metrics."""

from __future__ import annotations

import json
from datetime import timedelta

import click

from opensandbox.models.sandboxes import NetworkPolicy, SandboxFilter

from opensandbox_cli.client import ClientContext
from opensandbox_cli.utils import DURATION, KEY_VALUE, handle_errors


@click.group("sandbox", invoke_without_command=True)
@click.pass_context
def sandbox_group(ctx: click.Context) -> None:
    """Manage sandbox lifecycle."""
    if ctx.invoked_subcommand is None:
        click.echo(ctx.get_help())


# Alias: osb sb ...
sandbox_group.name = "sandbox"


# ---- create ---------------------------------------------------------------

@sandbox_group.command("create")
@click.option("--image", "-i", required=True, help="Container image (e.g. python:3.11).")
@click.option("--timeout", "-t", "timeout", type=DURATION, default=None, help="Sandbox lifetime (e.g. 10m, 1h).")
@click.option("--env", "-e", "envs", multiple=True, type=KEY_VALUE, help="Environment variable (KEY=VALUE). Repeatable.")
@click.option("--metadata", "-m", "metadata_kv", multiple=True, type=KEY_VALUE, help="Metadata (KEY=VALUE). Repeatable.")
@click.option("--resource", "resources_kv", multiple=True, type=KEY_VALUE, help="Resource limit (e.g. cpu=1 memory=2Gi). Repeatable.")
@click.option("--entrypoint", default=None, help="Entrypoint command (JSON array or shell string).")
@click.option("--network-policy-file", type=click.Path(exists=True), default=None, help="Network policy JSON file.")
@click.option("--skip-health-check", is_flag=True, default=False, help="Skip waiting for sandbox readiness.")
@click.option("--ready-timeout", type=DURATION, default=None, help="Max wait time for sandbox readiness (e.g. 30s).")
@click.pass_obj
@handle_errors
def sandbox_create(
    obj: ClientContext,
    image: str,
    timeout: timedelta | None,
    envs: tuple[tuple[str, str], ...],
    metadata_kv: tuple[tuple[str, str], ...],
    resources_kv: tuple[tuple[str, str], ...],
    entrypoint: str | None,
    network_policy_file: str | None,
    skip_health_check: bool,
    ready_timeout: timedelta | None,
) -> None:
    """Create a new sandbox."""
    from opensandbox.sync.sandbox import SandboxSync

    kwargs: dict = {
        "connection_config": obj.connection_config,
        "skip_health_check": skip_health_check,
    }
    if timeout is not None:
        kwargs["timeout"] = timeout
    if ready_timeout is not None:
        kwargs["ready_timeout"] = ready_timeout
    if envs:
        kwargs["env"] = dict(envs)
    if metadata_kv:
        kwargs["metadata"] = dict(metadata_kv)
    if resources_kv:
        kwargs["resource"] = dict(resources_kv)
    if entrypoint:
        try:
            kwargs["entrypoint"] = json.loads(entrypoint)
        except json.JSONDecodeError:
            kwargs["entrypoint"] = ["sh", "-c", entrypoint]
    if network_policy_file:
        with open(network_policy_file) as f:
            kwargs["network_policy"] = NetworkPolicy(**json.load(f))

    sandbox = SandboxSync.create(image, **kwargs)
    obj.output.print_dict(
        {"id": sandbox.id, "status": "created"},
        title="Sandbox Created",
    )


# ---- list -----------------------------------------------------------------

@sandbox_group.command("list")
@click.option("--state", "-s", "states", multiple=True, help="Filter by state (Pending, Running, Paused, ...). Repeatable.")
@click.option("--metadata", "-m", "metadata_kv", multiple=True, type=KEY_VALUE, help="Metadata filter (KEY=VALUE). Repeatable.")
@click.option("--page", type=int, default=None, help="Page number (0-indexed).")
@click.option("--page-size", type=int, default=None, help="Items per page.")
@click.pass_obj
@handle_errors
def sandbox_list(
    obj: ClientContext,
    states: tuple[str, ...],
    metadata_kv: tuple[tuple[str, str], ...],
    page: int | None,
    page_size: int | None,
) -> None:
    """List sandboxes."""
    mgr = obj.get_manager()
    filt = SandboxFilter(
        states=list(states) if states else None,
        metadata=dict(metadata_kv) if metadata_kv else None,
        page=page,
        page_size=page_size,
    )
    result = mgr.list_sandbox_infos(filt)
    obj.output.print_models(
        result.sandbox_infos,
        columns=["id", "status", "image", "created_at", "expires_at"],
        title="Sandboxes",
    )


# ---- get ------------------------------------------------------------------

@sandbox_group.command("get")
@click.argument("sandbox_id")
@click.pass_obj
@handle_errors
def sandbox_get(obj: ClientContext, sandbox_id: str) -> None:
    """Get sandbox details."""
    mgr = obj.get_manager()
    info = mgr.get_sandbox_info(sandbox_id)
    obj.output.print_model(info, title="Sandbox Info")


# ---- kill -----------------------------------------------------------------

@sandbox_group.command("kill")
@click.argument("sandbox_ids", nargs=-1, required=True)
@click.pass_obj
@handle_errors
def sandbox_kill(obj: ClientContext, sandbox_ids: tuple[str, ...]) -> None:
    """Terminate one or more sandboxes."""
    mgr = obj.get_manager()
    for sid in sandbox_ids:
        mgr.kill_sandbox(sid)
        click.echo(f"Killed: {sid}")


# ---- pause ----------------------------------------------------------------

@sandbox_group.command("pause")
@click.argument("sandbox_id")
@click.pass_obj
@handle_errors
def sandbox_pause(obj: ClientContext, sandbox_id: str) -> None:
    """Pause a running sandbox."""
    mgr = obj.get_manager()
    mgr.pause_sandbox(sandbox_id)
    click.echo(f"Paused: {sandbox_id}")


# ---- resume ---------------------------------------------------------------

@sandbox_group.command("resume")
@click.argument("sandbox_id")
@click.pass_obj
@handle_errors
def sandbox_resume(obj: ClientContext, sandbox_id: str) -> None:
    """Resume a paused sandbox."""
    mgr = obj.get_manager()
    mgr.resume_sandbox(sandbox_id)
    click.echo(f"Resumed: {sandbox_id}")


# ---- renew ----------------------------------------------------------------

@sandbox_group.command("renew")
@click.argument("sandbox_id")
@click.option("--timeout", "-t", required=True, type=DURATION, help="New TTL duration (e.g. 30m, 2h).")
@click.pass_obj
@handle_errors
def sandbox_renew(obj: ClientContext, sandbox_id: str, timeout: timedelta) -> None:
    """Renew sandbox expiration."""
    mgr = obj.get_manager()
    resp = mgr.renew_sandbox(sandbox_id, timeout)
    obj.output.print_dict(
        {"sandbox_id": sandbox_id, "expires_at": str(resp.expires_at)},
        title="Sandbox Renewed",
    )


# ---- endpoint -------------------------------------------------------------

@sandbox_group.command("endpoint")
@click.argument("sandbox_id")
@click.option("--port", "-p", required=True, type=int, help="Port number.")
@click.pass_obj
@handle_errors
def sandbox_endpoint(obj: ClientContext, sandbox_id: str, port: int) -> None:
    """Get the public endpoint for a sandbox port."""
    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        ep = sandbox.get_endpoint(port)
        obj.output.print_model(ep, title="Sandbox Endpoint")
    finally:
        sandbox.close()


# ---- health ---------------------------------------------------------------

@sandbox_group.command("health")
@click.argument("sandbox_id")
@click.pass_obj
@handle_errors
def sandbox_health(obj: ClientContext, sandbox_id: str) -> None:
    """Check sandbox health."""
    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        healthy = sandbox.is_healthy()
        obj.output.print_dict(
            {"sandbox_id": sandbox_id, "healthy": healthy},
            title="Health Check",
        )
    finally:
        sandbox.close()


# ---- metrics --------------------------------------------------------------

@sandbox_group.command("metrics")
@click.argument("sandbox_id")
@click.pass_obj
@handle_errors
def sandbox_metrics(obj: ClientContext, sandbox_id: str) -> None:
    """Get sandbox resource metrics."""
    sandbox = obj.connect_sandbox(sandbox_id)
    try:
        m = sandbox.get_metrics()
        obj.output.print_model(m, title="Sandbox Metrics")
    finally:
        sandbox.close()
