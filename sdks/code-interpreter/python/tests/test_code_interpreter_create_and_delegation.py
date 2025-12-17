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
import pytest
from opensandbox.config import ConnectionConfig
from opensandbox.exceptions import InvalidArgumentException
from opensandbox.models.sandboxes import SandboxEndpoint

from code_interpreter import CodeInterpreter


class _FakeSandbox:
    def __init__(self) -> None:
        self._id = __import__("uuid").uuid4()
        self.connection_config = ConnectionConfig(protocol="http")
        self.files = object()
        self.commands = object()
        self.metrics = object()

    @property
    def id(self):
        return self._id

    async def get_endpoint(self, port: int) -> SandboxEndpoint:
        return SandboxEndpoint(endpoint="localhost:44772", port=port)

    async def is_healthy(self) -> bool:
        return True

    async def get_info(self):  # pragma: no cover
        raise RuntimeError("not used")

    async def get_metrics(self):  # pragma: no cover
        raise RuntimeError("not used")

    async def renew(self, timeout):  # pragma: no cover
        raise RuntimeError("not used")


@pytest.mark.asyncio
async def test_create_requires_sandbox() -> None:
    with pytest.raises(InvalidArgumentException):
        await CodeInterpreter.create(sandbox=None)  # type: ignore[arg-type]


@pytest.mark.asyncio
async def test_create_wires_code_service_and_delegates_properties() -> None:
    sbx = _FakeSandbox()
    ci = await CodeInterpreter.create(sandbox=sbx)  # type: ignore[arg-type]

    assert ci.id == sbx.id
    assert ci.files is sbx.files
    assert ci.commands is sbx.commands
    assert ci.metrics is sbx.metrics

    # codes service should be present and callable (no network)
    assert ci.codes is not None
