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

from unittest.mock import patch

import pytest
from fastapi import HTTPException

from opensandbox_server.services.docker_windows_profile import (
    apply_windows_runtime_host_config_defaults,
    escape_batch_env_value,
    resolve_windows_execd_download_url,
    validate_windows_runtime_prerequisites,
)


def test_apply_windows_runtime_host_config_defaults_injects_required_defaults():
    host_cfg = {"network_mode": "host"}
    updated = apply_windows_runtime_host_config_defaults(host_cfg, "sbx-1")

    assert updated["devices"] == ["/dev/kvm", "/dev/net/tun"]
    assert "NET_ADMIN" in updated["cap_add"]
    assert "opensandbox-win-storage-sbx-1:/storage:rw" in updated["binds"]
    assert "opensandbox-win-oem-sbx-1:/oem:rw" in updated["binds"]


def test_apply_windows_runtime_host_config_defaults_removes_net_admin_from_cap_drop():
    host_cfg = {
        "cap_drop": ["NET_ADMIN", "SYS_ADMIN"],
        "binds": ["/tmp/data:/data:rw"],
    }
    updated = apply_windows_runtime_host_config_defaults(host_cfg, "sbx-2")

    assert "NET_ADMIN" not in (updated.get("cap_drop") or [])
    assert "SYS_ADMIN" in (updated.get("cap_drop") or [])
    assert "/tmp/data:/data:rw" in updated["binds"]


def test_validate_windows_runtime_prerequisites_reports_missing_devices():
    with patch("opensandbox_server.services.docker_windows_profile.os.path.exists", return_value=False):
        with pytest.raises(HTTPException) as exc_info:
            validate_windows_runtime_prerequisites()

    assert exc_info.value.status_code == 400
    assert "/dev/kvm" in exc_info.value.detail["message"]
    assert "/dev/net/tun" in exc_info.value.detail["message"]


def test_resolve_windows_execd_download_url_prefers_request_env_override():
    url = resolve_windows_execd_download_url(
        {"EXECD_DOWNLOAD_URL": "https://example.com/custom.exe"},
        "ghcr.io/opensandbox/execd:v1.2.3",
        "arm64",
    )
    assert url == "https://example.com/custom.exe"


def test_resolve_windows_execd_download_url_uses_image_tag_and_arch():
    url = resolve_windows_execd_download_url(
        {},
        "ghcr.io/opensandbox/execd:v1.2.3",
        "arm64",
    )
    assert "docker%2Fexecd%2Fv1.2.3" in url
    assert url.endswith("execd_v1.2.3_windows_arm64.exe")


def test_resolve_windows_execd_download_url_handles_registry_port():
    url = resolve_windows_execd_download_url(
        {},
        "registry.internal:5000/opensandbox/execd:v9.9.9",
        "amd64",
    )
    assert "docker%2Fexecd%2Fv9.9.9" in url
    assert url.endswith("execd_v9.9.9_windows_amd64.exe")


def test_resolve_windows_execd_download_url_falls_back_when_tag_missing():
    url = resolve_windows_execd_download_url(
        {},
        "ghcr.io/opensandbox/execd",
        "x86_64",
    )
    assert "docker%2Fexecd%2Fv1.0.11" in url
    assert url.endswith("execd_v1.0.11_windows_amd64.exe")


def test_resolve_windows_execd_download_url_rejects_non_release_tag():
    with pytest.raises(HTTPException) as exc_info:
        resolve_windows_execd_download_url(
            {},
            "ghcr.io/opensandbox/execd:latest",
            "amd64",
        )
    assert exc_info.value.status_code == 400
    assert "release-like execd image tag" in exc_info.value.detail["message"]


def test_resolve_windows_execd_download_url_override_allows_non_release_tag():
    url = resolve_windows_execd_download_url(
        {"EXECD_DOWNLOAD_URL": "https://example.com/custom.exe"},
        "ghcr.io/opensandbox/execd:latest",
        "amd64",
    )
    assert url == "https://example.com/custom.exe"


def test_escape_batch_env_value_escapes_percent_signs():
    assert escape_batch_env_value("docker%2Fexecd") == "docker%%2Fexecd"
