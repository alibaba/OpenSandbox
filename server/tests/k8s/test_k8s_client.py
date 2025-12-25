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
Unit tests for K8sClient.
"""

from unittest.mock import patch, MagicMock
import pytest

from src.config import KubernetesRuntimeConfig
from src.services.k8s.client import K8sClient


class TestK8sClient:
    """K8sClient unit tests"""
    
    def test_init_with_kubeconfig_loads_successfully(self, k8s_runtime_config):
        """
        Test case: Verify successful initialization with kubeconfig path
        """
        with patch('kubernetes.config.load_kube_config') as mock_load:
            client = K8sClient(k8s_runtime_config)
            
            assert client.config == k8s_runtime_config
            mock_load.assert_called_once_with(
                config_file=k8s_runtime_config.kubeconfig_path
            )
    
    def test_init_with_incluster_config_loads_successfully(self):
        """
        Test case: Verify successful initialization with in-cluster config
        """
        config = KubernetesRuntimeConfig(
            kubeconfig_path=None,
            namespace="test-ns"
        )
        
        with patch('kubernetes.config.load_incluster_config') as mock_load:
            client = K8sClient(config)
            
            assert client.config == config
            mock_load.assert_called_once()
    
    def test_init_with_invalid_kubeconfig_raises_exception(self):
        """
        Test case: Verify exception raised with invalid config file
        """
        config = KubernetesRuntimeConfig(
            kubeconfig_path="/invalid/path",
            namespace="test-ns"
        )
        
        with patch('kubernetes.config.load_kube_config') as mock_load:
            mock_load.side_effect = Exception("Config file not found")
            
            with pytest.raises(Exception) as exc_info:
                K8sClient(config)
            
            assert "Failed to load Kubernetes configuration" in str(exc_info.value)
    
    def test_get_core_v1_api_returns_singleton(self, k8s_runtime_config):
        """
        Test case: Verify CoreV1Api returns singleton
        """
        with patch('kubernetes.config.load_kube_config'), \
             patch('kubernetes.client.CoreV1Api') as mock_api_class:
            
            mock_api_instance = MagicMock()
            mock_api_class.return_value = mock_api_instance
            
            client = K8sClient(k8s_runtime_config)
            
            # First call
            api1 = client.get_core_v1_api()
            # Second call
            api2 = client.get_core_v1_api()
            
            # Should return same instance
            assert api1 is api2
            # API should be created only once
            assert mock_api_class.call_count == 1
    
    def test_get_custom_objects_api_returns_singleton(self, k8s_runtime_config):
        """
        Test case: Verify CustomObjectsApi returns singleton
        """
        with patch('kubernetes.config.load_kube_config'), \
             patch('kubernetes.client.CustomObjectsApi') as mock_api_class:
            
            mock_api_instance = MagicMock()
            mock_api_class.return_value = mock_api_instance
            
            client = K8sClient(k8s_runtime_config)
            
            # First call
            api1 = client.get_custom_objects_api()
            # Second call
            api2 = client.get_custom_objects_api()
            
            # Should return same instance
            assert api1 is api2
            # API should be created only once
            assert mock_api_class.call_count == 1
    
    def test_get_core_v1_api_creates_on_first_call(self, k8s_runtime_config):
        """
        Test case: Verify API client is created on first call
        """
        with patch('kubernetes.config.load_kube_config'), \
             patch('kubernetes.client.CoreV1Api') as mock_api_class:
            
            client = K8sClient(k8s_runtime_config)
            
            # Should not create API on initialization
            assert mock_api_class.call_count == 0
            
            # Create on first call
            client.get_core_v1_api()
            assert mock_api_class.call_count == 1
