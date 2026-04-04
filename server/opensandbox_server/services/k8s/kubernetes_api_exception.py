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

"""Helpers for mapping ``kubernetes.client.ApiException`` to HTTP-friendly status and message."""

from __future__ import annotations

import json
from typing import Optional

from fastapi import status
from kubernetes.client import ApiException

__all__ = [
    "http_status_from_kubernetes_api_exception",
    "kubernetes_api_exception_message",
]


def kubernetes_api_exception_message(exc: ApiException) -> Optional[str]:
    """
    Best-effort human-readable message from the apiserver response.

    Parses JSON ``body`` for ``message``, ``reason``, or first ``details.causes``
    entry; non-JSON bodies return a trimmed string. Falls back to ``exc.reason``.
    Returns ``None`` when nothing useful is present (callers may supply defaults).
    """
    body = getattr(exc, "body", None)
    if body:
        if isinstance(body, bytes):
            body = body.decode("utf-8", errors="replace")
        if isinstance(body, str) and body.strip():
            try:
                data = json.loads(body)
            except json.JSONDecodeError:
                return body.strip()[:4000]
            if isinstance(data, dict):
                msg = data.get("message")
                if msg:
                    return str(msg)
                msg = data.get("reason")
                if msg:
                    return str(msg)
                details = data.get("details")
                if isinstance(details, dict):
                    causes = details.get("causes")
                    if isinstance(causes, list) and causes:
                        c0 = causes[0]
                        if isinstance(c0, dict) and c0.get("message"):
                            return str(c0["message"])
            return str(data)[:4000]
    reason = getattr(exc, "reason", None)
    if reason:
        return str(reason)
    return None


def http_status_from_kubernetes_api_exception(exc: ApiException) -> int:
    """Return ``exc.status`` when it is a valid HTTP code, else 500."""
    st = getattr(exc, "status", None)
    if isinstance(st, int) and 100 <= st <= 599:
        return st
    return status.HTTP_500_INTERNAL_SERVER_ERROR
