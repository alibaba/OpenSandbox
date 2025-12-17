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
Command service adapter implementation.

Implementation of Commands that adapts openapi-python-client generated CommandApi.
This adapter handles command execution within sandboxes, providing both
synchronous and streaming execution modes with proper session management.
"""

import json
import logging

import httpx

from opensandbox.adapters.converter.event_node import EventNode
from opensandbox.adapters.converter.exception_converter import (
    ExceptionConverter,
)
from opensandbox.adapters.converter.execution_converter import (
    ExecutionConverter,
)
from opensandbox.adapters.converter.execution_event_dispatcher import (
    ExecutionEventDispatcher,
)
from opensandbox.adapters.converter.response_handler import handle_api_error
from opensandbox.config import ConnectionConfig
from opensandbox.exceptions import InvalidArgumentException, SandboxApiException
from opensandbox.models.execd import Execution, ExecutionHandlers, RunCommandOpts
from opensandbox.models.sandboxes import SandboxEndpoint
from opensandbox.services.command import Commands

logger = logging.getLogger(__name__)


class CommandsAdapter(Commands):
    """
    Implementation of Commands that adapts openapi-python-client generated CommandApi.

    This adapter handles command execution within sandboxes, providing both
    synchronous and streaming execution modes with proper session management.

    The adapter uses direct httpx streaming for command execution to handle
    Server-Sent Events (SSE) properly, while using the generated API client
    for simpler operations like interrupt.
    """

    RUN_COMMAND_PATH = "/command"
    INTERRUPT_COMMAND_PATH = "/command/{execution_id}/interrupt"

    def __init__(
        self,
        connection_config: ConnectionConfig,
        execd_endpoint: SandboxEndpoint,
    ) -> None:
        """
        Initialize the command service adapter.

        Args:
            connection_config: Connection configuration (shared transport, headers, timeouts)
            execd_endpoint: Endpoint for execd service
        """
        self.connection_config = connection_config
        self.execd_endpoint = execd_endpoint
        from opensandbox.api.execd import Client

        protocol = self.connection_config.protocol
        base_url = f"{protocol}://{self.execd_endpoint.endpoint}"
        timeout_seconds = self.connection_config.request_timeout.total_seconds()
        timeout = httpx.Timeout(timeout_seconds)

        headers = {
            "User-Agent": self.connection_config.user_agent,
            **self.connection_config.headers,
        }

        # Execd API does not require authentication
        self._client = Client(
            base_url=base_url,
            timeout=timeout,
        )

        # Inject httpx client (adapter-owned)
        self._httpx_client = httpx.AsyncClient(
            base_url=base_url,
            headers=headers,
            timeout=timeout,
            transport=self.connection_config.transport,
        )
        self._client.set_async_httpx_client(self._httpx_client)

        # SSE client (read timeout disabled)
        sse_headers = {
            **headers,
            "Accept": "text/event-stream",
            "Cache-Control": "no-cache",
        }
        self._sse_client = httpx.AsyncClient(
            headers=sse_headers,
            timeout=httpx.Timeout(
                connect=timeout_seconds,
                read=None,
                write=timeout_seconds,
                pool=None,
            ),
            transport=self.connection_config.transport,
        )

    async def _get_client(self):
        """Return the client for execd API (no auth required)."""
        return self._client

    def _get_execd_url(self, path: str) -> str:
        """Build URL for execd endpoint."""
        protocol = self.connection_config.protocol
        return f"{protocol}://{self.execd_endpoint.endpoint}{path}"

    async def _get_sse_client(self) -> httpx.AsyncClient:
        """Return SSE client (read timeout disabled) for execd streaming."""
        return self._sse_client

    async def run(
        self,
        command: str,
        *,
        opts: RunCommandOpts | None = None,
        handlers: ExecutionHandlers | None = None,
    ) -> Execution:
        """Execute a shell command within the sandbox.

        This method uses direct httpx streaming to handle SSE responses
        from the execd service.
        """
        if not command.strip():
            raise InvalidArgumentException("Command cannot be empty")

        try:
            # Convert domain model to API model
            opts = opts or RunCommandOpts()
            json_body = ExecutionConverter.to_api_run_command_json(command, opts)

            # Prepare URL
            url = self._get_execd_url(self.RUN_COMMAND_PATH)

            execution = Execution(
                id=None,
                execution_count=None,
                result=[],
                error=None,
            )

            # Use SSE client for streaming responses (read timeout disabled)
            client = await self._get_sse_client()

            # Use streaming request for SSE
            async with client.stream("POST", url, json=json_body) as response:
                if response.status_code != 200:
                    await response.aread()
                    error_body = response.text
                    logger.error(
                        f"Failed to run command. Status: {response.status_code}, Body: {error_body}"
                    )
                    raise SandboxApiException(
                        message=f"Failed to run command. Status code: {response.status_code}",
                        status_code=response.status_code,
                    )

                dispatcher = ExecutionEventDispatcher(execution, handlers)

                async for line in response.aiter_lines():
                    if not line.strip():
                        continue

                    # Handle potential SSE format "data: ..."
                    data = line
                    if data.startswith("data:"):
                        data = data[5:].strip()

                    try:
                        event_dict = json.loads(data)
                        event_node = EventNode(**event_dict)
                        await dispatcher.dispatch(event_node)
                    except Exception as e:
                        logger.error(f"Failed to parse SSE line: {line}", exc_info=e)

            return execution

        except Exception as e:
            logger.error(
                "Failed to run command (length: %s)",
                len(command),
                exc_info=e,
            )
            raise ExceptionConverter.to_sandbox_exception(e) from e

    async def interrupt(self, execution_id: str) -> None:
        """Interrupt a running command execution."""
        try:
            from opensandbox.api.execd.api.command import interrupt_command

            client = await self._get_client()
            response_obj = await interrupt_command.asyncio_detailed(
                client=client,
                id=execution_id,
            )

            handle_api_error(response_obj, "Interrupt command")

        except Exception as e:
            logger.error("Failed to interrupt command", exc_info=e)
            raise ExceptionConverter.to_sandbox_exception(e) from e
