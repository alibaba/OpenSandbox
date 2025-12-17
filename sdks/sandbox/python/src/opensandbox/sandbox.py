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
Main Sandbox client implementation.
"""

import asyncio
import logging
import time
from collections.abc import Awaitable, Callable
from datetime import datetime, timedelta, timezone
from typing import Any
from uuid import UUID

from opensandbox.adapters.factory import AdapterFactory
from opensandbox.config import ConnectionConfig
from opensandbox.constants import DEFAULT_EXECD_PORT
from opensandbox.exceptions import (
    InvalidArgumentException,
    SandboxException,
    SandboxInternalException,
    SandboxReadyTimeoutException,
    SandboxUnhealthyException,
)
from opensandbox.models.sandboxes import (
    SandboxEndpoint,
    SandboxImageSpec,
    SandboxInfo,
    SandboxMetrics,
)
from opensandbox.services import (
    Commands,
    Filesystem,
    Health,
    Metrics,
    Sandboxes,
)

logger = logging.getLogger(__name__)


class Sandbox:
    """
    Main entrypoint for the Open Sandbox SDK providing secure, isolated execution environments.

    This class provides a comprehensive interface for interacting with containerized sandbox
    environments, combining lifecycle management with high-level operations for file system
    access, command execution, and real-time monitoring.

    Key Features:

    - **Secure Isolation**: Complete Linux OS access in isolated containers
    - **File System Operations**: Create, read, update, delete files and directories
    - **Multi-language Execution**: Support for Python, Java, Bash, and other languages
    - **Real-time Command Execution**: Streaming output with timeout handling
    - **Resource Management**: CPU, memory, and storage constraints
    - **Lifecycle Management**: Create, pause, resume, terminate operations
    - **Health Monitoring**: Automatic readiness detection and status tracking

    Usage Example:

    ```python
    from opensandbox.models.sandboxes import SandboxImageSpec, SandboxImageAuth
    from opensandbox.models.execd import RunCommandOpts

    # Create with simple image (positional argument)
    sandbox = await Sandbox.create(
        "python:3.11",
        resource={"cpu": "1", "memory": "500Mi"},
        timeout=timedelta(minutes=30)
    )

    # Or with private registry auth
    sandbox = await Sandbox.create(
        SandboxImageSpec(
            "my-registry.com/my-image:latest",
            auth=SandboxImageAuth(username="user", password="pass")
        ),
    )

    # Use the sandbox
    await sandbox.files.write_file("script.py", "print('Hello World')")
    result = await sandbox.commands.run("python script.py")
    print(result.logs.stdout[0].text)  # Output: Hello World

    # Always clean up resources
    await sandbox.kill()
    await sandbox.close()
    ```
    """

    def __init__(
        self,
        sandbox_id: UUID,
        sandbox_service: Sandboxes,
        filesystem_service: Filesystem,
        command_service: Commands,
        health_service: Health,
        metrics_service: Metrics,
        connection_config: ConnectionConfig,
        custom_health_check: Callable[["Sandbox"], Awaitable[bool]] | None = None,
    ) -> None:
        """
        Internal constructor for Sandbox. Use Sandbox.create() or Sandbox.connect() instead.
        """
        self.id = sandbox_id
        self._sandbox_service = sandbox_service
        self._filesystem_service = filesystem_service
        self._command_service = command_service
        self._health_service = health_service
        self._metrics_service = metrics_service
        self._connection_config = connection_config
        self._custom_health_check = custom_health_check

    @property
    def files(self) -> Filesystem:
        """
        Provides access to file system operations within the sandbox.

        Allows writing, reading, listing, and deleting files and directories.
        """
        return self._filesystem_service

    @property
    def commands(self) -> Commands:
        """
        Provides access to command execution operations.

        Allows running shell commands, capturing output, and managing processes.
        """
        return self._command_service

    @property
    def metrics(self) -> Metrics:
        """
        Provides access to sandbox metrics and monitoring.

        Allows retrieving resource usage statistics (CPU, memory) and other performance metrics.
        """
        return self._metrics_service

    @property
    def connection_config(self) -> ConnectionConfig:
        """Provides access to the connection configuration (including shared transport)."""
        return self._connection_config

    async def get_info(self) -> SandboxInfo:
        """
        Get the current status of this sandbox.

        Returns:
            Current sandbox status including state and metadata

        Raises:
            SandboxException: if status cannot be retrieved
        """
        return await self._sandbox_service.get_sandbox_info(self.id)

    async def get_endpoint(self, port: int) -> SandboxEndpoint:
        """
        Get a specific network endpoint for this sandbox.

        Args:
            port: The port number to get the endpoint for

        Returns:
            Endpoint information including connection details

        Raises:
            SandboxException: if endpoint cannot be retrieved
        """
        return await self._sandbox_service.get_sandbox_endpoint(self.id, port)

    async def get_metrics(self) -> SandboxMetrics:
        """
        Get the current resource usage metrics for this sandbox.

        Returns:
            Current sandbox metrics including CPU, memory, and I/O statistics

        Raises:
            SandboxException: if metrics cannot be retrieved
        """
        return await self._metrics_service.get_metrics(self.id)

    async def renew(self, timeout: timedelta) -> None:
        """
        Renew the sandbox expiration time to delay automatic termination.

        The new expiration time will be set to the current time plus the provided duration.

        Args:
            timeout: Duration to add to the current time to set the new expiration

        Raises:
            SandboxException: if the operation fails
        """
        # Use timezone-aware UTC datetime to avoid cross-timezone ambiguity.
        new_expiration = datetime.now(timezone.utc) + timeout
        logger.info(
            f"Renewing sandbox {self.id} timeout, estimated expiration: {new_expiration}"
        )
        await self._sandbox_service.renew_sandbox_expiration(self.id, new_expiration)

    async def pause(self) -> None:
        """
        Pause the sandbox while preserving its state.

        The sandbox will transition to PAUSED state and can be resumed later.
        All running processes will be suspended.

        Raises:
            SandboxException: if pause operation fails
        """
        logger.info(f"Pausing sandbox: {self.id}")
        await self._sandbox_service.pause_sandbox(self.id)

    async def resume(self) -> None:
        """
        Resume a previously paused sandbox.

        The sandbox will transition from PAUSED to RUNNING state and all
        suspended processes will be resumed.

        Raises:
            SandboxException: if resume operation fails
        """
        logger.info(f"Resuming sandbox: {self.id}")
        await self._sandbox_service.resume_sandbox(self.id)

    async def kill(self) -> None:
        """
        Send a termination signal to the remote sandbox instance.

        This is an irreversible operation that stops the sandbox immediately.

        Note: This method does NOT close the local resources. Use close() or
        async context manager to clean up local resources.

        Raises:
            SandboxException: if termination fails
        """
        logger.info(f"Killing sandbox: {self.id}")
        await self._sandbox_service.kill_sandbox(self.id)

    async def close(self) -> None:
        """
        Close local resources associated with this sandbox.

        This method closes HTTP client resources and other local resources.
        It does NOT terminate the remote sandbox instance. Call kill() first
        if you want to terminate the remote sandbox.

        Note: This method logs errors but does not raise exceptions to avoid
        issues in context manager cleanup.
        """
        try:
            # Close transport only when SDK owns it (default transport).
            await self._connection_config.close_transport_if_owned()
            logger.debug(f"Closed resources for sandbox {self.id}")
        except Exception as e:
            logger.warning(
                f"Error closing resources for sandbox {self.id}: {e}",
                exc_info=True
            )

    async def is_healthy(self) -> bool:
        """
        Check if the sandbox is healthy and responsive.

        Returns:
            True if sandbox is healthy, False otherwise
        """
        if self._custom_health_check:
            return await self._custom_health_check(self)
        return await self._ping()

    async def _ping(self) -> bool:
        """Check if the sandbox is alive."""
        try:
            return await self._health_service.ping(self.id)
        except Exception:
            return False

    async def check_ready(
        self,
        timeout: timedelta,
        polling_interval: timedelta,
    ) -> None:
        """
        Wait for the sandbox to pass health checks with polling.

        Args:
            timeout: Maximum time to wait for health check to pass
            polling_interval: Time between health check attempts

        Raises:
            SandboxReadyTimeoutException: if health check doesn't pass within timeout
            SandboxException: if health check fails
        """
        logger.info(
            f"Waiting for sandbox {self.id} to pass health check (timeout: {timeout.total_seconds()}s)"
        )

        deadline = time.time() + timeout.total_seconds()
        attempt = 0
        last_exception: Exception | None = None

        while time.time() < deadline:
            attempt += 1
            logger.debug(f"Health check attempt #{attempt} for sandbox {self.id}")

            try:
                is_healthy = await self.is_healthy()
                if is_healthy:
                    logger.info(
                        f"Sandbox {self.id} passed health check after {attempt} attempts"
                    )
                    return
                last_exception = None
                logger.debug(f"Health check attempt #{attempt} returned false")
            except Exception as e:
                last_exception = e
                is_healthy = False
                logger.debug(
                    f"Health check attempt #{attempt} failed with exception: {e}"
                )

            if not is_healthy:
                await asyncio.sleep(polling_interval.total_seconds())

        error_detail = (
            f"Last error: {last_exception}"
            if last_exception
            else "Health check returned false continuously"
        )

        final_message = (
            f"Sandbox health check timed out after {timeout.total_seconds()}s "
            f"({attempt} attempts). {error_detail}"
        )

        logger.error(final_message)
        raise SandboxReadyTimeoutException(final_message)

    @classmethod
    async def create(
        cls,
        image: SandboxImageSpec | str,
        *,
        timeout: timedelta = timedelta(minutes=10),
        ready_timeout: timedelta = timedelta(seconds=30),
        env: dict[str, str] | None = None,
        metadata: dict[str, str] | None = None,
        resource: dict[str, str] | None = None,
        entrypoint: list[str] | None = None,
        connection_config: ConnectionConfig | None = None,
        health_check: Callable[["Sandbox"], Awaitable[bool]] | None = None,
        health_check_polling_interval: timedelta = timedelta(milliseconds=200),
    ) -> "Sandbox":
        """
        Create a new sandbox instance with the specified configuration.

        Args:
            image: Container image specification including image reference and optional auth
            timeout: Maximum sandbox lifetime
            ready_timeout: Maximum time to wait for sandbox to become ready
            env: Environment variables for the sandbox
            metadata: Custom metadata for the sandbox
            resource: Resource limits (CPU, memory, etc.)
            entrypoint: Command to run as entrypoint
            connection_config: Connection configuration
            health_check: Custom async health check function
            health_check_polling_interval: Time between health check attempts

        Returns:
            Fully configured and ready Sandbox instance

        Raises:
            SandboxException: if sandbox creation or initialization fails
        """
        config = connection_config or ConnectionConfig()
        entrypoint = entrypoint or ["tail", "-f", "/dev/null"]
        env = env or {}
        metadata = metadata or {}
        resource = resource or {"cpu": "1", "memory": "2Gi"}

        if isinstance(image, str):
            image = SandboxImageSpec(image=image)

        logger.info(
            f"Creating sandbox with image: {image.image} (timeout: {timeout.total_seconds()}s)"
        )

        factory = AdapterFactory(config)
        sandbox_id = None
        sandbox_service = None

        try:
            sandbox_service = factory.create_sandbox_service()
            response = await sandbox_service.create_sandbox(
                image,
                entrypoint,
                env,
                metadata,
                timeout,
                resource,
            )
            sandbox_id = response.id

            execd_endpoint = await sandbox_service.get_sandbox_endpoint(
                response.id, DEFAULT_EXECD_PORT
            )

            sandbox = cls(
                sandbox_id=response.id,
                sandbox_service=sandbox_service,
                filesystem_service=factory.create_filesystem_service(execd_endpoint),
                command_service=factory.create_command_service(execd_endpoint),
                health_service=factory.create_health_service(execd_endpoint),
                metrics_service=factory.create_metrics_service(execd_endpoint),
                connection_config=config,
                custom_health_check=health_check,
            )

            await sandbox.check_ready(ready_timeout, health_check_polling_interval)

            logger.info(f"Sandbox {sandbox.id} is ready")
            return sandbox

        except Exception as e:
            # Cleanup on failure
            if sandbox_id and sandbox_service:
                try:
                    logger.warning(
                        f"Creation failed, cleaning up sandbox: {sandbox_id}"
                    )
                    await sandbox_service.kill_sandbox(sandbox_id)
                except Exception as cleanup_ex:
                    logger.error(
                        f"Failed to cleanup sandbox {sandbox_id}", exc_info=cleanup_ex
                    )

            await config.close_transport_if_owned()

            if isinstance(e, SandboxException):
                raise
            logger.error("Unexpected exception during sandbox creation", exc_info=e)
            raise SandboxInternalException(
                f"Internal exception when creating sandbox: {e}"
            ) from e

    @classmethod
    async def connect(
        cls,
        sandbox_id: str | UUID,
        connection_config: ConnectionConfig | None = None,
        health_check: Callable[["Sandbox"], Awaitable[bool]] | None = None,
    ) -> "Sandbox":
        """
        Connect to an existing sandbox instance by ID.

        Args:
            sandbox_id: ID of the existing sandbox
            connection_config: Connection configuration
            health_check: Custom async health check function

        Returns:
            Connected Sandbox instance

        Raises:
            InvalidArgumentException: if required configuration is missing
            SandboxException: if sandbox connection fails
        """
        if not sandbox_id:
            raise InvalidArgumentException("Sandbox ID must be specified")

        if isinstance(sandbox_id, str):
            sandbox_id = UUID(sandbox_id)

        config = connection_config or ConnectionConfig()

        logger.info(f"Connecting to sandbox: {sandbox_id}")

        factory = AdapterFactory(config)

        try:
            sandbox_service = factory.create_sandbox_service()
            execd_endpoint = await sandbox_service.get_sandbox_endpoint(
                sandbox_id, DEFAULT_EXECD_PORT
            )

            sandbox = cls(
                sandbox_id=sandbox_id,
                sandbox_service=sandbox_service,
                filesystem_service=factory.create_filesystem_service(execd_endpoint),
                command_service=factory.create_command_service(execd_endpoint),
                health_service=factory.create_health_service(execd_endpoint),
                metrics_service=factory.create_metrics_service(execd_endpoint),
                connection_config=config,
                custom_health_check=health_check,
            )

            if not await sandbox.is_healthy():
                raise SandboxUnhealthyException(
                    f"Failed to connect: sandbox {sandbox_id} is unhealthy"
                )

            logger.info(f"Connected to sandbox {sandbox_id}")
            return sandbox

        except Exception as e:
            await config.close_transport_if_owned()

            if isinstance(e, SandboxException):
                raise
            logger.error("Unexpected exception during sandbox connection", exc_info=e)
            raise SandboxInternalException(f"Failed to connect to sandbox: {e}") from e

    async def __aenter__(self) -> "Sandbox":
        """Async context manager entry."""
        return self

    async def __aexit__(self, exc_type: Any, exc_val: Any, exc_tb: Any) -> None:
        """Async context manager exit."""
        await self.close()
