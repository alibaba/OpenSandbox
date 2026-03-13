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

import json
import os
from datetime import timedelta
from typing import Any

from opensandbox.config.connection_sync import ConnectionConfigSync


def normalize_domain(base_url: str) -> str:
    base_url = base_url.strip().rstrip("/")
    if base_url.endswith("/v1"):
        base_url = base_url[:-3]
    return base_url


def build_connection_config(credentials: dict[str, Any]) -> ConnectionConfigSync:
    # Try credentials first, then fall back to environment variables
    base_url = credentials.get("opensandbox_base_url", "") or os.environ.get("OPENSANDBOX_BASE_URL", "")
    api_key = credentials.get("opensandbox_api_key", "") or os.environ.get("OPENSANDBOX_API_KEY", "")
    
    if not base_url:
        raise ValueError("opensandbox_base_url is required (via credentials or OPENSANDBOX_BASE_URL env var)")
    if not api_key:
        raise ValueError("opensandbox_api_key is required (via credentials or OPENSANDBOX_API_KEY env var)")
    
    return ConnectionConfigSync(
        domain=normalize_domain(base_url),
        api_key=api_key,
    )


def parse_optional_json(value: str | None, label: str) -> dict[str, str] | None:
    if value is None or value == "":
        return None
    try:
        parsed = json.loads(value)
    except json.JSONDecodeError as exc:
        raise ValueError(f"{label} must be valid JSON.") from exc
    if not isinstance(parsed, dict):
        raise ValueError(f"{label} must be a JSON object.")
    return {str(k): str(v) for k, v in parsed.items()}


def parse_minutes(value: Any | None, default: int) -> timedelta:
    if value in (None, ""):
        return timedelta(minutes=default)
    return timedelta(minutes=float(value))


def parse_seconds(value: Any | None, default: int) -> timedelta:
    if value in (None, ""):
        return timedelta(seconds=default)
    return timedelta(seconds=float(value))