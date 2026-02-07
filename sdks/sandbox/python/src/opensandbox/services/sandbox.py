#
# Copyright 2025 Alibaba Group Holding Ltd.
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
#
"""
Sandbox service interface.

Protocol for sandbox lifecycle management operations.
"""

from datetime import datetime, timedelta
from typing import Protocol

from opensandbox.models.sandboxes import (
    NetworkPolicy,
    PagedSandboxInfos,
    SandboxCreateResponse,
    SandboxEndpoint,
    SandboxFilter,
    SandboxImageSpec,
    SandboxInfo,
    SandboxRenewResponse,
    Volume,
)


class Sandboxes(Protocol):
    """
    Core sandbox lifecycle management service.

    This service provides a clean abstraction over sandbox creation, management,
    and termination operations, completely isolating business logic from API implementation details.
    """

    async def create_sandbox(
        self,
        spec: SandboxImageSpec,
        entrypoint: list[str],
        env: dict[str, str],
        metadata: dict[str, str],
        timeout: timedelta,
        resource: dict[str, str],
        network_policy: NetworkPolicy | None,
        extensions: dict[str, str],
        volumes: list[Volume] | None,
    ) -> SandboxCreateResponse:
        """
        Create a new sandbox with the specified configuration.

        Args:
            spec: Container image specification for provisioning the sandbox.
            entrypoint: Command to run as the sandbox's main process.
            env: Environment variables injected into the sandbox runtime.
            metadata: User-defined metadata used for management and filtering.
            timeout: Sandbox lifetime. The server may terminate the sandbox when it expires.
            resource: Runtime resource limits (e.g. cpu/memory). Exact semantics are server-defined.
            network_policy: Optional outbound network policy (egress).
            extensions: Opaque extension parameters passed through to the server as-is.
                Prefer namespaced keys (e.g. ``storage.id``).
            volumes: Optional list of volume mounts for persistent storage.

        Returns:
            Sandbox create response

        Raises:
            SandboxException: if the operation fails
        """
        ...

    async def get_sandbox_info(self, sandbox_id: str) -> SandboxInfo:
        """
        Retrieve information about an existing sandbox.

        Args:
            sandbox_id: Unique identifier of the sandbox

        Returns:
            Current sandbox information

        Raises:
            SandboxException: if the operation fails
        """
        ...

    async def list_sandboxes(self, filter: SandboxFilter) -> PagedSandboxInfos:
        """
        List sandboxes with optional filtering.

        Args:
            filter: Optional filter criteria

        Returns:
            List of sandbox information matching the filter

        Raises:
            SandboxException: if the operation fails
        """
        ...

    async def get_sandbox_endpoint(
        self, sandbox_id: str, port: int, use_server_proxy: bool = False
    ) -> SandboxEndpoint:
        """
        Get sandbox endpoint.

        Args:
            sandbox_id: Sandbox ID
            port: Endpoint port number
            use_server_proxy: Whether to use server proxy for endpoint

        Returns:
            Target sandbox endpoint

        Raises:
            SandboxException: if the operation fails
        """
        ...

    async def pause_sandbox(self, sandbox_id: str) -> None:
        """
        Pause a running sandbox, preserving its state.

        Args:
            sandbox_id: Unique identifier of the sandbox

        Raises:
            SandboxException: if the operation fails
        """
        ...

    async def resume_sandbox(self, sandbox_id: str) -> None:
        """
        Resume a paused sandbox.

        Args:
            sandbox_id: Unique identifier of the sandbox

        Raises:
            SandboxException: if the operation fails
        """
        ...

    async def renew_sandbox_expiration(
        self, sandbox_id: str, new_expiration_time: datetime
    ) -> SandboxRenewResponse:
        """
        Renew the expiration time of a sandbox.

        Args:
            sandbox_id: Unique identifier of the sandbox
            new_expiration_time: New expiration timestamp

        Returns:
            Renew response including the new expiration time.

        Raises:
            SandboxException: if the operation fails
        """
        ...

    async def kill_sandbox(self, sandbox_id: str) -> None:
        """
        Terminate a sandbox and release all associated resources.

        Args:
            sandbox_id: Unique identifier of the sandbox

        Raises:
            SandboxException: if the operation fails
        """
        ...
