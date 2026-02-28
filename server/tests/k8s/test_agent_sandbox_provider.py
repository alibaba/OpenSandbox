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

from src.api.schema import ImageSpec, NetworkPolicy, NetworkRule
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
            "metadata": {"name": "test-id", "uid": "test-uid"}
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

        assert result == {"name": "test-id", "uid": "test-uid"}

        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        assert body["apiVersion"] == "agents.x-k8s.io/v1alpha1"
        assert body["kind"] == "Sandbox"
        assert body["metadata"]["name"] == "test-id"
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
        mock_api.get_namespaced_custom_object.side_effect = [
            ApiException(status=404),
            ApiException(status=404),
        ]

        result = provider.get_workload("test-id", "test-ns")

        assert result is None

    def test_get_workload_falls_back_to_legacy_name(self, mock_k8s_client):
        """
        Test case: Verify legacy sandbox-<id> name is used when primary lookup 404s
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.get_namespaced_custom_object.side_effect = [
            ApiException(status=404),
            {"metadata": {"name": "sandbox-test-id"}},
        ]

        result = provider.get_workload("test-id", "test-ns")

        assert result["metadata"]["name"] == "sandbox-test-id"
        assert mock_api.get_namespaced_custom_object.call_args_list[0].kwargs["name"] == "test-id"
        assert mock_api.get_namespaced_custom_object.call_args_list[1].kwargs["name"] == "sandbox-test-id"

    def test_get_workload_reraises_non_404_exceptions(self, mock_k8s_client):
        """
        Test case: Verify non-404 exceptions are re-raised
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.get_namespaced_custom_object.side_effect = ApiException(status=500)

        with pytest.raises(ApiException) as exc_info:
            provider.get_workload("test-id", "test-ns")

        assert exc_info.value.status == 500

    def test_get_workload_prefers_informer_cache(self, mock_k8s_client, monkeypatch):
        """
        Test case: Use informer cache when already synced
        """
        cached = {"metadata": {"name": "test-id"}}

        class FakeInformer:
            def __init__(self):
                self.started = False
                self.has_synced = True

            def start(self):
                self.started = True

            def get(self, name):
                return cached if name == "test-id" else None

            def update_cache(self, obj):
                self.updated = obj

        fake_informer = FakeInformer()
        provider = AgentSandboxProvider(
            mock_k8s_client,
            informer_factory=lambda ns: fake_informer,
        )

        result = provider.get_workload("test-id", "test-ns")

        assert result == cached
        assert fake_informer.started is True
        mock_k8s_client.get_custom_objects_api().get_namespaced_custom_object.assert_not_called()

    def test_create_workload_updates_informer_cache(self, mock_k8s_client):
        """
        Test case: informer cache is updated immediately after create
        """
        created_body = {"metadata": {"name": "test-id", "uid": "test-uid"}}

        class FakeInformer:
            def __init__(self):
                self.started = False
                self.updated = None

            def start(self):
                self.started = True

            def update_cache(self, obj):
                self.updated = obj

        fake_informer = FakeInformer()
        provider = AgentSandboxProvider(
            mock_k8s_client,
            informer_factory=lambda ns: fake_informer,
        )
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = created_body

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

        assert result == {"name": "test-id", "uid": "test-uid"}
        assert fake_informer.updated == created_body
        assert fake_informer.started is True

    def test_get_informer_single_instance_per_namespace(self, mock_k8s_client):
        """
        Test case: informer is created only once per namespace even with repeated calls
        """

        class FakeInformer:
            def __init__(self):
                self.started = 0

            def start(self):
                self.started += 1

            def update_cache(self, obj):
                self.updated = obj

        factory_calls = {"count": 0}

        def factory(ns):
            factory_calls["count"] += 1
            return FakeInformer()

        provider = AgentSandboxProvider(
            mock_k8s_client,
            informer_factory=factory,
        )

        informer1 = provider._get_informer("test-ns")
        informer2 = provider._get_informer("test-ns")

        assert informer1 is informer2
        assert factory_calls["count"] == 1
        assert informer1.started == 1

    def test_update_expiration_patches_spec(self, mock_k8s_client):
        """
        Test case: Verify expiration time update
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.get_namespaced_custom_object.return_value = {"metadata": {"name": "sandbox-test-id"}}

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

        endpoint = provider.get_endpoint_info(workload, 8080, "sandbox-123")

        assert endpoint.endpoint == "10.0.0.9:8080"
        assert endpoint.headers is None

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

        endpoint = provider.get_endpoint_info(workload, 9000, "sandbox-123")

        assert endpoint.endpoint == "svc.example.com:9000"
        assert endpoint.headers is None


class TestAgentSandboxProviderEgress:
    """AgentSandboxProvider egress sidecar tests"""

    def test_create_workload_without_network_policy_no_sidecar(self, mock_k8s_client):
        """
        Test case: Verify no sidecar is added when network_policy is None
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "test-id", "uid": "test-uid"}
        }

        expires_at = datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)

        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={},
            resource_limits={},
            labels={},
            expires_at=expires_at,
            execd_image="execd:latest",
            network_policy=None,
            egress_image=None,
        )

        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        pod_spec = body["spec"]["podTemplate"]["spec"]
        containers = pod_spec["containers"]
        
        # Should only have main container
        assert len(containers) == 1
        assert containers[0]["name"] == "sandbox"
        # Should not have securityContext with sysctls
        assert "securityContext" not in pod_spec or "sysctls" not in pod_spec.get("securityContext", {})

    def test_create_workload_with_network_policy_adds_sidecar(self, mock_k8s_client):
        """
        Test case: Verify egress sidecar is added when network_policy is provided
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "test-id", "uid": "test-uid"}
        }

        expires_at = datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[NetworkRule(action="allow", target="pypi.org")],
        )

        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={},
            resource_limits={},
            labels={},
            expires_at=expires_at,
            execd_image="execd:latest",
            network_policy=network_policy,
            egress_image="opensandbox/egress:v1.0.1",
        )

        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        pod_spec = body["spec"]["podTemplate"]["spec"]
        containers = pod_spec["containers"]
        
        # Should have both main container and sidecar
        assert len(containers) == 2
        
        # Find sidecar container
        sidecar = next((c for c in containers if c["name"] == "egress"), None)
        assert sidecar is not None
        assert sidecar["image"] == "opensandbox/egress:v1.0.1"
        
        # Verify sidecar has environment variable
        env_vars = {e["name"]: e["value"] for e in sidecar.get("env", [])}
        assert "OPENSANDBOX_EGRESS_RULES" in env_vars
        
        # Verify sidecar has NET_ADMIN capability
        assert "securityContext" in sidecar
        assert "capabilities" in sidecar["securityContext"]
        assert "add" in sidecar["securityContext"]["capabilities"]
        assert "NET_ADMIN" in sidecar["securityContext"]["capabilities"]["add"]

    def test_create_workload_with_network_policy_adds_ipv6_disable_sysctls(self, mock_k8s_client):
        """
        Test case: Verify IPv6 disable sysctls are added to Pod spec
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "test-id", "uid": "test-uid"}
        }

        expires_at = datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[NetworkRule(action="allow", target="example.com")],
        )

        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={},
            resource_limits={},
            labels={},
            expires_at=expires_at,
            execd_image="execd:latest",
            network_policy=network_policy,
            egress_image="opensandbox/egress:v1.0.1",
        )

        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        pod_spec = body["spec"]["podTemplate"]["spec"]
        
        # Verify securityContext with sysctls exists
        assert "securityContext" in pod_spec
        assert "sysctls" in pod_spec["securityContext"]
        
        sysctls = pod_spec["securityContext"]["sysctls"]
        sysctl_names = {s["name"] for s in sysctls}
        
        # Verify all IPv6 disable sysctls are present
        assert "net.ipv6.conf.all.disable_ipv6" in sysctl_names
        assert "net.ipv6.conf.default.disable_ipv6" in sysctl_names
        assert "net.ipv6.conf.lo.disable_ipv6" in sysctl_names
        
        # Verify all values are "1"
        for sysctl in sysctls:
            assert sysctl["value"] == "1"

    def test_create_workload_with_network_policy_drops_net_admin_from_main_container(self, mock_k8s_client):
        """
        Test case: Verify main container drops NET_ADMIN when network_policy is enabled
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "test-id", "uid": "test-uid"}
        }

        expires_at = datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[NetworkRule(action="allow", target="example.com")],
        )

        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={},
            resource_limits={},
            labels={},
            expires_at=expires_at,
            execd_image="execd:latest",
            network_policy=network_policy,
            egress_image="opensandbox/egress:v1.0.1",
        )

        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        pod_spec = body["spec"]["podTemplate"]["spec"]
        containers = pod_spec["containers"]
        
        # Find main container
        main_container = next((c for c in containers if c["name"] == "sandbox"), None)
        assert main_container is not None
        
        # Verify main container has securityContext
        assert "securityContext" in main_container
        assert "capabilities" in main_container["securityContext"]
        assert "drop" in main_container["securityContext"]["capabilities"]
        assert "NET_ADMIN" in main_container["securityContext"]["capabilities"]["drop"]

    def test_create_workload_without_egress_image_no_sidecar(self, mock_k8s_client):
        """
        Test case: Verify no sidecar is added when egress_image is None even if network_policy exists
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "test-id", "uid": "test-uid"}
        }

        expires_at = datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[NetworkRule(action="allow", target="example.com")],
        )

        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={},
            resource_limits={},
            labels={},
            expires_at=expires_at,
            execd_image="execd:latest",
            network_policy=network_policy,
            egress_image=None,
        )

        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        pod_spec = body["spec"]["podTemplate"]["spec"]
        containers = pod_spec["containers"]
        
        # Should only have main container
        assert len(containers) == 1
        assert containers[0]["name"] == "sandbox"

    def test_egress_sidecar_contains_network_policy_in_env(self, mock_k8s_client):
        """
        Test case: Verify sidecar environment variable contains serialized network policy
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "test-id", "uid": "test-uid"}
        }

        expires_at = datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[
                NetworkRule(action="allow", target="pypi.org"),
                NetworkRule(action="deny", target="*.malicious.com"),
            ],
        )

        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={},
            resource_limits={},
            labels={},
            expires_at=expires_at,
            execd_image="execd:latest",
            network_policy=network_policy,
            egress_image="opensandbox/egress:v1.0.1",
        )

        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        pod_spec = body["spec"]["podTemplate"]["spec"]
        containers = pod_spec["containers"]
        
        sidecar = next((c for c in containers if c["name"] == "egress"), None)
        assert sidecar is not None
        
        env_vars = {e["name"]: e["value"] for e in sidecar.get("env", [])}
        assert "OPENSANDBOX_EGRESS_RULES" in env_vars
        
        # Verify the environment variable contains valid JSON with network policy
        import json
        policy_json = json.loads(env_vars["OPENSANDBOX_EGRESS_RULES"])
        assert policy_json["defaultAction"] == "deny"
        assert len(policy_json["egress"]) == 2
        assert policy_json["egress"][0]["action"] == "allow"
        assert policy_json["egress"][0]["target"] == "pypi.org"

    def test_main_container_no_security_context_without_network_policy(self, mock_k8s_client):
        """
        Test case: Verify main container has no securityContext when network_policy is None
        """
        provider = AgentSandboxProvider(mock_k8s_client)
        mock_api = mock_k8s_client.get_custom_objects_api()
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "test-id", "uid": "test-uid"}
        }

        expires_at = datetime(2025, 12, 31, 10, 0, 0, tzinfo=timezone.utc)

        provider.create_workload(
            sandbox_id="test-id",
            namespace="test-ns",
            image_spec=ImageSpec(uri="python:3.11"),
            entrypoint=["/bin/bash"],
            env={},
            resource_limits={},
            labels={},
            expires_at=expires_at,
            execd_image="execd:latest",
            network_policy=None,
            egress_image=None,
        )

        body = mock_api.create_namespaced_custom_object.call_args.kwargs["body"]
        pod_spec = body["spec"]["podTemplate"]["spec"]
        containers = pod_spec["containers"]
        
        main_container = containers[0]
        # Main container should not have securityContext when no network policy
        assert "securityContext" not in main_container
