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
            "/opt/opensandbox/bin/bootstrap.sh",
            "/usr/bin/python",
            "app.py"
        ]
    
    def test_create_workload_converts_env_to_list(self, mock_k8s_client):
        """
        Test case: Verify environment variable dict converted to list.
        Also verifies EXECD environment variable is automatically injected.
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
        
        # Should have user env vars plus EXECD
        assert len(env_vars) == 3
        env_dict = {e["name"]: e["value"] for e in env_vars}
        assert env_dict["FOO"] == "bar"
        assert env_dict["BAZ"] == "qux"
        # Verify EXECD is automatically injected
        assert env_dict["EXECD"] == "/opt/opensandbox/bin/execd"

    def test_create_workload_merges_template_volumes_and_mounts(self, mock_k8s_client, tmp_path):
        """
        Test case: Verify template volumes/volumeMounts are merged into runtime manifest
        """
        template_file = tmp_path / "template.yaml"
        template_file.write_text(
            """
spec:
  template:
    spec:
      volumes:
        - name: sandbox-shared-data
          emptyDir: {}
      containers:
        - name: sandbox
          image: ubuntu:latest
          volumeMounts:
            - name: sandbox-shared-data
              mountPath: /data
"""
        )
        provider = BatchSandboxProvider(mock_k8s_client, str(template_file))
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
        spec = body["spec"]["template"]["spec"]

        volume_names = [v["name"] for v in spec["volumes"]]
        assert "sandbox-shared-data" in volume_names
        assert "opensandbox-bin" in volume_names

        # Runtime container should stay intact (template image should not override)
        container = spec["containers"][0]
        assert container["name"] == "sandbox"
        assert container["image"] == "python:3.11"

        mount_names = [m["name"] for m in container["volumeMounts"]]
        assert "sandbox-shared-data" in mount_names
        assert "opensandbox-bin" in mount_names

    def test_create_workload_dedupes_template_volume_and_mount_names(self, mock_k8s_client, tmp_path):
        """
        Test case: Verify template entries do not duplicate runtime volumes/volumeMounts
        """
        template_file = tmp_path / "template.yaml"
        template_file.write_text(
            """
spec:
  template:
    spec:
      volumes:
        - name: opensandbox-bin
          emptyDir: {}
        - name: sandbox-shared-data
          emptyDir: {}
      containers:
        - name: sandbox
          volumeMounts:
            - name: opensandbox-bin
              mountPath: /opt/opensandbox/bin
            - name: sandbox-shared-data
              mountPath: /data
"""
        )
        provider = BatchSandboxProvider(mock_k8s_client, str(template_file))
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
        spec = body["spec"]["template"]["spec"]

        volume_names = [v["name"] for v in spec["volumes"]]
        assert volume_names.count("opensandbox-bin") == 1
        assert "sandbox-shared-data" in volume_names

        mount_names = [m["name"] for m in spec["containers"][0]["volumeMounts"]]
        assert mount_names.count("opensandbox-bin") == 1
        assert "sandbox-shared-data" in mount_names
    
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

    # ===== Pool-based Creation Tests =====
    
    def test_create_workload_poolref_ignores_image_spec(self, mock_k8s_client):
        """
        Test that pool-based creation ignores image_spec parameter.
        
        Pool already defines the image, so image_spec is not used even if provided.
        This verifies backward compatibility - no error is raised.
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "sandbox-test-id", "uid": "test-uid"}
        }
        
        result = provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["python", "app.py"],
            env={},
            resource_limits={},
            labels={},
            expires_at=datetime(2025, 12, 31, tzinfo=timezone.utc),
            execd_image="execd:latest",
            extensions={"poolRef": "my-pool"}
        )
        
        # Should succeed and return workload info
        assert result == {"name": "sandbox-test-id", "uid": "test-uid"}
        
        # Verify poolRef is used
        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        assert body["spec"]["poolRef"] == "my-pool"
    
    def test_create_workload_poolref_ignores_resource_limits(self, mock_k8s_client):
        """
        Test that pool-based creation ignores resource_limits parameter.
        
        Pool already defines the resources, so resource_limits is not used even if provided.
        This verifies backward compatibility - no error is raised.
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "sandbox-test-id", "uid": "test-uid"}
        }
        
        result = provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri=""),
            entrypoint=["python", "app.py"],
            env={},
            resource_limits={"cpu": "1", "memory": "1Gi"},
            labels={},
            expires_at=datetime(2025, 12, 31, tzinfo=timezone.utc),
            execd_image="execd:latest",
            extensions={"poolRef": "my-pool"}
        )
        
        # Should succeed and return workload info
        assert result == {"name": "sandbox-test-id", "uid": "test-uid"}
        
        # Verify poolRef is used
        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        assert body["spec"]["poolRef"] == "my-pool"
    
    def test_create_workload_poolref_allows_entrypoint_and_env(self, mock_k8s_client):
        """
        Test that pool-based creation allows customizing entrypoint and env.
        
        Verifies taskTemplate structure is correctly generated with user's entrypoint and env.
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "sandbox-test-id", "uid": "test-uid"}
        }
        
        result = provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri=""),
            entrypoint=["python", "app.py"],
            env={"FOO": "bar"},
            resource_limits={},
            labels={},
            expires_at=datetime(2025, 12, 31, tzinfo=timezone.utc),
            execd_image="execd:latest",
            extensions={"poolRef": "my-pool"}
        )
        
        assert result == {"name": "sandbox-test-id", "uid": "test-uid"}
        
        # Verify the call
        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        assert body["spec"]["poolRef"] == "my-pool"
        assert "taskTemplate" in body["spec"]
        
        # Verify taskTemplate structure
        task_template = body["spec"]["taskTemplate"]
        assert "spec" in task_template
        assert "process" in task_template["spec"]
        command = task_template["spec"]["process"]["command"]
        assert command[0] == "/bin/sh"
        assert command[1] == "-c"
        # Command should contain bootstrap.sh execution
        # Example: /opt/opensandbox/bin/bootstrap.sh python app.py &
        assert "/opt/opensandbox/bin/bootstrap.sh python app.py" in command[2]
        assert command[2].endswith(" &")
        assert task_template["spec"]["process"]["env"] == [{"name": "FOO", "value": "bar"}]
    
    def test_build_task_template_with_env(self, mock_k8s_client):
        """
        Test _build_task_template with environment variables.
        
        Verifies:
        - Command uses shell wrapper: /bin/sh -c "..."
        - Entrypoint executed via bootstrap.sh in background (&)
        - Env list formatted correctly for K8s
        
        Generated command example:
        /bin/sh -c "/opt/opensandbox/bin/bootstrap.sh /usr/bin/python app.py &"
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        
        result = provider._build_task_template(
            entrypoint=["/usr/bin/python", "app.py"],
            env={"KEY1": "value1", "KEY2": "value2"}
        )
        
        assert "spec" in result
        assert "process" in result["spec"]
        process_task = result["spec"]["process"]
        
        # Verify command structure
        command = process_task["command"]
        assert command[0] == "/bin/sh"
        assert command[1] == "-c"
        # Should execute via bootstrap.sh in background (&)
        assert "/opt/opensandbox/bin/bootstrap.sh" in command[2]
        assert "/usr/bin/python" in command[2]
        assert "app.py" in command[2]
        # Should end with & (run in background)
        assert command[2].endswith("&")
        
        # Verify env list
        assert process_task["env"] == [
            {"name": "KEY1", "value": "value1"},
            {"name": "KEY2", "value": "value2"}
        ]
    
    def test_build_task_template_without_env(self, mock_k8s_client):
        """
        Test _build_task_template without environment variables.
        
        Verifies command is wrapped in shell and executes via bootstrap.sh in background.
        
        Generated command example:
        /bin/sh -c "/opt/opensandbox/bin/bootstrap.sh /usr/bin/python app.py &"
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        
        result = provider._build_task_template(
            entrypoint=["/usr/bin/python", "app.py"],
            env={}
        )
        
        assert "spec" in result
        assert "process" in result["spec"]
        process_task = result["spec"]["process"]
        assert process_task["env"] == []
        # Without env, command directly calls bootstrap.sh in background
        command = process_task["command"]
        assert command[0] == "/bin/sh"
        assert command[1] == "-c"
        # Check escaped entrypoint
        assert "/opt/opensandbox/bin/bootstrap.sh" in command[2]
        assert "/usr/bin/python" in command[2]
        assert "app.py" in command[2]
        assert command[2].endswith(" &")
    
    def test_build_task_template_uses_default_env_path(self, mock_k8s_client):
        """
        Test that taskTemplate executes bootstrap.sh properly.
        
        Verifies:
        - Entrypoint is properly escaped
        - Command runs in background
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        
        result = provider._build_task_template(
            entrypoint=["python", "app.py"],
            env={"TEST_VAR": "test_value"}
        )
        
        command = result["spec"]["process"]["command"][2]
        # Should execute bootstrap.sh in background
        assert "/opt/opensandbox/bin/bootstrap.sh" in command
        assert "python" in command
        assert "app.py" in command
        assert command.endswith(" &")
    
    def test_build_task_template_escapes_special_characters(self, mock_k8s_client):
        """
        Test that taskTemplate properly escapes arguments with spaces, quotes, and special chars.
        
        This prevents shell injection and ensures arguments are preserved correctly.
        For example: ['python', '-c', 'print("a b")'] should work correctly.
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        
        result = provider._build_task_template(
            entrypoint=["python", "-c", 'print("hello world")'],
            env={"KEY": "value with spaces", "QUOTE": "it's fine"}
        )
        
        command = result["spec"]["process"]["command"][2]
        
        # Verify entrypoint args are properly escaped
        assert "python" in command
        assert "-c" in command
        # The python code with spaces and quotes should be properly escaped
        assert "'print(" in command or '"print(' in command  # Escaped
        
        # Verify env is passed through env list, not in command
        env_list = result["spec"]["process"]["env"]
        assert {"name": "KEY", "value": "value with spaces"} in env_list
        assert {"name": "QUOTE", "value": "it's fine"} in env_list
    
    def test_create_workload_poolref_builds_correct_manifest(self, mock_k8s_client):
        """
        Test complete pool-based BatchSandbox manifest structure.
        
        Verifies:
        - Basic metadata (apiVersion, kind, name, labels)
        - Pool-specific fields (poolRef, taskTemplate, expireTime)
        - No template field (pool mode doesn't use pod template)
        """
        provider = BatchSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "sandbox-test-id", "uid": "test-uid"}
        }
        
        expires_at = datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)
        
        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri=""),
            entrypoint=["python", "app.py"],
            env={"FOO": "bar"},
            resource_limits={},
            labels={"test": "label"},
            expires_at=expires_at,
            execd_image="execd:latest",
            extensions={"poolRef": "test-pool"}
        )
        
        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        
        # Verify basic structure
        assert body["apiVersion"] == "sandbox.opensandbox.io/v1alpha1"
        assert body["kind"] == "BatchSandbox"
        assert body["metadata"]["name"] == "sandbox-test-id"
        assert body["metadata"]["labels"] == {"test": "label"}
        
        # Verify pool-specific fields
        assert body["spec"]["replicas"] == 1
        assert body["spec"]["poolRef"] == "test-pool"
        assert body["spec"]["expireTime"] == "2025-12-31T10:00:00+00:00"
        assert "taskTemplate" in body["spec"]
        
        # Verify no template field (pool-based doesn't use template)
        assert "template" not in body["spec"]
