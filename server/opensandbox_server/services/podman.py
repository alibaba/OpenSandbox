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

"""
Podman-based implementation of SandboxService.

Inherits from DockerSandboxService and communicates with Podman through its
Docker-compatible API socket.  Only the handful of operations where Podman's
compat layer diverges from Docker are overridden here; everything else
(container lifecycle, image management, volume mounts, port mapping, egress
sidecar, bootstrap injection, …) is reused as-is from the parent class.
"""

import logging
import os
import sys
from typing import Dict, Optional

import docker as docker_mod
from urllib3.util.retry import Retry

from opensandbox_server.config import AppConfig, get_config
from opensandbox_server.services.docker import DockerSandboxService

logger = logging.getLogger(__name__)


class PodmanSandboxService(DockerSandboxService):
    """Sandbox service backed by Podman via the Docker-compatible API."""

    @classmethod
    def _supported_runtime_types(cls) -> tuple[str, ...]:
        return ("podman",)


    def __init__(self, config: Optional[AppConfig] = None):
        app_config = config or get_config()
        self._podman_base_url = self._resolve_podman_url(app_config)
        super().__init__(config=config)
        self._configure_retry_adapter()

    def _create_docker_client(self):
        """Create a Docker SDK client connected to the Podman compat socket."""
        kwargs: dict = {"timeout": self._api_timeout}
        if self._podman_base_url:
            kwargs["base_url"] = self._podman_base_url
            logger.info("Connecting to Podman at %s", self._podman_base_url)
            return docker_mod.DockerClient(**kwargs)
        # Fall back to environment / default detection.
        return super()._create_docker_client()

    def _configure_retry_adapter(self) -> None:
        """Patch the existing transport adapter to retry on idle disconnects.

        Podman (especially on Windows named pipes) may close idle connections
        earlier than Docker.  The Docker SDK reuses HTTP connections by default,
        so a subsequent API call on a stale connection hits
        ``RemoteDisconnected``.  Rather than replacing the transport adapter
        (which would break named-pipe support), we patch ``max_retries`` on
        the adapter that the Docker SDK already installed.
        """
        try:
            retry = Retry(total=3, connect=3, read=1, backoff_factor=0.1)
            adapter = self.docker_client.api.get_adapter("http+docker://")
            adapter.max_retries = retry
            logger.debug("Retry policy patched on existing Docker SDK adapter.")
        except Exception:  # noqa: BLE001
            logger.debug("Could not configure retry policy for Podman client.")

    @staticmethod
    def _resolve_podman_url(config: AppConfig) -> Optional[str]:
        """Return the Podman API URL without mutating ``os.environ``."""
        if os.environ.get("DOCKER_HOST"):
            return None

        socket_path = config.podman.socket_path
        if socket_path:
            return socket_path if "://" in socket_path else f"unix://{socket_path}"

        return PodmanSandboxService._detect_podman_socket()

    @staticmethod
    def _detect_podman_socket() -> Optional[str]:
        """Return the first reachable Podman API socket for the current platform."""
        if sys.platform == "win32":
            return _check_windows_pipe("podman-machine-default")

        if sys.platform == "darwin":
            home = os.environ.get("HOME", "")
            candidates = [
                f"{home}/.local/share/containers/podman/machine/podman.sock",
                f"{home}/.local/share/containers/podman/machine/qemu/podman.sock",
            ]
        else:  # linux
            xdg = os.environ.get(
                "XDG_RUNTIME_DIR",
                f"/run/user/{os.getuid()}",
            )
            candidates = [
                f"{xdg}/podman/podman.sock",  # rootless
                "/run/podman/podman.sock",  # rootful
            ]

        for path in candidates:
            if os.path.exists(path):
                return f"unix://{path}"

        return None

    def _connection_error_hint(self, error: Exception) -> str:
        """Return a Podman-specific hint when the API socket is unreachable."""
        msg = str(error)
        if isinstance(error, FileNotFoundError) or "No such file or directory" in msg:
            docker_host = os.environ.get("DOCKER_HOST", "")
            base = self._podman_base_url or docker_host
            return (
                " Podman API socket seems unavailable. "
                "Make sure Podman is installed and the socket is active "
                "(run 'systemctl --user start podman.socket' on Linux, "
                "or 'podman machine start' on macOS/Windows). "
                f"(current target='{base}')"
            )
        return ""

    def _update_container_labels(self, container, labels: Dict[str, str]) -> None:
        """Skip container label updates — Podman does not support this operation.

        Expiration is already tracked in-memory via ``_sandbox_expirations`` by
        ``_schedule_expiration()``.  The only consequence of skipping the label
        write is that a server restart after ``renew_expiration`` will fall back
        to the original expiration timestamp stored in the container label at
        creation time.  This is an acceptable degradation — the sandbox may
        expire earlier than the renewed time, matching the behaviour the parent
        class already tolerates when the label update fails (see the
        ``except (DockerException, TypeError)`` guard in ``renew_expiration``).
        """
        logger.debug(
            "Skipping container label update on Podman (not supported): %s",
            container.id[:12] if hasattr(container, "id") else "unknown",
        )


def _check_windows_pipe(pipe_name: str) -> Optional[str]:
    """Verify that a Windows named pipe exists by attempting to open it."""
    win_path = f"\\\\.\\pipe\\{pipe_name}"
    try:
        # Opening with os.open works for named pipes on Windows and is
        # cheap — we close immediately without reading.
        fd = os.open(win_path, os.O_RDONLY)
        os.close(fd)
    except OSError:
        return None
    # The Docker SDK expects forward slashes in the npipe:// URL.
    return f"npipe:////./pipe/{pipe_name}"
