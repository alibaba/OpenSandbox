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
Unit tests for AgentSandboxProvider.
"""

from datetime import datetime, timezone
from types import SimpleNamespace
from unittest.mock import MagicMock

import pytest
from kubernetes.client import ApiException

from src.api.schema import ImageSpec
from src.services.k8s.agent_sandbox_provider import AgentSandboxProvider


class TestAgentSandboxProvider:
    """AgentSandboxProvider unit tests"""

    def test_init_sets_crd_constants_correctly(self, mock_k8s_client):
        """
        Test case: Verify CRD constants set correctly
        """
        provider = AgentSandboxProvider(mock_k8s_client)

        assert provider.group == "agents.x-k8s.io"
        assert provider.version == "v1alpha1"
        assert provider.plural == "sandboxes"

    def test_create_workload_builds_correct_manifest_init_mode(self, mock_k8s_client):
        """
        Test case: Verify created manifest structure with init mode
        """
        provider = AgentSandboxProvider(
            mock_k8s_client,
            shutdown_policy="Delete",
            service_account="agent-sa",
        )
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
            execd_image="execd:latest",
        )

        assert result == {"name": "sandbox-test-id", "uid": "test-uid"}

        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        assert body["apiVersion"] == "agents.x-k8s.io/v1alpha1"
        assert body["kind"] == "Sandbox"
        assert body["metadata"]["name"] == "sandbox-test-id"
        assert body["metadata"]["namespace"] == "test-ns"
        assert body["spec"]["replicas"] == 1
        assert body["spec"]["shutdownTime"] == "2025-12-31T10:00:00+00:00"
        assert body["spec"]["shutdownPolicy"] == "Delete"
        assert body["spec"]["podTemplate"]["spec"]["serviceAccountName"] == "agent-sa"
        assert "initContainers" in body["spec"]["podTemplate"]["spec"]
        assert "containers" in body["spec"]["podTemplate"]["spec"]
        assert "volumes" in body["spec"]["podTemplate"]["spec"]

    def test_get_workload_returns_none_on_404(self, mock_k8s_client):
        """
        Test case: Verify None returned on 404 exception
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.side_effect = ApiException(status=404)

        result = provider.get_workload("test-id", "test-ns")

        assert result is None

    def test_get_workload_reraises_non_404_exceptions(self, mock_k8s_client):
        """
        Test case: Verify non-404 exceptions are re-raised
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.side_effect = ApiException(status=500)

        with pytest.raises(ApiException) as exc_info:
            provider.get_workload("test-id", "test-ns")

        assert exc_info.value.status == 500

    def test_update_expiration_patches_spec(self, mock_k8s_client):
        """
        Test case: Verify expiration time update
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.list_namespaced_custom_object.return_value = {
            "items": [{"metadata": {"name": "sandbox-test-id"}}]
        }

        expires_at = datetime(2025, 12, 31, 0, 0, 0, tzinfo=timezone.utc)
        provider.update_expiration("test-id", "test-ns", expires_at)

        call_kwargs = mock_api.patch_namespaced_custom_object.call_args.kwargs
        assert call_kwargs["body"] == {
            "spec": {"shutdownTime": "2025-12-31T00:00:00+00:00"}
        }

    def test_get_expiration_parses_z_suffix(self):
        """
        Test case: Verify handling time with Z suffix
        """
        provider = AgentSandboxProvider(MagicMock())
        workload = {"spec": {"shutdownTime": "2025-12-31T10:00:00Z"}}

        result = provider.get_expiration(workload)

        assert result == datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)

    def test_get_status_ready_condition_true(self):
        """
        Test case: Verify Ready True is Running
        """
        provider = AgentSandboxProvider(MagicMock())
        workload = {
            "status": {
                "conditions": [
                    {
                        "type": "Ready",
                        "status": "True",
                        "reason": "SandboxReady",
                        "message": "Ready",
                        "lastTransitionTime": "2025-12-31T10:00:00Z",
                    }
                ]
            },
            "metadata": {"creationTimestamp": "2025-12-31T09:00:00Z"},
        }

        result = provider.get_status(workload)

        assert result["state"] == "Running"
        assert result["reason"] == "SandboxReady"
        assert result["message"] == "Ready"

    def test_get_status_expired_condition(self):
        """
        Test case: Verify SandboxExpired reason maps to Terminated
        """
        provider = AgentSandboxProvider(MagicMock())
        workload = {
            "status": {
                "conditions": [
                    {
                        "type": "Ready",
                        "status": "False",
                        "reason": "SandboxExpired",
                        "message": "Expired",
                        "lastTransitionTime": "2025-12-31T10:00:00Z",
                    }
                ]
            },
            "metadata": {"creationTimestamp": "2025-12-31T09:00:00Z"},
        }

        result = provider.get_status(workload)

        assert result["state"] == "Terminated"
        assert result["reason"] == "SandboxExpired"

    def test_get_status_falls_back_to_pod_state(self, mock_k8s_client):
        """
        Test case: Verify status fallback uses pod selector state
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        core_api = mock_k8s_client.get_core_v1_api()
        core_api.list_namespaced_pod.return_value = MagicMock(
            items=[
                SimpleNamespace(
                    status=SimpleNamespace(phase="Running", pod_ip="10.0.0.2")
                )
            ]
        )
        workload = {
            "status": {"conditions": [], "selector": "app=sandbox"},
            "metadata": {"creationTimestamp": "2025-12-31T09:00:00Z", "namespace": "test-ns"},
        }

        result = provider.get_status(workload)

        assert result["state"] == "Running"
        assert result["reason"] == "POD_READY"

    def test_get_endpoint_info_prefers_running_pod(self, mock_k8s_client):
        """
        Test case: Verify endpoint uses running pod IP
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        core_api = mock_k8s_client.get_core_v1_api()
        core_api.list_namespaced_pod.return_value = MagicMock(
            items=[
                SimpleNamespace(
                    status=SimpleNamespace(phase="Running", pod_ip="10.0.0.9")
                )
            ]
        )
        workload = {
            "status": {"selector": "app=sandbox"},
            "metadata": {"namespace": "test-ns"},
        }

        endpoint = provider.get_endpoint_info(workload, 8080)

        assert endpoint == "10.0.0.9:8080"

    def test_get_endpoint_info_falls_back_to_service_fqdn(self, mock_k8s_client):
        """
        Test case: Verify endpoint falls back to serviceFQDN on pod lookup failure
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        core_api = mock_k8s_client.get_core_v1_api()
        core_api.list_namespaced_pod.side_effect = Exception("boom")
        workload = {
            "status": {"selector": "app=sandbox", "serviceFQDN": "svc.example.com"},
            "metadata": {"namespace": "test-ns"},
        }

        endpoint = provider.get_endpoint_info(workload, 9000)

        assert endpoint == "svc.example.com:9000"
