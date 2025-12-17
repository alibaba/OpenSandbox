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
Synchronous Sandbox client implementation.
"""

import logging
import time
from collections.abc import Callable
from datetime import datetime, timedelta, timezone
from typing import Any
from uuid import UUID

from opensandbox.config.connection_sync import ConnectionConfigSync
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
from opensandbox.sync.adapters.factory import AdapterFactorySync
from opensandbox.sync.services import (
    CommandsSync,
    FilesystemSync,
    HealthSync,
    MetricsSync,
    SandboxesSync,
)

logger = logging.getLogger(__name__)


class SandboxSync:
    """
    Main synchronous entrypoint for the Open Sandbox SDK.

    This class mirrors the async :class:`opensandbox.sandbox.Sandbox` API, but all
    operations are **blocking** and executed in the current thread.

    Key Features:

    - **Secure Isolation**: Complete Linux OS access in isolated containers
    - **File System Operations**: Create, read, update, delete files and directories
    - **Multi-language Execution**: Support for Python, Java, Bash, and other languages
    - **Real-time Command Execution**: Streaming output via SSE (Server-Sent Events)
    - **Resource Management**: CPU, memory, and storage constraints
    - **Lifecycle Management**: Create, pause, resume, terminate operations
    - **Health Monitoring**: Readiness polling and status tracking

    Notes:

    - **Blocking**: Do not call these methods directly from an asyncio event loop thread.
      If you need non-blocking behavior, prefer the async :class:`~opensandbox.sandbox.Sandbox`.
    - **Resource cleanup**: :meth:`close` closes *local* HTTP resources only. It does **not**
      terminate the remote sandbox instance. Call :meth:`kill` to stop the remote sandbox.

    Usage Example:

    ```python
    from datetime import timedelta
    from opensandbox.models.sandboxes import SandboxImageSpec
    from opensandbox.models.execd import RunCommandOpts
    from opensandbox.sync.sandbox import SandboxSync

    # Create a sandbox (blocking)
    sandbox = SandboxSync.create(
        "python:3.11",
        resource={"cpu": "1", "memory": "500Mi"},
        timeout=timedelta(minutes=30),
    )

    # Use the sandbox
    sandbox.files.write_file("script.py", "print('Hello World')")
    result = sandbox.commands.run("python script.py")

    # Always clean up resources
    sandbox.kill()   # terminate remote sandbox
    sandbox.close()  # close local HTTP resources

    # Or use a context manager for automatic close():
    with SandboxSync.create("python:3.11") as sandbox:
        # Note on lifecycle:
        # - Exiting the context manager will call `sandbox.close()` (local HTTP resources only).
        # - You must still call `sandbox.kill()` to terminate the remote sandbox instance.
        sandbox.commands.run("python -c \"print('hi')\"")
        sandbox.kill()
    ```
    """

    def __init__(
        self,
        sandbox_id: UUID,
        sandbox_service: SandboxesSync,
        filesystem_service: FilesystemSync,
        command_service: CommandsSync,
        health_service: HealthSync,
        metrics_service: MetricsSync,
        connection_config: ConnectionConfigSync,
        custom_health_check: Callable[["SandboxSync"], bool] | None = None,
    ) -> None:
        """
        Internal constructor for SandboxSync. Use :meth:`create` or :meth:`connect` instead.
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
    def files(self) -> FilesystemSync:
        """
        Provides access to file system operations within the sandbox.

        Allows writing, reading, listing, and deleting files and directories.
        """
        return self._filesystem_service

    @property
    def commands(self) -> CommandsSync:
        """
        Provides access to command execution operations.

        Supports both one-shot command execution and SSE streaming output.
        """
        return self._command_service

    @property
    def metrics(self) -> MetricsSync:
        """
        Provides access to sandbox metrics and monitoring.

        Allows retrieving resource usage statistics (CPU, memory) and other performance metrics.
        """
        return self._metrics_service

    @property
    def connection_config(self) -> ConnectionConfigSync:
        """Provides access to the connection configuration (including shared transport)."""
        return self._connection_config

    def get_info(self) -> SandboxInfo:
        """
        Get the current status of this sandbox.

        Returns:
            Current sandbox status including state and metadata

        Raises:
            SandboxException: if status cannot be retrieved
        """
        return self._sandbox_service.get_sandbox_info(self.id)

    def get_endpoint(self, port: int) -> SandboxEndpoint:
        """
        Get a specific network endpoint for this sandbox.

        Args:
            port: The port number to get the endpoint for

        Returns:
            Endpoint information including connection details

        Raises:
            SandboxException: if endpoint cannot be retrieved
        """
        return self._sandbox_service.get_sandbox_endpoint(self.id, port)

    def get_metrics(self) -> SandboxMetrics:
        """
        Get the current resource usage metrics for this sandbox.

        Returns:
            Current sandbox metrics including CPU, memory, and I/O statistics

        Raises:
            SandboxException: if metrics cannot be retrieved
        """
        return self._metrics_service.get_metrics(self.id)

    def renew(self, timeout: timedelta) -> None:
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
            "Renewing sandbox %s timeout, estimated expiration: %s",
            self.id,
            new_expiration,
        )
        self._sandbox_service.renew_sandbox_expiration(self.id, new_expiration)

    def pause(self) -> None:
        """
        Pause the sandbox while preserving its state.

        The sandbox will transition to PAUSED state and can be resumed later.
        All running processes will be suspended.

        Raises:
            SandboxException: if pause operation fails
        """
        logger.info("Pausing sandbox: %s", self.id)
        self._sandbox_service.pause_sandbox(self.id)

    def resume(self) -> None:
        """
        Resume a previously paused sandbox.

        The sandbox will transition from PAUSED to RUNNING state and all
        suspended processes will be resumed.

        Raises:
            SandboxException: if resume operation fails
        """
        logger.info("Resuming sandbox: %s", self.id)
        self._sandbox_service.resume_sandbox(self.id)

    def kill(self) -> None:
        """
        Send a termination signal to the remote sandbox instance.

        This is an irreversible operation that stops the sandbox immediately.

        Note: This method does NOT close the local resources. Use :meth:`close` or
        the sync context manager to clean up local resources.

        Raises:
            SandboxException: if termination fails
        """
        logger.info("Killing sandbox: %s", self.id)
        self._sandbox_service.kill_sandbox(self.id)

    def close(self) -> None:
        """
        Close local resources associated with this sandbox.

        This method closes HTTP client resources and other local resources.
        It does NOT terminate the remote sandbox instance. Call :meth:`kill` first
        if you want to terminate the remote sandbox.

        Note: This method logs errors but does not raise exceptions to avoid
        issues in context manager cleanup.
        """
        try:
            self._connection_config.close_transport_if_owned()
            logger.debug("Closed resources for sandbox %s", self.id)
        except Exception as e:
            logger.warning("Error closing resources for sandbox %s: %s", self.id, e, exc_info=True)

    def is_healthy(self) -> bool:
        """
        Check if the sandbox is healthy and responsive.

        Returns:
            True if sandbox is healthy, False otherwise
        """
        if self._custom_health_check:
            return self._custom_health_check(self)
        try:
            return self._health_service.ping(self.id)
        except Exception:
            return False

    def check_ready(self, timeout: timedelta, polling_interval: timedelta) -> None:
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
            "Waiting for sandbox %s to pass health check (timeout: %ss)",
            self.id,
            timeout.total_seconds(),
        )

        deadline = time.time() + timeout.total_seconds()
        attempt = 0
        last_exception: Exception | None = None

        while time.time() < deadline:
            attempt += 1
            logger.debug("Health check attempt #%s for sandbox %s", attempt, self.id)
            try:
                if self.is_healthy():
                    logger.info(
                        "Sandbox %s passed health check after %s attempts",
                        self.id,
                        attempt,
                    )
                    return
                last_exception = None
            except Exception as e:
                last_exception = e

            time.sleep(polling_interval.total_seconds())

        error_detail = f"Last error: {last_exception}" if last_exception else "Health check returned false continuously"
        final_message = (
            f"Sandbox health check timed out after {timeout.total_seconds()}s ({attempt} attempts). {error_detail}"
        )
        logger.error(final_message)
        raise SandboxReadyTimeoutException(final_message)

    @classmethod
    def create(
        cls,
        image: SandboxImageSpec | str,
        *,
        timeout: timedelta = timedelta(minutes=10),
        ready_timeout: timedelta = timedelta(seconds=30),
        env: dict[str, str] | None = None,
        metadata: dict[str, str] | None = None,
        resource: dict[str, str] | None = None,
        entrypoint: list[str] | None = None,
        connection_config: ConnectionConfigSync | None = None,
        health_check: Callable[["SandboxSync"], bool] | None = None,
        health_check_polling_interval: timedelta = timedelta(milliseconds=200),
    ) -> "SandboxSync":
        """
        Create a new sandbox instance with the specified configuration (blocking).

        Args:
            image: Container image specification including image reference and optional auth
            timeout: Maximum sandbox lifetime
            ready_timeout: Maximum time to wait for sandbox to become ready
            env: Environment variables for the sandbox
            metadata: Custom metadata for the sandbox
            resource: Resource limits (CPU, memory, etc.)
            entrypoint: Command to run as entrypoint
            connection_config: Connection configuration
            health_check: Custom sync health check function
            health_check_polling_interval: Time between health check attempts

        Returns:
            Fully configured and ready SandboxSync instance

        Raises:
            SandboxException: if sandbox creation or initialization fails
        """
        config = connection_config or ConnectionConfigSync()
        entrypoint = entrypoint or ["tail", "-f", "/dev/null"]
        env = env or {}
        metadata = metadata or {}
        resource = resource or {"cpu": "1", "memory": "2Gi"}

        if isinstance(image, str):
            image = SandboxImageSpec(image=image)

        logger.info(
            "Creating sandbox with image: %s (timeout: %ss)",
            image.image,
            timeout.total_seconds(),
        )

        factory = AdapterFactorySync(config)
        sandbox_id: UUID | None = None
        sandbox_service: SandboxesSync | None = None

        try:
            sandbox_service = factory.create_sandbox_service()
            response = sandbox_service.create_sandbox(
                image, entrypoint, env, metadata, timeout, resource
            )
            sandbox_id = response.id
            execd_endpoint = sandbox_service.get_sandbox_endpoint(response.id, DEFAULT_EXECD_PORT)

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

            sandbox.check_ready(ready_timeout, health_check_polling_interval)
            logger.info("Sandbox %s is ready", sandbox.id)
            return sandbox
        except Exception as e:
            if sandbox_id and sandbox_service:
                try:
                    logger.warning("Creation failed, cleaning up sandbox: %s", sandbox_id)
                    sandbox_service.kill_sandbox(sandbox_id)
                except Exception:
                    pass
            config.close_transport_if_owned()
            if isinstance(e, SandboxException):
                raise
            raise SandboxInternalException(f"Internal exception when creating sandbox: {e}") from e

    @classmethod
    def connect(
        cls,
        sandbox_id: str | UUID,
        connection_config: ConnectionConfigSync | None = None,
        health_check: Callable[["SandboxSync"], bool] | None = None,
    ) -> "SandboxSync":
        """
        Connect to an existing sandbox instance by ID (blocking).

        Args:
            sandbox_id: ID of the existing sandbox
            connection_config: Connection configuration
            health_check: Custom sync health check function

        Returns:
            Connected SandboxSync instance

        Raises:
            InvalidArgumentException: if required configuration is missing
            SandboxException: if sandbox connection fails
        """
        if not sandbox_id:
            raise InvalidArgumentException("Sandbox ID must be specified")
        if isinstance(sandbox_id, str):
            sandbox_id = UUID(sandbox_id)

        config = connection_config or ConnectionConfigSync()
        logger.info("Connecting to sandbox: %s", sandbox_id)
        factory = AdapterFactorySync(config)

        try:
            sandbox_service = factory.create_sandbox_service()
            execd_endpoint = sandbox_service.get_sandbox_endpoint(sandbox_id, DEFAULT_EXECD_PORT)

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
            if not sandbox.is_healthy():
                raise SandboxUnhealthyException(f"Failed to connect: sandbox {sandbox_id} is unhealthy")
            logger.info("Connected to sandbox %s", sandbox_id)
            return sandbox
        except Exception as e:
            config.close_transport_if_owned()
            if isinstance(e, SandboxException):
                raise
            raise SandboxInternalException(f"Failed to connect to sandbox: {e}") from e

    def __enter__(self) -> "SandboxSync":
        """Sync context manager entry."""
        return self

    def __exit__(self, exc_type: Any, exc_val: Any, exc_tb: Any) -> None:
        """Sync context manager exit."""
        self.close()
