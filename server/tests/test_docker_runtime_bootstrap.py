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

import io
import tarfile
from unittest.mock import MagicMock, patch

from opensandbox_server.config import AppConfig, IngressConfig, RuntimeConfig, ServerConfig
from opensandbox_server.services.docker import DockerSandboxService
from opensandbox_server.services.docker.runtime import BOOTSTRAP_PATH, DEFAULT_EXECD_ENVS_PATH


def _app_config() -> AppConfig:
    return AppConfig(
        server=ServerConfig(),
        runtime=RuntimeConfig(type="docker", execd_image="ghcr.io/opensandbox/platform:latest"),
        ingress=IngressConfig(mode="direct"),
    )


def _extract_bootstrap_script(archive_bytes: bytes) -> str:
    with tarfile.open(fileobj=io.BytesIO(archive_bytes), mode="r:") as tar:
        member = tar.getmember(BOOTSTRAP_PATH.lstrip("/"))
        extracted = tar.extractfile(member)
        assert extracted is not None
        return extracted.read().decode("utf-8")


@patch("opensandbox_server.services.docker.docker_service.docker")
def test_install_bootstrap_script_sets_default_execd_envs(mock_docker):
    mock_docker.from_env.return_value = MagicMock()
    service = DockerSandboxService(config=_app_config())
    mock_container = MagicMock()

    with patch.object(service, "_ensure_directory") as mock_ensure_dir, patch.object(
        service, "_docker_operation"
    ):
        service._install_bootstrap_script(mock_container, "test-sandbox")

    mock_ensure_dir.assert_called_once()
    archive_bytes = mock_container.put_archive.call_args.kwargs["data"]
    script = _extract_bootstrap_script(archive_bytes)

    assert 'if [ -z "${EXECD_ENVS:-}" ]; then' in script
    assert f'EXECD_ENVS="{DEFAULT_EXECD_ENVS_PATH}"' in script
    assert 'export EXECD_ENVS' in script
    assert 'mkdir -p "$(dirname "$EXECD_ENVS")"' in script
    assert 'touch "$EXECD_ENVS"' in script
    assert 'exec "$@"' in script
