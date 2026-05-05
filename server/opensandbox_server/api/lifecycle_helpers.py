# Copyright 2025 Alibaba Group Holding Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.

"""Shared helpers for lifecycle routes: scoping, reserved metadata, audit logging."""

from __future__ import annotations

import logging
from typing import Optional

from fastapi import Request

from fastapi.exceptions import HTTPException

from opensandbox_server.api.schema import CreateSandboxRequest, ListSandboxesRequest, SandboxFilter
from opensandbox_server.config import AppConfig
from opensandbox_server.middleware.request_id import get_request_id
from opensandbox_server.middleware.authorization import authorize_action, is_user_scoped
from opensandbox_server.middleware.principal import Principal

logger = logging.getLogger(__name__)


def get_principal(request: Request) -> Optional[Principal]:
    return getattr(request.state, "principal", None)


def merge_list_scope_from_request(http_request: Request, body: ListSandboxesRequest, config: AppConfig) -> ListSandboxesRequest:
    """AND server-side owner/team scope into list metadata filters for user principals."""
    return _merge_list_scope_inner(body, get_principal(http_request), config)


def _merge_list_scope_inner(
    request: ListSandboxesRequest,
    principal: Optional[Principal],
    config: AppConfig,
) -> ListSandboxesRequest:
    if not is_user_scoped(principal):
        return request
    assert principal is not None
    owner_k = config.authz.owner_metadata_key
    team_k = config.authz.team_metadata_key
    meta = dict(request.filter.metadata or {})
    meta[owner_k] = principal.canonical_owner
    if principal.canonical_team is not None:
        meta[team_k] = principal.canonical_team
    new_filter = SandboxFilter(
        state=request.filter.state,
        metadata=meta,
    )
    return ListSandboxesRequest(filter=new_filter, pagination=request.pagination)


def apply_reserved_metadata_for_create(
    req: CreateSandboxRequest,
    principal: Optional[Principal],
    config: AppConfig,
) -> CreateSandboxRequest:
    if not is_user_scoped(principal):
        return req
    assert principal is not None
    meta = dict(req.metadata or {})
    meta[config.authz.owner_metadata_key] = principal.canonical_owner
    if principal.canonical_team is not None:
        meta[config.authz.team_metadata_key] = principal.canonical_team
    return req.model_copy(update={"metadata": meta})


def authorize_mutating_action(
    request: Request,
    principal: Optional[Principal],
    action: str,
    *,
    owner_key: str,
    team_key: str,
    sandbox_id: Optional[str] = None,
    sandbox=None,
) -> None:
    """Calls authorize_action and emits a mutation_audit entry when 403 is raised."""
    try:
        authorize_action(principal, action, owner_key=owner_key, team_key=team_key, sandbox=sandbox)
    except HTTPException:
        log_mutation_audit(request, action=action, sandbox_id=sandbox_id, outcome="forbidden")
        raise


def log_mutation_audit(
    request: Request,
    *,
    action: str,
    sandbox_id: Optional[str],
    outcome: str,
    error_code: Optional[str] = None,
) -> None:
    principal = get_principal(request)
    rid = get_request_id() or request.headers.get("X-Request-ID") or "-"
    subj = getattr(principal, "subject", None) if principal else None
    team = getattr(principal, "canonical_team", None) if principal else None
    role = getattr(principal, "role", None) if principal else None
    src = getattr(principal, "source", None) if principal else None
    logger.info(
        "mutation_audit request_id=%s action=%s sandbox_id=%s outcome=%s error_code=%s "
        "principal_source=%s principal_subject=%s principal_team=%s principal_role=%s",
        rid,
        action,
        sandbox_id,
        outcome,
        error_code,
        src,
        subj,
        team,
        role,
    )
