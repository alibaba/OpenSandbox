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
API routes for OpenSandbox DevOps diagnostics.

All endpoints return plain text for easy consumption by humans and AI agents.
"""

import logging
from typing import Optional

from fastapi import APIRouter, HTTPException, Query, status
from fastapi.responses import PlainTextResponse

from opensandbox_server.api.lifecycle import sandbox_service

logger = logging.getLogger(__name__)
router = APIRouter(tags=["DevOps"])


@router.get(
    "/sandboxes/{sandbox_id}/diagnostics/logs",
    response_class=PlainTextResponse,
    status_code=status.HTTP_200_OK,
    responses={
        200: {"description": "Container logs as plain text", "content": {"text/plain": {}}},
        404: {"description": "Sandbox not found"},
    },
)
def get_sandbox_logs(
    sandbox_id: str,
    tail: int = Query(100, ge=1, le=10000, description="Number of trailing log lines"),
    since: Optional[str] = Query(None, description="Only return logs newer than this duration (e.g. 10m, 1h)"),
) -> PlainTextResponse:
    """Retrieve container logs for a sandbox."""
    text = sandbox_service.get_sandbox_logs(sandbox_id, tail=tail, since=since)
    return PlainTextResponse(content=text)


@router.get(
    "/sandboxes/{sandbox_id}/diagnostics/inspect",
    response_class=PlainTextResponse,
    status_code=status.HTTP_200_OK,
    responses={
        200: {"description": "Container inspection as plain text", "content": {"text/plain": {}}},
        404: {"description": "Sandbox not found"},
    },
)
def get_sandbox_inspect(sandbox_id: str) -> PlainTextResponse:
    """Retrieve detailed inspection info for a sandbox container."""
    text = sandbox_service.get_sandbox_inspect(sandbox_id)
    return PlainTextResponse(content=text)


@router.get(
    "/sandboxes/{sandbox_id}/diagnostics/events",
    response_class=PlainTextResponse,
    status_code=status.HTTP_200_OK,
    responses={
        200: {"description": "Sandbox events as plain text", "content": {"text/plain": {}}},
        404: {"description": "Sandbox not found"},
    },
)
def get_sandbox_events(
    sandbox_id: str,
    limit: int = Query(50, ge=1, le=500, description="Maximum number of events to return"),
) -> PlainTextResponse:
    """Retrieve events related to a sandbox."""
    text = sandbox_service.get_sandbox_events(sandbox_id, limit=limit)
    return PlainTextResponse(content=text)


@router.get(
    "/sandboxes/{sandbox_id}/diagnostics/summary",
    response_class=PlainTextResponse,
    status_code=status.HTTP_200_OK,
    responses={
        200: {"description": "Combined diagnostics summary as plain text", "content": {"text/plain": {}}},
        404: {"description": "Sandbox not found"},
    },
)
def get_sandbox_diagnostics_summary(
    sandbox_id: str,
    tail: int = Query(50, ge=1, le=10000, description="Number of trailing log lines"),
    event_limit: int = Query(20, ge=1, le=500, description="Maximum number of events"),
) -> PlainTextResponse:
    """One-shot diagnostics summary: inspect + events + logs."""
    sections: list[str] = []

    sections.append("=" * 72)
    sections.append("SANDBOX DIAGNOSTICS SUMMARY")
    sections.append(f"Sandbox ID: {sandbox_id}")
    sections.append("=" * 72)

    # Inspect — let HTTPException (e.g. 404) propagate so callers get a proper error
    sections.append("")
    sections.append("-" * 40)
    sections.append("INSPECT")
    sections.append("-" * 40)
    try:
        sections.append(sandbox_service.get_sandbox_inspect(sandbox_id))
    except HTTPException:
        raise
    except Exception:
        logger.exception("Failed to collect sandbox inspect diagnostics for %s", sandbox_id)
        sections.append("[error] Failed to collect inspect diagnostics.")

    # Events
    sections.append("")
    sections.append("-" * 40)
    sections.append("EVENTS")
    sections.append("-" * 40)
    try:
        sections.append(sandbox_service.get_sandbox_events(sandbox_id, limit=event_limit))
    except HTTPException:
        raise
    except Exception:
        logger.exception("Failed to collect sandbox event diagnostics for %s", sandbox_id)
        sections.append("[error] Failed to collect event diagnostics.")

    # Logs
    sections.append("")
    sections.append("-" * 40)
    sections.append("LOGS (last {} lines)".format(tail))
    sections.append("-" * 40)
    try:
        sections.append(sandbox_service.get_sandbox_logs(sandbox_id, tail=tail))
    except HTTPException:
        raise
    except Exception:
        logger.exception("Failed to collect sandbox log diagnostics for %s", sandbox_id)
        sections.append("[error] Failed to collect log diagnostics.")

    return PlainTextResponse(content="\n".join(sections) + "\n")
