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

"""
Windows-profile helpers for Docker sandbox provisioning.

Keep profile-specific command/platform branching logic outside the main
Docker service flow for readability and focused unit testing.
"""

from __future__ import annotations

import io
import logging
import os
import re
import tarfile
import time
from threading import Lock
from typing import Callable, Optional, TYPE_CHECKING
from uuid import uuid4

from docker.errors import DockerException
from fastapi import HTTPException, status

from opensandbox_server.services.constants import SandboxErrorCodes

if TYPE_CHECKING:
    from opensandbox_server.api.schema import PlatformSpec

WINDOWS_REQUIRED_DEVICES = ("/dev/kvm", "/dev/net/tun")
WINDOWS_REQUIRED_CAP_ADD = ("NET_ADMIN",)
WINDOWS_EXECD_DOWNLOAD_URL_ENV = "EXECD_DOWNLOAD_URL"
DEFAULT_WINDOWS_EXECD_RELEASE_TAG = "v1.0.11"
DEFAULT_WINDOWS_EXECD_ARCH = "amd64"


def is_windows_platform(platform: Optional["PlatformSpec"]) -> bool:
    return bool(platform and platform.os == "windows")


def normalize_bootstrap_command(
    bootstrap_command: list[str],
    requested_windows_platform: bool,
) -> list[str]:
    # For linux profile, normalize single-string command with spaces
    # so bootstrap can exec reliably.
    if requested_windows_platform:
        return bootstrap_command
    if len(bootstrap_command) != 1 or " " not in bootstrap_command[0]:
        return bootstrap_command

    import shlex

    return shlex.split(bootstrap_command[0])


def validate_windows_runtime_prerequisites() -> None:
    """
    Validate host device paths required by dockur/windows runtime profile.
    """
    missing = [device for device in WINDOWS_REQUIRED_DEVICES if not os.path.exists(device)]
    if not missing:
        return
    raise HTTPException(
        status_code=status.HTTP_400_BAD_REQUEST,
        detail={
            "code": SandboxErrorCodes.INVALID_PARAMETER,
            "message": (
                "Windows profile requires host devices to be present: "
                f"{', '.join(missing)}."
            ),
        },
    )


def _normalize_windows_arch(arch: Optional[str]) -> str:
    if not arch:
        return DEFAULT_WINDOWS_EXECD_ARCH
    normalized = arch.strip().lower()
    if normalized in {"amd64", "x86_64"}:
        return "amd64"
    if normalized in {"arm64", "aarch64"}:
        return "arm64"
    return DEFAULT_WINDOWS_EXECD_ARCH


def _extract_image_tag(image_ref: str) -> Optional[str]:
    image_without_digest = image_ref.split("@", 1)[0]
    last_slash = image_without_digest.rfind("/")
    last_colon = image_without_digest.rfind(":")
    if last_colon <= last_slash:
        return None
    tag = image_without_digest[last_colon + 1 :].strip()
    return tag or None


def _build_windows_execd_download_url(release_tag: str, arch: str) -> str:
    return (
        "https://github.com/alibaba/OpenSandbox/releases/download/"
        f"docker%2Fexecd%2F{release_tag}/execd_{release_tag}_windows_{arch}.exe"
    )


def _is_release_tag(tag: str) -> bool:
    return bool(re.match(r"^v\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.\-]+)?$", tag))


def resolve_windows_execd_download_url(
    env: Optional[dict[str, Optional[str]]],
    execd_image: str,
    platform_arch: Optional[str],
) -> str:
    if not env:
        override = None
    else:
        override = env.get(WINDOWS_EXECD_DOWNLOAD_URL_ENV)
    if override is not None:
        cleaned = override.strip()
        if cleaned:
            return cleaned

    extracted_tag = _extract_image_tag(execd_image)
    if extracted_tag and not _is_release_tag(extracted_tag):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail={
                "code": SandboxErrorCodes.INVALID_PARAMETER,
                "message": (
                    f"Windows profile requires a release-like execd image tag, got '{extracted_tag}'. "
                    "Use an image tag like v1.0.11 or provide EXECD_DOWNLOAD_URL explicitly."
                ),
            },
        )
    release_tag = extracted_tag or DEFAULT_WINDOWS_EXECD_RELEASE_TAG
    arch = _normalize_windows_arch(platform_arch)
    return _build_windows_execd_download_url(release_tag, arch)


def escape_batch_env_value(value: str) -> str:
    # Batch script escapes: percent signs must be doubled.
    return value.replace("%", "%%")


def apply_windows_runtime_host_config_defaults(
    host_config_kwargs: dict,
    sandbox_id: str,
) -> dict:
    """
    Apply runtime defaults required by Windows profile.
    """
    updated = dict(host_config_kwargs)

    # Keep sandbox-local named volumes for Windows VM storage and OEM scripts.
    default_binds = [
        f"opensandbox-win-storage-{sandbox_id}:/storage:rw",
        f"opensandbox-win-oem-{sandbox_id}:/oem:rw",
    ]
    existing_binds = list(updated.get("binds") or [])
    updated["binds"] = existing_binds + default_binds

    existing_devices = list(updated.get("devices") or [])
    existing_devices.extend(WINDOWS_REQUIRED_DEVICES)
    updated["devices"] = existing_devices

    cap_drop = [cap for cap in (updated.get("cap_drop") or []) if cap != "NET_ADMIN"]
    if cap_drop:
        updated["cap_drop"] = cap_drop
    else:
        updated.pop("cap_drop", None)

    cap_add = set(updated.get("cap_add") or [])
    cap_add.update(WINDOWS_REQUIRED_CAP_ADD)
    updated["cap_add"] = sorted(cap_add)

    return updated


def fetch_execd_install_bat(
    *,
    docker_client,
    execd_image: str,
    cache: dict[str, bytes],
    cache_lock: Lock,
    docker_operation: Callable[[str, Optional[str]], object],
    logger: logging.Logger,
) -> bytes:
    """Fetch install.bat from execd image and memoize in caller-provided cache."""
    cached = cache.get("install_bat")
    if cached is not None:
        return cached

    with cache_lock:
        cached = cache.get("install_bat")
        if cached is not None:
            return cached

        container = None
        try:
            with docker_operation("execd install.bat cache create container", "execd-cache"):
                container = docker_client.containers.create(
                    image=execd_image,
                    command=["tail", "-f", "/dev/null"],
                    name=f"sandbox-execd-installbat-{uuid4()}",
                )
            with docker_operation("execd install.bat cache start container", "execd-cache"):
                container.start()
            with docker_operation("execd install.bat cache read archive", "execd-cache"):
                stream, _ = container.get_archive("/install.bat")
                tar_blob = b"".join(stream)
            with tarfile.open(fileobj=io.BytesIO(tar_blob), mode="r:*") as tar:
                member = next(
                    (m for m in tar.getmembers() if m.isfile() and m.name.endswith("install.bat")),
                    None,
                )
                if member is None:
                    raise HTTPException(
                        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                        detail={
                            "code": SandboxErrorCodes.EXECD_DISTRIBUTION_FAILED,
                            "message": "install.bat was not found in execd image archive.",
                        },
                    )
                extracted = tar.extractfile(member)
                if extracted is None:
                    raise HTTPException(
                        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                        detail={
                            "code": SandboxErrorCodes.EXECD_DISTRIBUTION_FAILED,
                            "message": "Failed to extract install.bat from execd image archive.",
                        },
                    )
                data = extracted.read()
        except HTTPException:
            raise
        except DockerException as exc:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.EXECD_DISTRIBUTION_FAILED,
                    "message": f"Failed to fetch install.bat from execd image: {str(exc)}",
                },
            ) from exc
        finally:
            if container is not None:
                try:
                    with docker_operation("execd install.bat cache cleanup container", "execd-cache"):
                        container.remove(force=True)
                except DockerException as cleanup_exc:
                    logger.warning("Failed to cleanup temporary execd install.bat container: %s", cleanup_exc)

        cache["install_bat"] = data
        return data


def install_windows_oem_scripts(
    *,
    container,
    sandbox_id: str,
    windows_execd_download_url: Optional[str],
    install_bat_bytes: bytes,
    ensure_directory: Callable[[object, str, Optional[str]], None],
    docker_operation: Callable[[str, Optional[str]], object],
) -> None:
    """
    Install OEM scripts for dockur/windows:
    - C:\\OEM\\install.bat wrapper with EXECD_DOWNLOAD_URL override
    - C:\\OEM\\opensandbox-install.bat from execd image
    """
    ensure_directory(container, "/oem", sandbox_id)
    safe_url = escape_batch_env_value(windows_execd_download_url or "")
    wrapper_content = (
        "@echo off\r\n"
        "setlocal enableextensions\r\n"
        f"set \"EXECD_DOWNLOAD_URL={safe_url}\"\r\n"
        "call \"C:\\OEM\\opensandbox-install.bat\"\r\n"
        "exit /b %errorlevel%\r\n"
    ).encode("utf-8")

    tar_stream = io.BytesIO()
    with tarfile.open(fileobj=tar_stream, mode="w") as tar:
        wrapper = tarfile.TarInfo(name="oem/install.bat")
        wrapper.mode = 0o644
        wrapper.size = len(wrapper_content)
        wrapper.mtime = int(time.time())
        tar.addfile(wrapper, io.BytesIO(wrapper_content))

        base_script = tarfile.TarInfo(name="oem/opensandbox-install.bat")
        base_script.mode = 0o644
        base_script.size = len(install_bat_bytes)
        base_script.mtime = int(time.time())
        tar.addfile(base_script, io.BytesIO(install_bat_bytes))
    tar_stream.seek(0)
    try:
        with docker_operation("install windows OEM scripts", sandbox_id):
            container.put_archive(path="/", data=tar_stream.getvalue())
    except DockerException as exc:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail={
                "code": SandboxErrorCodes.BOOTSTRAP_INSTALL_FAILED,
                "message": f"Failed to install windows OEM scripts: {str(exc)}",
            },
        ) from exc
