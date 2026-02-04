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

import argparse
import os

import uvicorn

from src.config import CONFIG_ENV_VAR


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Run the OpenSandbox server.",
    )
    parser.add_argument(
        "--config",
        help="Path to the server config TOML file (overrides SANDBOX_CONFIG_PATH).",
    )
    parser.add_argument(
        "--reload",
        action="store_true",
        help="Enable auto-reload (development only).",
    )
    return parser


def main() -> None:
    parser = _build_parser()
    args = parser.parse_args()

    if args.config:
        os.environ[CONFIG_ENV_VAR] = args.config

    from src import main as server_main  # local import after env is set

    uvicorn.run(
        "src.main:app",
        host=server_main.app_config.server.host,
        port=server_main.app_config.server.port,
        reload=args.reload,
        log_config=server_main._log_config,
    )


if __name__ == "__main__":
    main()
