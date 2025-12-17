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

import asyncio
import os
from datetime import timedelta

from opensandbox import Sandbox
from opensandbox.config import ConnectionConfig


async def main() -> None:
    domain = os.getenv("SANDBOX_DOMAIN", "localhost:8080")
    api_key = os.getenv("SANDBOX_API_KEY")
    image = os.getenv("SANDBOX_IMAGE", "ubuntu:22.04")

    config = ConnectionConfig(
        domain=domain,
        api_key=api_key,
        request_timeout=timedelta(seconds=60),
    )

    sandbox = await Sandbox.create(
        image,
        connection_config=config,
    )

    async with sandbox:
        # Execute a simple echo command
        execution = await sandbox.commands.run("echo 'Hello, OpenSandbox!'")

        # Print the output
        for msg in execution.logs.stdout:
            print(msg.text)
        for msg in execution.logs.stderr:
            print(f"[stderr] {msg.text}")

        if execution.error:
            print(f"[error] {execution.error.name}: {execution.error.value}")

        await sandbox.kill()


if __name__ == "__main__":
    asyncio.run(main())
