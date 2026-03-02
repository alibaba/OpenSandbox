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

from datetime import timedelta
from uuid import uuid4

import pytest

from opensandbox.config.connection_sync import ConnectionConfigSync
from opensandbox.exceptions import SandboxReadyTimeoutException
from opensandbox.sync.sandbox import SandboxSync


class _Noop:
    pass


def test_sync_check_ready_timeout_message_includes_troubleshooting_hints() -> None:
    def _always_false(_: SandboxSync) -> bool:
        return False

    sbx = SandboxSync(
        sandbox_id=str(uuid4()),
        sandbox_service=_Noop(),
        filesystem_service=_Noop(),
        command_service=_Noop(),
        health_service=_Noop(),
        metrics_service=_Noop(),
        connection_config=ConnectionConfigSync(
            domain="10.0.0.2:8080",
            use_server_proxy=False,
        ),
        custom_health_check=_always_false,
    )

    with pytest.raises(SandboxReadyTimeoutException) as exc_info:
        sbx.check_ready(timeout=timedelta(seconds=0.01), polling_interval=timedelta(seconds=0))

    message = str(exc_info.value)
    assert "ConnectionConfig(domain=10.0.0.2:8080, use_server_proxy=False)" in message
    assert "ConnectionConfigSync(use_server_proxy=True)" in message
