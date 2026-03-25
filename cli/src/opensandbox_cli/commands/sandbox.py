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
    """📦 Manage sandbox lifecycle."""
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

    with obj.output.spinner("Creating sandbox..."):
        sandbox = SandboxSync.create(image, **kwargs)
    obj.output.success_panel(
        {"id": sandbox.id, "image": image, "status": "created"},
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
    with obj.output.spinner("Fetching sandboxes..."):
        result = mgr.list_sandbox_infos(filt)
    if not result.sandbox_infos:
        if obj.output.fmt in ("json", "yaml"):
            obj.output.print_rows(
                [], columns=["id", "status", "image", "created_at", "expires_at"],
                title="Sandboxes",
            )
        else:
            obj.output.info("No sandboxes found.")
        return

    raw_rows = [info.model_dump(mode="json") for info in result.sandbox_infos]

    # For machine-readable formats, preserve the original structure
    if obj.output.fmt in ("json", "yaml"):
        obj.output.print_rows(
            raw_rows,
            columns=["id", "status", "image", "created_at", "expires_at"],
            title="Sandboxes",
        )
        return

    # Flatten nested status/image objects for clean table display
    rows = []
    for d in raw_rows:
        flat = dict(d)
        status_val = flat.get("status")
        if isinstance(status_val, dict):
            flat["status"] = status_val.get("state", str(status_val))
        image_val = flat.get("image")
        if isinstance(image_val, dict):
            flat["image"] = image_val.get("image", str(image_val))
        rows.append(flat)

    obj.output.print_rows(
        rows,
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
    sandbox_id = obj.resolve_sandbox_id(sandbox_id)
    mgr = obj.get_manager()
    info = mgr.get_sandbox_info(sandbox_id)
    d = info.model_dump(mode="json")

    # For machine-readable formats, preserve the original structure
    if obj.output.fmt in ("json", "yaml"):
        obj.output.print_dict(d, title="Sandbox Info")
        return

    # Flatten nested objects for clean table display
    status_val = d.get("status")
    if isinstance(status_val, dict):
        d["status"] = status_val.get("state", str(status_val))
        if status_val.get("reason"):
            d["status_reason"] = status_val["reason"]
        if status_val.get("message"):
            d["status_message"] = status_val["message"]
    image_val = d.get("image")
    if isinstance(image_val, dict):
        d["image"] = image_val.get("image", str(image_val))
    obj.output.print_dict(d, title="Sandbox Info")


# ---- kill -----------------------------------------------------------------

@sandbox_group.command("kill")
@click.argument("sandbox_ids", nargs=-1, required=True)
@click.pass_obj
@handle_errors
def sandbox_kill(obj: ClientContext, sandbox_ids: tuple[str, ...]) -> None:
    """Terminate one or more sandboxes."""
    mgr = obj.get_manager()
    for sid in sandbox_ids:
        resolved = obj.resolve_sandbox_id(sid)
        with obj.output.spinner(f"Killing sandbox {resolved}..."):
            mgr.kill_sandbox(resolved)
        obj.output.success(f"Sandbox terminated: {resolved}")


# ---- pause ----------------------------------------------------------------

@sandbox_group.command("pause")
@click.argument("sandbox_id")
@click.pass_obj
@handle_errors
def sandbox_pause(obj: ClientContext, sandbox_id: str) -> None:
    """Pause a running sandbox."""
    sandbox_id = obj.resolve_sandbox_id(sandbox_id)
    mgr = obj.get_manager()
    with obj.output.spinner("Pausing sandbox..."):
        mgr.pause_sandbox(sandbox_id)
    obj.output.success(f"Sandbox paused: {sandbox_id}")


# ---- resume ---------------------------------------------------------------

@sandbox_group.command("resume")
@click.argument("sandbox_id")
@click.pass_obj
@handle_errors
def sandbox_resume(obj: ClientContext, sandbox_id: str) -> None:
    """Resume a paused sandbox."""
    sandbox_id = obj.resolve_sandbox_id(sandbox_id)
    mgr = obj.get_manager()
    with obj.output.spinner("Resuming sandbox..."):
        mgr.resume_sandbox(sandbox_id)
    obj.output.success(f"Sandbox resumed: {sandbox_id}")


# ---- renew ----------------------------------------------------------------

@sandbox_group.command("renew")
@click.argument("sandbox_id")
@click.option("--timeout", "-t", required=True, type=DURATION, help="New TTL duration (e.g. 30m, 2h).")
@click.pass_obj
@handle_errors
def sandbox_renew(obj: ClientContext, sandbox_id: str, timeout: timedelta) -> None:
    """Renew sandbox expiration."""
    sandbox_id = obj.resolve_sandbox_id(sandbox_id)
    mgr = obj.get_manager()
    with obj.output.spinner("Renewing sandbox..."):
        resp = mgr.renew_sandbox(sandbox_id, timeout)
    obj.output.success_panel(
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
        if obj.output.fmt == "table":
            if healthy:
                obj.output.success(f"Sandbox {sandbox_id} is healthy")
            else:
                obj.output.error(f"Sandbox {sandbox_id} is unhealthy")
        else:
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
