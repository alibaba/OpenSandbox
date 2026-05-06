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

import tempfile
from pathlib import Path

from fastapi import FastAPI, Request
from fastapi.testclient import TestClient

from opensandbox_server.config import AppConfig, IngressConfig, RuntimeConfig, ServerConfig
from opensandbox_server.middleware.auth import AuthMiddleware, get_current_tenant
from opensandbox_server.tenants.loader import TenantLoader


def _app_config_with_api_key() -> AppConfig:
    return AppConfig(
        server=ServerConfig(api_key="secret-key"),
        runtime=RuntimeConfig(type="docker", execd_image="opensandbox/execd:latest"),
        ingress=IngressConfig(mode="direct"),
    )


def _build_test_app():
    app = FastAPI()
    config = _app_config_with_api_key()
    app.add_middleware(AuthMiddleware, config=config)

    @app.get("/secured")
    def secured_endpoint():
        return {"ok": True}

    return app


def test_auth_middleware_rejects_missing_key():
    app = _build_test_app()
    client = TestClient(app)
    response = client.get("/secured")
    assert response.status_code == 401
    assert response.json()["code"] == "MISSING_API_KEY"


def test_auth_middleware_accepts_valid_key():
    app = _build_test_app()
    client = TestClient(app)
    response = client.get("/secured", headers={"OPEN-SANDBOX-API-KEY": "secret-key"})
    assert response.status_code == 200
    assert response.json() == {"ok": True}


def test_auth_middleware_skips_validation_for_proxy_to_sandbox():
    """Proxy-to-sandbox paths must not require API key; server only forwards to sandbox."""
    app = _build_test_app()

    @app.get("/sandboxes/{sandbox_id}/proxy/{port}/{full_path:path}")
    def proxy_echo(sandbox_id: str, port: int, full_path: str):
        return {"proxied": True, "sandbox_id": sandbox_id, "port": port, "path": full_path}

    client = TestClient(app)
    # No OPEN-SANDBOX-API-KEY header; should still succeed for proxy path
    response = client.get("/sandboxes/abc-123/proxy/8080/foo/bar")
    assert response.status_code == 200
    assert response.json()["proxied"] is True
    assert response.json()["sandbox_id"] == "abc-123"
    assert response.json()["port"] == 8080
    assert response.json()["path"] == "foo/bar"


def test_auth_middleware_v1_proxy_path_exempt():
    """V1 prefix proxy path is also exempt."""
    app = _build_test_app()

    @app.get("/v1/sandboxes/{sandbox_id}/proxy/{port}/{full_path:path}")
    def proxy_echo(sandbox_id: str, port: int, full_path: str):
        return {"proxied": True}

    client = TestClient(app)
    response = client.get("/v1/sandboxes/sid/proxy/443/")
    assert response.status_code == 200
    assert response.json()["proxied"] is True


def test_auth_middleware_requires_key_for_non_proxy_paths_containing_proxy_and_sandboxes():
    """Paths that contain both 'proxy' and 'sandboxes' but not in proxy-route shape still require auth."""
    app = _build_test_app()

    @app.get("/proxy/sandboxes/anything")
    def fake_proxy():
        return {"reached": True}

    client = TestClient(app)
    response = client.get("/proxy/sandboxes/anything")
    assert response.status_code == 401
    assert response.json()["code"] == "MISSING_API_KEY"


def test_auth_middleware_requires_key_for_malformed_proxy_port():
    """Malformed port (non-numeric) must get 401, not 422; limits unauthenticated surface."""
    app = _build_test_app()

    @app.get("/sandboxes/{sandbox_id}/proxy/{port}/{full_path:path}")
    def proxy_echo(sandbox_id: str, port: int, full_path: str):
        return {"proxied": True}

    client = TestClient(app)
    response = client.get("/sandboxes/s1/proxy/not-a-port/x")
    assert response.status_code == 401
    assert response.json()["code"] == "MISSING_API_KEY"


def test_auth_middleware_is_proxy_path_rejects_traversal():
    """Paths containing '..' are never considered proxy (no auth bypass)."""
    assert AuthMiddleware._is_proxy_path("/sandboxes/abc/proxy/8080/../other") is False
    assert AuthMiddleware._is_proxy_path("/sandboxes/../admin/proxy/8080") is False


def test_auth_middleware_is_proxy_path_accepts_valid_shapes():
    """Only exact proxy route shape (including numeric port) is accepted."""
    assert AuthMiddleware._is_proxy_path("/sandboxes/id/proxy/8080") is True
    assert AuthMiddleware._is_proxy_path("/sandboxes/id/proxy/8080/") is True
    assert AuthMiddleware._is_proxy_path("/v1/sandboxes/id/proxy/443/path") is True
    assert AuthMiddleware._is_proxy_path("/proxy/sandboxes/x") is False
    assert AuthMiddleware._is_proxy_path("/foo/sandboxes/id/proxy/8080") is False
    # Non-numeric port must not skip auth (malformed path → 401, not 422)
    assert AuthMiddleware._is_proxy_path("/sandboxes/s1/proxy/not-a-port/x") is False
    assert AuthMiddleware._is_proxy_path("/sandboxes/s1/proxy/8080x/") is False


# ---------------------------------------------------------------------------
# Multi-tenant auth
# ---------------------------------------------------------------------------

TENANTS_TOML = """\
[[tenants]]
name = "alpha"
namespace = "ns-alpha"
api_keys = ["key-alpha-1", "key-alpha-2"]

[[tenants]]
name = "beta"
namespace = "ns-beta"
api_keys = ["key-beta-1"]
"""


def _multi_tenant_config() -> AppConfig:
    return AppConfig(
        server=ServerConfig(api_key=""),  # must be empty in multi-tenant mode
        runtime=RuntimeConfig(type="kubernetes", execd_image="opensandbox/execd:latest"),
        ingress=IngressConfig(mode="direct"),
    )


def _build_multi_tenant_app(loader: TenantLoader):
    app = FastAPI()
    config = _multi_tenant_config()
    app.add_middleware(AuthMiddleware, config=config, tenant_loader=loader)

    @app.get("/secured")
    def secured_endpoint(request: Request):
        tenant = get_current_tenant()
        return {"ok": True, "tenant": tenant.name, "namespace": tenant.namespace}

    return app


def test_multi_tenant_accepts_valid_tenant_key():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = Path(tmpdir) / "tenants.toml"
        p.write_text(TENANTS_TOML)
        loader = TenantLoader(p)
        try:
            client = TestClient(_build_multi_tenant_app(loader))
            resp = client.get("/secured", headers={"OPEN-SANDBOX-API-KEY": "key-alpha-1"})
            assert resp.status_code == 200
            assert resp.json()["tenant"] == "alpha"
            assert resp.json()["namespace"] == "ns-alpha"
        finally:
            loader.stop()


def test_multi_tenant_accepts_second_key_same_tenant():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = Path(tmpdir) / "tenants.toml"
        p.write_text(TENANTS_TOML)
        loader = TenantLoader(p)
        try:
            client = TestClient(_build_multi_tenant_app(loader))
            resp = client.get("/secured", headers={"OPEN-SANDBOX-API-KEY": "key-alpha-2"})
            assert resp.status_code == 200
            assert resp.json()["tenant"] == "alpha"
        finally:
            loader.stop()


def test_multi_tenant_accepts_different_tenant_key():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = Path(tmpdir) / "tenants.toml"
        p.write_text(TENANTS_TOML)
        loader = TenantLoader(p)
        try:
            client = TestClient(_build_multi_tenant_app(loader))
            resp = client.get("/secured", headers={"OPEN-SANDBOX-API-KEY": "key-beta-1"})
            assert resp.status_code == 200
            assert resp.json()["tenant"] == "beta"
            assert resp.json()["namespace"] == "ns-beta"
        finally:
            loader.stop()


def test_multi_tenant_rejects_unknown_key():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = Path(tmpdir) / "tenants.toml"
        p.write_text(TENANTS_TOML)
        loader = TenantLoader(p)
        try:
            client = TestClient(_build_multi_tenant_app(loader))
            resp = client.get("/secured", headers={"OPEN-SANDBOX-API-KEY": "unknown-key"})
            assert resp.status_code == 401
            assert resp.json()["code"] == "INVALID_API_KEY"
        finally:
            loader.stop()


def test_multi_tenant_rejects_missing_key():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = Path(tmpdir) / "tenants.toml"
        p.write_text(TENANTS_TOML)
        loader = TenantLoader(p)
        try:
            client = TestClient(_build_multi_tenant_app(loader))
            resp = client.get("/secured")
            assert resp.status_code == 401
            assert resp.json()["code"] == "MISSING_API_KEY"
        finally:
            loader.stop()


def test_multi_tenant_proxy_path_still_exempt():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = Path(tmpdir) / "tenants.toml"
        p.write_text(TENANTS_TOML)
        loader = TenantLoader(p)
        try:
            app = FastAPI()
            config = _multi_tenant_config()
            app.add_middleware(AuthMiddleware, config=config, tenant_loader=loader)

            @app.get("/sandboxes/{sandbox_id}/proxy/{port}/{full_path:path}")
            def proxy_echo(sandbox_id: str, port: int, full_path: str):
                return {"proxied": True}

            client = TestClient(app)
            resp = client.get("/sandboxes/sbx-1/proxy/8080/foo")
            assert resp.status_code == 200
            assert resp.json()["proxied"] is True
        finally:
            loader.stop()
