# Copyright 2026 Alibaba Group Holding Ltd.
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

from __future__ import annotations

from collections.abc import Generator
from typing import Any

from dify_plugin import Tool
from dify_plugin.entities.tool import ToolInvokeMessage
from loguru import logger
from opensandbox.exceptions import SandboxException
from opensandbox.models.execd import RunCommandOpts
from opensandbox.sync.sandbox import SandboxSync

from tools.utils import build_connection_config


class SandboxRunTool(Tool):
    def _invoke(self, tool_parameters: dict[str, Any]) -> Generator[ToolInvokeMessage]:
        sandbox_id = tool_parameters.get("sandbox_id", "")
        command = tool_parameters.get("command", "")
        if not sandbox_id or not command:
            yield self.create_text_message("sandbox_id and command are required.")
            return

        background = bool(tool_parameters.get("background", False))
        working_directory = tool_parameters.get("working_directory") or None
        opts = RunCommandOpts(background=background, working_directory=working_directory)

        config = build_connection_config(self.runtime.credentials)
        sandbox = None

        try:
            sandbox = SandboxSync.connect(sandbox_id, connection_config=config)
            execution = sandbox.commands.run(command, opts=opts)

            stdout = "\n".join(msg.text for msg in execution.logs.stdout)
            stderr = "\n".join(msg.text for msg in execution.logs.stderr)
            payload = {
                "execution_id": execution.id,
                "stdout": stdout,
                "stderr": stderr,
            }
            if execution.id:
                yield self.create_variable_message("execution_id", execution.id)
            yield self.create_variable_message("stdout", stdout)
            yield self.create_variable_message("stderr", stderr)
            yield self.create_json_message(payload)
        except SandboxException as exc:
            logger.exception("OpenSandbox error")
            yield self.create_text_message(f"OpenSandbox error: {exc}")
        except Exception as exc:
            logger.exception("Unexpected error")
            yield self.create_text_message(f"Unexpected error: {exc}")
        finally:
            try:
                if sandbox is not None:
                    sandbox.close()
            except Exception:
                pass