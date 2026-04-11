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

from datetime import datetime, timezone

from fastapi.testclient import TestClient

from opensandbox_server.api import lifecycle
from opensandbox_server.api.schema import ImageSpec, Sandbox, SandboxStatus


class TestHealthCheck:

    def test_health_check(self, client: TestClient):
        response = client.get("/health")
        assert response.status_code == 200
        assert response.json() == {"status": "healthy"}


class TestAuthentication:

    def test_missing_api_key(self, client: TestClient):
        response = client.get("/sandboxes/123e4567-e89b-12d3-a456-426614174000")
        assert response.status_code == 401
        assert "MISSING_API_KEY" in response.json()["code"]

    def test_missing_api_key_v1_prefix(self, client: TestClient):
        response = client.get("/v1/sandboxes/123e4567-e89b-12d3-a456-426614174000")
        assert response.status_code == 401
        assert "MISSING_API_KEY" in response.json()["code"]


class TestGetSandbox:

    def test_get_sandbox_omits_optional_none_fields(
        self,
        client: TestClient,
        auth_headers: dict,
        monkeypatch,
    ):
        now = datetime.now(timezone.utc)
        sandbox = Sandbox(
            id="sandbox-123",
            image=ImageSpec(uri="python:3.11"),
            status=SandboxStatus(state="Running"),
            metadata=None,
            entrypoint=["python"],
            expires_at=None,
            created_at=now,
        )

        class StubService:
            @staticmethod
            def get_sandbox(sandbox_id: str) -> Sandbox:
                return sandbox

        monkeypatch.setattr(lifecycle, "sandbox_service", StubService())

        response = client.get("/sandboxes/sandbox-123", headers=auth_headers)
        assert response.status_code == 200

        payload = response.json()
        assert payload["id"] == "sandbox-123"
        assert payload["entrypoint"] == ["python"]
        assert "metadata" not in payload
        assert "expiresAt" not in payload
        assert "createdAt" in payload
        assert payload["status"]["state"] == "Running"
        assert "reason" not in payload["status"]
        assert "message" not in payload["status"]
        assert "lastTransitionAt" not in payload["status"]
