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
from __future__ import annotations

import json

import httpx
import pytest

from opensandbox.adapters.command_adapter import CommandsAdapter
from opensandbox.config import ConnectionConfig
from opensandbox.exceptions import InvalidArgumentException, SandboxApiException
from opensandbox.models.sandboxes import SandboxEndpoint


class _SseTransport(httpx.AsyncBaseTransport):
    def __init__(self) -> None:
        self.last_request: httpx.Request | None = None

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        self.last_request = request
        body = request.content.decode("utf-8") if isinstance(request.content, (bytes, bytearray)) else ""
        payload = json.loads(body) if body else {}

        if request.url.path == "/command" and payload.get("command") == "echo hi":
            sse = (
                b'data: {"type":"init","text":"exec-1","timestamp":1}\n\n'
                b'\n'
                b'data: {"type":"stdout","text":"hi","timestamp":2}\n\n'
                b"not-json\n\n"
                b'data: {"type":"result","results":{"text":"ok"},"timestamp":3}\n\n'
                b'data: {"type":"execution_complete","timestamp":4,"execution_time":5}\n\n'
            )
            return httpx.Response(
                200,
                headers={"Content-Type": "text/event-stream"},
                content=sse,
                request=request,
            )

        return httpx.Response(500, content=b"boom", request=request)


@pytest.mark.asyncio
async def test_run_command_streaming_happy_path_updates_execution() -> None:
    transport = _SseTransport()
    cfg = ConnectionConfig(protocol="http", transport=transport)
    endpoint = SandboxEndpoint(endpoint="localhost:44772", port=44772)
    adapter = CommandsAdapter(cfg, endpoint)

    execution = await adapter.run("echo hi")
    assert execution.id == "exec-1"
    assert execution.logs.stdout[0].text == "hi"
    assert execution.result[0].text == "ok"

    assert transport.last_request is not None
    assert transport.last_request.headers.get("accept") == "text/event-stream"


@pytest.mark.asyncio
async def test_run_command_rejects_blank_command() -> None:
    cfg = ConnectionConfig(protocol="http")
    endpoint = SandboxEndpoint(endpoint="localhost:44772", port=44772)
    adapter = CommandsAdapter(cfg, endpoint)

    with pytest.raises(InvalidArgumentException):
        await adapter.run("   ")


@pytest.mark.asyncio
async def test_run_command_non_200_raises_api_exception() -> None:
    transport = _SseTransport()
    cfg = ConnectionConfig(protocol="http", transport=transport)
    endpoint = SandboxEndpoint(endpoint="localhost:44772", port=44772)
    adapter = CommandsAdapter(cfg, endpoint)

    with pytest.raises(SandboxApiException):
        await adapter.run("other")
