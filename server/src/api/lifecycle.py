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
API routes for OpenSandbox Lifecycle API.

This module defines FastAPI routes that map to the OpenAPI specification endpoints.
All business logic is delegated to the service layer that backs each operation.
"""

import logging
from collections.abc import Mapping
from typing import List, Optional, cast

import anyio
import httpx
import websockets
from websockets.asyncio.client import ClientConnection
from websockets.typing import Origin
from fastapi import APIRouter, Header, Query, Request, WebSocket, status
from fastapi.exceptions import HTTPException
from fastapi.responses import Response, StreamingResponse
from starlette.websockets import WebSocketDisconnect

from src.api.schema import (
    CreateSandboxRequest,
    CreateSandboxResponse,
    Endpoint,
    ErrorResponse,
    ListSandboxesRequest,
    ListSandboxesResponse,
    PaginationRequest,
    RenewSandboxExpirationRequest,
    RenewSandboxExpirationResponse,
    Sandbox,
    SandboxFilter,
)
from src.services.factory import create_sandbox_service

# RFC 2616 Section 13.5.1
HOP_BY_HOP_HEADERS = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailers",
    "transfer-encoding",
    "upgrade",
}

# Headers that shouldn't be forwarded to untrusted/internal backends
SENSITIVE_HEADERS = {
    "authorization",
    "cookie",
}

WEBSOCKET_HANDSHAKE_HEADERS = {
    "origin",
    "sec-websocket-extensions",
    "sec-websocket-key",
    "sec-websocket-protocol",
    "sec-websocket-version",
}

# Initialize router
router = APIRouter(tags=["Sandboxes"])
logger = logging.getLogger(__name__)

# Initialize service based on configuration from config.toml (defaults to docker)
sandbox_service = create_sandbox_service()


def _build_proxy_target_url(
    endpoint: Endpoint,
    full_path: str,
    query_string: str,
    *,
    websocket: bool = False,
) -> str:
    """Build the backend URL from an endpoint plus optional path/query suffix."""
    scheme = "ws" if websocket else "http"
    base = endpoint.endpoint.rstrip("/")
    normalized_path = full_path.lstrip("/")
    url = f"{scheme}://{base}"
    if normalized_path:
        url = f"{url}/{normalized_path}"
    if query_string:
        url = f"{url}?{query_string}"
    return url


def _filter_proxy_headers(
    headers: Mapping[str, str],
    endpoint_headers: Optional[dict[str, str]] = None,
    *,
    extra_excluded: Optional[set[str]] = None,
) -> dict[str, str]:
    """Drop transport/auth headers while preserving app-level headers."""
    excluded = set(HOP_BY_HOP_HEADERS) | set(SENSITIVE_HEADERS)
    if extra_excluded:
        excluded.update(extra_excluded)

    forwarded: dict[str, str] = {}
    for key, value in headers.items():
        key_lower = key.lower()
        if key_lower != "host" and key_lower not in excluded:
            forwarded[key] = value

    if endpoint_headers:
        forwarded.update(endpoint_headers)
    return forwarded


async def _proxy_http_request(
    request: Request,
    sandbox_id: str,
    port: int,
    full_path: str,
) -> StreamingResponse:
    endpoint = sandbox_service.get_endpoint(sandbox_id, port)
    target_url = _build_proxy_target_url(endpoint, full_path, request.url.query)
    client: httpx.AsyncClient = request.app.state.http_client

    try:
        upgrade_header = request.headers.get("Upgrade", "")
        if upgrade_header.lower() == "websocket":
            raise HTTPException(
                status_code=400, detail="Websocket upgrade is not supported yet"
            )

        headers = _filter_proxy_headers(request.headers, endpoint.headers)
        req = client.build_request(
            method=request.method,
            url=target_url,
            headers=headers,
            content=request.stream(),
        )

        resp = await client.send(req, stream=True)

        return StreamingResponse(
            content=resp.aiter_bytes(),
            status_code=resp.status_code,
            headers=resp.headers,
        )
    except httpx.ConnectError as e:
        raise HTTPException(
            status_code=502,
            detail=f"Could not connect to the backend sandbox {endpoint}: {e}",
        ) from e
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(
            status_code=500, detail=f"An internal error occurred in the proxy: {e}"
        ) from e


async def _relay_client_messages(
    websocket: WebSocket,
    backend: ClientConnection,
    cancel_scope: anyio.CancelScope,
) -> None:
    try:
        while True:
            message = await websocket.receive()
            if message["type"] == "websocket.receive":
                if message.get("text") is not None:
                    await backend.send(message["text"])
                elif message.get("bytes") is not None:
                    await backend.send(message["bytes"])
            elif message["type"] == "websocket.disconnect":
                await backend.close(
                    code=message.get("code", status.WS_1000_NORMAL_CLOSURE),
                    reason=message.get("reason") or "",
                )
                return
    except WebSocketDisconnect as exc:
        await backend.close(code=exc.code, reason=getattr(exc, "reason", "") or "")
    finally:
        cancel_scope.cancel()


async def _relay_backend_messages(
    websocket: WebSocket,
    backend: ClientConnection,
    cancel_scope: anyio.CancelScope,
) -> None:
    try:
        while True:
            payload = await backend.recv()
            if isinstance(payload, bytes):
                await websocket.send_bytes(payload)
            else:
                await websocket.send_text(payload)
    except websockets.ConnectionClosed as exc:
        try:
            await websocket.close(
                code=exc.code or status.WS_1000_NORMAL_CLOSURE,
                reason=exc.reason or "",
            )
        except RuntimeError:
            pass
    finally:
        cancel_scope.cancel()


async def _proxy_websocket_request(
    websocket: WebSocket,
    sandbox_id: str,
    port: int,
    full_path: str,
) -> None:
    try:
        endpoint = sandbox_service.get_endpoint(sandbox_id, port)
    except HTTPException as exc:
        logger.warning(
            "Rejecting websocket proxy request for sandbox=%s port=%s: %s",
            sandbox_id,
            port,
            exc.detail,
        )
        await websocket.close(code=status.WS_1011_INTERNAL_ERROR)
        return

    target_url = _build_proxy_target_url(
        endpoint,
        full_path,
        websocket.url.query,
        websocket=True,
    )
    headers = _filter_proxy_headers(
        websocket.headers,
        endpoint.headers,
        extra_excluded=WEBSOCKET_HANDSHAKE_HEADERS,
    )
    subprotocols = list(websocket.scope.get("subprotocols", []))
    origin = cast(Origin | None, websocket.headers.get("origin"))

    try:
        async with websockets.connect(
            target_url,
            additional_headers=headers or None,
            subprotocols=subprotocols or None,
            origin=origin,
        ) as backend:
            await websocket.accept(subprotocol=backend.subprotocol)
            async with anyio.create_task_group() as task_group:
                task_group.start_soon(
                    _relay_client_messages,
                    websocket,
                    backend,
                    task_group.cancel_scope,
                )
                task_group.start_soon(
                    _relay_backend_messages,
                    websocket,
                    backend,
                    task_group.cancel_scope,
                )
    except websockets.InvalidStatus as exc:
        logger.warning(
            "Backend websocket handshake failed for sandbox=%s port=%s: %s",
            sandbox_id,
            port,
            exc,
        )
        await websocket.close(code=status.WS_1008_POLICY_VIOLATION)
    except OSError as exc:
        logger.warning(
            "Could not connect websocket proxy for sandbox=%s port=%s: %s",
            sandbox_id,
            port,
            exc,
        )
        await websocket.close(code=status.WS_1011_INTERNAL_ERROR)
    except Exception:
        logger.exception(
            "Unexpected websocket proxy failure for sandbox=%s port=%s",
            sandbox_id,
            port,
        )
        try:
            await websocket.close(code=status.WS_1011_INTERNAL_ERROR)
        except RuntimeError:
            pass


# ============================================================================
# Sandbox CRUD Operations
# ============================================================================

@router.post(
    "/sandboxes",
    response_model=CreateSandboxResponse,
    response_model_exclude_none=True,
    status_code=status.HTTP_202_ACCEPTED,
    responses={
        202: {"description": "Sandbox creation accepted for asynchronous provisioning"},
        400: {"model": ErrorResponse, "description": "The request was invalid or malformed"},
        401: {"model": ErrorResponse, "description": "Authentication credentials are missing or invalid"},
        409: {"model": ErrorResponse, "description": "The operation conflicts with the current state"},
        500: {"model": ErrorResponse, "description": "An unexpected server error occurred"},
    },
)
async def create_sandbox(
    request: CreateSandboxRequest,
    x_request_id: Optional[str] = Header(None, alias="X-Request-ID", description="Unique request identifier for tracing"),
) -> CreateSandboxResponse:
    """
    Create a sandbox from a container image.

    Creates a new sandbox from a container image with optional resource limits,
    environment variables, and metadata. Sandboxes are provisioned directly from
    the specified image without requiring a pre-created template.

    Args:
        request: Sandbox creation request
        x_request_id: Unique request identifier for tracing (optional; server generates if omitted).

    Returns:
        CreateSandboxResponse: Accepted sandbox creation request

    Raises:
        HTTPException: If sandbox creation scheduling fails
    """

    return sandbox_service.create_sandbox(request)


# Search endpoint
@router.get(
    "/sandboxes",
    response_model=ListSandboxesResponse,
    response_model_exclude_none=True,
    responses={
        200: {"description": "Paginated collection of sandboxes"},
        400: {"model": ErrorResponse, "description": "The request was invalid or malformed"},
        401: {"model": ErrorResponse, "description": "Authentication credentials are missing or invalid"},
        500: {"model": ErrorResponse, "description": "An unexpected server error occurred"},
    },
)
async def list_sandboxes(
    state: Optional[List[str]] = Query(None, description="Filter by lifecycle state. Pass multiple times for OR logic."),
    metadata: Optional[str] = Query(None, description="Arbitrary metadata key-value pairs for filtering (URL encoded)."),
    page: int = Query(1, ge=1, description="Page number for pagination"),
    page_size: int = Query(20, ge=1, le=200, alias="pageSize", description="Number of items per page"),
    x_request_id: Optional[str] = Header(None, alias="X-Request-ID", description="Unique request identifier for tracing"),
) -> ListSandboxesResponse:
    """
    List sandboxes with optional filtering and pagination.

    List all sandboxes with optional filtering and pagination using query parameters.
    All filter conditions use AND logic. Multiple `state` parameters use OR logic within states.

    Args:
        state: Filter by lifecycle state.
        metadata: Arbitrary metadata key-value pairs for filtering.
        page: Page number for pagination.
        page_size: Number of items per page.
        x_request_id: Unique request identifier for tracing (optional; server generates if omitted).

    Returns:
        ListSandboxesResponse: Paginated list of sandboxes
    """
    # Parse metadata query string into dictionary
    metadata_dict = {}
    if metadata:
        from urllib.parse import parse_qsl
        try:
            # Parse query string format: key=value&key2=value2
            # strict_parsing=True rejects malformed segments like "a=1&broken"
            parsed = parse_qsl(metadata, keep_blank_values=True, strict_parsing=True)
            metadata_dict = dict(parsed)
        except Exception as e:
            from fastapi import HTTPException
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={"code": "INVALID_METADATA_FORMAT", "message": f"Invalid metadata format: {str(e)}"}
            )

    # Construct request object
    request = ListSandboxesRequest(
        filter=SandboxFilter(state=state, metadata=metadata_dict if metadata_dict else None),
        pagination=PaginationRequest(page=page, pageSize=page_size)
    )

    import logging
    logger = logging.getLogger(__name__)
    logger.info("ListSandboxes: %s", request.filter)

    # Delegate to the service layer for filtering and pagination
    return sandbox_service.list_sandboxes(request)


@router.get(
    "/sandboxes/{sandbox_id}",
    response_model=Sandbox,
    response_model_exclude_none=True,
    responses={
        200: {"description": "Sandbox current state and metadata"},
        401: {"model": ErrorResponse, "description": "Authentication credentials are missing or invalid"},
        403: {"model": ErrorResponse, "description": "The authenticated user lacks permission for this operation"},
        404: {"model": ErrorResponse, "description": "The requested resource does not exist"},
        500: {"model": ErrorResponse, "description": "An unexpected server error occurred"},
    },
)
async def get_sandbox(
    sandbox_id: str,
    x_request_id: Optional[str] = Header(None, alias="X-Request-ID", description="Unique request identifier for tracing"),
) -> Sandbox:
    """
    Fetch a sandbox by id.

    Returns the complete sandbox information including image specification,
    status, metadata, and timestamps.

    Args:
        sandbox_id: Unique sandbox identifier
        x_request_id: Unique request identifier for tracing (optional; server generates if omitted).

    Returns:
        Sandbox: Complete sandbox information

    Raises:
        HTTPException: If sandbox not found or access denied
    """
    # Delegate to the service layer for sandbox lookup
    return sandbox_service.get_sandbox(sandbox_id)


@router.delete(
    "/sandboxes/{sandbox_id}",
    status_code=status.HTTP_204_NO_CONTENT,
    responses={
        204: {"description": "Sandbox successfully deleted"},
        401: {"model": ErrorResponse, "description": "Authentication credentials are missing or invalid"},
        403: {"model": ErrorResponse, "description": "The authenticated user lacks permission for this operation"},
        404: {"model": ErrorResponse, "description": "The requested resource does not exist"},
        409: {"model": ErrorResponse, "description": "The operation conflicts with the current state"},
        500: {"model": ErrorResponse, "description": "An unexpected server error occurred"},
    },
)
async def delete_sandbox(
    sandbox_id: str,
    x_request_id: Optional[str] = Header(None, alias="X-Request-ID", description="Unique request identifier for tracing"),
) -> Response:
    """
    Delete a sandbox.

    Terminates sandbox execution. The sandbox will transition through Stopping state to Terminated.

    Args:
        sandbox_id: Unique sandbox identifier
        x_request_id: Unique request identifier for tracing (optional; server generates if omitted).

    Returns:
        Response: 204 No Content

    Raises:
        HTTPException: If sandbox not found or deletion fails
    """
    # Delegate to the service layer for deletion
    sandbox_service.delete_sandbox(sandbox_id)
    return Response(status_code=status.HTTP_204_NO_CONTENT)


# ============================================================================
# Sandbox Lifecycle Operations
# ============================================================================

@router.post(
    "/sandboxes/{sandbox_id}/pause",
    status_code=status.HTTP_202_ACCEPTED,
    responses={
        202: {"description": "Pause operation accepted"},
        401: {"model": ErrorResponse, "description": "Authentication credentials are missing or invalid"},
        403: {"model": ErrorResponse, "description": "The authenticated user lacks permission for this operation"},
        404: {"model": ErrorResponse, "description": "The requested resource does not exist"},
        409: {"model": ErrorResponse, "description": "The operation conflicts with the current state"},
        500: {"model": ErrorResponse, "description": "An unexpected server error occurred"},
    },
)
async def pause_sandbox(
    sandbox_id: str,
    x_request_id: Optional[str] = Header(None, alias="X-Request-ID", description="Unique request identifier for tracing"),
) -> Response:
    """
    Pause execution while retaining state.

    Pauses a running sandbox while preserving its state.
    Poll GET /sandboxes/{sandboxId} to track state transition to Paused.

    Args:
        sandbox_id: Unique sandbox identifier
        x_request_id: Unique request identifier for tracing (optional; server generates if omitted).

    Returns:
        Response: 202 Accepted

    Raises:
        HTTPException: If sandbox not found or cannot be paused
    """
    # Delegate to the service layer for pause orchestration
    sandbox_service.pause_sandbox(sandbox_id)
    return Response(status_code=status.HTTP_202_ACCEPTED)


@router.post(
    "/sandboxes/{sandbox_id}/resume",
    status_code=status.HTTP_202_ACCEPTED,
    responses={
        202: {"description": "Resume operation accepted"},
        401: {"model": ErrorResponse, "description": "Authentication credentials are missing or invalid"},
        403: {"model": ErrorResponse, "description": "The authenticated user lacks permission for this operation"},
        404: {"model": ErrorResponse, "description": "The requested resource does not exist"},
        409: {"model": ErrorResponse, "description": "The operation conflicts with the current state"},
        500: {"model": ErrorResponse, "description": "An unexpected server error occurred"},
    },
)
async def resume_sandbox(
    sandbox_id: str,
    x_request_id: Optional[str] = Header(None, alias="X-Request-ID", description="Unique request identifier for tracing"),
) -> Response:
    """
    Resume a paused sandbox.

    Resumes execution of a paused sandbox.
    Poll GET /sandboxes/{sandboxId} to track state transition to Running.

    Args:
        sandbox_id: Unique sandbox identifier
        x_request_id: Unique request identifier for tracing (optional; server generates if omitted).

    Returns:
        Response: 202 Accepted

    Raises:
        HTTPException: If sandbox not found or cannot be resumed
    """
    # Delegate to the service layer for resume orchestration
    sandbox_service.resume_sandbox(sandbox_id)
    return Response(status_code=status.HTTP_202_ACCEPTED)


@router.post(
    "/sandboxes/{sandbox_id}/renew-expiration",
    response_model=RenewSandboxExpirationResponse,
    response_model_exclude_none=True,
    responses={
        200: {"description": "Sandbox expiration updated successfully"},
        400: {"model": ErrorResponse, "description": "The request was invalid or malformed"},
        401: {"model": ErrorResponse, "description": "Authentication credentials are missing or invalid"},
        403: {"model": ErrorResponse, "description": "The authenticated user lacks permission for this operation"},
        404: {"model": ErrorResponse, "description": "The requested resource does not exist"},
        409: {"model": ErrorResponse, "description": "The operation conflicts with the current state"},
        500: {"model": ErrorResponse, "description": "An unexpected server error occurred"},
    },
)
async def renew_sandbox_expiration(
    sandbox_id: str,
    request: RenewSandboxExpirationRequest,
    x_request_id: Optional[str] = Header(None, alias="X-Request-ID", description="Unique request identifier for tracing"),
) -> RenewSandboxExpirationResponse:
    """
    Renew sandbox expiration.

    Renews the absolute expiration time of a sandbox.
    The new expiration time must be in the future and after the current expiresAt time.

    Args:
        sandbox_id: Unique sandbox identifier
        request: Renewal request with new expiration time
        x_request_id: Unique request identifier for tracing (optional; server generates if omitted).

    Returns:
        RenewSandboxExpirationResponse: Updated expiration time

    Raises:
        HTTPException: If sandbox not found or renewal fails
    """
    # Delegate to the service layer for expiration updates
    return sandbox_service.renew_expiration(sandbox_id, request)


# ============================================================================
# Sandbox Endpoints
# ============================================================================

@router.get(
    "/sandboxes/{sandbox_id}/endpoints/{port}",
    response_model=Endpoint,
    response_model_exclude_none=True,
    responses={
        200: {"description": "Endpoint retrieved successfully"},
        401: {"model": ErrorResponse, "description": "Authentication credentials are missing or invalid"},
        403: {"model": ErrorResponse, "description": "The authenticated user lacks permission for this operation"},
        404: {"model": ErrorResponse, "description": "The requested resource does not exist"},
        500: {"model": ErrorResponse, "description": "An unexpected server error occurred"},
    },
)
async def get_sandbox_endpoint(
    request: Request,
    sandbox_id: str,
    port: int,
    use_server_proxy: bool = Query(False, description="Whether to return a server-proxied URL"),
    x_request_id: Optional[str] = Header(None, alias="X-Request-ID", description="Unique request identifier for tracing"),
) -> Endpoint:
    """
    Get sandbox access endpoint.

    Returns the public access endpoint URL for accessing a service running on a specific port
    within the sandbox. The service must be listening on the specified port inside the sandbox
    for the endpoint to be available.

    Args:
        request: FastAPI request object
        sandbox_id: Unique sandbox identifier
        port: Port number where the service is listening inside the sandbox (1-65535)
        use_server_proxy: Whether to return a server-proxied URL
        x_request_id: Unique request identifier for tracing (optional; server generates if omitted).

    Returns:
        Endpoint: Public endpoint URL

    Raises:
        HTTPException: If sandbox not found or endpoint not available
    """
    # Delegate to the service layer for endpoint resolution
    endpoint = sandbox_service.get_endpoint(sandbox_id, port)

    if use_server_proxy:
        # Construct proxy URL
        base_url = str(request.base_url).rstrip("/")
        base_url = base_url.replace("https://", "").replace("http://", "")
        endpoint.endpoint = f"{base_url}/sandboxes/{sandbox_id}/proxy/{port}"

    return endpoint


@router.api_route(
    "/sandboxes/{sandbox_id}/proxy/{port}",
    methods=["GET", "POST", "PUT", "DELETE", "PATCH"],
)
async def proxy_sandbox_endpoint_root(request: Request, sandbox_id: str, port: int):
    """Proxy HTTP requests targeting the backend root path."""
    return await _proxy_http_request(request, sandbox_id, port, "")


@router.api_route(
    "/sandboxes/{sandbox_id}/proxy/{port}/{full_path:path}",
    methods=["GET", "POST", "PUT", "DELETE", "PATCH"],
)
async def proxy_sandbox_endpoint_request(
    request: Request,
    sandbox_id: str,
    port: int,
    full_path: str,
):
    """Proxy HTTP requests to sandbox-backed services."""
    return await _proxy_http_request(request, sandbox_id, port, full_path)


@router.websocket("/sandboxes/{sandbox_id}/proxy/{port}")
async def proxy_sandbox_endpoint_root_websocket(
    websocket: WebSocket,
    sandbox_id: str,
    port: int,
):
    """Proxy websocket requests targeting the backend root path."""
    await _proxy_websocket_request(websocket, sandbox_id, port, "")


@router.websocket("/sandboxes/{sandbox_id}/proxy/{port}/{full_path:path}")
async def proxy_sandbox_endpoint_request_websocket(
    websocket: WebSocket,
    sandbox_id: str,
    port: int,
    full_path: str,
):
    """Proxy websocket requests to sandbox-backed services."""
    await _proxy_websocket_request(websocket, sandbox_id, port, full_path)
