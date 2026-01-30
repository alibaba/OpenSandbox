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

from __future__ import annotations

import asyncio
import base64
from dataclasses import dataclass, field
from datetime import timedelta
from typing import Any

from mcp.server.fastmcp import Context, FastMCP
from mcp.server.session import ServerSession
from opensandbox import Sandbox, SandboxManager
from opensandbox import __version__ as opensandbox_version
from opensandbox.config import ConnectionConfig
from opensandbox.models.execd import Execution, ExecutionHandlers, RunCommandOpts
from opensandbox.models.filesystem import (
    ContentReplaceEntry,
    EntryInfo,
    MoveEntry,
    SearchEntry,
    SetPermissionEntry,
    WriteEntry,
)
from opensandbox.models.sandboxes import (
    NetworkPolicy,
    PagedSandboxInfos,
    SandboxEndpoint,
    SandboxFilter,
    SandboxImageAuth,
    SandboxImageSpec,
    SandboxInfo,
    SandboxMetrics,
    SandboxRenewResponse,
)
from pydantic import BaseModel, Field, model_validator


@dataclass
class ServerState:
    sandboxes: dict[str, Sandbox] = field(default_factory=dict)
    connection_config: ConnectionConfig = field(default_factory=ConnectionConfig)
    lock: asyncio.Lock = field(default_factory=asyncio.Lock)

    async def add(self, sandbox: Sandbox) -> None:
        async with self.lock:
            self.sandboxes[sandbox.id] = sandbox

    async def get(self, sandbox_id: str) -> Sandbox | None:
        async with self.lock:
            return self.sandboxes.get(sandbox_id)

    async def remove(self, sandbox_id: str) -> Sandbox | None:
        async with self.lock:
            return self.sandboxes.pop(sandbox_id, None)


class StatusResponse(BaseModel):
    status: str = Field(description="Operation status string.")


class WriteFileEntryInput(BaseModel):
    path: str = Field(description="Destination file path.")
    content: str | None = Field(
        default=None, description="Text content to write (utf-8 by default)."
    )
    content_base64: str | None = Field(
        default=None, description="Binary content encoded as base64."
    )
    mode: int = Field(default=755, description="Unix file permissions.")
    owner: str | None = Field(default=None, description="Owner username.")
    group: str | None = Field(default=None, description="Group name.")
    encoding: str | None = Field(default="utf-8", description="Text encoding.")

    @model_validator(mode="after")
    def validate_content(self) -> WriteFileEntryInput:
        has_text = self.content is not None
        has_base64 = self.content_base64 is not None
        if has_text == has_base64:
            raise ValueError("Provide exactly one of content or content_base64.")
        return self


class DirectoryEntryInput(BaseModel):
    path: str = Field(description="Directory path.")
    mode: int = Field(default=755, description="Unix permissions for the directory.")
    owner: str | None = Field(default=None, description="Owner username.")
    group: str | None = Field(default=None, description="Group name.")


class MoveEntryInput(BaseModel):
    source: str = Field(description="Source path.")
    destination: str = Field(description="Destination path.")


class SetPermissionEntryInput(BaseModel):
    path: str = Field(description="Target path.")
    mode: int | None = Field(default=None, description="Unix permissions.")
    owner: str | None = Field(default=None, description="Owner username.")
    group: str | None = Field(default=None, description="Group name.")


class ContentReplaceEntryInput(BaseModel):
    path: str = Field(description="Target file path.")
    old_content: str = Field(description="Text to replace.")
    new_content: str = Field(description="Replacement text.")


class SandboxInfoResponse(BaseModel):
    sandbox_id: str = Field(description="Sandbox identifier.")
    info: SandboxInfo = Field(description="Sandbox info payload.")


class SandboxHealthResponse(BaseModel):
    sandbox_id: str = Field(description="Sandbox identifier.")
    healthy: bool = Field(description="Sandbox health status.")


class FileReadTextResponse(BaseModel):
    path: str = Field(description="File path.")
    content: str = Field(description="File content.")


class FileReadBytesResponse(BaseModel):
    path: str = Field(description="File path.")
    content_base64: str = Field(description="Base64-encoded file content.")


def register_tools(
    mcp: FastMCP,
    *,
    prefix: str = "",
    state: ServerState | None = None,
    connection_config: ConnectionConfig | None = None,
) -> ServerState:
    """Register sandbox tools on a FastMCP instance."""
    config = (connection_config or ConnectionConfig()).with_transport_if_missing()
    state = state or ServerState(connection_config=config)
    name_prefix = f"{prefix}_" if prefix else ""

    def tool():
        def decorator(func):
            if name_prefix:
                func.__name__ = f"{name_prefix}{func.__name__}"
            return mcp.tool()(func)

        return decorator

    async def _get_or_connect_sandbox(
        sandbox_id: str,
        *,
        connect_if_missing: bool,
    ) -> Sandbox:
        sandbox = await state.get(sandbox_id)
        if sandbox is not None:
            return sandbox
        if not connect_if_missing:
            raise ValueError(
                "Sandbox not found in local registry. Call sandbox_connect or "
                "set connect_if_missing=True with connection parameters."
            )
        sandbox = await Sandbox.connect(
            sandbox_id, connection_config=state.connection_config
        )
        await state.add(sandbox)
        return sandbox

    @tool()
    def server_healthcheck() -> str:
        """Return a simple health signal for monitoring.

        Returns:
            "ok" when the MCP server is running.
        """
        return "ok"

    @tool()
    def server_sdk_version() -> str:
        """Return the underlying OpenSandbox SDK version.

        Returns:
            Version string (e.g., "0.6.2").
        """
        return opensandbox_version

    @tool()
    async def sandbox_create(
        image: str,
        ctx: Context[ServerSession, None] | None = None,
        *,
        auth_username: str | None = None,
        auth_password: str | None = None,
        timeout_seconds: float = 600,
        ready_timeout_seconds: float = 30,
        health_check_polling_interval_ms: int = 200,
        skip_health_check: bool = False,
        env: dict[str, str] | None = None,
        metadata: dict[str, str] | None = None,
        resource: dict[str, str] | None = None,
        network_policy: NetworkPolicy | None = None,
        extensions: dict[str, str] | None = None,
        entrypoint: list[str] | None = None,
    ) -> SandboxInfoResponse:
        """Create a sandbox and store it in the MCP server session.

        This allocates a new sandbox instance using the OpenSandbox API and
        tracks it locally so subsequent tool calls can reuse it.

        Parameters:
            image: Container image reference (e.g., "python:3.11").
            ctx: MCP context for progress reporting (optional).
            auth_username: Registry username for private images.
            auth_password: Registry password/token for private images.
            timeout_seconds: Sandbox lifetime in seconds (absolute TTL).
            ready_timeout_seconds: Max time to wait for readiness checks.
            health_check_polling_interval_ms: Interval between health checks in ms.
            skip_health_check: If True, return before readiness checks complete.
            env: Environment variables for the sandbox.
            metadata: Custom metadata for the sandbox (string map).
            resource: Resource limits (cpu/memory/etc.) as string map.
            network_policy: Optional egress network policy (NetworkPolicy model).
                Example: NetworkPolicy(
                    default_action="deny",
                    egress=[{"action": "allow", "target": "pypi.org"}],
                )
            extensions: Opaque extension parameters passed through to the server.
            entrypoint: Entrypoint command list.

        Returns:
            A dict with:
                sandbox_id: The new sandbox identifier.
                info: Sandbox info payload from the SDK.

        Raises:
            ValueError: If auth_username/auth_password are incomplete.
            Exception: If sandbox creation fails.

        Example:
            result = await sandbox_create(
                image="python:3.11",
                env={"PYTHONPATH": "/app"},
                resource={"cpu": "1", "memory": "2Gi"},
            )
        """
        if ctx:
            await ctx.report_progress(progress=0.1, total=1.0, message="Validating input")
        image_auth = None
        if auth_username or auth_password:
            if not auth_username or not auth_password:
                raise ValueError("auth_username and auth_password must be provided together")
            image_auth = SandboxImageAuth(
                username=auth_username,
                password=auth_password,
            )
        image_spec = SandboxImageSpec(image=image, auth=image_auth)
        if ctx:
            await ctx.report_progress(
                progress=0.3, total=1.0, message="Creating sandbox"
            )
        sandbox = await Sandbox.create(
            image_spec,
            timeout=timedelta(seconds=timeout_seconds),
            ready_timeout=timedelta(seconds=ready_timeout_seconds),
            env=env,
            metadata=metadata,
            resource=resource,
            network_policy=network_policy,
            extensions=extensions,
            entrypoint=entrypoint,
            health_check_polling_interval=timedelta(
                milliseconds=health_check_polling_interval_ms
            ),
            skip_health_check=skip_health_check,
            connection_config=state.connection_config,
        )
        await state.add(sandbox)
        if ctx:
            await ctx.report_progress(
                progress=0.8, total=1.0, message="Fetching sandbox info"
            )
        info = await sandbox.get_info()
        if ctx:
            await ctx.report_progress(progress=1.0, total=1.0, message="Done")
        return SandboxInfoResponse(sandbox_id=sandbox.id, info=info)

    @tool()
    async def sandbox_connect(
        sandbox_id: str,
        *,
        connect_timeout_seconds: float = 30,
        health_check_polling_interval_ms: int = 200,
        skip_health_check: bool = False,
    ) -> SandboxInfoResponse:
        """Connect to an existing sandbox and store it locally.

        Use this when a sandbox already exists and you want to use it in this
        MCP server session without creating a new one.

        Parameters:
            sandbox_id: Existing sandbox identifier.
            connect_timeout_seconds: Max time to wait for readiness.
            health_check_polling_interval_ms: Interval between health checks in ms.
            skip_health_check: If True, return before readiness checks complete.

        Returns:
            A dict with:
                sandbox_id: The sandbox identifier.
                info: Sandbox info payload from the SDK.

        Example:
            result = await sandbox_connect(sandbox_id="sbx_123")
        """
        sandbox = await Sandbox.connect(
            sandbox_id,
            connection_config=state.connection_config,
            connect_timeout=timedelta(seconds=connect_timeout_seconds),
            health_check_polling_interval=timedelta(
                milliseconds=health_check_polling_interval_ms
            ),
            skip_health_check=skip_health_check,
        )
        await state.add(sandbox)
        info = await sandbox.get_info()
        return SandboxInfoResponse(sandbox_id=sandbox.id, info=info)

    @tool()
    async def sandbox_resume(
        sandbox_id: str,
        *,
        resume_timeout_seconds: float = 30,
        health_check_polling_interval_ms: int = 200,
        skip_health_check: bool = False,
    ) -> SandboxInfoResponse:
        """Resume a paused sandbox and store it locally.

        Parameters:
            sandbox_id: Paused sandbox identifier.
            resume_timeout_seconds: Max time to wait for readiness.
            health_check_polling_interval_ms: Interval between health checks in ms.
            skip_health_check: If True, return before readiness checks complete.

        Returns:
            A dict with:
                sandbox_id: The sandbox identifier.
                info: Sandbox info payload from the SDK.
        """
        sandbox = await Sandbox.resume(
            sandbox_id,
            connection_config=state.connection_config,
            resume_timeout=timedelta(seconds=resume_timeout_seconds),
            health_check_polling_interval=timedelta(
                milliseconds=health_check_polling_interval_ms
            ),
            skip_health_check=skip_health_check,
        )
        await state.add(sandbox)
        info = await sandbox.get_info()
        return SandboxInfoResponse(sandbox_id=sandbox.id, info=info)

    @tool()
    async def sandbox_pause(
        sandbox_id: str,
    ) -> StatusResponse:
        """Pause a sandbox by ID.

        Parameters:
            sandbox_id: Target sandbox identifier.

        Returns:
            {"status": "paused"} when successful.
        """
        sandbox = await state.get(sandbox_id)
        if sandbox is None:
            manager = await SandboxManager.create(
                connection_config=state.connection_config
            )
            try:
                await manager.pause_sandbox(sandbox_id)
            finally:
                await manager.close()
        else:
            await sandbox.pause()
        return StatusResponse(status="paused")

    @tool()
    async def sandbox_kill(
        sandbox_id: str,
    ) -> StatusResponse:
        """Terminate a sandbox by ID and remove it from local registry.

        Parameters:
            sandbox_id: Target sandbox identifier.

        Returns:
            {"status": "killed"} when successful.
        """
        sandbox = await state.remove(sandbox_id)
        if sandbox is None:
            manager = await SandboxManager.create(
                connection_config=state.connection_config
            )
            try:
                await manager.kill_sandbox(sandbox_id)
            finally:
                await manager.close()
        else:
            await sandbox.kill()
            await sandbox.close()
        return StatusResponse(status="killed")

    @tool()
    async def sandbox_close(sandbox_id: str) -> StatusResponse:
        """Close local resources for a connected sandbox.

        This does NOT terminate the remote sandbox. Use sandbox_kill for that.

        Parameters:
            sandbox_id: Target sandbox identifier.

        Returns:
            {"status": "closed"} if removed, or {"status": "not_found"}.
        """
        sandbox = await state.remove(sandbox_id)
        if sandbox is None:
            return StatusResponse(status="not_found")
        await sandbox.close()
        return StatusResponse(status="closed")

    @tool()
    async def sandbox_get_info(
        sandbox_id: str,
    ) -> SandboxInfo:
        """Fetch sandbox info by ID.

        Parameters:
            sandbox_id: Target sandbox identifier.

        Returns:
            Sandbox info dict from the SDK.
        """
        sandbox = await state.get(sandbox_id)
        if sandbox is not None:
            return await sandbox.get_info()
        manager = await SandboxManager.create(
            connection_config=state.connection_config
        )
        try:
            info = await manager.get_sandbox_info(sandbox_id)
        finally:
            await manager.close()
        return info

    @tool()
    async def sandbox_list(
        ctx: Context[ServerSession, None] | None = None,
        *,
        filter: SandboxFilter | None = None,
    ) -> PagedSandboxInfos:
        """List sandboxes matching filter criteria.

        Parameters:
            ctx: MCP context for progress reporting (optional).
            filter: SandboxFilter object (states, metadata, page, page_size).

        Returns:
            Paginated sandbox list.
        """
        if ctx:
            await ctx.report_progress(progress=0.1, total=1.0, message="Listing sandboxes")
        filter = filter or SandboxFilter()
        manager = await SandboxManager.create(
            connection_config=state.connection_config
        )
        try:
            result = await manager.list_sandbox_infos(
                filter
            )
        finally:
            await manager.close()
        if ctx:
            await ctx.report_progress(progress=1.0, total=1.0, message="Done")
        return result

    @tool()
    async def sandbox_renew(
        sandbox_id: str,
        *,
        timeout_seconds: float,
    ) -> SandboxRenewResponse:
        """Renew sandbox expiration time.

        Parameters:
            sandbox_id: Target sandbox identifier.
            timeout_seconds: Additional lifetime in seconds.

        Returns:
            Renew response dict including new expiration time.
        """
        sandbox = await state.get(sandbox_id)
        if sandbox is None:
            manager = await SandboxManager.create(
                connection_config=state.connection_config
            )
            try:
                response = await manager.renew_sandbox(
                    sandbox_id, timedelta(seconds=timeout_seconds)
                )
            finally:
                await manager.close()
        else:
            response = await sandbox.renew(timedelta(seconds=timeout_seconds))
        return response

    @tool()
    async def command_run(
        sandbox_id: str,
        command: str,
        *,
        background: bool = False,
        working_directory: str | None = None,
        connect_if_missing: bool = False,
    ) -> Execution:
        """Run a command inside a sandbox.
        Parameters:
            sandbox_id: Target sandbox identifier.
            command: Shell command to execute (supports pipes/redirects).
            background: If True, run asynchronously and return immediately.
            working_directory: Working directory for the command.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            Execution result dict with id, exit_code, logs, and duration.

        Example:
            result = await command_run("sbx_123", "ls -la", working_directory="/")
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        opts = RunCommandOpts(
            background=background,
            working_directory=working_directory,
        )
        execution = await sandbox.commands.run(command, opts=opts)
        return execution

    @tool()
    async def command_run_stream(
        sandbox_id: str,
        command: str,
        ctx: Context[ServerSession, None],
        *,
        background: bool = False,
        working_directory: str | None = None,
        connect_if_missing: bool = False,
    ) -> Execution:
        """Run a command and stream execution logs via MCP notifications.

        Parameters:
            sandbox_id: Target sandbox identifier.
            command: Shell command to execute.
            ctx: MCP context used for streaming logs.
            background: If True, run asynchronously and return immediately.
            working_directory: Working directory for the command.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            Execution result dict with id, exit_code, logs, and duration.

        Notes:
            - Streams stdout/stderr as MCP log messages.
            - Sends start/end events via ctx.info.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        opts = RunCommandOpts(
            background=background,
            working_directory=working_directory,
        )

        async def on_stdout(message: Any) -> None:
            text = getattr(message, "text", None)
            if text:
                await ctx.info(text)

        async def on_stderr(message: Any) -> None:
            text = getattr(message, "text", None)
            if text:
                await ctx.error(text)

        async def on_result(message: Any) -> None:
            text = getattr(message, "text", None)
            if text:
                await ctx.debug(text)

        async def on_init(message: Any) -> None:
            execution_id = getattr(message, "id", None)
            if execution_id:
                await ctx.info(f"Execution started: {execution_id}")

        async def on_complete(message: Any) -> None:
            duration = getattr(message, "execution_time_in_millis", None)
            if duration is not None:
                await ctx.info(f"Execution completed in {duration}ms")

        async def on_error(message: Any) -> None:
            name = getattr(message, "name", None)
            value = getattr(message, "value", None)
            if name or value:
                await ctx.error(f"{name or 'ExecutionError'}: {value or ''}".strip())

        handlers = ExecutionHandlers(
            on_stdout=on_stdout,
            on_stderr=on_stderr,
            on_result=on_result,
            on_init=on_init,
            on_execution_complete=on_complete,
            on_error=on_error,
        )
        execution = await sandbox.commands.run(command, opts=opts, handlers=handlers)
        return execution

    @tool()
    async def command_interrupt(
        sandbox_id: str,
        execution_id: str,
        *,
        connect_if_missing: bool = False,
    ) -> StatusResponse:
        """Interrupt a running command execution.

        Parameters:
            sandbox_id: Target sandbox identifier.
            execution_id: Execution identifier to interrupt.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"status": "interrupted"} when successful.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        await sandbox.commands.interrupt(execution_id)
        return StatusResponse(status="interrupted")

    @tool()
    async def file_read_text(
        sandbox_id: str,
        path: str,
        *,
        encoding: str = "utf-8",
        range_header: str | None = None,
        connect_if_missing: bool = False,
    ) -> FileReadTextResponse:
        """Read a text file from the sandbox.

        Parameters:
            sandbox_id: Target sandbox identifier.
            path: File path to read.
            encoding: Text encoding.
            range_header: Optional byte range header (e.g., "bytes=0-1023").
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"path": "...", "content": "..."}.

        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        content = await sandbox.files.read_file(
            path, encoding=encoding, range_header=range_header
        )
        return FileReadTextResponse(path=path, content=content)

    @tool()
    async def file_read_bytes(
        sandbox_id: str,
        path: str,
        *,
        range_header: str | None = None,
        connect_if_missing: bool = False,
    ) -> FileReadBytesResponse:
        """Read a binary file from the sandbox (base64 encoded).

        Parameters:
            sandbox_id: Target sandbox identifier.
            path: File path to read.
            range_header: Optional byte range header.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"path": "...", "content_base64": "..."}.

        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        content = await sandbox.files.read_bytes(path, range_header=range_header)
        return FileReadBytesResponse(
            path=path,
            content_base64=base64.b64encode(content).decode("ascii"),
        )

    @tool()
    async def file_write_text(
        sandbox_id: str,
        path: str,
        content: str,
        *,
        encoding: str = "utf-8",
        mode: int = 755,
        owner: str | None = None,
        group: str | None = None,
        connect_if_missing: bool = False,
    ) -> StatusResponse:
        """Write a text file inside the sandbox.

        Parameters:
            sandbox_id: Target sandbox identifier.
            path: Destination file path.
            content: File content.
            encoding: Text encoding.
            mode: Unix file permissions.
            owner: Owner username.
            group: Group name.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"status": "written"} when successful.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        await sandbox.files.write_file(
            path,
            content,
            encoding=encoding,
            mode=mode,
            owner=owner,
            group=group,
        )
        return StatusResponse(status="written")

    @tool()
    async def file_write_bytes(
        sandbox_id: str,
        path: str,
        content_base64: str,
        *,
        mode: int = 755,
        owner: str | None = None,
        group: str | None = None,
        connect_if_missing: bool = False,
    ) -> StatusResponse:
        """Write a binary file inside the sandbox (base64 input).

        Parameters:
            sandbox_id: Target sandbox identifier.
            path: Destination file path.
            content_base64: Base64-encoded file content.
            mode: Unix file permissions.
            owner: Owner username.
            group: Group name.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"status": "written"} when successful.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        data = base64.b64decode(content_base64.encode("ascii"))
        await sandbox.files.write_file(
            path,
            data,
            mode=mode,
            owner=owner,
            group=group,
        )
        return StatusResponse(status="written")

    @tool()
    async def file_write_files(
        sandbox_id: str,
        entries: list[WriteFileEntryInput],
        *,
        connect_if_missing: bool = False,
    ) -> StatusResponse:
        """Batch write files in a single request.

        Parameters:
            sandbox_id: Target sandbox identifier.
            entries: List of write entries. Each entry supports:
                path (str, required)
                content (str) or content_base64 (str)
                mode (int), owner (str), group (str), encoding (str)
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"status": "written"} when successful.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        write_entries: list[WriteEntry] = []
        for entry in entries:
            entry_dict = entry.model_dump(exclude_none=True)
            if "content_base64" in entry_dict:
                entry_dict["data"] = base64.b64decode(
                    str(entry_dict.pop("content_base64")).encode("ascii")
                )
            else:
                entry_dict["data"] = entry_dict.pop("content")
            write_entries.append(WriteEntry(**entry_dict))
        await sandbox.files.write_files(write_entries)
        return StatusResponse(status="written")

    @tool()
    async def file_delete(
        sandbox_id: str,
        paths: list[str],
        *,
        connect_if_missing: bool = False,
    ) -> StatusResponse:
        """Delete files inside the sandbox.

        Parameters:
            sandbox_id: Target sandbox identifier.
            paths: File paths to delete.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"status": "deleted"} when successful.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        await sandbox.files.delete_files(paths)
        return StatusResponse(status="deleted")

    @tool()
    async def file_get_info(
        sandbox_id: str,
        paths: list[str],
        *,
        connect_if_missing: bool = False,
    ) -> dict[str, EntryInfo]:
        """Get file metadata for paths.

        Parameters:
            sandbox_id: Target sandbox identifier.
            paths: File or directory paths.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            Dict mapping path -> entry info.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        info = await sandbox.files.get_file_info(paths)
        return info

    @tool()
    async def file_search(
        sandbox_id: str,
        path: str,
        pattern: str,
        *,
        connect_if_missing: bool = False,
    ) -> list[EntryInfo]:
        """Search for files matching a pattern.

        Parameters:
            sandbox_id: Target sandbox identifier.
            path: Base directory to search.
            pattern: Glob pattern (e.g., "*.py").
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            List of entry info objects.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        results = await sandbox.files.search(SearchEntry(path=path, pattern=pattern))
        return results

    @tool()
    async def file_create_directories(
        sandbox_id: str,
        entries: list[DirectoryEntryInput],
        *,
        connect_if_missing: bool = False,
    ) -> StatusResponse:
        """Create directories inside the sandbox.

        Parameters:
            sandbox_id: Target sandbox identifier.
            entries: List of directory entries (path, mode, owner, group).
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"status": "created"} when successful.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        write_entries = [
            WriteEntry(**entry.model_dump(exclude_none=True)) for entry in entries
        ]
        await sandbox.files.create_directories(write_entries)
        return StatusResponse(status="created")

    @tool()
    async def file_delete_directories(
        sandbox_id: str,
        paths: list[str],
        *,
        connect_if_missing: bool = False,
    ) -> StatusResponse:
        """Delete directories inside the sandbox.

        Parameters:
            sandbox_id: Target sandbox identifier.
            paths: Directory paths to delete.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"status": "deleted"} when successful.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        await sandbox.files.delete_directories(paths)
        return StatusResponse(status="deleted")

    @tool()
    async def file_move(
        sandbox_id: str,
        entries: list[MoveEntryInput],
        *,
        connect_if_missing: bool = False,
    ) -> StatusResponse:
        """Move or rename files/directories inside the sandbox.

        Parameters:
            sandbox_id: Target sandbox identifier.
            entries: List of move entries (source, destination).
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"status": "moved"} when successful.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        move_entries = [
            MoveEntry(**entry.model_dump(exclude_none=True)) for entry in entries
        ]
        await sandbox.files.move_files(move_entries)
        return StatusResponse(status="moved")

    @tool()
    async def file_set_permissions(
        sandbox_id: str,
        entries: list[SetPermissionEntryInput],
        *,
        connect_if_missing: bool = False,
    ) -> StatusResponse:
        """Set permissions/ownership for files/directories.

        Parameters:
            sandbox_id: Target sandbox identifier.
            entries: List of permission entries (path, mode, owner, group).
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"status": "updated"} when successful.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        permission_entries = [
            SetPermissionEntry(**entry.model_dump(exclude_none=True))
            for entry in entries
        ]
        await sandbox.files.set_permissions(permission_entries)
        return StatusResponse(status="updated")

    @tool()
    async def file_replace_contents(
        sandbox_id: str,
        entries: list[ContentReplaceEntryInput],
        *,
        connect_if_missing: bool = False,
    ) -> StatusResponse:
        """Replace content inside files.

        Parameters:
            sandbox_id: Target sandbox identifier.
            entries: List of replace entries (path, old_content, new_content).
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"status": "updated"} when successful.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        replace_entries = [
            ContentReplaceEntry(**entry.model_dump(exclude_none=True))
            for entry in entries
        ]
        await sandbox.files.replace_contents(replace_entries)
        return StatusResponse(status="updated")

    @tool()
    async def sandbox_get_metrics(
        sandbox_id: str,
        *,
        connect_if_missing: bool = False,
    ) -> SandboxMetrics:
        """Get resource metrics for a sandbox.

        Parameters:
            sandbox_id: Target sandbox identifier.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            Metrics dict.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        metrics = await sandbox.get_metrics()
        return metrics

    @tool()
    async def sandbox_get_endpoint(
        sandbox_id: str,
        port: int,
        *,
        connect_if_missing: bool = False,
    ) -> SandboxEndpoint:
        """Get a sandbox network endpoint for a specific port.

        Parameters:
            sandbox_id: Target sandbox identifier.
            port: Port number inside the sandbox.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            Endpoint info dict.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        endpoint = await sandbox.get_endpoint(port)
        return endpoint

    @tool()
    async def sandbox_healthcheck(
        sandbox_id: str,
        *,
        connect_if_missing: bool = False,
    ) -> SandboxHealthResponse:
        """Check if a sandbox is healthy.

        Parameters:
            sandbox_id: Target sandbox identifier.
            connect_if_missing: Connect if sandbox not in local registry.

        Returns:
            {"sandbox_id": "...", "healthy": true|false}.
        """
        sandbox = await _get_or_connect_sandbox(
            sandbox_id,
            connect_if_missing=connect_if_missing,
        )
        healthy = await sandbox.is_healthy()
        return SandboxHealthResponse(sandbox_id=sandbox_id, healthy=healthy)

    return state


def create_server(connection_config: ConnectionConfig | None = None) -> FastMCP:
    """Create the MCP server instance for OpenSandbox."""
    mcp = FastMCP(
        "OpenSandbox Sandbox",
        instructions=(
            "Use these tools to create and manage isolated sandboxes. "
            "Always keep track of the sandbox_id returned by sandbox_create/connect. "
            "Use command_run for non-streaming execution and command_run_stream when "
            "you need incremental logs. Call sandbox_kill to terminate remote sandboxes "
            "and sandbox_close to release local resources. For large files, prefer "
            "range reads until streaming is implemented."
        ),
    )
    register_tools(mcp, connection_config=connection_config)
    return mcp
