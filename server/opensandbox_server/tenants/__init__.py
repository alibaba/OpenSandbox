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

from opensandbox_server.tenants.context import get_current_tenant, set_current_tenant
from opensandbox_server.tenants.file_provider import (
    DEFAULT_TENANTS_CONFIG_PATH,
    TENANTS_CONFIG_ENV_VAR,
    FileTenantProvider,
    _resolve_tenants_path,
)
from opensandbox_server.tenants.http_provider import (
    HTTPTenantProvider,
    HTTPTenantProviderConfig,
)
from opensandbox_server.tenants.models import TenantEntry
from opensandbox_server.tenants.provider import TenantProvider, TenantProviderUnavailable

__all__ = [
    "TenantEntry",
    "TenantProvider",
    "TenantProviderUnavailable",
    "FileTenantProvider",
    "HTTPTenantProvider",
    "HTTPTenantProviderConfig",
    "DEFAULT_TENANTS_CONFIG_PATH",
    "TENANTS_CONFIG_ENV_VAR",
    "get_current_tenant",
    "set_current_tenant",
    "_resolve_tenants_path",
]
