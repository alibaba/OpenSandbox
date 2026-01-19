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
Docker-based implementation of SandboxService.

This module provides a Docker implementation of the sandbox service interface,
using Docker containers for sandbox lifecycle management.
"""

from __future__ import annotations

import inspect
import io
import math
import logging
import os
import tarfile
import time
import socket
import random
from contextlib import contextmanager
from datetime import datetime, timedelta, timezone
from dataclasses import dataclass
from threading import Lock, Timer
from typing import Any, Dict, Optional
from uuid import uuid4

import docker
from docker.errors import DockerException, ImageNotFound
from fastapi import HTTPException, status

from src.api.schema import (
    CreateSandboxRequest,
    CreateSandboxResponse,
    Endpoint,
    ImageSpec,
    ListSandboxesRequest,
    ListSandboxesResponse,
    PaginationInfo,
    RenewSandboxExpirationRequest,
    RenewSandboxExpirationResponse,
    Sandbox,
    SandboxStatus,
)
from src.config import AppConfig, get_config
from src.services.constants import (
    SANDBOX_EXPIRES_AT_LABEL,
    SANDBOX_ID_LABEL,
    SANDBOX_EMBEDDING_PROXY_PORT_LABEL,
    SANDBOX_HTTP_PORT_LABEL,
    SandboxErrorCodes,
)
from src.services.helpers import (
    matches_filter,
    parse_memory_limit,
    parse_nano_cpus,
    parse_timestamp,
)
from src.services.sandbox_service import SandboxService
from src.services.validators import ensure_entrypoint, ensure_future_expiration, ensure_metadata_labels

logger = logging.getLogger(__name__)


def _resolve_docker_timeout(default: int = 180) -> int:
    env_value = os.environ.get("DOCKER_API_TIMEOUT")
    if not env_value:
        return default
    try:
        timeout = int(env_value)
        if timeout <= 0:
            raise ValueError
        return timeout
    except ValueError:
        logger.warning("Invalid DOCKER_API_TIMEOUT='%s'; falling back to %s seconds.", env_value, default)
        return default

OPENSANDBOX_DIR = "/opt/opensandbox"
EXECED_INSTALL_PATH = os.path.join(OPENSANDBOX_DIR, "execd")
BOOTSTRAP_PATH = os.path.join(OPENSANDBOX_DIR, "bootstrap.sh")

HOST_NETWORK_MODE = "host"
BRIDGE_NETWORK_MODE = "bridge"
PENDING_FAILURE_TTL_SECONDS = int(os.environ.get("PENDING_FAILURE_TTL", "3600"))
DOCKER_CLIENT_TIMEOUT = _resolve_docker_timeout()


@dataclass
class PendingSandbox:
    request: CreateSandboxRequest
    created_at: datetime
    expires_at: datetime
    status: SandboxStatus

class DockerSandboxService(SandboxService):
    """
    Docker-based implementation of SandboxService.

    This class implements sandbox lifecycle operations using Docker containers.
    """

    def __init__(self, config: Optional[AppConfig] = None):
        """
        Initialize Docker sandbox service.

        Initializes Docker service from environment variables.
        The service will read configuration from:
        - DOCKER_HOST: Docker daemon URL (e.g., 'unix://var/run/docker.sock' or 'tcp://127.0.0.1:2376')
        - DOCKER_TLS_CERTDIR: Directory containing TLS certificates
        - Other Docker environment variables as needed

        Note: Connection is not verified at initialization time.
        Connection errors will be raised when Docker operations are performed.
        """
        self.app_config = config or get_config()
        runtime_config = self.app_config.runtime
        if runtime_config.type != "docker":
            raise ValueError("DockerSandboxService requires runtime.type = 'docker'.")

        self.execd_image = runtime_config.execd_image
        self.network_mode = (self.app_config.docker.network_mode or HOST_NETWORK_MODE).lower()
        if self.network_mode not in {HOST_NETWORK_MODE, BRIDGE_NETWORK_MODE}:
            raise ValueError(f"Unsupported Docker network_mode '{self.network_mode}'.")
        self._execd_archive_cache: Optional[bytes] = None
        try:
            # Initialize Docker service from environment variables
            client_kwargs = {}
            try:
                signature = inspect.signature(docker.from_env)
                if "timeout" in signature.parameters:
                    client_kwargs["timeout"] = DOCKER_CLIENT_TIMEOUT
            except (ValueError, TypeError):
                logger.debug("Unable to introspect docker.from_env signature; using default parameters.")
            self.docker_client = docker.from_env(**client_kwargs)
            if not client_kwargs:
                try:
                    self.docker_client.api.timeout = DOCKER_CLIENT_TIMEOUT
                except AttributeError:
                    logger.debug("Docker client API does not expose timeout attribute.")
            logger.info("Docker service initialized from environment")
        except Exception as e:  # noqa: BLE001
            # Common failure mode on macOS/dev machines: Docker daemon not running or socket path wrong.
            hint = ""
            msg = str(e)
            if isinstance(e, FileNotFoundError) or "No such file or directory" in msg:
                docker_host = os.environ.get("DOCKER_HOST", "")
                hint = (
                    " Docker daemon seems unavailable (unix socket not found). "
                    "Make sure Docker Desktop (or Colima/Rancher Desktop) is running. "
                    "If you use Colima on macOS, you may need to set "
                    "DOCKER_HOST=unix://${HOME}/.colima/default/docker.sock before starting the server. "
                    f"(current DOCKER_HOST='{docker_host}')"
                )
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail={
                    "code": SandboxErrorCodes.DOCKER_INITIALIZATION_ERROR,
                    "message": f"Failed to initialize Docker service: {str(e)}.{hint}",
                },
            )
        self._expiration_lock = Lock()
        self._execd_archive_lock = Lock()
        self._sandbox_expirations: Dict[str, datetime] = {}
        self._expiration_timers: Dict[str, Timer] = {}
        self._pending_sandboxes: Dict[str, PendingSandbox] = {}
        self._pending_lock = Lock()
        self._pending_cleanup_timers: Dict[str, Timer] = {}
        self._restore_existing_sandboxes()

    @contextmanager
    def _docker_operation(self, action: str, sandbox_id: Optional[str] = None):
        """Context manager to log duration for Docker API calls."""
        op_id = sandbox_id or "shared"
        start = time.perf_counter()
        try:
            yield
        except Exception as exc:
            elapsed_ms = (time.perf_counter() - start) * 1000
            logger.warning(
                "sandbox=%s | action=%s | duration=%.2f | error=%s",
                op_id,
                action,
                elapsed_ms,
                exc,
            )
            raise
        else:
            elapsed_ms = (time.perf_counter() - start) * 1000
            logger.info(
                "sandbox=%s | action=%s | duration=%.2f",
                op_id,
                action,
                elapsed_ms,
            )

    def _get_container_by_sandbox_id(self, sandbox_id: str):
        """Helper to fetch the Docker container associated with a sandbox ID."""
        label_selector = f"{SANDBOX_ID_LABEL}={sandbox_id}"
        try:
            containers = self.docker_client.containers.list(all=True, filters={"label": label_selector})
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.CONTAINER_QUERY_FAILED,
                    "message": f"Failed to query sandbox containers: {str(exc)}",
                },
            ) from exc

        if not containers:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail={
                    "code": SandboxErrorCodes.SANDBOX_NOT_FOUND,
                    "message": f"Sandbox {sandbox_id} not found.",
                },
            )

        return containers[0]

    def _schedule_expiration(self, sandbox_id: str, expires_at: datetime) -> None:
        """Schedule automatic sandbox termination at expiration time."""
        # Delay might already be negative if the timer should fire immediately
        delay = max(0.0, (expires_at - datetime.now(timezone.utc)).total_seconds())
        timer = Timer(delay, self._expire_sandbox, args=(sandbox_id,))
        timer.daemon = True
        with self._expiration_lock:
            # Replace existing timer (if any) so renew operations take effect immediately
            existing = self._expiration_timers.pop(sandbox_id, None)
            if existing:
                existing.cancel()
            self._sandbox_expirations[sandbox_id] = expires_at
            self._expiration_timers[sandbox_id] = timer
        timer.start()

    def _remove_expiration_tracking(self, sandbox_id: str) -> None:
        """Remove expiration tracking and cancel any pending timers."""
        with self._expiration_lock:
            timer = self._expiration_timers.pop(sandbox_id, None)
            if timer:
                timer.cancel()
            self._sandbox_expirations.pop(sandbox_id, None)

    def _get_tracked_expiration(
        self,
        sandbox_id: str,
        labels: Dict[str, str],
        fallback: datetime,
    ) -> datetime:
        """Return the known expiration timestamp for the sandbox."""
        with self._expiration_lock:
            tracked = self._sandbox_expirations.get(sandbox_id)
        if tracked:
            return tracked
        label_value = labels.get(SANDBOX_EXPIRES_AT_LABEL)
        if label_value:
            return parse_timestamp(label_value)
        return fallback

    def _expire_sandbox(self, sandbox_id: str) -> None:
        """Timer callback to terminate expired sandboxes."""
        try:
            container = self._get_container_by_sandbox_id(sandbox_id)
        except HTTPException as exc:
            if exc.status_code != status.HTTP_404_NOT_FOUND:
                logger.warning("Failed to fetch sandbox %s for expiration: %s", sandbox_id, exc.detail)
            self._remove_expiration_tracking(sandbox_id)
            return

        try:
            state = container.attrs.get("State", {})
            if state.get("Running", False):
                container.kill()
        except DockerException as exc:
            logger.warning("Failed to stop expired sandbox %s: %s", sandbox_id, exc)

        try:
            container.remove(force=True)
        except DockerException as exc:
            logger.warning("Failed to remove expired sandbox %s: %s", sandbox_id, exc)

        self._remove_expiration_tracking(sandbox_id)

    def _restore_existing_sandboxes(self) -> None:
        """On startup, rebuild expiration timers for containers already running."""
        try:
            containers = self.docker_client.containers.list(
                all=True,
                filters={"label": [SANDBOX_ID_LABEL]},
            )
        except DockerException as exc:
            logger.warning("Failed to restore existing sandboxes: %s", exc)
            return

        restored = 0
        now = datetime.now(timezone.utc)
        for container in containers:
            labels = container.attrs.get("Config", {}).get("Labels") or {}
            sandbox_id = labels.get(SANDBOX_ID_LABEL)
            if not sandbox_id:
                continue
            # Sandbox IDs now follow standard UUID4 format (hyphenated strings)
            expires_label = labels.get(SANDBOX_EXPIRES_AT_LABEL)
            if expires_label:
                expires_at = parse_timestamp(expires_label)
            else:
                logger.warning(
                    "Sandbox %s missing expires-at label; skipping expiration scheduling.",
                    sandbox_id,
                )
                continue

            if expires_at <= now:
                logger.info("Sandbox %s already expired; terminating now.", sandbox_id)
                self._expire_sandbox(sandbox_id)
                continue

            self._schedule_expiration(sandbox_id, expires_at)
            restored += 1

        if restored:
            logger.info("Restored expiration timers for %d sandbox(es).", restored)

    def _fetch_execd_archive(self) -> bytes:
        """Fetch (and memoize) the execd archive from the platform container."""
        if self._execd_archive_cache is not None:
            return self._execd_archive_cache

        with self._execd_archive_lock:
            # Double-check locking to ensure only one thread initializes the cache
            if self._execd_archive_cache is not None:
                return self._execd_archive_cache

            container = None
            try:
                try:
                    # Prefer a locally built image (e.g., opensandbox/execd:local); pull only if missing.
                    self.docker_client.images.get(self.execd_image)
                    logger.info("Found execd image %s locally; skipping pull", self.execd_image)
                except ImageNotFound:
                    with self._docker_operation(
                        f"pull execd image {self.execd_image}",
                        "execd-cache",
                    ):
                        self.docker_client.images.pull(self.execd_image)

                with self._docker_operation("execd cache create container", "execd-cache"):
                    container = self.docker_client.containers.create(
                        image=self.execd_image,
                        command=["tail", "-f", "/dev/null"],
                        name=f"sandbox-execd-{uuid4()}",
                        detach=True,
                        auto_remove=False,
                    )
                with self._docker_operation("execd cache start container", "execd-cache"):
                    container.start()
                    container.reload()
                    logger.info("Created sandbox execd archive for container %s", container.id)
            except DockerException as exc:
                raise HTTPException(
                    status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                    detail={
                        "code": SandboxErrorCodes.EXECD_START_FAILED,
                        "message": f"Failed to start execd container: {str(exc)}",
                    },
                ) from exc

            try:
                with self._docker_operation("execd cache read archive", "execd-cache"):
                    stream, _ = container.get_archive("/execd")
                    data = b"".join(stream)
            except DockerException as exc:
                raise HTTPException(
                    status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                    detail={
                        "code": SandboxErrorCodes.EXECD_DISTRIBUTION_FAILED,
                        "message": f"Failed to read execd artifacts: {str(exc)}",
                    },
                ) from exc
            finally:
                if container:
                    try:
                        with self._docker_operation("execd cache cleanup container", "execd-cache"):
                            container.remove(force=True)
                    except DockerException as cleanup_exc:
                        logger.warning("Failed to cleanup temporary execd container: %s", cleanup_exc)

            self._execd_archive_cache = data
            logger.info("Dumped execd archive to memory")
            return data

    def _container_to_sandbox(self, container, sandbox_id: Optional[str] = None) -> Sandbox:
        labels = container.attrs.get("Config", {}).get("Labels") or {}
        resolved_id = sandbox_id or labels.get(SANDBOX_ID_LABEL)
        if not resolved_id:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.SANDBOX_NOT_FOUND,
                    "message": "Container missing sandbox ID label.",
                },
            )

        status_section = container.attrs.get("State", {})
        status_value = (status_section.get("Status") or container.status or "").lower()
        running = status_section.get("Running", False)
        paused = status_section.get("Paused", False)
        restarting = status_section.get("Restarting", False)
        exit_code = status_section.get("ExitCode")
        finished_at = status_section.get("FinishedAt")

        if running and not paused:
            state = "Running"
            reason = "CONTAINER_RUNNING"
            message = "Sandbox container is running."
        elif paused:
            state = "Paused"
            reason = "CONTAINER_PAUSED"
            message = "Sandbox container is paused."
        elif restarting:
            state = "Running"
            reason = "CONTAINER_RESTARTING"
            message = "Sandbox container is restarting."
        elif status_value in {"created", "starting"}:
            state = "Pending"
            reason = "CONTAINER_STARTING"
            message = "Sandbox container is starting."
        elif status_value in {"exited", "dead"}:
            if exit_code == 0:
                state = "Terminated"
                reason = "CONTAINER_EXITED"
                message = "Sandbox container exited successfully."
            else:
                state = "Failed"
                reason = "CONTAINER_EXITED_ERROR"
                message = f"Sandbox container exited with code {exit_code}."
        else:
            state = "Unknown"
            reason = "CONTAINER_STATE_UNKNOWN"
            message = f"Sandbox container is in state '{status_value or 'unknown'}'."

        metadata = {
            key: value
            for key, value in labels.items()
            if key not in {SANDBOX_ID_LABEL, SANDBOX_EXPIRES_AT_LABEL}
        } or None
        entrypoint = container.attrs.get("Config", {}).get("Cmd") or []
        if isinstance(entrypoint, str):
            entrypoint = [entrypoint]
        image_tags = container.image.tags
        image_uri = image_tags[0] if image_tags else container.image.short_id
        image_spec = ImageSpec(uri=image_uri)

        created_at = parse_timestamp(container.attrs.get("Created"))
        last_transition_at = (
            parse_timestamp(finished_at) if finished_at and finished_at != "0001-01-01T00:00:00Z" else created_at
        )
        expires_at = self._get_tracked_expiration(resolved_id, labels, created_at)

        status_info = SandboxStatus(
            state=state,
            reason=reason,
            message=message,
            last_transition_at=last_transition_at,
        )

        return Sandbox(
            id=resolved_id,
            image=image_spec,
            status=status_info,
            metadata=metadata,
            entrypoint=entrypoint,
            expiresAt=expires_at,
            createdAt=created_at,
        )

    def _ensure_directory(self, container, path: str, sandbox_id: Optional[str] = None) -> None:
        """Create a directory within the target container if it does not exist."""
        if not path or path == "/":
            return
        normalized_path = path.rstrip("/")
        if not normalized_path:
            return
        tar_stream = io.BytesIO()
        with tarfile.open(fileobj=tar_stream, mode="w") as tar:
            dir_info = tarfile.TarInfo(name=normalized_path.lstrip("/"))
            dir_info.type = tarfile.DIRTYPE
            dir_info.mode = 0o755
            dir_info.mtime = int(time.time())
            tar.addfile(dir_info)
        tar_stream.seek(0)
        try:
            with self._docker_operation(f"ensure directory {normalized_path}", sandbox_id):
                container.put_archive(path="/", data=tar_stream.getvalue())
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.EXECD_DISTRIBUTION_FAILED,
                    "message": f"Failed to create directory {path} in sandbox: {str(exc)}",
                },
            ) from exc

    def _copy_execd_to_container(self, container, sandbox_id: str) -> None:
        """Copy execd artifacts from the platform container into the sandbox."""
        archive = self._fetch_execd_archive()
        target_parent = os.path.dirname(EXECED_INSTALL_PATH.rstrip("/")) or "/"
        self._ensure_directory(container, target_parent, sandbox_id)
        try:
            with self._docker_operation("copy execd archive to sandbox", sandbox_id):
                container.put_archive(path=target_parent, data=archive)
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.EXECD_DISTRIBUTION_FAILED,
                    "message": f"Failed to copy execd into sandbox: {str(exc)}",
                },
            ) from exc

    def _install_bootstrap_script(self, container, sandbox_id: str) -> None:
        """Install the bootstrap launcher that starts execd then chains to user command."""
        script_path = BOOTSTRAP_PATH
        script_dir = os.path.dirname(script_path)
        self._ensure_directory(container, script_dir, sandbox_id)
        execd_binary = EXECED_INSTALL_PATH
        script_content = "\n".join(
            [
                "#!/bin/sh",
                "set -e",
                f"{execd_binary} >/tmp/execd.log 2>&1 &",
                'exec "$@"',
                "",
            ]
        ).encode("utf-8")

        tar_stream = io.BytesIO()
        with tarfile.open(fileobj=tar_stream, mode="w") as tar:
            info = tarfile.TarInfo(name=script_path.lstrip("/"))
            info.mode = 0o755
            info.size = len(script_content)
            info.mtime = int(time.time())
            tar.addfile(info, io.BytesIO(script_content))
        tar_stream.seek(0)
        try:
            with self._docker_operation("install bootstrap script", sandbox_id):
                container.put_archive(path="/", data=tar_stream.getvalue())
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.BOOTSTRAP_INSTALL_FAILED,
                    "message": f"Failed to install bootstrap launcher: {str(exc)}",
                },
            ) from exc

    def _prepare_sandbox_runtime(self, container, sandbox_id: str) -> None:
        """Copy execd artifacts and bootstrap launcher into the sandbox container."""
        self._copy_execd_to_container(container, sandbox_id)
        self._install_bootstrap_script(container, sandbox_id)

    def _prepare_creation_context(
        self,
        request: CreateSandboxRequest,
    ) -> tuple[str, datetime, datetime]:
        sandbox_id = self.generate_sandbox_id()
        created_at = datetime.now(timezone.utc)
        expires_at = created_at + timedelta(seconds=request.timeout)
        return sandbox_id, created_at, expires_at

    @staticmethod
    def _allocate_host_port(min_port: int = 40000, max_port: int = 60000, attempts: int = 50) -> Optional[int]:
        """Find an available TCP port on the host within the given range."""
        for _ in range(attempts):
            port = random.randint(min_port, max_port)
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
                sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                try:
                    sock.bind(("0.0.0.0", port))
                except OSError:
                    continue
                return port
        return None

    def create_sandbox(self, request: CreateSandboxRequest) -> CreateSandboxResponse:
        """
        Create a new sandbox from a container image using Docker.

        Args:
            request: Sandbox creation request

        Returns:
            CreateSandboxResponse: Created sandbox information

        Raises:
            HTTPException: If sandbox creation fails
        """
        ensure_entrypoint(request.entrypoint)
        ensure_metadata_labels(request.metadata)
        sandbox_id, created_at, expires_at = self._prepare_creation_context(request)
        return self._provision_sandbox(sandbox_id, request, created_at, expires_at)

    def _async_provision_worker(
        self,
        sandbox_id: str,
        request: CreateSandboxRequest,
        created_at: datetime,
        expires_at: datetime,
    ) -> None:
        try:
            self._provision_sandbox(sandbox_id, request, created_at, expires_at)
        except HTTPException as exc:
            message = exc.detail.get("message") if isinstance(exc.detail, dict) else str(exc)
            self._mark_pending_failed(sandbox_id, message or "Sandbox provisioning failed.")
            self._cleanup_failed_containers(sandbox_id)
            self._schedule_pending_cleanup(sandbox_id)
        except Exception as exc:  # noqa: BLE001
            logger.exception("Unexpected error provisioning sandbox %s: %s", sandbox_id, exc)
            self._mark_pending_failed(sandbox_id, str(exc))
            self._cleanup_failed_containers(sandbox_id)
            self._schedule_pending_cleanup(sandbox_id)
        else:
            self._remove_pending_sandbox(sandbox_id)

    def _mark_pending_failed(self, sandbox_id: str, message: str) -> None:
        with self._pending_lock:
            pending = self._pending_sandboxes.get(sandbox_id)
            if not pending:
                return
            pending.status = SandboxStatus(
                state="Failed",
                reason="PROVISIONING_ERROR",
                message=message,
                last_transition_at=datetime.now(timezone.utc),
            )

    def _cleanup_failed_containers(self, sandbox_id: str) -> None:
        """
        Best-effort cleanup for containers left behind after a failed provision.
        """
        label_selector = f"{SANDBOX_ID_LABEL}={sandbox_id}"
        try:
            containers = self.docker_client.containers.list(all=True, filters={"label": label_selector})
        except DockerException as exc:
            logger.warning("sandbox=%s | cleanup listing failed containers: %s", sandbox_id, exc)
            return

        for container in containers:
            try:
                with self._docker_operation("cleanup failed sandbox container", sandbox_id):
                    container.remove(force=True)
            except DockerException as exc:
                logger.warning("sandbox=%s | failed to remove leftover container %s: %s", sandbox_id, container.id, exc)

    def _remove_pending_sandbox(self, sandbox_id: str) -> None:
        with self._pending_lock:
            timer = self._pending_cleanup_timers.pop(sandbox_id, None)
            if timer:
                timer.cancel()
            self._pending_sandboxes.pop(sandbox_id, None)

    def _get_pending_sandbox(self, sandbox_id: str) -> Optional[PendingSandbox]:
        with self._pending_lock:
            pending = self._pending_sandboxes.get(sandbox_id)
            return pending

    def _iter_pending_sandboxes(self) -> list[tuple[str, PendingSandbox]]:
        with self._pending_lock:
            return list(self._pending_sandboxes.items())

    @staticmethod
    def _pending_to_sandbox(sandbox_id: str, pending: PendingSandbox) -> Sandbox:
        return Sandbox(
            id=sandbox_id,
            image=pending.request.image,
            status=pending.status,
            metadata=pending.request.metadata,
            entrypoint=pending.request.entrypoint,
            expiresAt=pending.expires_at,
            createdAt=pending.created_at,
        )

    def _update_container_labels(self, container, labels: Dict[str, str]) -> None:
        """
        Update container labels, falling back to raw API if docker-py lacks support.
        """
        try:
            container.update(labels=labels)
        except TypeError:
            # Older docker-py versions do not accept labels; call low-level API directly.
            url = self.docker_client.api._url(f"/containers/{container.id}/update")  # noqa: SLF001
            data = {"Labels": labels}
            self.docker_client.api._post_json(url, data=data)  # noqa: SLF001
        container.reload()

    def _schedule_pending_cleanup(self, sandbox_id: str) -> None:
        def _cleanup():
            self._remove_pending_sandbox(sandbox_id)

        timer = Timer(PENDING_FAILURE_TTL_SECONDS, _cleanup)
        timer.daemon = True
        with self._pending_lock:
            existing = self._pending_cleanup_timers.pop(sandbox_id, None)
            if existing:
                existing.cancel()
            self._pending_cleanup_timers[sandbox_id] = timer
        timer.start()

    def _pull_image(
        self,
        image_uri: str,
        auth_config: Optional[dict],
        sandbox_id: str,
    ) -> None:
        try:
            with self._docker_operation(f"pull image {image_uri}", sandbox_id):
                self.docker_client.images.pull(image_uri, auth_config=auth_config)
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.IMAGE_PULL_FAILED,
                    "message": f"Failed to pull image {image_uri}: {str(exc)}",
                },
            ) from exc

    def _ensure_image_available(
        self,
        image_uri: str,
        auth_config: Optional[dict],
        sandbox_id: str,
    ) -> None:
        try:
            with self._docker_operation(f"inspect image {image_uri}", sandbox_id):
                self.docker_client.images.get(image_uri)
                logger.debug("Sandbox %s using cached image %s", sandbox_id, image_uri)
        except ImageNotFound:
            self._pull_image(image_uri, auth_config, sandbox_id)
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.IMAGE_PULL_FAILED,
                    "message": f"Failed to inspect image {image_uri}: {str(exc)}",
                },
            ) from exc

    def _provision_sandbox(
        self,
        sandbox_id: str,
        request: CreateSandboxRequest,
        created_at: datetime,
        expires_at: datetime,
    ) -> CreateSandboxResponse:
        metadata = request.metadata or {}
        labels = {key: str(value) for key, value in metadata.items()}
        labels[SANDBOX_ID_LABEL] = sandbox_id

        env_dict = request.env or {}
        environment = []
        for key, value in env_dict.items():
            if value is None:
                continue
            environment.append(f"{key}={value}")

        image_uri = request.image.uri
        auth_config = None
        if request.image.auth:
            auth_config = {
                "username": request.image.auth.username,
                "password": request.image.auth.password,
            }

        self._ensure_image_available(image_uri, auth_config, sandbox_id)

        resource_limits = request.resource_limits.root or {}
        mem_limit = parse_memory_limit(resource_limits.get("memory"))
        nano_cpus = parse_nano_cpus(resource_limits.get("cpu"))

        host_config_kwargs: Dict[str, Any] = {"network_mode": self.network_mode}
        security_opts: list[str] = []
        docker_cfg = self.app_config.docker
        if docker_cfg.no_new_privileges:
            security_opts.append("no-new-privileges:true")
        if docker_cfg.apparmor_profile:
            security_opts.append(f"apparmor={docker_cfg.apparmor_profile}")
        if docker_cfg.seccomp_profile:
            security_opts.append(f"seccomp={docker_cfg.seccomp_profile}")
        if security_opts:
            host_config_kwargs["security_opt"] = security_opts
        if docker_cfg.drop_capabilities:
            host_config_kwargs["cap_drop"] = docker_cfg.drop_capabilities
        if docker_cfg.pids_limit is not None:
            host_config_kwargs["pids_limit"] = docker_cfg.pids_limit
        if mem_limit:
            host_config_kwargs["mem_limit"] = mem_limit
        if nano_cpus:
            host_config_kwargs["nano_cpus"] = nano_cpus

        exposed_ports: Optional[list[str]] = None
        if self.network_mode == BRIDGE_NETWORK_MODE:
            host_execd_port = self._allocate_host_port()
            host_http_port = self._allocate_host_port()
            if host_execd_port is None or host_http_port is None:
                raise HTTPException(
                    status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                    detail={
                        "code": SandboxErrorCodes.CONTAINER_START_FAILED,
                        "message": "Failed to allocate host ports for sandbox container.",
                    },
                )
            # ensure distinct ports
            while host_http_port == host_execd_port:
                host_http_port = self._allocate_host_port()
                if host_http_port is None:
                    raise HTTPException(
                        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                        detail={
                            "code": SandboxErrorCodes.CONTAINER_START_FAILED,
                            "message": "Failed to allocate distinct host ports for sandbox container.",
                        },
                    )

            port_bindings = {
                "44772": ("0.0.0.0", host_execd_port),
                "8080": ("0.0.0.0", host_http_port),
            }
            host_config_kwargs["port_bindings"] = port_bindings
            exposed_ports = list(port_bindings.keys())
            labels[SANDBOX_EMBEDDING_PROXY_PORT_LABEL] = str(host_execd_port)
            labels[SANDBOX_HTTP_PORT_LABEL] = str(host_http_port)

        labels[SANDBOX_EXPIRES_AT_LABEL] = expires_at.isoformat()

        container_name = f"sandbox-{sandbox_id}"

        # Keep user command in CMD and override container ENTRYPOINT with our bootstrap.
        # This avoids relying on the base image ENTRYPOINT (which may be a non-shell binary).
        bootstrap_command = request.entrypoint

        host_config = self.docker_client.api.create_host_config(**host_config_kwargs)
        container_id: Optional[str] = None
        container = None
        try:
            with self._docker_operation("create sandbox container", sandbox_id):
                response = self.docker_client.api.create_container(
                    image=image_uri,
                    entrypoint=[BOOTSTRAP_PATH],
                    command=bootstrap_command,
                    ports=exposed_ports,
                    name=container_name,
                    environment=environment,
                    labels=labels,
                    host_config=host_config,
                )
            container_id = response.get("Id")
            if not container_id:
                raise HTTPException(
                    status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                    detail={
                        "code": SandboxErrorCodes.CONTAINER_START_FAILED,
                        "message": "Docker did not return a container ID.",
                    },
                )
            container = self.docker_client.containers.get(container_id)
            self._prepare_sandbox_runtime(container, sandbox_id)
            with self._docker_operation("start sandbox container", sandbox_id):
                container.start()
        except DockerException as exc:
            if container is not None:
                try:
                    with self._docker_operation("cleanup sandbox container", sandbox_id):
                        container.remove(force=True)
                except DockerException as cleanup_exc:
                    logger.warning(
                        "Failed to cleanup container for sandbox %s: %s",
                        sandbox_id,
                        cleanup_exc,
                    )
            elif container_id:
                try:
                    with self._docker_operation("cleanup sandbox container (API)", sandbox_id):
                        self.docker_client.api.remove_container(container_id, force=True)
                except DockerException as cleanup_exc:
                    logger.warning(
                        "Failed to cleanup container for sandbox %s: %s",
                        sandbox_id,
                        cleanup_exc,
                    )
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.CONTAINER_START_FAILED,
                    "message": f"Failed to create or start container: {str(exc)}",
                },
            ) from exc

        status_info = SandboxStatus(
            state="Running",
            reason="CONTAINER_RUNNING",
            message="Sandbox container started successfully.",
            last_transition_at=created_at,
        )

        # Track timeout so the sandbox is cleaned up automatically
        self._schedule_expiration(sandbox_id, expires_at)

        return CreateSandboxResponse(
            id=sandbox_id,
            status=status_info,
            metadata=request.metadata,
            expiresAt=expires_at,
            createdAt=created_at,
            entrypoint=request.entrypoint,
        )

    def list_sandboxes(self, request: ListSandboxesRequest) -> ListSandboxesResponse:
        """
        List sandboxes with optional filtering and pagination.
        """
        try:
            containers = self.docker_client.containers.list(
                all=True,
                filters={"label": [SANDBOX_ID_LABEL]},
            )
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.CONTAINER_QUERY_FAILED,
                    "message": f"Failed to query sandbox containers: {str(exc)}",
                },
            ) from exc

        sandboxes_by_id: dict[str, Sandbox] = {}
        container_ids: set[str] = set()
        for container in containers:
            labels = container.attrs.get("Config", {}).get("Labels") or {}
            sandbox_id = labels.get(SANDBOX_ID_LABEL)
            if not sandbox_id:
                continue
            sandbox_obj = self._container_to_sandbox(container, sandbox_id)
            container_ids.add(sandbox_id)
            if matches_filter(sandbox_obj, request.filter):
                sandboxes_by_id[sandbox_id] = sandbox_obj

        for sandbox_id, pending in self._iter_pending_sandboxes():
            if sandbox_id in container_ids:
                # If a real container exists, prefer its state regardless of filter outcome.
                continue
            sandbox_obj = self._pending_to_sandbox(sandbox_id, pending)
            if matches_filter(sandbox_obj, request.filter):
                sandboxes_by_id[sandbox_id] = sandbox_obj

        sandboxes: list[Sandbox] = list(sandboxes_by_id.values())

        sandboxes.sort(key=lambda s: s.created_at or datetime.min, reverse=True)

        if request.pagination:
            page = request.pagination.page
            page_size = request.pagination.page_size
        else:
            page = 1
            page_size = 20

        total_items = len(sandboxes)
        total_pages = math.ceil(total_items / page_size) if total_items else 0
        start_index = (page - 1) * page_size
        end_index = start_index + page_size
        items = sandboxes[start_index:end_index]
        has_next_page = page < total_pages

        pagination_info = PaginationInfo(
            page=page,
            page_size=page_size,
            total_items=total_items,
            total_pages=total_pages,
            has_next_page=has_next_page,
        )

        return ListSandboxesResponse(items=items, pagination=pagination_info)

    def get_sandbox(self, sandbox_id: str) -> Sandbox:
        """
        Fetch a sandbox by id.

        Args:
            sandbox_id: Unique sandbox identifier

        Returns:
            Sandbox: Complete sandbox information

        Raises:
            HTTPException: If sandbox not found
        """
        # Prefer real container state; fall back to pending record only if no container exists.
        try:
            container = self._get_container_by_sandbox_id(sandbox_id)
        except HTTPException as exc:
            if exc.status_code != status.HTTP_404_NOT_FOUND:
                raise
            pending = self._get_pending_sandbox(sandbox_id)
            if pending:
                return self._pending_to_sandbox(sandbox_id, pending)
            raise
        return self._container_to_sandbox(container, sandbox_id)

    def delete_sandbox(self, sandbox_id: str) -> None:
        """
        Delete a sandbox using Docker.

        Args:
            sandbox_id: Unique sandbox identifier

        Raises:
            HTTPException: If sandbox not found or deletion fails
        """
        container = self._get_container_by_sandbox_id(sandbox_id)
        try:
            try:
                with self._docker_operation("kill sandbox container", sandbox_id):
                    container.kill()
            except DockerException as exc:
                # Ignore error if container is already stopped
                if "is not running" not in str(exc).lower():
                    raise
            with self._docker_operation("remove sandbox container", sandbox_id):
                container.remove(force=True)
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.SANDBOX_DELETE_FAILED,
                    "message": f"Failed to delete sandbox container: {str(exc)}",
                },
            ) from exc
        finally:
            self._remove_expiration_tracking(sandbox_id)

    def pause_sandbox(self, sandbox_id: str) -> None:
        """
        Pause a running sandbox using Docker.

        Args:
            sandbox_id: Unique sandbox identifier

        Raises:
            HTTPException: If sandbox not found or cannot be paused
        """
        container = self._get_container_by_sandbox_id(sandbox_id)
        state = container.attrs.get("State", {})
        if not state.get("Running", False):
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail={
                    "code": SandboxErrorCodes.SANDBOX_NOT_RUNNING,
                    "message": "Sandbox is not in a running state.",
                },
            )

        try:
            with self._docker_operation("pause sandbox container", sandbox_id):
                container.pause()
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.SANDBOX_PAUSE_FAILED,
                    "message": f"Failed to pause sandbox container: {str(exc)}",
                },
            ) from exc

    def resume_sandbox(self, sandbox_id: str) -> None:
        """
        Resume a paused sandbox using Docker.

        Args:
            sandbox_id: Unique sandbox identifier

        Raises:
            HTTPException: If sandbox not found or cannot be resumed
        """
        container = self._get_container_by_sandbox_id(sandbox_id)
        state = container.attrs.get("State", {})
        if not state.get("Paused", False):
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail={
                    "code": SandboxErrorCodes.SANDBOX_NOT_PAUSED,
                    "message": "Sandbox is not in a paused state.",
                },
            )

        try:
            with self._docker_operation("resume sandbox container", sandbox_id):
                container.unpause()
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.SANDBOX_RESUME_FAILED,
                    "message": f"Failed to resume sandbox container: {str(exc)}",
                },
            ) from exc

    def renew_expiration(
        self,
        sandbox_id: str,
        request: RenewSandboxExpirationRequest,
    ) -> RenewSandboxExpirationResponse:
        """
        Renew sandbox expiration time.

        Args:
            sandbox_id: Unique sandbox identifier
            request: Renewal request with new expiration time

        Returns:
            RenewSandboxExpirationResponse: Updated expiration time

        Raises:
            HTTPException: If sandbox not found or renewal fails
        """
        container = self._get_container_by_sandbox_id(sandbox_id)
        new_expiration = ensure_future_expiration(request.expires_at)

        labels = container.attrs.get("Config", {}).get("Labels") or {}

        # Persist the new timeout in memory; it will also be respected on restart via _restore_existing_sandboxes
        self._schedule_expiration(sandbox_id, new_expiration)
        labels[SANDBOX_EXPIRES_AT_LABEL] = new_expiration.isoformat()
        try:
            with self._docker_operation("update sandbox labels", sandbox_id):
                self._update_container_labels(container, labels)
        except (DockerException, TypeError) as exc:
            logger.warning("Failed to refresh labels for sandbox %s: %s", sandbox_id, exc)

        return RenewSandboxExpirationResponse(expires_at=new_expiration)

    def get_endpoint(self, sandbox_id: str, port: int, resolve_internal: bool = False) -> Endpoint:
        """
        Get sandbox access endpoint.

        Args:
            sandbox_id: Unique sandbox identifier
            port: Port number where the service is listening inside the sandbox
            resolve_internal: If True, return the internal container IP (for proxy), ignoring router config.

        Returns:
            Endpoint: Public endpoint URL

        Raises:
            HTTPException: If sandbox not found or endpoint not available
        """
        try:
            self.validate_port(port)
        except ValueError as exc:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.INVALID_PORT,
                    "message": str(exc),
                },
            ) from exc

        if resolve_internal:
            container = self._get_container_by_sandbox_id(sandbox_id)
            return self._resolve_internal_endpoint(container, port)

        public_host = self._resolve_public_host()

        if self.network_mode == HOST_NETWORK_MODE:
            return Endpoint(endpoint=f"{public_host}:{port}")

        if self.network_mode == BRIDGE_NETWORK_MODE:
            container = self._get_container_by_sandbox_id(sandbox_id)
            labels = container.attrs.get("Config", {}).get("Labels") or {}
            execd_host_port = self._parse_host_port_label(
                labels.get(SANDBOX_EMBEDDING_PROXY_PORT_LABEL),
                SANDBOX_EMBEDDING_PROXY_PORT_LABEL,
            )
            http_host_port = self._parse_host_port_label(
                labels.get(SANDBOX_HTTP_PORT_LABEL),
                SANDBOX_HTTP_PORT_LABEL,
            )

            if port == 8080:
                if http_host_port is None:
                    raise HTTPException(
                        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                        detail={
                            "code": SandboxErrorCodes.NETWORK_MODE_ENDPOINT_UNAVAILABLE,
                            "message": "Missing host port mapping for container port 8080.",
                        },
                    )
                return Endpoint(endpoint=f"{public_host}:{http_host_port}")

            if execd_host_port is None:
                raise HTTPException(
                    status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                    detail={
                        "code": SandboxErrorCodes.NETWORK_MODE_ENDPOINT_UNAVAILABLE,
                        "message": "Missing host port mapping for execd proxy port 44772.",
                    },
                )
            return Endpoint(endpoint=f"{public_host}:{execd_host_port}/proxy/{port}")

        raise HTTPException(
            status_code=status.HTTP_501_NOT_IMPLEMENTED,
            detail={
                "code": SandboxErrorCodes.NETWORK_MODE_ENDPOINT_UNAVAILABLE,
                "message": (
                    f"Endpoint resolution for Docker network mode '{self.network_mode}' "
                    "is not implemented yet."
                ),
            },
        )

    def _resolve_public_host(self) -> str:
        host_cfg = (self.app_config.server.host or "").strip()
        host_key = host_cfg.lower()
        if host_key in {"", "0.0.0.0", "::"}:
            return self._resolve_bind_ip(socket.AF_INET)
        return host_cfg

    def _resolve_internal_endpoint(self, container, port: int) -> Endpoint:
        """Return the internal endpoint used when bypassing host mapping."""
        if self.network_mode == HOST_NETWORK_MODE:
            return Endpoint(endpoint=f"127.0.0.1:{port}")

        ip_address = self._extract_bridge_ip(container)
        return Endpoint(endpoint=f"{ip_address}:{port}")

    @staticmethod
    def _parse_host_port_label(value: Optional[str], label_name: str) -> Optional[int]:
        if not value:
            return None
        try:
            port = int(value)
            if port <= 0 or port > 65535:
                raise ValueError
            return port
        except ValueError:
            logger.warning("Invalid port label %s=%s", label_name, value)
            return None

    @staticmethod
    def _extract_bridge_ip(container) -> str:
        """Extract the IP address assigned to a container on a bridge network."""
        network_settings = container.attrs.get("NetworkSettings", {}) or {}
        ip_address = network_settings.get("IPAddress")

        if not ip_address:
            networks = network_settings.get("Networks", {}) or {}
            for net_conf in networks.values():
                if net_conf and net_conf.get("IPAddress"):
                    ip_address = net_conf.get("IPAddress")
                    break

        if not ip_address:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.NETWORK_MODE_ENDPOINT_UNAVAILABLE,
                    "message": "Container is running but has no assigned IP address.",
                },
            )
        return ip_address
