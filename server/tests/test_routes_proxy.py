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

import httpx
from fastapi.testclient import TestClient

from src.api import lifecycle
from src.api.schema import Endpoint


class _FakeStreamingResponse:
    def __init__(
        self, status_code: int = 200, headers: dict | None = None, chunks: list[bytes] | None = None
    ):
        self.status_code = status_code
        self.headers = httpx.Headers(headers or {})
        self._chunks = chunks or []

    async def aiter_bytes(self):
        for chunk in self._chunks:
            yield chunk


class _FakeAsyncClient:
    def __init__(self):
        self.built = None
        self.response = _FakeStreamingResponse()
        self.raise_connect_error = False
        self.raise_generic_error = False

    def build_request(
        self,
        method: str,
        url: str,
        headers: dict,
        content,
        params: str | None = None,
    ):
        self.built = {
            "method": method,
            "url": url,
            "params": params,
            "headers": headers,
            "content": content,
        }
        return self.built

    async def send(self, req, stream: bool = True):
        if self.raise_connect_error:
            raise httpx.ConnectError("connection refused")
        if self.raise_generic_error:
            raise RuntimeError("unexpected proxy error")
        return self.response


def test_proxy_forwards_filtered_headers_and_query(
    client: TestClient,
    auth_headers: dict,
    monkeypatch,
) -> None:
    class StubService:
        @staticmethod
        def get_endpoint(sandbox_id: str, port: int, resolve_internal: bool = False) -> Endpoint:
            assert sandbox_id == "sbx-123"
            assert port == 44772
            assert resolve_internal is True
            return Endpoint(endpoint="10.57.1.91:40109")

    monkeypatch.setattr(lifecycle, "sandbox_service", StubService())

    fake_client = _FakeAsyncClient()
    fake_client.response = _FakeStreamingResponse(
        status_code=201,
        headers={"x-backend": "yes"},
        chunks=[b"proxy-ok"],
    )
    client.app.state.http_client = fake_client

    headers = {
        **auth_headers,
        "Authorization": "Bearer top-secret",
        "Cookie": "sid=secret",
        "Connection": "keep-alive, X-Hop-Temp",
        "Upgrade": "h2c",
        "Trailer": "X-Checksum",
        "X-Hop-Temp": "drop-me",
        "X-Trace": "trace-1",
    }

    response = client.post(
        "/v1/sandboxes/sbx-123/proxy/44772/api/run",
        params={"q": "search"},
        headers=headers,
        content=b'{"hello":"world"}',
    )

    assert response.status_code == 201
    assert response.content == b"proxy-ok"
    assert response.headers.get("x-backend") == "yes"

    assert fake_client.built is not None
    assert fake_client.built["method"] == "POST"
    assert fake_client.built["url"] == "http://10.57.1.91:40109/api/run"
    assert fake_client.built["params"] == "q=search"
    forwarded_headers = fake_client.built["headers"]
    lowered_headers = {k.lower(): v for k, v in forwarded_headers.items()}
    assert "host" not in lowered_headers
    assert "connection" not in lowered_headers
    assert "upgrade" not in lowered_headers
    assert "trailer" not in lowered_headers
    assert "authorization" not in lowered_headers
    assert "cookie" not in lowered_headers
    assert "x-hop-temp" not in lowered_headers
    assert lowered_headers.get("x-trace") == "trace-1"


def test_proxy_forwards_get_request_with_query_params(
    client: TestClient,
    auth_headers: dict,
    monkeypatch,
) -> None:
    """Test that GET requests with query parameters are forwarded correctly.

    This test verifies the fix for issue #484 where GET requests with query
    parameters were failing with 400 MISSING_QUERY when using use_server_proxy.
    The query string should be passed via httpx params, not embedded in URL.
    """
    class StubService:
        @staticmethod
        def get_endpoint(sandbox_id: str, port: int, resolve_internal: bool = False) -> Endpoint:
            assert sandbox_id == "sbx-123"
            assert port == 44772
            assert resolve_internal is True
            return Endpoint(endpoint="10.57.1.91:40109")

    monkeypatch.setattr(lifecycle, "sandbox_service", StubService())

    fake_client = _FakeAsyncClient()
    fake_client.response = _FakeStreamingResponse(
        status_code=200,
        headers={"content-type": "application/json"},
        chunks=[b'[{"name":"file.txt","size":100}]'],
    )
    client.app.state.http_client = fake_client

    response = client.get(
        "/v1/sandboxes/sbx-123/proxy/44772/files/search",
        params={"path": "/workspace"},
        headers=auth_headers,
    )

    assert response.status_code == 200
    assert fake_client.built is not None
    assert fake_client.built["method"] == "GET"
    assert fake_client.built["url"] == "http://10.57.1.91:40109/files/search"
    assert fake_client.built["params"] == "path=/workspace"
    assert fake_client.built["content"] is None


def test_proxy_forwards_delete_request_with_body(
    client: TestClient,
    auth_headers: dict,
    monkeypatch,
) -> None:
    """Test that DELETE requests with body payload are forwarded correctly.

    This verifies that DELETE requests with JSON/body payload are not
    incorrectly stripped when proxying.
    """
    class StubService:
        @staticmethod
        def get_endpoint(sandbox_id: str, port: int, resolve_internal: bool = False) -> Endpoint:
            return Endpoint(endpoint="10.57.1.91:40109")

    monkeypatch.setattr(lifecycle, "sandbox_service", StubService())

    fake_client = _FakeAsyncClient()
    fake_client.response = _FakeStreamingResponse(
        status_code=200,
        headers={"content-type": "application/json"},
        chunks=[b'{"deleted":true}'],
    )
    client.app.state.http_client = fake_client

    response = client.delete(
        "/v1/sandboxes/sbx-123/proxy/44772/resources",
        headers=auth_headers,
        content=b'{"id": "resource-123"}',
    )

    assert response.status_code == 200
    assert fake_client.built is not None
    assert fake_client.built["method"] == "DELETE"
    assert fake_client.built["content"] is not None


def test_proxy_filters_response_hop_by_hop_headers(
    client: TestClient,
    auth_headers: dict,
    monkeypatch,
) -> None:
    class StubService:
        @staticmethod
        def get_endpoint(sandbox_id: str, port: int, resolve_internal: bool = False) -> Endpoint:
            assert resolve_internal is True
            return Endpoint(endpoint="10.57.1.91:40109")

    monkeypatch.setattr(lifecycle, "sandbox_service", StubService())

    fake_client = _FakeAsyncClient()
    fake_client.response = _FakeStreamingResponse(
        status_code=200,
        headers={
            "x-backend": "yes",
            "Connection": "keep-alive, X-Hop-Temp",
            "Keep-Alive": "timeout=5",
            "Trailer": "X-Checksum",
            "X-Hop-Temp": "drop-me",
        },
        chunks=[b"proxy-ok"],
    )
    client.app.state.http_client = fake_client

    response = client.get(
        "/v1/sandboxes/sbx-123/proxy/44772/healthz",
        headers=auth_headers,
    )

    assert response.status_code == 200
    assert response.content == b"proxy-ok"
    assert response.headers.get("x-backend") == "yes"
    assert response.headers.get("connection") is None
    assert response.headers.get("keep-alive") is None
    assert response.headers.get("trailer") is None
    assert response.headers.get("x-hop-temp") is None


def test_proxy_rejects_websocket_upgrade(
    client: TestClient,
    auth_headers: dict,
    monkeypatch,
) -> None:
    class StubService:
        @staticmethod
        def get_endpoint(sandbox_id: str, port: int, resolve_internal: bool = False) -> Endpoint:
            return Endpoint(endpoint="10.57.1.91:40109")

    monkeypatch.setattr(lifecycle, "sandbox_service", StubService())
    client.app.state.http_client = _FakeAsyncClient()

    response = client.get(
        "/v1/sandboxes/sbx-123/proxy/44772/ws",
        headers={**auth_headers, "Upgrade": "websocket"},
    )

    assert response.status_code == 400
    assert response.json()["message"] == "Websocket upgrade is not supported yet"


def test_proxy_rejects_websocket_upgrade_for_post_and_mixed_case_header(
    client: TestClient,
    auth_headers: dict,
    monkeypatch,
) -> None:
    class StubService:
        @staticmethod
        def get_endpoint(sandbox_id: str, port: int, resolve_internal: bool = False) -> Endpoint:
            return Endpoint(endpoint="10.57.1.91:40109")

    monkeypatch.setattr(lifecycle, "sandbox_service", StubService())
    client.app.state.http_client = _FakeAsyncClient()

    response = client.post(
        "/v1/sandboxes/sbx-123/proxy/44772/ws",
        headers={**auth_headers, "Upgrade": "WebSocket"},
        content=b"{}",
    )

    assert response.status_code == 400
    assert response.json()["message"] == "Websocket upgrade is not supported yet"


def test_proxy_maps_connect_error_to_502(
    client: TestClient,
    auth_headers: dict,
    monkeypatch,
) -> None:
    class StubService:
        @staticmethod
        def get_endpoint(sandbox_id: str, port: int, resolve_internal: bool = False) -> Endpoint:
            return Endpoint(endpoint="10.57.1.91:40109")

    monkeypatch.setattr(lifecycle, "sandbox_service", StubService())
    fake_client = _FakeAsyncClient()
    fake_client.raise_connect_error = True
    client.app.state.http_client = fake_client

    response = client.get(
        "/v1/sandboxes/sbx-123/proxy/44772/healthz",
        headers=auth_headers,
    )

    assert response.status_code == 502
    assert "Could not connect to the backend sandbox" in response.json()["message"]


def test_proxy_maps_unexpected_error_to_500(
    client: TestClient,
    auth_headers: dict,
    monkeypatch,
) -> None:
    class StubService:
        @staticmethod
        def get_endpoint(sandbox_id: str, port: int, resolve_internal: bool = False) -> Endpoint:
            return Endpoint(endpoint="10.57.1.91:40109")

    monkeypatch.setattr(lifecycle, "sandbox_service", StubService())
    fake_client = _FakeAsyncClient()
    fake_client.raise_generic_error = True
    client.app.state.http_client = fake_client

    response = client.get(
        "/v1/sandboxes/sbx-123/proxy/44772/healthz",
        headers=auth_headers,
    )

    assert response.status_code == 500
    assert "An internal error occurred in the proxy" in response.json()["message"]
