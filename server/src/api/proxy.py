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
HTTP proxy routes for reaching services inside sandboxes via the lifecycle API.
"""

import httpx
from fastapi import APIRouter, Request
from fastapi.exceptions import HTTPException
from fastapi.responses import StreamingResponse

from src.api import lifecycle

# RFC 2616 Section 13.5.1
HOP_BY_HOP_HEADERS = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailer",
    "transfer-encoding",
    "upgrade",
}

# Headers that shouldn't be forwarded to untrusted/internal backends
SENSITIVE_HEADERS = {
    "authorization",
    "cookie",
}

router = APIRouter(tags=["Sandboxes"])


@router.api_route(
    "/sandboxes/{sandbox_id}/proxy/{port}/{full_path:path}",
    methods=["GET", "POST", "PUT", "DELETE", "PATCH"],
)
async def proxy_sandbox_endpoint_request(request: Request, sandbox_id: str, port: int, full_path: str):
    """
    Receives all incoming requests, determines the target sandbox from path parameter,
    and asynchronously proxies the request to it.
    """

    endpoint = lifecycle.sandbox_service.get_endpoint(sandbox_id, port, resolve_internal=True)

    proxy_renew = getattr(request.app.state, "proxy_renew_coordinator", None)
    if proxy_renew is not None:
        proxy_renew.schedule(sandbox_id)

    target_host = endpoint.endpoint
    query_string = request.url.query

    client: httpx.AsyncClient = request.app.state.http_client

    try:
        upgrade_header = request.headers.get("Upgrade", "")
        if upgrade_header.lower() == "websocket":
            raise HTTPException(status_code=400, detail="Websocket upgrade is not supported yet")

        # Filter headers
        hop_by_hop = set(HOP_BY_HOP_HEADERS)
        connection_header = request.headers.get("connection")
        if connection_header:
            hop_by_hop.update(
                header.strip().lower()
                for header in connection_header.split(",")
                if header.strip()
            )
        headers = {}
        for key, value in request.headers.items():
            key_lower = key.lower()
            if (
                key_lower != "host"
                and key_lower not in hop_by_hop
                and key_lower not in SENSITIVE_HEADERS
            ):
                headers[key] = value

        req = client.build_request(
            method=request.method,
            url=f"http://{target_host}/{full_path}",
            params=query_string if query_string else None,
            headers=headers,
            content=request.stream() if request.method in ("POST", "PUT", "PATCH", "DELETE") else None,
        )

        resp = await client.send(req, stream=True)

        hop_by_hop = set(HOP_BY_HOP_HEADERS)
        connection_header = resp.headers.get("connection")
        if connection_header:
            hop_by_hop.update(
                header.strip().lower()
                for header in connection_header.split(",")
                if header.strip()
            )
        response_headers = {
            key: value
            for key, value in resp.headers.items()
            if key.lower() not in hop_by_hop
        }

        return StreamingResponse(
            content=resp.aiter_bytes(),
            status_code=resp.status_code,
            headers=response_headers,
        )
    except httpx.ConnectError as e:
        raise HTTPException(
            status_code=502,
            detail=f"Could not connect to the backend sandbox {endpoint}: {e}",
        )
    except HTTPException:
        # Preserve explicit HTTP exceptions raised above (e.g. websocket upgrade not supported).
        raise
    except Exception as e:
        raise HTTPException(
            status_code=500, detail=f"An internal error occurred in the proxy: {e}"
        )
