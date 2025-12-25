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
Unit tests for provider_factory.
"""

import pytest
from unittest.mock import patch

from src.services.k8s.provider_factory import (
    register_provider,
    create_workload_provider,
    list_available_providers,
    PROVIDER_TYPE_BATCHSANDBOX,
)
from src.services.k8s.workload_provider import WorkloadProvider
from src.services.k8s.batchsandbox_provider import BatchSandboxProvider





class TestProviderFactory:
    """provider_factory unit tests"""
    
    def test_register_and_create_batchsandbox_provider(self, mock_k8s_client, k8s_runtime_config):
        """
        Test case: Register and create BatchSandbox provider
        
        Purpose: Verify that BatchSandbox provider can be created through factory method
        """
        provider = create_workload_provider(
            PROVIDER_TYPE_BATCHSANDBOX,
            mock_k8s_client,
            k8s_runtime_config
        )
        
        assert isinstance(provider, BatchSandboxProvider)
        assert provider.k8s_client == mock_k8s_client
    
    def test_create_provider_case_insensitive(self, mock_k8s_client):
        """
        Test case: Case-insensitive provider creation
        
        Purpose: Verify that provider type name is case-insensitive
        """
        provider1 = create_workload_provider("BatchSandbox", mock_k8s_client)
        provider2 = create_workload_provider(PROVIDER_TYPE_BATCHSANDBOX, mock_k8s_client)
        provider3 = create_workload_provider("BATCHSANDBOX", mock_k8s_client)
        
        assert isinstance(provider1, BatchSandboxProvider)
        assert isinstance(provider2, BatchSandboxProvider)
        assert isinstance(provider3, BatchSandboxProvider)
    
    def test_create_provider_with_none_type_uses_default(self, mock_k8s_client):
        """
        Test case: None type uses default provider
        
        Purpose: Verify that the first registered provider is used when provider_type is None
        """
        provider = create_workload_provider(None, mock_k8s_client)
        
        # Should use the first registered provider (batchsandbox)
        assert isinstance(provider, BatchSandboxProvider)
    
    def test_create_provider_with_invalid_type_raises_error(self, mock_k8s_client):
        """
        Test case: Invalid provider type raises exception
        
        Purpose: Verify that ValueError is raised when passing unregistered provider type
        """
        with pytest.raises(ValueError, match="Unsupported workload provider type"):
            create_workload_provider("invalid", mock_k8s_client)
    
    def test_create_batchsandbox_with_template_file(self, mock_k8s_client, k8s_runtime_config, tmp_path):
        """
        Test case: Create BatchSandbox provider with template file
        
        Purpose: Verify that factory method correctly passes template file path to BatchSandboxProvider
        """
        # Create temporary template file
        template_file = tmp_path / "test_template.yaml"
        template_file.write_text("""
apiVersion: execution.alibaba-inc.com/v1alpha1
kind: BatchSandbox
metadata:
  name: test-template
spec:
  template:
    spec:
      nodeSelector:
        gpu: "true"
""")
        
        k8s_runtime_config.batchsandbox_template_file = str(template_file)
        
        with patch.object(BatchSandboxProvider, '__init__', return_value=None) as mock_init:
            create_workload_provider(PROVIDER_TYPE_BATCHSANDBOX, mock_k8s_client, k8s_runtime_config)
            
            # Verify that template_file_path parameter was passed
            mock_init.assert_called_once()
            call_kwargs = mock_init.call_args.kwargs
            assert 'template_file_path' in call_kwargs
            assert call_kwargs['template_file_path'] == str(template_file)
    
    def test_list_available_providers(self):
        """
        Test case: Get registered providers
        
        Purpose: Verify that list of all registered provider types can be retrieved
        """
        providers = list_available_providers()
        
        assert isinstance(providers, list)
        assert PROVIDER_TYPE_BATCHSANDBOX in providers
    
    def test_register_custom_provider(self, mock_k8s_client, isolated_registry):
        """
        Test case: Register custom provider
        
        Purpose: Verify that new provider type can be dynamically registered
        """
        # Create a custom provider class
        class CustomProvider(WorkloadProvider):
            def __init__(self, k8s_client):
                self.k8s_client = k8s_client
            
            def create_workload(self, *args, **kwargs):
                pass
            
            def get_workload(self, *args, **kwargs):
                pass
            
            def delete_workload(self, *args, **kwargs):
                pass
            
            def list_workloads(self, *args, **kwargs):
                pass
            
            def update_expiration(self, *args, **kwargs):
                pass
            
            def get_expiration(self, *args, **kwargs):
                pass
            
            def get_status(self, *args, **kwargs):
                pass
            
            def get_endpoint_info(self, *args, **kwargs):
                pass
        
        # Register custom provider
        register_provider("custom", CustomProvider)
        
        # Verify that custom provider can be created
        provider = create_workload_provider("custom", mock_k8s_client)
        assert isinstance(provider, CustomProvider)
        
        # Verify it's registered
        assert "custom" in list_available_providers()
    
    def test_create_batchsandbox_without_config(self, mock_k8s_client):
        """
        Test case: Create BatchSandbox provider without config
        
        Purpose: Verify that provider can be created even without k8s_config
        """
        provider = create_workload_provider(PROVIDER_TYPE_BATCHSANDBOX, mock_k8s_client, None)
        
        assert isinstance(provider, BatchSandboxProvider)
        assert provider.k8s_client == mock_k8s_client
    
    def test_create_provider_with_empty_registry_raises_error(self, mock_k8s_client, isolated_registry):
        """
        Test case: Creating provider with empty registry raises exception
        
        Purpose: Verify that ValueError is raised when no provider is registered and type is None
        """
        from src.services.k8s import provider_factory
        
        # Clear the registry to test empty registry scenario
        provider_factory._PROVIDER_REGISTRY.clear()
        
        # Verify that ValueError is raised when registry is empty and type is None
        with pytest.raises(ValueError, match="No workload providers are registered"):
            create_workload_provider(None, mock_k8s_client)
