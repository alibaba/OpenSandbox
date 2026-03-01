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

from datetime import datetime, timedelta, timezone

from fastapi.testclient import TestClient

from src.api import lifecycle
from src.api.schema import (
    ImageSpec,
    ListSandboxesResponse,
    PaginationInfo,
    Sandbox,
    SandboxStatus,
)


def test_list_sandboxes_parses_filters_and_pagination(
    client: TestClient,
    auth_headers: dict,
    monkeypatch,
) -> None:
    now = datetime.now(timezone.utc)
    captured_requests: list[object] = []

    class StubService:
        @staticmethod
        def list_sandboxes(request) -> ListSandboxesResponse:
            captured_requests.append(request)
            return ListSandboxesResponse(
                items=[
                    Sandbox(
                        id="sbx-001",
                        image=ImageSpec(uri="python:3.11"),
                        status=SandboxStatus(state="Running"),
                        metadata={"team": "infra", "project": "alpha"},
                        entrypoint=["python", "-V"],
                        expiresAt=now + timedelta(hours=1),
                        createdAt=now,
                    )
                ],
                pagination=PaginationInfo(
                    page=2,
                    pageSize=5,
                    totalItems=8,
                    totalPages=2,
                    hasNextPage=False,
                ),
            )

    monkeypatch.setattr(lifecycle, "sandbox_service", StubService())

    response = client.get(
        "/v1/sandboxes",
        params={
            "state": ["Running", "Paused"],
            "metadata": "team=infra&project=alpha",
            "page": 2,
            "pageSize": 5,
        },
        headers=auth_headers,
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["pagination"]["page"] == 2
    assert payload["pagination"]["pageSize"] == 5
    assert payload["items"][0]["status"]["state"] == "Running"
    assert captured_requests[0].filter.state == ["Running", "Paused"]
    assert captured_requests[0].filter.metadata == {"team": "infra", "project": "alpha"}
    assert captured_requests[0].pagination.page == 2
    assert captured_requests[0].pagination.page_size == 5


def test_list_sandboxes_validates_page_bounds(
    client: TestClient,
    auth_headers: dict,
) -> None:
    response = client.get(
        "/v1/sandboxes",
        params={"page": 0},
        headers=auth_headers,
    )

    assert response.status_code == 422


def test_list_sandboxes_validates_page_size_upper_bound(
    client: TestClient,
    auth_headers: dict,
) -> None:
    response = client.get(
        "/v1/sandboxes",
        params={"pageSize": 201},
        headers=auth_headers,
    )

    assert response.status_code == 422


def test_list_sandboxes_requires_api_key(client: TestClient) -> None:
    response = client.get("/v1/sandboxes")

    assert response.status_code == 401
    assert response.json()["code"] == "MISSING_API_KEY"
