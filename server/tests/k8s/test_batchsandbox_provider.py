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
Unit tests for BatchSandboxProvider.
"""

import pytest
from datetime import datetime, timezone
from unittest.mock import MagicMock
from kubernetes.client import ApiException

from src.api.schema import ImageSpec
from src.services.k8s.batchsandbox_provider import BatchSandboxProvider


class TestBatchSandboxProvider:
    """BatchSandboxProvider unit tests"""
    
    # ===== Initialization Tests =====
    
    def test_init_without_template_creates_provider(self, mock_k8s_client):
        """
        Test case: Verify normal initialization without template
        """
        provider = BatchSandboxProvider(mock_k8s_client, template_file_path=None)
        
        assert provider.k8s_client == mock_k8s_client
        assert provider.template_manager._template is None
        assert provider.group == "sandbox.opensandbox.io"
        assert provider.version == "v1alpha1"
        assert provider.plural == "batchsandboxes"
    
    def test_init_with_template_loads_template(self, mock_k8s_client, tmp_path):
        """
        Test case: Verify correct loading with template
        """
        template_file = tmp_path / "template.yaml"
        template_file.write_text("spec:\n  replicas: 1")
        
        provider = BatchSandboxProvider(mock_k8s_client, str(template_file))
        
        assert provider.template_manager._template is not None
    
    def test_init_sets_crd_constants_correctly(self, mock_k8s_client):
        """
        Test case: Verify CRD constants set correctly
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        
        assert provider.group == "sandbox.opensandbox.io"
        assert provider.version == "v1alpha1"
        assert provider.plural == "batchsandboxes"
    
    # ===== Workload Creation Tests =====
    
    def test_create_workload_builds_correct_manifest(self, mock_k8s_client):
        """
        Test case: Verify created manifest structure is correct
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "sandbox-test-id", "uid": "test-uid"}
        }
        
        expires_at = datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)
        
        result = provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={"FOO": "bar"},
            resource_limits={"cpu": "1", "memory": "1Gi"},
            labels={"opensandbox.io/id": "test-id"},
            expires_at=expires_at,
            execd_image="execd:latest"
        )
        
        assert result == {"name": "sandbox-test-id", "uid": "test-uid"}
        
        # Verify API call
        call_args = mock_api.create_namespaced_custom_object.call_args
        body = call_args.kwargs["body"]
        
        assert body["apiVersion"] == "sandbox.opensandbox.io/v1alpha1"
        assert body["kind"] == "BatchSandbox"
        assert body["metadata"]["name"] == "sandbox-test-id"
        assert body["metadata"]["namespace"] == "test-ns"
        assert body["spec"]["replicas"] == 1
        assert body["spec"]["expireTime"] == "2025-12-31T10:00:00+00:00"
        assert "template" in body["spec"]
        assert "initContainers" in body["spec"]["template"]["spec"]
        assert "containers" in body["spec"]["template"]["spec"]
        assert "volumes" in body["spec"]["template"]["spec"]
    
    def test_create_workload_builds_execd_init_container(self, mock_k8s_client):
        """
        Test case: Verify execd init container built correctly
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "sandbox-test", "uid": "uid"}
        }
        
        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={},
            resource_limits={},
            labels={},
            expires_at=datetime(2025, 12, 31, tzinfo=timezone.utc),
            execd_image="execd:test"
        )
        
        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        init_container = body["spec"]["template"]["spec"]["initContainers"][0]
        
        assert init_container["name"] == "execd-installer"
        assert init_container["image"] == "execd:test"
        assert init_container["command"] == ["/bin/sh", "-c"]
        assert "bootstrap.sh" in init_container["args"][0]
        assert init_container["volumeMounts"][0]["name"] == "opensandbox-bin"
    
    def test_create_workload_wraps_entrypoint_with_bootstrap(self, mock_k8s_client):
        """
        Test case: Verify user entrypoint is wrapped with bootstrap
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "sandbox-test", "uid": "uid"}
        }
        
        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/usr/bin/python", "app.py"],
            env={},
            resource_limits={},
            labels={},
            expires_at=datetime(2025, 12, 31, tzinfo=timezone.utc),
            execd_image="execd:latest"
        )
        
        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        main_container = body["spec"]["template"]["spec"]["containers"][0]
        
        assert main_container["command"] == [
            "/opt/opensandbox/execd/bootstrap.sh",
            "/usr/bin/python",
            "app.py"
        ]
    
    def test_create_workload_converts_env_to_list(self, mock_k8s_client):
        """
        Test case: Verify environment variable dict converted to list
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "sandbox-test", "uid": "uid"}
        }
        
        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={"FOO": "bar", "BAZ": "qux"},
            resource_limits={},
            labels={},
            expires_at=datetime(2025, 12, 31, tzinfo=timezone.utc),
            execd_image="execd:latest"
        )
        
        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        env_vars = body["spec"]["template"]["spec"]["containers"][0]["env"]
        
        assert len(env_vars) == 2
        env_dict = {e["name"]: e["value"] for e in env_vars}
        assert env_dict == {"FOO": "bar", "BAZ": "qux"}
    
    def test_create_workload_sets_resource_limits_and_requests(self, mock_k8s_client):
        """
        Test case: Verify resource limits set correctly
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "sandbox-test", "uid": "uid"}
        }
        
        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={},
            resource_limits={"cpu": "1", "memory": "1Gi"},
            labels={},
            expires_at=datetime(2025, 12, 31, tzinfo=timezone.utc),
            execd_image="execd:latest"
        )
        
        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        resources = body["spec"]["template"]["spec"]["containers"][0]["resources"]
        
        assert resources["limits"] == {"cpu": "1", "memory": "1Gi"}
        assert resources["requests"] == {"cpu": "1", "memory": "1Gi"}
    
    def test_create_workload_handles_empty_resource_limits(self, mock_k8s_client):
        """
        Test case: Verify resources not set when resource limits are empty
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "sandbox-test", "uid": "uid"}
        }
        
        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={},
            resource_limits={},
            labels={},
            expires_at=datetime(2025, 12, 31, tzinfo=timezone.utc),
            execd_image="execd:latest"
        )
        
        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        container = body["spec"]["template"]["spec"]["containers"][0]
        
        assert "resources" not in container
    
    # ===== Workload Query Tests =====
    
    def test_get_workload_finds_existing_sandbox(
        self, mock_k8s_client, mock_batchsandbox_list_response
    ):
        """
        Test case: Verify successfully querying existing sandbox
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.return_value = mock_batchsandbox_list_response
        
        result = provider.get_workload("test-id", "test-ns")
        
        assert result is not None
        assert result["metadata"]["name"] == "sandbox-test-id"
    
    def test_get_workload_returns_none_when_not_found(self, mock_k8s_client):
        """
        Test case: Verify None returned when not found
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.return_value = {"items": []}
        
        result = provider.get_workload("test-id", "test-ns")
        
        assert result is None
    
    def test_get_workload_handles_404_gracefully(self, mock_k8s_client):
        """
        Test case: Verify None returned on 404 exception
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        
        # Mock 404 exception
        error = ApiException(status=404)
        mock_api.list_namespaced_custom_object.side_effect = error
        
        result = provider.get_workload("test-id", "test-ns")
        
        assert result is None
    
    def test_get_workload_reraises_non_404_exceptions(self, mock_k8s_client):
        """
        Test case: Verify non-404 exceptions are re-raised
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        
        # Mock 500 exception
        error = ApiException(status=500)
        mock_api.list_namespaced_custom_object.side_effect = error
        
        with pytest.raises(ApiException) as exc_info:
            provider.get_workload("test-id", "test-ns")
        
        assert exc_info.value.status == 500
    
    def test_get_workload_logs_unexpected_errors(self, mock_k8s_client):
        """
        Test case: Verify unexpected errors are re-raised
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.side_effect = RuntimeError("Unexpected")
        
        with pytest.raises(RuntimeError, match="Unexpected"):
            provider.get_workload("test-id", "test-ns")
    
    # ===== Workload List Tests =====
    
    def test_list_workloads_returns_items(
        self, mock_k8s_client, mock_batchsandbox_list_response
    ):
        """
        Test case: Verify list query returns results
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.return_value = mock_batchsandbox_list_response
        
        result = provider.list_workloads("test-ns", "opensandbox.io/id")
        
        assert len(result) == 1
        assert result[0]["metadata"]["name"] == "sandbox-test-id"
    
    def test_list_workloads_returns_empty_on_404(self, mock_k8s_client):
        """
        Test case: Verify empty list returned on 404
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.side_effect = ApiException(status=404)
        
        result = provider.list_workloads("test-ns", "opensandbox.io/id")
        
        assert result == []
    
    # ===== Workload Deletion Tests =====
    
    def test_delete_workload_deletes_existing_sandbox(
        self, mock_k8s_client, mock_batchsandbox_list_response
    ):
        """
        Test case: Verify successfully deleting existing sandbox
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.return_value = mock_batchsandbox_list_response
        
        provider.delete_workload("test-id", "test-ns")
        
        mock_api.delete_namespaced_custom_object.assert_called_once_with(
            group="sandbox.opensandbox.io",
            version="v1alpha1",
            namespace="test-ns",
            plural="batchsandboxes",
            name="sandbox-test-id",
            grace_period_seconds=0
        )
    
    def test_delete_workload_raises_when_not_found(self, mock_k8s_client):
        """
        Test case: Verify exception raised when not found
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.return_value = {"items": []}
        
        with pytest.raises(Exception) as exc_info:
            provider.delete_workload("test-id", "test-ns")
        
        assert "not found" in str(exc_info.value)
    
    def test_delete_workload_sets_grace_period_zero(
        self, mock_k8s_client, mock_batchsandbox_list_response
    ):
        """
        Test case: Verify immediate deletion (grace period = 0)
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.return_value = mock_batchsandbox_list_response
        
        provider.delete_workload("test-id", "test-ns")
        
        call_kwargs = mock_api.delete_namespaced_custom_object.call_args.kwargs
        assert call_kwargs["grace_period_seconds"] == 0
    
    # ===== Expiration Time Management Tests =====
    
    def test_update_expiration_patches_spec(
        self, mock_k8s_client, mock_batchsandbox_list_response
    ):
        """
        Test case: Verify expiration time update
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.return_value = mock_batchsandbox_list_response
        
        expires_at = datetime(2025, 12, 31, 0, 0, 0, tzinfo=timezone.utc)
        provider.update_expiration("test-id", "test-ns", expires_at)
        
        call_kwargs = mock_api.patch_namespaced_custom_object.call_args.kwargs
        assert call_kwargs["body"] == {
            "spec": {"expireTime": "2025-12-31T00:00:00+00:00"}
        }
    
    def test_get_expiration_parses_iso_format(self):
        """
        Test case: Verify parsing ISO format time
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "spec": {"expireTime": "2025-12-31T10:00:00+00:00"}
        }
        
        result = provider.get_expiration(workload)
        
        assert result == datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)
    
    def test_get_expiration_handles_z_suffix(self):
        """
        Test case: Verify handling time with Z suffix
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "spec": {"expireTime": "2025-12-31T10:00:00Z"}
        }
        
        result = provider.get_expiration(workload)
        
        assert result == datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)
    
    def test_get_expiration_returns_none_on_invalid_format(self):
        """
        Test case: Verify None returned on invalid format
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "spec": {"expireTime": "invalid-date"}
        }
        
        # Should return None and not raise exception
        result = provider.get_expiration(workload)
        
        assert result is None
    
    def test_get_expiration_returns_none_when_missing(self):
        """
        Test case: Verify None returned when missing
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {"spec": {}}
        
        result = provider.get_expiration(workload)
        
        assert result is None
    
    # ===== Status Retrieval Tests =====
    
    def test_get_status_running_with_ip(self):
        """
        Test case: Verify status when Pod is Ready and has IP
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "status": {"replicas": 1, "ready": 1, "allocated": 1},
            "metadata": {
                "annotations": {
                    "sandbox.opensandbox.io/endpoints": '["10.0.0.1"]'
                },
                "creationTimestamp": "2025-12-24T10:00:00Z"
            }
        }
        
        result = provider.get_status(workload)
        
        assert result["state"] == "Running"
        assert result["reason"] == "READY_WITH_IP"
        assert "IP assigned" in result["message"]
    
    def test_get_status_pending_ready_without_ip(self):
        """
        Test case: Verify status when Pod is Ready but has no IP (should be Pending)
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "status": {"replicas": 1, "ready": 1, "allocated": 1},
            "metadata": {"creationTimestamp": "2025-12-24T10:00:00Z"}
        }
        
        result = provider.get_status(workload)
        
        assert result["state"] == "Pending"
        assert result["reason"] == "POD_READY_NO_IP"
    
    def test_get_status_pending_scheduled(self):
        """
        Test case: Verify Pod is scheduled but not Ready
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "status": {"replicas": 1, "ready": 0, "allocated": 1},
            "metadata": {"creationTimestamp": "2025-12-24T10:00:00Z"}
        }
        
        result = provider.get_status(workload)
        
        assert result["state"] == "Pending"
        assert result["reason"] == "POD_SCHEDULED"
    
    def test_get_status_pending_unallocated(self):
        """
        Test case: Verify Pod is not scheduled
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "status": {"replicas": 1, "ready": 0, "allocated": 0},
            "metadata": {"creationTimestamp": "2025-12-24T10:00:00Z"}
        }
        
        result = provider.get_status(workload)
        
        assert result["state"] == "Pending"
        assert result["reason"] == "BATCHSANDBOX_PENDING"
    
    # ===== Endpoint Information Tests =====
    
    def test_get_endpoint_info_parses_json_annotation(self):
        """
        Test case: Verify parsing IP from annotation
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "metadata": {
                "annotations": {
                    "sandbox.opensandbox.io/endpoints": '["10.0.0.1"]'
                }
            }
        }
        
        result = provider.get_endpoint_info(workload, 8080)
        
        assert result == "10.0.0.1:8080"
    
    def test_get_endpoint_info_uses_first_ip(self):
        """
        Test case: Verify using first IP when multiple IPs exist
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "metadata": {
                "annotations": {
                    "sandbox.opensandbox.io/endpoints": '["10.0.0.1", "10.0.0.2"]'
                }
            }
        }
        
        result = provider.get_endpoint_info(workload, 8080)
        
        assert result == "10.0.0.1:8080"
    
    def test_get_endpoint_info_returns_none_when_missing(self):
        """
        Test case: Verify None returned when annotation is missing
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {"metadata": {"annotations": {}}}
        
        result = provider.get_endpoint_info(workload, 8080)
        
        assert result is None
    
    def test_get_endpoint_info_returns_none_on_invalid_json(self):
        """
        Test case: Verify None returned on invalid JSON
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "metadata": {
                "annotations": {
                    "sandbox.opensandbox.io/endpoints": "invalid-json"
                }
            }
        }
        
        result = provider.get_endpoint_info(workload, 8080)
        
        assert result is None
    
    def test_get_endpoint_info_returns_none_on_empty_array(self):
        """
        Test case: Verify None returned on empty array
        """
        provider = BatchSandboxProvider(MagicMock())
        workload = {
            "metadata": {
                "annotations": {
                    "sandbox.opensandbox.io/endpoints": "[]"
                }
            }
        }
        
        result = provider.get_endpoint_info(workload, 8080)
        
        assert result is None
