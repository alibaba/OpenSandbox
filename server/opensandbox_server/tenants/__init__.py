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

"""Multi-tenancy support — config loading and namespace validation."""

from __future__ import annotations

import logging
import os
from pathlib import Path

from .models import TenantEntry, TenantsConfig  # noqa: F401

logger = logging.getLogger(__name__)

try:
    import tomllib
except ModuleNotFoundError:
    import tomli as tomllib  # type: ignore[import, no-redef]

DEFAULT_TENANTS_CONFIG_PATH = Path.home() / ".opensandbox" / "tenants.toml"
TENANTS_CONFIG_ENV_VAR = "SANDBOX_TENANTS_CONFIG_PATH"


def _resolve_path(path: str | Path | None = None) -> Path:
    if path is not None:
        return Path(path).expanduser()
    env_path = os.environ.get(TENANTS_CONFIG_ENV_VAR)
    if env_path:
        return Path(env_path).expanduser()
    return DEFAULT_TENANTS_CONFIG_PATH


def load_tenants_config(path: str | Path | None = None) -> TenantsConfig | None:
    """Return parsed TenantsConfig, or None if the file is absent (single-tenant mode)."""
    resolved = _resolve_path(path)
    if not resolved.exists():
        logger.info(f"tenants.toml not found at {resolved} — single-tenant mode.")
        return None
    data = tomllib.loads(resolved.read_text())
    raw_entries = data.get("tenants", [])
    entries = [
        TenantEntry(
            name=t["name"],
            namespace=t["namespace"],
            api_keys=t["api_keys"],
        )
        for t in raw_entries
    ]
    config = TenantsConfig(entries=entries)
    logger.info(f"Loaded {len(config.entries)} tenant(s) from {resolved}.")
    return config


def validate_namespaces(config: TenantsConfig, k8s_client) -> list[str]:
    """Check all tenant namespaces exist. Returns list of missing ones."""
    missing: list[str] = []
    core_v1 = k8s_client.get_core_v1_api()
    for entry in config.entries:
        try:
            core_v1.read_namespace(entry.namespace)
        except Exception:
            logger.warning(
                f"Tenant '{entry.name}' namespace '{entry.namespace}' not found or not accessible."
            )
            missing.append(entry.namespace)
    return missing
