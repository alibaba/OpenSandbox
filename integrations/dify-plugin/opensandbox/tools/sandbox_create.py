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

import shlex
from collections.abc import Generator
from typing import Any

from dify_plugin import Tool
from dify_plugin.entities.tool import ToolInvokeMessage
from loguru import logger
from opensandbox.exceptions import SandboxException
from opensandbox.sync.sandbox import SandboxSync

from tools.utils import build_connection_config, parse_minutes, parse_optional_json, parse_seconds


class SandboxCreateTool(Tool):
    def _invoke(self, tool_parameters: dict[str, Any]) -> Generator[ToolInvokeMessage]:
        image = tool_parameters.get("image", "")
        if not image:
            yield self.create_text_message("image is required.")
            return

        entrypoint_raw = tool_parameters.get("entrypoint")
        entrypoint = shlex.split(entrypoint_raw) if entrypoint_raw else None

        try:
            env = parse_optional_json(tool_parameters.get("env_json"), "env_json")
            metadata = parse_optional_json(tool_parameters.get("metadata_json"), "metadata_json")
            timeout = parse_minutes(tool_parameters.get("timeout_minutes"), default=10)
            ready_timeout = parse_seconds(tool_parameters.get("ready_timeout_seconds"), default=30)
        except ValueError as exc:
            yield self.create_text_message(str(exc))
            return

        config = build_connection_config(self.runtime.credentials)
        sandbox = None

        try:
            sandbox = SandboxSync.create(
                image,
                timeout=timeout,
                ready_timeout=ready_timeout,
                env=env,
                metadata=metadata,
                entrypoint=entrypoint,
                connection_config=config,
            )

            info = sandbox.get_info()
            payload = {
                "sandbox_id": sandbox.id,
                "state": info.status.state,
                "expires_at": info.expires_at.isoformat(),
            }
            yield self.create_variable_message("sandbox_id", sandbox.id)
            yield self.create_variable_message("state", info.status.state)
            yield self.create_variable_message("expires_at", info.expires_at.isoformat())
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