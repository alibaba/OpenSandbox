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

import posixpath
from unittest.mock import MagicMock, patch
from src.services.docker import DockerSandboxService, EXECED_INSTALL_PATH, BOOTSTRAP_PATH
from src.config import AppConfig, RuntimeConfig, ServerConfig

def _app_config() -> AppConfig:
    return AppConfig(
        server=ServerConfig(),
        runtime=RuntimeConfig(type="docker", execd_image="ghcr.io/opensandbox/platform:latest"),
    )

def test_container_internal_paths_use_posix_style():
    """Verify that container internal paths always use forward slashes."""
    assert "\\" not in EXECED_INSTALL_PATH
    assert "/" in EXECED_INSTALL_PATH
    assert "\\" not in BOOTSTRAP_PATH
    assert "/" in BOOTSTRAP_PATH
    assert EXECED_INSTALL_PATH == "/opt/opensandbox/execd"
    assert BOOTSTRAP_PATH == "/opt/opensandbox/bootstrap.sh"

@patch("src.services.docker.docker")
def test_copy_execd_to_container_uses_posix_dirname(mock_docker):
    """Verify _copy_execd_to_container uses posixpath for target directory."""
    service = DockerSandboxService(config=_app_config())
    mock_container = MagicMock()
    
    # Mock _fetch_execd_archive and _ensure_directory
    with patch.object(service, "_fetch_execd_archive", return_value=b"fake-archive"), \
         patch.object(service, "_ensure_directory") as mock_ensure_dir, \
         patch.object(service, "_docker_operation"):
        
        service._copy_execd_to_container(mock_container, "test-sandbox")
        
        # The target_parent should be posixpath.dirname(EXECED_INSTALL_PATH)
        expected_parent = posixpath.dirname(EXECED_INSTALL_PATH.rstrip("/")) or "/"
        mock_ensure_dir.assert_called_once_with(mock_container, expected_parent, "test-sandbox")

@patch("src.services.docker.docker")
def test_install_bootstrap_script_uses_posix_dirname(mock_docker):
    """Verify _install_bootstrap_script uses posixpath for script directory."""
    service = DockerSandboxService(config=_app_config())
    mock_container = MagicMock()
    
    with patch.object(service, "_ensure_directory") as mock_ensure_dir, \
         patch.object(service, "_docker_operation"):
        
        service._install_bootstrap_script(mock_container, "test-sandbox")
        
        # The script_dir should be posixpath.dirname(BOOTSTRAP_PATH)
        expected_dir = posixpath.dirname(BOOTSTRAP_PATH)
        mock_ensure_dir.assert_called_once_with(mock_container, expected_dir, "test-sandbox")
