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
Unit tests for PoolService (server/opensandbox_server/services/k8s/pool_service.py).

All tests mock K8sClient CRUD helpers so no cluster connection is needed.
"""

import pytest
from unittest.mock import MagicMock
from kubernetes.client import ApiException

from opensandbox_server.api.schema import (
    CreatePoolRequest,
    PoolCapacitySpec,
    UpdatePoolRequest,
)
from opensandbox_server.services.constants import SandboxErrorCodes
from opensandbox_server.services.k8s.pool_service import PoolService
from fastapi import HTTPException


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_raw_pool(
    name: str = "my-pool",
    namespace: str = "test-ns",
    buffer_max: int = 3,
    buffer_min: int = 1,
    pool_max: int = 10,
    pool_min: int = 0,
    total: int = 2,
    allocated: int = 1,
    available: int = 1,
    revision: str = "abc123",
) -> dict:
    """Return a fake Pool CRD dict as returned by the Kubernetes API."""
    return {
        "apiVersion": "sandbox.opensandbox.io/v1alpha1",
        "kind": "Pool",
        "metadata": {
            "name": name,
            "namespace": namespace,
            "creationTimestamp": "2025-01-01T00:00:00Z",
        },
        "spec": {
            "capacitySpec": {
                "bufferMax": buffer_max,
                "bufferMin": buffer_min,
                "poolMax": pool_max,
                "poolMin": pool_min,
            },
            "template": {"metadata": {}, "spec": {"containers": []}},
        },
        "status": {
            "total": total,
            "allocated": allocated,
            "available": available,
            "revision": revision,
        },
    }


def _make_pool_service(namespace: str = "test-ns") -> tuple[PoolService, MagicMock]:
    """Return a (PoolService, mock_k8s_client) pair."""
    mock_client = MagicMock()
    service = PoolService(mock_client, namespace=namespace)
    return service, mock_client


def _capacity_spec(
    buffer_max: int = 3,
    buffer_min: int = 1,
    pool_max: int = 10,
    pool_min: int = 0,
) -> PoolCapacitySpec:
    return PoolCapacitySpec(
        bufferMax=buffer_max,
        bufferMin=buffer_min,
        poolMax=pool_max,
        poolMin=pool_min,
    )


# ---------------------------------------------------------------------------
# _pool_from_raw
# ---------------------------------------------------------------------------

class TestPoolFromRaw:
    def test_full_pool_with_status(self):
        svc, _ = _make_pool_service()
        raw = _make_raw_pool()
        result = svc._pool_from_raw(raw)

        assert result.name == "my-pool"
        assert result.capacity_spec.buffer_max == 3
        assert result.capacity_spec.buffer_min == 1
        assert result.capacity_spec.pool_max == 10
        assert result.capacity_spec.pool_min == 0
        assert result.status is not None
        assert result.status.total == 2
        assert result.status.allocated == 1
        assert result.status.available == 1
        assert result.status.revision == "abc123"

    def test_pool_without_status(self):
        svc, _ = _make_pool_service()
        raw = _make_raw_pool()
        del raw["status"]
        result = svc._pool_from_raw(raw)

        assert result.status is None

    def test_pool_with_empty_status(self):
        """status key present but empty dict – treat as no status."""
        svc, _ = _make_pool_service()
        raw = _make_raw_pool()
        raw["status"] = {}
        result = svc._pool_from_raw(raw)
        # Empty dict is falsy – status should be None
        assert result.status is None

    def test_pool_capacity_defaults_to_zero_on_missing_fields(self):
        svc, _ = _make_pool_service()
        raw = {
            "metadata": {"name": "sparse-pool"},
            "spec": {"capacitySpec": {}},
        }
        result = svc._pool_from_raw(raw)
        assert result.capacity_spec.buffer_max == 0
        assert result.capacity_spec.pool_max == 0

    def test_pool_missing_capacity_spec_logs_and_defaults(self):
        svc, _ = _make_pool_service()
        raw = {
            "metadata": {"name": "no-cap"},
            "spec": {"template": {}},
        }
        result = svc._pool_from_raw(raw)
        assert result.capacity_spec.buffer_max == 0
        assert result.capacity_spec.pool_min == 0

    def test_pool_non_object_capacity_spec_raises_500(self):
        svc, _ = _make_pool_service()
        raw = {
            "metadata": {"name": "bad-cap"},
            "spec": {"capacitySpec": "not-a-dict"},
        }
        with pytest.raises(HTTPException) as exc_info:
            svc._pool_from_raw(raw)
        assert exc_info.value.status_code == 500
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_API_ERROR

    def test_pool_non_object_status_treated_as_unreconciled(self):
        svc, _ = _make_pool_service()
        raw = _make_raw_pool()
        raw["status"] = "broken"
        result = svc._pool_from_raw(raw)
        assert result.status is None


# ---------------------------------------------------------------------------
# create_pool
# ---------------------------------------------------------------------------

class TestCreatePool:
    def test_create_pool_calls_k8s_api_with_correct_manifest(self):
        svc, mock_k8s = _make_pool_service(namespace="opensandbox")
        raw = _make_raw_pool(name="ci-pool", namespace="opensandbox")
        mock_k8s.create_custom_object.return_value = raw

        request = CreatePoolRequest(
            name="ci-pool",
            template={"spec": {"containers": []}},
            capacitySpec=_capacity_spec(),
        )
        result = svc.create_pool(request)

        mock_k8s.create_custom_object.assert_called_once()
        call_kwargs = mock_k8s.create_custom_object.call_args.kwargs
        assert call_kwargs["group"] == "sandbox.opensandbox.io"
        assert call_kwargs["version"] == "v1alpha1"
        assert call_kwargs["plural"] == "pools"
        assert call_kwargs["namespace"] == "opensandbox"

        body = call_kwargs["body"]
        assert body["kind"] == "Pool"
        assert body["metadata"]["name"] == "ci-pool"
        assert body["spec"]["capacitySpec"]["bufferMax"] == 3
        assert body["spec"]["capacitySpec"]["poolMax"] == 10

        assert result.name == "ci-pool"

    def test_create_pool_returns_pool_response(self):
        svc, mock_k8s = _make_pool_service()
        raw = _make_raw_pool()
        mock_k8s.create_custom_object.return_value = raw

        request = CreatePoolRequest(
            name="my-pool",
            template={},
            capacitySpec=_capacity_spec(),
        )
        result = svc.create_pool(request)

        assert result.name == "my-pool"
        assert result.status is not None
        assert result.status.total == 2

    def test_create_pool_409_raises_http_conflict(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.create_custom_object.side_effect = ApiException(status=409)

        request = CreatePoolRequest(
            name="dup-pool",
            template={},
            capacitySpec=_capacity_spec(),
        )
        with pytest.raises(HTTPException) as exc_info:
            svc.create_pool(request)

        assert exc_info.value.status_code == 409
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_ALREADY_EXISTS

    def test_create_pool_5xx_api_error_raises_http_500(self):
        svc, mock_k8s = _make_pool_service()
        err = ApiException(status=500)
        err.reason = "Internal Server Error"
        mock_k8s.create_custom_object.side_effect = err

        request = CreatePoolRequest(name="p", template={}, capacitySpec=_capacity_spec())
        with pytest.raises(HTTPException) as exc_info:
            svc.create_pool(request)

        assert exc_info.value.status_code == 500
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_API_ERROR

    def test_create_pool_422_preserves_status_and_json_message(self):
        svc, mock_k8s = _make_pool_service()
        err = ApiException(status=422)
        err.body = '{"message": "admission webhook denied the request"}'
        mock_k8s.create_custom_object.side_effect = err

        request = CreatePoolRequest(name="p", template={}, capacitySpec=_capacity_spec())
        with pytest.raises(HTTPException) as exc_info:
            svc.create_pool(request)

        assert exc_info.value.status_code == 422
        assert exc_info.value.detail["message"] == "admission webhook denied the request"

    def test_create_pool_409_prefers_kubernetes_message(self):
        svc, mock_k8s = _make_pool_service()
        err = ApiException(status=409)
        err.body = '{"message": "pool.example.io already taken"}'
        mock_k8s.create_custom_object.side_effect = err

        request = CreatePoolRequest(name="p", template={}, capacitySpec=_capacity_spec())
        with pytest.raises(HTTPException) as exc_info:
            svc.create_pool(request)

        assert exc_info.value.status_code == 409
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_ALREADY_EXISTS
        assert exc_info.value.detail["message"] == "pool.example.io already taken"

    def test_create_pool_unexpected_exception_raises_http_500(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.create_custom_object.side_effect = RuntimeError("boom")

        request = CreatePoolRequest(name="p", template={}, capacitySpec=_capacity_spec())
        with pytest.raises(HTTPException) as exc_info:
            svc.create_pool(request)

        assert exc_info.value.status_code == 500
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_API_ERROR


# ---------------------------------------------------------------------------
# get_pool
# ---------------------------------------------------------------------------

class TestGetPool:
    def test_get_pool_returns_correct_pool(self):
        svc, mock_k8s = _make_pool_service()
        raw = _make_raw_pool(name="target-pool")
        mock_k8s.get_custom_object.return_value = raw

        result = svc.get_pool("target-pool")

        mock_k8s.get_custom_object.assert_called_once_with(
            group="sandbox.opensandbox.io",
            version="v1alpha1",
            namespace="test-ns",
            plural="pools",
            name="target-pool",
        )
        assert result.name == "target-pool"

    def test_get_pool_404_raises_http_not_found(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.get_custom_object.return_value = None

        with pytest.raises(HTTPException) as exc_info:
            svc.get_pool("ghost-pool")

        assert exc_info.value.status_code == 404
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_NOT_FOUND
        assert "ghost-pool" in exc_info.value.detail["message"]

    def test_get_pool_kubernetes_status_is_preserved(self):
        svc, mock_k8s = _make_pool_service()
        err = ApiException(status=503)
        err.reason = "Service Unavailable"
        mock_k8s.get_custom_object.side_effect = err

        with pytest.raises(HTTPException) as exc_info:
            svc.get_pool("p")

        assert exc_info.value.status_code == 503
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_API_ERROR
        assert "Unavailable" in exc_info.value.detail["message"]

    def test_get_pool_unexpected_exception_raises_http_500(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.get_custom_object.side_effect = ConnectionError("timeout")

        with pytest.raises(HTTPException) as exc_info:
            svc.get_pool("p")

        assert exc_info.value.status_code == 500


# ---------------------------------------------------------------------------
# list_pools
# ---------------------------------------------------------------------------

class TestListPools:
    def test_list_pools_returns_all_items(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.list_custom_objects.return_value = [
            _make_raw_pool(name="pool-a"),
            _make_raw_pool(name="pool-b"),
        ]

        result = svc.list_pools()

        assert len(result.items) == 2
        names = {p.name for p in result.items}
        assert names == {"pool-a", "pool-b"}

    def test_list_pools_empty_returns_empty_list(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.list_custom_objects.return_value = []

        result = svc.list_pools()
        assert result.items == []

    def test_list_pools_404_from_cluster_returns_503(self):
        """404 on list API means CRD/path unavailable — not an empty tenant."""
        svc, mock_k8s = _make_pool_service()
        mock_k8s.list_custom_objects.side_effect = ApiException(status=404)

        with pytest.raises(HTTPException) as exc_info:
            svc.list_pools()

        assert exc_info.value.status_code == 503
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_LIST_UNAVAILABLE

    def test_list_pools_5xx_raises_http_500(self):
        svc, mock_k8s = _make_pool_service()
        err = ApiException(status=500)
        err.reason = "Internal"
        mock_k8s.list_custom_objects.side_effect = err

        with pytest.raises(HTTPException) as exc_info:
            svc.list_pools()

        assert exc_info.value.status_code == 500
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_API_ERROR

    def test_list_pools_403_passthrough(self):
        svc, mock_k8s = _make_pool_service()
        err = ApiException(status=403)
        err.body = '{"message": "pods is forbidden: User cannot list resource"}'
        mock_k8s.list_custom_objects.side_effect = err

        with pytest.raises(HTTPException) as exc_info:
            svc.list_pools()

        assert exc_info.value.status_code == 403
        assert "forbidden" in exc_info.value.detail["message"].lower()

    def test_list_pools_unexpected_exception_raises_http_500(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.list_custom_objects.side_effect = RuntimeError("unexpected")

        with pytest.raises(HTTPException) as exc_info:
            svc.list_pools()

        assert exc_info.value.status_code == 500


# ---------------------------------------------------------------------------
# update_pool
# ---------------------------------------------------------------------------

class TestUpdatePool:
    def test_update_pool_sends_correct_patch(self):
        svc, mock_k8s = _make_pool_service()
        updated_raw = _make_raw_pool(buffer_max=5, pool_max=20)
        mock_k8s.patch_custom_object.return_value = updated_raw

        request = UpdatePoolRequest(capacitySpec=_capacity_spec(buffer_max=5, pool_max=20))
        result = svc.update_pool("my-pool", request)

        mock_k8s.patch_custom_object.assert_called_once()
        call_kwargs = mock_k8s.patch_custom_object.call_args.kwargs
        assert call_kwargs["name"] == "my-pool"
        assert call_kwargs["namespace"] == "test-ns"
        patch_body = call_kwargs["body"]
        assert patch_body["spec"]["capacitySpec"]["bufferMax"] == 5
        assert patch_body["spec"]["capacitySpec"]["poolMax"] == 20
        assert result.capacity_spec.buffer_max == 5

    def test_update_pool_404_raises_http_not_found(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.patch_custom_object.side_effect = ApiException(status=404)

        with pytest.raises(HTTPException) as exc_info:
            svc.update_pool("missing", UpdatePoolRequest(capacitySpec=_capacity_spec()))

        assert exc_info.value.status_code == 404
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_NOT_FOUND

    def test_update_pool_5xx_raises_http_500(self):
        svc, mock_k8s = _make_pool_service()
        err = ApiException(status=500)
        err.reason = "Timeout"
        mock_k8s.patch_custom_object.side_effect = err

        with pytest.raises(HTTPException) as exc_info:
            svc.update_pool("p", UpdatePoolRequest(capacitySpec=_capacity_spec()))

        assert exc_info.value.status_code == 500
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_API_ERROR

    def test_update_pool_unexpected_exception_raises_http_500(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.patch_custom_object.side_effect = ValueError("bad")

        with pytest.raises(HTTPException) as exc_info:
            svc.update_pool("p", UpdatePoolRequest(capacitySpec=_capacity_spec()))

        assert exc_info.value.status_code == 500


# ---------------------------------------------------------------------------
# delete_pool
# ---------------------------------------------------------------------------

class TestDeletePool:
    def test_delete_pool_calls_k8s_delete(self):
        svc, mock_k8s = _make_pool_service(namespace="opensandbox")

        svc.delete_pool("old-pool")

        mock_k8s.delete_custom_object.assert_called_once_with(
            group="sandbox.opensandbox.io",
            version="v1alpha1",
            namespace="opensandbox",
            plural="pools",
            name="old-pool",
            grace_period_seconds=0,
        )

    def test_delete_pool_returns_none(self):
        svc, mock_k8s = _make_pool_service()

        result = svc.delete_pool("p")
        assert result is None

    def test_delete_pool_404_raises_http_not_found(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.delete_custom_object.side_effect = ApiException(status=404)

        with pytest.raises(HTTPException) as exc_info:
            svc.delete_pool("ghost")

        assert exc_info.value.status_code == 404
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_NOT_FOUND
        assert "ghost" in exc_info.value.detail["message"]

    def test_delete_pool_5xx_raises_http_500(self):
        svc, mock_k8s = _make_pool_service()
        err = ApiException(status=500)
        err.reason = "Internal"
        mock_k8s.delete_custom_object.side_effect = err

        with pytest.raises(HTTPException) as exc_info:
            svc.delete_pool("p")

        assert exc_info.value.status_code == 500
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_POOL_API_ERROR

    def test_delete_pool_unexpected_exception_raises_http_500(self):
        svc, mock_k8s = _make_pool_service()
        mock_k8s.delete_custom_object.side_effect = OSError("io")

        with pytest.raises(HTTPException) as exc_info:
            svc.delete_pool("p")

        assert exc_info.value.status_code == 500


# ---------------------------------------------------------------------------
# _build_pool_manifest
# ---------------------------------------------------------------------------

class TestBuildPoolManifest:
    def test_manifest_has_correct_structure(self):
        svc, _ = _make_pool_service(namespace="prod")
        template = {"spec": {"containers": [{"name": "sandbox"}]}}
        cap = _capacity_spec(buffer_max=2, buffer_min=1, pool_max=8, pool_min=0)

        manifest = svc._build_pool_manifest("prod-pool", "prod", template, cap)

        assert manifest["apiVersion"] == "sandbox.opensandbox.io/v1alpha1"
        assert manifest["kind"] == "Pool"
        assert manifest["metadata"]["name"] == "prod-pool"
        assert manifest["metadata"]["namespace"] == "prod"
        assert manifest["spec"]["capacitySpec"]["bufferMax"] == 2
        assert manifest["spec"]["capacitySpec"]["poolMin"] == 0
        assert manifest["spec"]["template"] == template

    def test_manifest_capacity_values_are_exact(self):
        svc, _ = _make_pool_service()
        cap = _capacity_spec(buffer_max=99, buffer_min=7, pool_max=200, pool_min=5)
        manifest = svc._build_pool_manifest("p", "ns", {}, cap)
        cs = manifest["spec"]["capacitySpec"]
        assert cs["bufferMax"] == 99
        assert cs["bufferMin"] == 7
        assert cs["poolMax"] == 200
        assert cs["poolMin"] == 5
