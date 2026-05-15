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

"""Tenant data models for multi-tenancy support."""

from __future__ import annotations

from pydantic import BaseModel, field_validator, model_validator

from opensandbox_server.services.validators import LABEL_VALUE_RE


class TenantEntry(BaseModel):
    """A tenant with API keys and a K8s namespace."""

    name: str
    namespace: str
    api_keys: list[str]

    @field_validator("name")
    @classmethod
    def _must_be_valid_k8s_label_value(cls, v: str) -> str:
        if not v:
            raise ValueError("Tenant name must not be empty.")
        if len(v) > 63:
            raise ValueError(
                f"Tenant name '{v}' is {len(v)} chars; must be ≤ 63 "
                f"(Kubernetes label value limit)."
            )
        if not LABEL_VALUE_RE.match(v):
            raise ValueError(
                f"Tenant name '{v}' is not a valid Kubernetes label value. "
                f"Must match [{LABEL_VALUE_RE.pattern}]."
            )
        return v


class TenantsConfig(BaseModel):
    """Top-level container for tenant entries loaded from tenants.toml."""

    entries: list[TenantEntry]

    @model_validator(mode="after")
    def _reject_duplicate_keys(self) -> "TenantsConfig":
        seen: dict[str, str] = {}
        for entry in self.entries:
            for k in entry.api_keys:
                if k in seen:
                    raise ValueError(
                        f"Duplicate api_key across tenants: "
                        f"'{seen[k]}' and '{entry.name}' both declare '{k}'"
                    )
                seen[k] = entry.name
        return self
