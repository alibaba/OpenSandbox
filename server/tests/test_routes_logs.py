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

"""Tests for GET /sandboxes/{sandbox_id}/logs endpoint."""

from fastapi.testclient import TestClient
from fastapi import HTTPException, status

from src.api import lifecycle


def _make_log_gen(*chunks: bytes):
    """Return a generator that yields the given byte chunks."""
    def _gen():
        yield from chunks
    return _gen()


def test_get_sandbox_logs_returns_plain_text(client: TestClient, auth_headers: dict, monkeypatch):
    """Happy path: service returns log chunks and they are streamed as text/plain."""
    log_chunks = [b"line one\n", b"line two\n"]
    monkeypatch.setattr(
        lifecycle.sandbox_service,
        "get_logs",
        lambda sandbox_id, follow, tail, timestamps: _make_log_gen(*log_chunks),
    )

    resp = client.get("/sandboxes/abc-123/logs", headers=auth_headers)

    assert resp.status_code == 200
    assert "text/plain" in resp.headers["content-type"]
    assert resp.content == b"line one\nline two\n"


def test_get_sandbox_logs_passes_query_params(client: TestClient, auth_headers: dict, monkeypatch):
    """Query parameters (follow, tail, timestamps) are forwarded to the service."""
    captured = {}

    def _fake_get_logs(sandbox_id, follow, tail, timestamps):
        captured["sandbox_id"] = sandbox_id
        captured["follow"] = follow
        captured["tail"] = tail
        captured["timestamps"] = timestamps
        return _make_log_gen(b"log\n")

    monkeypatch.setattr(lifecycle.sandbox_service, "get_logs", _fake_get_logs)

    resp = client.get(
        "/sandboxes/my-sandbox/logs",
        params={"follow": "true", "tail": 50, "timestamps": "true"},
        headers=auth_headers,
    )

    assert resp.status_code == 200
    assert captured["sandbox_id"] == "my-sandbox"
    assert captured["follow"] is True
    assert captured["tail"] == 50
    assert captured["timestamps"] is True


def test_get_sandbox_logs_empty_stream(client: TestClient, auth_headers: dict, monkeypatch):
    """When the service returns an empty generator, a 200 with empty body is returned."""
    monkeypatch.setattr(
        lifecycle.sandbox_service,
        "get_logs",
        lambda sandbox_id, follow, tail, timestamps: _make_log_gen(),
    )

    resp = client.get("/sandboxes/empty-sandbox/logs", headers=auth_headers)

    assert resp.status_code == 200
    assert resp.content == b""


def test_get_sandbox_logs_not_found(client: TestClient, auth_headers: dict, monkeypatch):
    """When the service raises 404, the endpoint propagates it."""
    def _raise_not_found(sandbox_id, follow, tail, timestamps):
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Sandbox not found")

    monkeypatch.setattr(lifecycle.sandbox_service, "get_logs", _raise_not_found)

    resp = client.get("/sandboxes/missing/logs", headers=auth_headers)

    assert resp.status_code == 404


def test_get_sandbox_logs_requires_auth(client: TestClient):
    """Requests without an API key are rejected with 401."""
    resp = client.get("/sandboxes/abc-123/logs")
    assert resp.status_code == 401


def test_get_sandbox_logs_tail_must_be_positive(client: TestClient, auth_headers: dict):
    """tail=0 is invalid (minimum is 1); FastAPI should return 422."""
    resp = client.get("/sandboxes/abc-123/logs", params={"tail": 0}, headers=auth_headers)
    assert resp.status_code == 422
