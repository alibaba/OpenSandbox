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

from fastapi import FastAPI
from fastapi.testclient import TestClient

from src.config import AppConfig, IngressConfig, RuntimeConfig, ServerConfig
from src.middleware.auth import AuthMiddleware


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
