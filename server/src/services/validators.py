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
Shared validation helpers for container-based sandbox services.

These helpers centralize request validation so all container runtimes
enforce the same preconditions before performing runtime-specific work.
"""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Dict, Optional, Sequence

from fastapi import HTTPException, status
import re

from src.services.constants import SandboxErrorCodes


def ensure_entrypoint(entrypoint: Sequence[str]) -> None:
    """
    Ensure a sandbox entrypoint is provided.

    Raises:
        HTTPException: When entrypoint is empty.
    """
    if not entrypoint:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail={
                "code": SandboxErrorCodes.INVALID_ENTRYPOINT,
                "message": "Entrypoint must contain at least one command.",
            },
        )


DNS_LABEL_PATTERN = r"[a-z0-9]([-a-z0-9]*[a-z0-9])?"
DNS_SUBDOMAIN_RE = re.compile(rf"^(?:{DNS_LABEL_PATTERN}\.)*{DNS_LABEL_PATTERN}$")
LABEL_NAME_RE = re.compile(r"^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$")
LABEL_VALUE_RE = re.compile(r"^([A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?)?$")


def _is_valid_label_key(key: str) -> bool:
    if len(key) > 253 or "/" in key and len(key.split("/", 1)[0]) > 253:
        return False
    if "/" in key:
        prefix, name = key.split("/", 1)
        if not prefix or not name:
            return False
        if not DNS_SUBDOMAIN_RE.match(prefix):
            return False
    else:
        name = key
    if len(name) > 63 or not LABEL_NAME_RE.match(name):
        return False
    return True


def _is_valid_label_value(value: str) -> bool:
    if len(value) > 63:
        return False
    return bool(LABEL_VALUE_RE.match(value))


def ensure_metadata_labels(metadata: Optional[Dict[str, str]]) -> None:
    """
    Validate metadata keys/values against Kubernetes label rules.

    Raises:
        HTTPException: When a key/value is invalid.
    """
    if not metadata:
        return
    for key, value in metadata.items():
        if not isinstance(key, str) or not isinstance(value, str):
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.INVALID_METADATA_LABEL,
                    "message": "Metadata keys and values must be strings.",
                },
            )
        if not _is_valid_label_key(key):
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.INVALID_METADATA_LABEL,
                    "message": f"Metadata key '{key}' is not a valid Kubernetes label key.",
                },
            )
        if not _is_valid_label_value(value):
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.INVALID_METADATA_LABEL,
                    "message": f"Metadata value '{value}' is not a valid Kubernetes label value.",
                },
            )


def ensure_future_expiration(expires_at: datetime) -> datetime:
    """
    Validate and normalize expiration timestamps to UTC.

    Args:
        expires_at: Requested expiration time (timezone aware or naive).

    Returns:
        datetime: Normalized UTC expiration timestamp.

    Raises:
        HTTPException: If the timestamp is not in the future.
    """
    if expires_at.tzinfo is None:
        normalized = expires_at.replace(tzinfo=timezone.utc)
    else:
        normalized = expires_at.astimezone(timezone.utc)

    if normalized <= datetime.now(timezone.utc):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail={
                "code": SandboxErrorCodes.INVALID_EXPIRATION,
                "message": "New expiration time must be in the future.",
            },
        )

    return normalized


def ensure_valid_port(port: int) -> None:
    """
    Validate that a port falls within the 1-65535 range.

    Raises:
        HTTPException: When the port is out of range.
    """
    if port < 1 or port > 65535:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail={
                "code": SandboxErrorCodes.INVALID_PORT,
                "message": "Port must be between 1 and 65535.",
            },
        )


__all__ = [
    "ensure_entrypoint",
    "ensure_future_expiration",
    "ensure_valid_port",
    "ensure_metadata_labels",
]
