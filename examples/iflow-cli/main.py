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
from opensandbox.models.sandboxes import SandboxImageSpec


def _required_env(name: str) -> str:
    value = os.getenv(name)
    if not value:
        raise RuntimeError(f"{name} is required")
    return value


async def _print_execution_logs(execution) -> None:
    for msg in execution.logs.stdout:
        print(f"[stdout] {msg.text}")
    for msg in execution.logs.stderr:
        print(f"[stderr] {msg.text}")
    if execution.error:
        print(f"[error] {execution.error.name}: {execution.error.value}")


async def main() -> None:
    domain = os.getenv("SANDBOX_DOMAIN", "localhost:8080")
    api_key = os.getenv("SANDBOX_API_KEY")
    iflow_api_key = _required_env("IFLOW_API_KEY")
    iflow_base_url = os.getenv("IFLOW_BASE_URL", "https://apis.iflow.cn/v1")
    iflow_model_name = os.getenv("IFLOW_MODEL_NAME", "qwen3-coder-plus")
    image = os.getenv(
        "SANDBOX_IMAGE",
        "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:v1.0.1",
    )

    config = ConnectionConfig(
        domain=domain,
        api_key=api_key,
        request_timeout=timedelta(seconds=60),
    )

    sandbox = await Sandbox.create(
        image,
        connection_config=config,
        env={
            "IFLOW_apiKey": iflow_api_key,
            "IFLOW_baseUrl": iflow_base_url,
            "IFLOW_modelName": iflow_model_name,
        },
    )

    async with sandbox:
        # Install iFlow CLI (Node.js is already in the code-interpreter image)
        install_exec = await sandbox.commands.run(
            "npm install -g @iflow-ai/iflow-cli@latest"
        )
        await _print_execution_logs(install_exec)

        # Send a simple request via iFlow CLI
        run_exec = await sandbox.commands.run('iflow "Compute 1+1=?."')
        await _print_execution_logs(run_exec)

        await sandbox.kill()


if __name__ == "__main__":
    asyncio.run(main())
