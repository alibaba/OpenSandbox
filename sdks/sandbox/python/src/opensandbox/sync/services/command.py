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
Synchronous command service interface.

Defines the contract for **blocking** command execution operations inside a sandbox.
This is the sync counterpart of :mod:`opensandbox.services.command`.
"""

from typing import Protocol

from opensandbox.models.execd import Execution, RunCommandOpts
from opensandbox.models.execd_sync import ExecutionHandlersSync


class CommandsSync(Protocol):
    """
    Command execution service for sandbox environments (sync).

    This service provides secure command execution capabilities within sandbox environments,
    with support for SSE streaming output, timeout handling, and interruption.

    Notes:
        - All methods are **blocking** and executed in the current thread.
        - Streaming output is delivered via SSE and accumulated into an ``Execution`` object.
    """

    def run(
        self,
        command: str,
        *,
        opts: RunCommandOpts | None = None,
        handlers: ExecutionHandlersSync | None = None,
    ) -> Execution:
        """
        Execute a shell command in the sandbox environment.

        The command can be executed in streaming mode (SSE) based on request configuration
        and optional handlers.

        Args:
            command: Shell command text to execute
            opts: Command execution options (e.g. background, working_directory)
            handlers: Optional handlers for streaming events

        Returns:
            An ``Execution`` object representing the command execution result/events.

        Raises:
            SandboxException: If the operation fails.
        """
        ...

    def interrupt(self, execution_id: str) -> None:
        """
        Interrupt and terminate a running command execution.

        This typically sends a termination signal to the process associated with the given
        execution ID.

        Args:
            execution_id: Unique identifier of the execution to interrupt.

        Raises:
            SandboxException: If the operation fails.
        """
        ...
