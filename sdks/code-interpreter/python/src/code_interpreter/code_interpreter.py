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
Code Interpreter SDK providing secure, isolated code execution capabilities.

This module provides the main CodeInterpreter class that extends basic Sandbox
functionality with specialized code execution features, including multi-language
support, session management, and variable persistence.
"""

import logging
from datetime import datetime, timedelta, timezone
from uuid import UUID

from opensandbox.exceptions import (
    InvalidArgumentException,
    SandboxException,
    SandboxInternalException,
)
from opensandbox.models.sandboxes import (
    SandboxEndpoint,
    SandboxInfo,
    SandboxMetrics,
)
from opensandbox.sandbox import Sandbox

from code_interpreter.adapters.factory import AdapterFactory
from code_interpreter.services.code import Codes

logger = logging.getLogger(__name__)


class CodeInterpreter:
    """
    Code Interpreter SDK providing secure, isolated code execution capabilities.

    This class extends the basic Sandbox functionality with specialized code execution features,
    including multi-language support, session management, and variable persistence.

    Key Features:

    - Multi-language Code Execution: Support for Python, JavaScript, Bash, Java, Kotlin
    - Session Management: Persistent execution contexts with variable state
    - Sandbox Integration: Full access to underlying sandbox file system and command execution
    - Streaming Execution: Real-time code execution with output streaming
    - Variable Inspection: Access to execution variables and state

    Usage Example:

    ```python
    # First create a sandbox instance

    sandbox = await Sandbox.create(
        "python:3.11",
        resource={"cpu": "1", "memory": "2Gi"}
    )

    # Then create a code interpreter wrapping the sandbox
    interpreter = await CodeInterpreter.create(sandbox=sandbox)

    # Execute code with context
    from code_interpreter.models.code import SupportedLanguage
    context = await interpreter.codes.create_context(SupportedLanguage.PYTHON)
    result = await interpreter.codes.run("print('Hello World')", context=context)
    print(result.logs.stdout)  # Output: Hello World

    # Access underlying sandbox for file operations
    await interpreter.sandbox.files.write_files([
        WriteEntry(path="data.txt", data="Hello")
    ])
    file_result = await interpreter.codes.run(
        "with open('data.txt') as f: print(f.read())",
        context=context,
    )

    # Always clean up resources
    await interpreter.kill()
    await interpreter.sandbox.close()
    ```
    """

    def __init__(self, sandbox: Sandbox, code_service: Codes) -> None:
        """
        Initialize CodeInterpreter with sandbox and code service.

        Note: This constructor is for internal use. Use CodeInterpreter.create() instead.

        Args:
            sandbox: Underlying sandbox instance
            code_service: Code execution implementation
        """
        self._sandbox = sandbox
        self._code_service = code_service

    @property
    def sandbox(self) -> Sandbox:
        """
        Provides access to the underlying sandbox instance.

        Returns:
            The underlying sandbox instance
        """
        return self._sandbox

    @property
    def id(self) -> UUID:
        """
        Gets the unique identifier of this code interpreter (same as underlying sandbox ID).

        Returns:
            UUID of the code interpreter/sandbox
        """
        return self._sandbox.id

    @property
    def files(self):
        """
        Provides access to file system operations within the sandbox.

        Allows writing, reading, listing, and deleting files and directories.

        Returns:
            Service for filesystem manipulation
        """
        return self._sandbox.files

    @property
    def commands(self):
        """
        Provides access to command execution operations.

        Allows running shell commands, capturing output, and managing processes.

        Returns:
            Service for command execution
        """
        return self._sandbox.commands

    @property
    def metrics(self):
        """
        Provides access to sandbox metrics and monitoring.

        Allows retrieving resource usage statistics (CPU, memory) and other performance metrics.

        Returns:
            Service for metrics retrieval
        """
        return self._sandbox.metrics

    @property
    def codes(self) -> Codes:
        """
        Provides access to code execution operations.

        This service enables:
        - Multi-language code execution (Python, JavaScript, Bash, etc.)
        - Execution context management with persistent variables
        - Real-time output streaming and interruption capabilities

        Returns:
            Service for advanced code execution with session support
        """
        return self._code_service

    async def get_endpoint(self, port: int) -> SandboxEndpoint:
        """
        Gets a specific network endpoint for the underlying sandbox.

        This allows access to specific ports exposed by the sandbox, which can be
        useful for connecting to additional services or debugging interfaces.

        Args:
            port: The port number to get the endpoint for

        Returns:
            Endpoint information including host, port, and connection details

        Raises:
            SandboxException: If endpoint cannot be retrieved
        """
        return await self._sandbox.get_endpoint(port)

    async def get_info(self) -> SandboxInfo:
        """
        Gets the current status of this sandbox.

        Returns:
            Current sandbox status including state and metadata

        Raises:
            SandboxException: If status cannot be retrieved
        """
        return await self._sandbox.get_info()

    async def get_metrics(self) -> SandboxMetrics:
        """
        Gets the current resource usage metrics for the underlying sandbox.

        Provides real-time information about CPU usage, memory consumption,
        disk I/O, and other performance metrics.

        Returns:
            Current sandbox metrics including CPU, memory, and I/O statistics

        Raises:
            SandboxException: If metrics cannot be retrieved
        """
        return await self._sandbox.get_metrics()

    async def renew(self, timeout: timedelta | int) -> None:
        """
        Renew the sandbox expiration time to delay automatic termination.

        The new expiration time will be set to the current time plus the provided duration.

        Args:
            timeout: Duration to add to the current time to set the new expiration.
                    Can be timedelta or seconds as int.

        Raises:
            SandboxException: If the operation fails
        """
        if isinstance(timeout, int):
            timeout = timedelta(seconds=timeout)

        logger.info(
            "Renew code interpreter %s timeout, estimated expiration to %s",
            self.id,
            datetime.now(timezone.utc) + timeout,
        )
        await self._sandbox.renew(timeout)

    async def pause(self) -> None:
        """
        Pauses the sandbox while preserving its state.

        The sandbox will transition to PAUSED state and can be resumed later.
        All running processes will be suspended.

        Raises:
            SandboxException: If pause operation fails
        """
        logger.info("Pausing code interpreter: %s", self.id)
        await self._sandbox.pause()

    async def resume(self) -> None:
        """
        Resumes a previously paused code interpreter.

        The sandbox will transition from PAUSED to RUNNING state and all
        suspended processes will be resumed.

        Raises:
            SandboxException: If resume operation fails
        """
        logger.info("Resuming code interpreter: %s", self.id)
        await self._sandbox.resume()

    async def kill(self) -> None:
        """
        This method sends a termination signal to the remote sandbox instance, causing it to stop immediately.
        This is an irreversible operation.

        Note: This method does NOT close the local `Sandbox` object resources (like connection pools).
        You should call `close()` or use async context manager to clean up local resources.

        Raises:
            SandboxException: If termination fails
        """
        logger.info("Killing code interpreter: %s", self.id)
        await self._sandbox.kill()

    async def is_healthy(self) -> bool:
        """
        Checks if the code interpreter and its underlying sandbox are healthy and responsive.

        This performs health checks on both the sandbox infrastructure and code execution services.

        Returns:
            True if both sandbox and code execution services are healthy, False otherwise
        """
        return await self._sandbox.is_healthy()

    @classmethod
    async def create(cls, sandbox: Sandbox) -> "CodeInterpreter":
        """
        Creates a CodeInterpreter from an existing Sandbox instance.

        This factory method handles the creation and initialization of CodeInterpreter
        services, including the code execution service and language configuration.

        CodeInterpreter must be created by wrapping an existing Sandbox instance with
        code execution capabilities. This design ensures clear separation of concerns:
        - Sandbox handles infrastructure (containers, resources, networking)
        - CodeInterpreter adds code execution capabilities on top

        Args:
            sandbox: Existing sandbox instance to wrap with code execution capabilities

        Returns:
            CodeInterpreter instance wrapping the sandbox

        Raises:
            InvalidArgumentException: If sandbox is not provided
            SandboxException: If creation fails
            SandboxInternalException: If internal service initialization fails
        """
        if sandbox is None:
            raise InvalidArgumentException("Sandbox instance must be provided")

        logger.info("Creating code interpreter from sandbox: %s", sandbox.id)

        factory = AdapterFactory(sandbox.connection_config)

        try:
            # Connect to the execd daemon endpoint for code execution services
            from opensandbox.constants import DEFAULT_EXECD_PORT
            code_interpreter_endpoint = await sandbox.get_endpoint(DEFAULT_EXECD_PORT)
            code_execution_service = factory.create_code_execution_service(code_interpreter_endpoint)

            logger.info("Code interpreter %s created successfully", sandbox.id)

            return cls(sandbox, code_execution_service)
        except Exception as e:
            if isinstance(e, SandboxException):
                raise
            raise SandboxInternalException(
                f"Failed to create code interpreter: {e}", cause=e
            ) from e
