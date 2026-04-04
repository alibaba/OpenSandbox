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
Unit tests for KubernetesSandboxService.
"""

import pytest
from datetime import datetime, timedelta, timezone
from unittest.mock import MagicMock, patch
from fastapi import HTTPException

from opensandbox_server.services.k8s.kubernetes_service import KubernetesSandboxService
from opensandbox_server.services.constants import (
    OPEN_SANDBOX_EGRESS_AUTH_HEADER,
    SANDBOX_EGRESS_AUTH_TOKEN_METADATA_KEY,
    SANDBOX_MANUAL_CLEANUP_LABEL,
    SandboxErrorCodes,
)
from opensandbox_server.api.schema import ImageAuth, ListSandboxesRequest, NetworkPolicy
from opensandbox_server.config import EGRESS_MODE_DNS, EGRESS_MODE_DNS_NFT, EgressConfig
from opensandbox_server.api.schema import Endpoint


class TestKubernetesSandboxServiceInit:
    """KubernetesSandboxService initialization tests"""
    
    def test_init_with_valid_config_succeeds(self, k8s_app_config):
        """
        Test case: Successful initialization with valid config
        
        Purpose: Verify that service can be successfully initialized with valid Kubernetes config
        """
        with patch('opensandbox_server.services.k8s.kubernetes_service.K8sClient') as mock_k8s_client, \
             patch('opensandbox_server.services.k8s.kubernetes_service.create_workload_provider') as mock_create_provider:
            
            mock_provider = MagicMock()
            mock_create_provider.return_value = mock_provider
            
            service = KubernetesSandboxService(k8s_app_config)
            
            assert service.namespace == k8s_app_config.kubernetes.namespace
            assert service.execd_image == k8s_app_config.runtime.execd_image
            mock_k8s_client.assert_called_once_with(k8s_app_config.kubernetes)
            mock_create_provider.assert_called_once()
    
    def test_init_without_kubernetes_config_raises_error(self, app_config_no_k8s):
        """
        Test case: Raises exception when Kubernetes config is missing
        
        Purpose: Verify that ValueError is raised when kubernetes section is missing from config
        """
        # app_config_no_k8s still has kubernetes config, just without kubeconfig
        # This will cause K8sClient initialization to fail and raise HTTPException
        with pytest.raises(HTTPException) as exc_info:
            KubernetesSandboxService(app_config_no_k8s)
        
        assert exc_info.value.status_code == 503
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_INITIALIZATION_ERROR
    
    def test_init_with_wrong_runtime_type_raises_error(self, app_config_docker):
        """
        Test case: Raises exception with wrong runtime type
        
        Purpose: Verify that ValueError is raised when runtime.type is not 'kubernetes'
        """
        with pytest.raises(ValueError, match="requires runtime.type = 'kubernetes'"):
            KubernetesSandboxService(app_config_docker)
    
    def test_init_with_k8s_client_failure_raises_http_exception(self, k8s_app_config):
        """
        Test case: Raises HTTPException when K8sClient initialization fails
        
        Purpose: Verify that correct HTTPException is raised when K8sClient initialization fails
        """
        with patch('opensandbox_server.services.k8s.kubernetes_service.K8sClient') as mock_k8s_client:
            mock_k8s_client.side_effect = Exception("Failed to load kubeconfig")
            
            with pytest.raises(HTTPException) as exc_info:
                KubernetesSandboxService(k8s_app_config)
            
            assert exc_info.value.status_code == 503
            assert "code" in exc_info.value.detail
            assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_INITIALIZATION_ERROR


class TestKubernetesSandboxServiceCreate:
    """KubernetesSandboxService create_sandbox tests"""
    
    @pytest.mark.asyncio
    async def test_create_sandbox_with_valid_request_succeeds(
        self, k8s_service, create_sandbox_request, mock_workload
    ):
        """
        Test case: Successfully create sandbox with valid request
        
        Purpose: Verify that sandbox can be successfully created with valid CreateSandboxRequest
        """
        # Mock workload provider
        k8s_service.workload_provider.create_workload.return_value = {
            "name": "test-sandbox-123",
            "uid": "abc-123",
        }
        k8s_service.workload_provider.get_workload.return_value = mock_workload
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "",
            "message": "Pod is running",
            "last_transition_at": datetime.now(timezone.utc),
        }
        k8s_service.workload_provider.get_endpoint_info.return_value = "10.244.0.5:8080"
        k8s_service.workload_provider.get_expiration.return_value = datetime.now(timezone.utc) + timedelta(hours=1)
        
        response = await k8s_service.create_sandbox(create_sandbox_request)
        
        # CreateSandboxResponse uses 'id' field
        assert response.id is not None
        assert response.status.state == "Running"
        k8s_service.workload_provider.create_workload.assert_called_once()

    @pytest.mark.asyncio
    async def test_create_sandbox_uses_configured_timeout_and_poll_interval(
        self, k8s_service, create_sandbox_request, mock_workload
    ):
        """
        Test case: create_sandbox uses timeout and poll_interval from config

        Purpose: Verify that sandbox_create_timeout_seconds and
        sandbox_create_poll_interval_seconds are read from KubernetesRuntimeConfig
        and forwarded to _wait_for_sandbox_ready.
        """


        k8s_service.workload_provider.create_workload.return_value = {
            "name": "test-sandbox-123",
            "uid": "abc-123",
        }
        k8s_service.workload_provider.get_workload.return_value = mock_workload
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "",
            "message": "Pod is running",
            "last_transition_at": datetime.now(timezone.utc),
        }

        # Override config values
        k8s_service.app_config.kubernetes.sandbox_create_timeout_seconds = 120
        k8s_service.app_config.kubernetes.sandbox_create_poll_interval_seconds = 0.5

        with patch.object(k8s_service, "_wait_for_sandbox_ready", wraps=k8s_service._wait_for_sandbox_ready) as mock_wait:
            await k8s_service.create_sandbox(create_sandbox_request)

        mock_wait.assert_called_once()
        _, kwargs = mock_wait.call_args
        assert kwargs["timeout_seconds"] == 120
        assert kwargs["poll_interval_seconds"] == 0.5

    @pytest.mark.asyncio
    async def test_create_sandbox_rejects_image_auth_when_provider_not_supported(
        self, k8s_service, create_sandbox_request
    ):
        k8s_service.workload_provider.supports_image_auth.return_value = False
        create_sandbox_request.image.auth = ImageAuth(
            username="registry-user",
            password="registry-pass",
        )

        with pytest.raises(HTTPException) as exc_info:
            await k8s_service.create_sandbox(create_sandbox_request)

        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_PARAMETER
        k8s_service.workload_provider.create_workload.assert_not_called()

    @pytest.mark.asyncio
    async def test_create_sandbox_allows_image_auth_when_provider_supported(
        self, k8s_service, create_sandbox_request
    ):
        k8s_service.workload_provider.supports_image_auth.return_value = True
        create_sandbox_request.image.auth = ImageAuth(
            username="registry-user",
            password="registry-pass",
        )
        k8s_service.workload_provider.create_workload.return_value = {
            "name": "test-id", "uid": "uid-1"
        }
        k8s_service.workload_provider.get_workload.return_value = MagicMock()
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running", "reason": "", "message": "",
            "last_transition_at": datetime.now(timezone.utc),
        }

        # Should not raise
        await k8s_service.create_sandbox(create_sandbox_request)
        k8s_service.workload_provider.create_workload.assert_called_once()

    @pytest.mark.asyncio
    async def test_create_sandbox_with_no_timeout_calls_provider_with_expires_at_none_and_manual_cleanup_label(
        self, k8s_service, create_sandbox_request
    ):
        """When timeout is None (manual cleanup), provider receives expires_at=None and manual-cleanup label."""
        create_sandbox_request.timeout = None
        k8s_service.workload_provider.create_workload.return_value = {
            "name": "test-id", "uid": "uid-1"
        }
        k8s_service.workload_provider.get_workload.return_value = MagicMock()
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running", "reason": "", "message": "",
            "last_transition_at": datetime.now(timezone.utc),
        }

        await k8s_service.create_sandbox(create_sandbox_request)

        k8s_service.workload_provider.create_workload.assert_called_once()
        _, kwargs = k8s_service.workload_provider.create_workload.call_args
        assert kwargs["expires_at"] is None
        assert kwargs["labels"].get(SANDBOX_MANUAL_CLEANUP_LABEL) == "true"

    @pytest.mark.asyncio
    async def test_create_sandbox_with_network_policy_passes_egress_token_and_annotations(
        self, k8s_service, create_sandbox_request
    ):
        create_sandbox_request.network_policy = NetworkPolicy(default_action="deny", egress=[])
        k8s_service.app_config.egress = EgressConfig(image="opensandbox/egress:v1.0.4")
        k8s_service.workload_provider.create_workload.return_value = {
            "name": "test-id", "uid": "uid-1"
        }
        k8s_service.workload_provider.get_workload.return_value = MagicMock()
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running", "reason": "", "message": "",
            "last_transition_at": datetime.now(timezone.utc),
        }

        with patch(
            "opensandbox_server.services.k8s.kubernetes_service.generate_egress_token",
            return_value="egress-token",
        ):
            await k8s_service.create_sandbox(create_sandbox_request)

        _, kwargs = k8s_service.workload_provider.create_workload.call_args
        assert kwargs["egress_auth_token"] == "egress-token"
        assert kwargs["egress_mode"] == EGRESS_MODE_DNS
        assert kwargs["annotations"][SANDBOX_EGRESS_AUTH_TOKEN_METADATA_KEY] == "egress-token"

    @pytest.mark.asyncio
    async def test_create_sandbox_with_network_policy_passes_egress_mode_dns_nft_from_config(
        self, k8s_service, create_sandbox_request
    ):
        create_sandbox_request.network_policy = NetworkPolicy(default_action="deny", egress=[])
        k8s_service.app_config.egress = EgressConfig(
            image="opensandbox/egress:v1.0.4",
            mode=EGRESS_MODE_DNS_NFT,
        )
        k8s_service.workload_provider.create_workload.return_value = {
            "name": "test-id", "uid": "uid-1"
        }
        k8s_service.workload_provider.get_workload.return_value = MagicMock()
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running", "reason": "", "message": "",
            "last_transition_at": datetime.now(timezone.utc),
        }

        with patch(
            "opensandbox_server.services.k8s.kubernetes_service.generate_egress_token",
            return_value="egress-token",
        ):
            await k8s_service.create_sandbox(create_sandbox_request)

        _, kwargs = k8s_service.workload_provider.create_workload.call_args
        assert kwargs["egress_mode"] == EGRESS_MODE_DNS_NFT

    def test_get_endpoint_merges_egress_auth_header_from_instance_metadata(
        self, k8s_service
    ):
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {
                "annotations": {
                    SANDBOX_EGRESS_AUTH_TOKEN_METADATA_KEY: "egress-token",
                }
            }
        }
        k8s_service.workload_provider.get_endpoint_info.return_value = Endpoint(
            endpoint="gateway.example.com",
            headers={"OpenSandbox-Ingress-To": "sbx-123-44772"},
        )

        endpoint = k8s_service.get_endpoint("sbx-123", 44772)

        assert endpoint.endpoint == "gateway.example.com"
        assert endpoint.headers == {
            "OpenSandbox-Ingress-To": "sbx-123-44772",
            OPEN_SANDBOX_EGRESS_AUTH_HEADER: "egress-token",
        }

    @pytest.mark.asyncio
    async def test_create_sandbox_rejects_timeout_above_configured_maximum(
        self, k8s_service, create_sandbox_request
    ):
        k8s_service.app_config.server.max_sandbox_timeout_seconds = 3600
        create_sandbox_request.timeout = 7200

        with pytest.raises(HTTPException) as exc_info:
            await k8s_service.create_sandbox(create_sandbox_request)

        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_PARAMETER
        assert "configured maximum of 3600s" in exc_info.value.detail["message"]
        k8s_service.workload_provider.create_workload.assert_not_called()


class TestWaitForSandboxReady:
    """_wait_for_sandbox_ready method tests"""
    
    @pytest.mark.asyncio
    async def test_wait_for_running_pod_succeeds(self, k8s_service, mock_workload):
        """
        Test case: Successfully wait for Running Pod
        
        Purpose: Verify that it returns immediately when Pod enters Running state
        """
        k8s_service.workload_provider.get_workload.return_value = mock_workload
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "",
            "message": "Pod is running",
            "last_transition_at": datetime.now(timezone.utc),
        }
        
        result = await k8s_service._wait_for_sandbox_ready("test-sandbox-id", timeout_seconds=10)
        
        assert result == mock_workload
    
    @pytest.mark.asyncio
    async def test_wait_for_pending_then_running_succeeds(self, k8s_service, mock_workload):
        """
        Test case: Successfully wait from Pending to Allocated to Running
        
        Purpose: Verify normal waiting when Pod transitions through Pending -> Allocated -> Running
        """
        # Mock state transition: Pending -> Allocated -> Running
        status_sequence = [
            {"state": "Pending", "reason": "", "message": "Pending", "last_transition_at": datetime.now(timezone.utc)},
            {"state": "Allocated", "reason": "IP_ASSIGNED", "message": "IP assigned", "last_transition_at": datetime.now(timezone.utc)},
            {"state": "Running", "reason": "", "message": "Running", "last_transition_at": datetime.now(timezone.utc)},
        ]
        
        k8s_service.workload_provider.get_workload.return_value = mock_workload
        k8s_service.workload_provider.get_status.side_effect = status_sequence
        
        result = await k8s_service._wait_for_sandbox_ready("test-sandbox-id", timeout_seconds=10, poll_interval_seconds=0.1)
        
        assert result == mock_workload
        assert k8s_service.workload_provider.get_status.call_count == 2
    
    @pytest.mark.asyncio
    async def test_wait_for_allocated_pod_returns_immediately(self, k8s_service, mock_workload):
        """
        Test case: Returns immediately when Pod reaches Allocated state (IP assigned)
        
        Purpose: Verify that Allocated state (IP assigned) is treated as ready
        """
        k8s_service.workload_provider.get_workload.return_value = mock_workload
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Allocated",
            "reason": "IP_ASSIGNED",
            "message": "Pod has IP assigned",
            "last_transition_at": datetime.now(timezone.utc),
        }
        
        result = await k8s_service._wait_for_sandbox_ready("test-sandbox-id", timeout_seconds=10)
        
        assert result == mock_workload
    
    @pytest.mark.asyncio
    async def test_wait_timeout_raises_exception(self, k8s_service, mock_workload):
        """
        Test case: Raises exception on wait timeout
        
        Purpose: Verify that HTTPException is raised when wait times out
        """
        k8s_service.workload_provider.get_workload.return_value = mock_workload
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Pending",
            "reason": "",
            "message": "Still pending",
            "last_transition_at": datetime.now(timezone.utc),
        }
        
        with pytest.raises(HTTPException) as exc_info:
            await k8s_service._wait_for_sandbox_ready("test-sandbox-id", timeout_seconds=1, poll_interval_seconds=0.5)
        
        assert exc_info.value.status_code == 504  # Gateway Timeout
        assert "timeout" in exc_info.value.detail["message"].lower()


class TestKubernetesSandboxServiceRenew:
    def test_renew_expiration_rejects_manual_cleanup_sandbox(self, k8s_service):
        k8s_service.workload_provider.get_workload.return_value = MagicMock()
        k8s_service.workload_provider.get_expiration.return_value = None
        request = MagicMock(expires_at=datetime.now(timezone.utc) + timedelta(hours=1))

        with pytest.raises(HTTPException) as exc_info:
            k8s_service.renew_expiration("test-sandbox-id", request)

        assert exc_info.value.status_code == 409
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_EXPIRATION
        assert (
            exc_info.value.detail["message"]
            == "Sandbox test-sandbox-id does not have automatic expiration enabled."
        )


class TestGetSandbox:
    """get_sandbox method tests"""
    
    def test_get_existing_sandbox_succeeds(self, k8s_service, mock_workload):
        """
        Test case: Successfully get existing sandbox
        
        Purpose: Verify that existing sandbox details can be successfully retrieved
        """
        k8s_service.workload_provider.get_workload.return_value = mock_workload
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "",
            "message": "Running",
            "last_transition_at": datetime.now(timezone.utc),
        }
        k8s_service.workload_provider.get_endpoint_info.return_value = "10.0.0.1:8080"
        k8s_service.workload_provider.get_expiration.return_value = datetime.now(timezone.utc) + timedelta(hours=1)
        
        # Use sandbox_id from mock_workload
        sandbox = k8s_service.get_sandbox("test-sandbox-123")
        
        # Sandbox uses 'id' field
        assert sandbox.id == "test-sandbox-123"
        assert sandbox.status.state == "Running"
    
    def test_get_nonexistent_sandbox_raises_404(self, k8s_service):
        """
        Test case: Raises 404 for nonexistent sandbox
        
        Purpose: Verify that 404 exception is raised when getting nonexistent sandbox
        """
        k8s_service.workload_provider.get_workload.return_value = None
        
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.get_sandbox("nonexistent-id")
        
        assert exc_info.value.status_code == 404
        assert "not found" in exc_info.value.detail["message"].lower()


class TestDeleteSandbox:
    """delete_sandbox method tests"""
    
    def test_delete_existing_sandbox_succeeds(self, k8s_service, mock_workload):
        """
        Test case: Successfully delete existing sandbox
        
        Purpose: Verify that existing sandbox can be successfully deleted
        """
        k8s_service.workload_provider.get_workload.return_value = mock_workload
        k8s_service.workload_provider.delete_workload.return_value = None
        
        k8s_service.delete_sandbox("test-sandbox-id")
        
        k8s_service.workload_provider.delete_workload.assert_called_once_with(
            sandbox_id="test-sandbox-id",
            namespace=k8s_service.namespace
        )
    
    def test_delete_nonexistent_sandbox_raises_404(self, k8s_service):
        """
        Test case: Raises 404 when deleting nonexistent sandbox
        
        Purpose: Verify that 404 exception is raised when deleting nonexistent sandbox
        """
        # Mock delete_workload to raise exception containing "not found"
        k8s_service.workload_provider.delete_workload.side_effect = Exception("Sandbox not found")
        
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.delete_sandbox("nonexistent-id")
        
        assert exc_info.value.status_code == 404


class TestListSandboxes:
    """list_sandboxes method tests"""
    
    def test_list_all_sandboxes_succeeds(self, k8s_service, mock_workload):
        """
        Test case: Successfully list all sandboxes
        
        Purpose: Verify that all sandboxes can be successfully listed
        """
        k8s_service.workload_provider.list_workloads.return_value = [mock_workload]
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "",
            "message": "Running",
            "last_transition_at": datetime.now(timezone.utc),
        }
        k8s_service.workload_provider.get_endpoint_info.return_value = "10.0.0.1:8080"
        k8s_service.workload_provider.get_expiration.return_value = datetime.now(timezone.utc) + timedelta(hours=1)
        
        from opensandbox_server.api.schema import PaginationRequest
        request = ListSandboxesRequest(pagination=PaginationRequest(page=1, page_size=20))
        response = k8s_service.list_sandboxes(request)
        
        # Sandbox in items uses 'id' field
        assert len(response.items) == 1
        assert response.items[0].id == "test-sandbox-123"
        assert response.pagination.total_items == 1
    
    def test_list_sandboxes_with_pagination(self, k8s_service, mock_workload):
        """
        Test case: List sandboxes with pagination
        
        Purpose: Verify that pagination functionality works correctly
        """
        # Create multiple mock workloads using mock_workload as template
        workloads = []
        for i in range(10):
            workload = {
                "metadata": {
                    "name": f"sandbox-{i}",
                    "uid": f"uid-{i}",
                    "labels": {
                        "opensandbox.io/id": f"sandbox-{i}",
                    },
                    "annotations": mock_workload["metadata"]["annotations"].copy(),
                    "creationTimestamp": datetime.now(timezone.utc).isoformat(),
                },
                "spec": {},
                "status": {},
            }
            workloads.append(workload)
        
        k8s_service.workload_provider.list_workloads.return_value = workloads
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "",
            "message": "Running",
            "last_transition_at": datetime.now(timezone.utc),
        }
        k8s_service.workload_provider.get_endpoint_info.return_value = "10.0.0.1:8080"
        k8s_service.workload_provider.get_expiration.return_value = datetime.now(timezone.utc) + timedelta(hours=1)
        
        from opensandbox_server.api.schema import PaginationRequest
        request = ListSandboxesRequest(pagination=PaginationRequest(page=1, page_size=5))
        response = k8s_service.list_sandboxes(request)
        
        assert len(response.items) == 5
        assert response.pagination.page == 1
        assert response.pagination.page_size == 5
        assert response.pagination.total_items == 10
        assert response.pagination.total_pages == 2
    
    def test_list_sandboxes_sorted_by_creation_time(self, k8s_service, mock_workload):
        """
        Test case: Verify sandboxes are sorted by creation time (newest first)
        
        Purpose: Verify that list_sandboxes returns sandboxes sorted by created_at in descending order
        """
        # Create workloads with different creation times
        base_time = datetime.now(timezone.utc)
        workloads = []
        
        # Create sandboxes with specific creation times
        # We'll create them in random order to verify sorting works
        creation_times = [
            base_time - timedelta(hours=5),  # Oldest
            base_time - timedelta(hours=2),
            base_time - timedelta(hours=1),
            base_time - timedelta(minutes=30),
            base_time,  # Newest
        ]
        
        for i, created_at in enumerate(creation_times):
            workload = {
                "metadata": {
                    "name": f"sandbox-{i}",
                    "uid": f"uid-{i}",
                    "labels": {
                        "opensandbox.io/id": f"sandbox-{i}",
                    },
                    "annotations": mock_workload["metadata"]["annotations"].copy(),
                    "creationTimestamp": created_at.isoformat(),
                },
                "spec": {},
                "status": {},
            }
            workloads.append(workload)
        
        k8s_service.workload_provider.list_workloads.return_value = workloads
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "",
            "message": "Running",
            "last_transition_at": datetime.now(timezone.utc),
        }
        k8s_service.workload_provider.get_endpoint_info.return_value = "10.0.0.1:8080"
        k8s_service.workload_provider.get_expiration.return_value = datetime.now(timezone.utc) + timedelta(hours=1)
        
        from opensandbox_server.api.schema import PaginationRequest
        request = ListSandboxesRequest(pagination=PaginationRequest(page=1, page_size=10))
        response = k8s_service.list_sandboxes(request)
        
        # Verify all items are returned
        assert len(response.items) == 5
        
        # Verify they are sorted by creation time (newest first)
        # The order should be: index 4 (newest), 3, 2, 1, 0 (oldest)
        assert response.items[0].id == "sandbox-4"  # Newest
        assert response.items[1].id == "sandbox-3"
        assert response.items[2].id == "sandbox-2"
        assert response.items[3].id == "sandbox-1"
        assert response.items[4].id == "sandbox-0"  # Oldest
        
        # Also verify the creation times are in descending order
        for i in range(len(response.items) - 1):
            assert response.items[i].created_at >= response.items[i + 1].created_at


class TestRenewExpiration:
    """renew_sandbox_expiration method tests"""
    
    def test_renew_expiration_succeeds(self, k8s_service, mock_workload):
        """
        Test case: Successfully renew expiration
        
        Purpose: Verify that sandbox expiration can be successfully renewed
        """
        current_expiration = datetime.now(timezone.utc) + timedelta(hours=1)
        new_expiration = datetime.now(timezone.utc) + timedelta(hours=2)
        
        k8s_service.workload_provider.get_workload.return_value = mock_workload
        k8s_service.workload_provider.update_expiration.return_value = None
        k8s_service.workload_provider.get_expiration.return_value = current_expiration
        
        from opensandbox_server.api.schema import RenewSandboxExpirationRequest
        request = RenewSandboxExpirationRequest(expires_at=new_expiration)
        
        response = k8s_service.renew_expiration("test-sandbox-id", request)
        
        assert response.expires_at == new_expiration
        k8s_service.workload_provider.update_expiration.assert_called_once_with(
            sandbox_id="test-sandbox-id",
            namespace=k8s_service.namespace,
            expires_at=new_expiration
        )
    
    def test_renew_with_past_time_raises_error(self, k8s_service, mock_workload):
        """
        Test case: Raises exception when renewing with past time
        
        Purpose: Verify that HTTPException is raised when renewing with past time
        """
        past_time = datetime.now(timezone.utc) - timedelta(hours=1)
        
        k8s_service.workload_provider.get_workload.return_value = mock_workload
        
        from opensandbox_server.api.schema import RenewSandboxExpirationRequest
        request = RenewSandboxExpirationRequest(expires_at=past_time)
        
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.renew_expiration("test-sandbox-id", request)
        
        assert exc_info.value.status_code == 400

    def test_renew_returns_409_when_sandbox_has_no_expiration(self, k8s_service):
        """Renew is rejected with 409 when sandbox has no TTL (manual cleanup)."""
        k8s_service.workload_provider.get_workload.return_value = MagicMock()
        k8s_service.workload_provider.get_expiration.return_value = None
        from opensandbox_server.api.schema import RenewSandboxExpirationRequest
        request = RenewSandboxExpirationRequest(
            expires_at=datetime.now(timezone.utc) + timedelta(hours=1)
        )

        with pytest.raises(HTTPException) as exc_info:
            k8s_service.renew_expiration("no-ttl-sandbox", request)

        assert exc_info.value.status_code == 409
        assert "does not have automatic expiration" in exc_info.value.detail["message"]
        k8s_service.workload_provider.update_expiration.assert_not_called()

    def test_renew_rejects_time_before_current_expires_at(self, k8s_service):
        """Renew is rejected when the new expiresAt is not after the current expiresAt (spec alignment)."""
        current_expiration = datetime.now(timezone.utc) + timedelta(hours=2)
        # Request a time that is in the future but earlier than the current expiration
        earlier_time = datetime.now(timezone.utc) + timedelta(hours=1)

        k8s_service.workload_provider.get_workload.return_value = MagicMock()
        k8s_service.workload_provider.get_expiration.return_value = current_expiration

        from opensandbox_server.api.schema import RenewSandboxExpirationRequest
        request = RenewSandboxExpirationRequest(expires_at=earlier_time)

        with pytest.raises(HTTPException) as exc_info:
            k8s_service.renew_expiration("test-sandbox-id", request)

        assert exc_info.value.status_code == 400
        assert "after the current expiresAt" in exc_info.value.detail["message"]
        k8s_service.workload_provider.update_expiration.assert_not_called()
