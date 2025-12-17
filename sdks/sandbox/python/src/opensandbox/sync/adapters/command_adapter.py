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
Synchronous command adapter implementation (including SSE streaming).
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
from opensandbox.adapters.converter.response_handler import handle_api_error
from opensandbox.config.connection_sync import ConnectionConfigSync
from opensandbox.exceptions import InvalidArgumentException, SandboxApiException
from opensandbox.models.execd import Execution, RunCommandOpts
from opensandbox.models.execd_sync import ExecutionHandlersSync
from opensandbox.models.sandboxes import SandboxEndpoint
from opensandbox.sync.adapters.converter.execution_event_dispatcher import (
    ExecutionEventDispatcherSync,
)
from opensandbox.sync.services.command import CommandsSync

logger = logging.getLogger(__name__)


class CommandsAdapterSync(CommandsSync):
    """
    Synchronous implementation of :class:`~opensandbox.sync.services.command.CommandsSync`.

    This adapter wraps openapi-python-client generated clients for simple operations and
    uses direct ``httpx`` streaming for SSE (Server-Sent Events) command execution output.
    """

    RUN_COMMAND_PATH = "/command"

    def __init__(self, connection_config: ConnectionConfigSync, execd_endpoint: SandboxEndpoint) -> None:
        """
        Initialize the command adapter (sync).

        Args:
            connection_config: Connection configuration (shared transport, headers, timeouts)
            execd_endpoint: Endpoint for execd service
        """
        self.connection_config = connection_config
        self.execd_endpoint = execd_endpoint

        from opensandbox.api.execd import Client

        base_url = f"{self.connection_config.protocol}://{self.execd_endpoint.endpoint}"
        timeout_seconds = self.connection_config.request_timeout.total_seconds()
        timeout = httpx.Timeout(timeout_seconds)

        headers = {"User-Agent": self.connection_config.user_agent, **self.connection_config.headers}

        self._client = Client(base_url=base_url, timeout=timeout)

        self._httpx_client = httpx.Client(
            base_url=base_url,
            headers=headers,
            timeout=timeout,
            transport=self.connection_config.transport,
        )
        self._client.set_httpx_client(self._httpx_client)

        # SSE client (read timeout disabled)
        sse_headers = {
            **headers,
            "Accept": "text/event-stream",
            "Cache-Control": "no-cache",
        }
        self._sse_client = httpx.Client(
            headers=sse_headers,
            timeout=httpx.Timeout(
                connect=timeout_seconds,
                read=None,
                write=timeout_seconds,
                pool=None,
            ),
            transport=self.connection_config.transport,
        )

    def _get_execd_url(self, path: str) -> str:
        """Build URL for execd endpoint."""
        return f"{self.connection_config.protocol}://{self.execd_endpoint.endpoint}{path}"

    def run(
        self,
        command: str,
        *,
        opts: RunCommandOpts | None = None,
        handlers: ExecutionHandlersSync | None = None,
    ) -> Execution:
        if not command.strip():
            raise InvalidArgumentException("Command cannot be empty")

        try:
            opts = opts or RunCommandOpts()
            json_body = ExecutionConverter.to_api_run_command_json(command, opts)
            url = self._get_execd_url(self.RUN_COMMAND_PATH)

            execution = Execution(id=None, execution_count=None, result=[], error=None)
            dispatcher = ExecutionEventDispatcherSync(execution, handlers)

            with self._sse_client.stream("POST", url, json=json_body) as response:
                if response.status_code != 200:
                    response.read()
                    raise SandboxApiException(
                        message=f"Failed to run command. Status code: {response.status_code}",
                        status_code=response.status_code,
                    )

                for line in response.iter_lines():
                    if not line or not line.strip():
                        continue
                    data = line
                    if data.startswith("data:"):
                        data = data[5:].strip()
                    try:
                        event_dict = json.loads(data)
                        event_node = EventNode(**event_dict)
                        dispatcher.dispatch(event_node)
                    except Exception as e:
                        logger.error("Failed to parse SSE line: %s", line, exc_info=e)

            return execution

        except Exception as e:
            logger.error("Failed to run command (length: %s)", len(command), exc_info=e)
            raise ExceptionConverter.to_sandbox_exception(e) from e

    def interrupt(self, execution_id: str) -> None:
        """
        Interrupt a running command execution.

        Args:
            execution_id: Execution id returned by execd for the running command
        """
        try:
            from opensandbox.api.execd.api.command import interrupt_command

            response_obj = interrupt_command.sync_detailed(client=self._client, id=execution_id)
            handle_api_error(response_obj, "Interrupt command")
        except Exception as e:
            logger.error("Failed to interrupt command", exc_info=e)
            raise ExceptionConverter.to_sandbox_exception(e) from e
