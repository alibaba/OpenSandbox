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

from code_interpreter import CodeInterpreter, SupportedLanguage
from opensandbox import Sandbox
from opensandbox.config import ConnectionConfig

async def main() -> None:
    domain = os.getenv("SANDBOX_DOMAIN", "localhost:8080")
    api_key = os.getenv("SANDBOX_API_KEY")
    image = os.getenv("SANDBOX_IMAGE", "opensandbox/code-interpreter:latest")
    entry_point = "/opt/opensandbox/code-interpreter.sh"
    python_version = os.getenv("PYTHON_VERSION", "3.11")

    config = ConnectionConfig(
        domain=domain,
        api_key=api_key,
        request_timeout=timedelta(seconds=60),
    )

    sandbox = await Sandbox.create(
        image,
        connection_config=config,
        entrypoint=[entry_point],
        env={"PYTHON_VERSION": python_version},
    )

    async with sandbox:
        interpreter = await CodeInterpreter.create(sandbox=sandbox)
        ctx = await interpreter.codes.create_context(SupportedLanguage.PYTHON)

        execution = await interpreter.codes.run(
            "import platform\n"
            "print('hello from code-interpreter sandbox')\n"
            "result = {'py': platform.python_version(), 'sum': 1 + 1}\n"
            "result",
            context=ctx,
        )

        for msg in execution.logs.stdout:
            print(f"[stdout] {msg.text}")

        if execution.result:
            print(f"[result] {execution.result[0].text}")

        await interpreter.kill()


if __name__ == "__main__":
    asyncio.run(main())
