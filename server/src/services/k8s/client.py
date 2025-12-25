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
Kubernetes client wrapper for managing cluster connections and API access.
"""

from typing import Optional
from kubernetes import client, config
from kubernetes.client import CoreV1Api, CustomObjectsApi

from src.config import KubernetesRuntimeConfig


class K8sClient:
    """
    Wrapper for Kubernetes API client with configuration management.
    
    Handles both in-cluster and kubeconfig-based authentication.
    """
    
    def __init__(self, k8s_config: KubernetesRuntimeConfig):
        """
        Initialize Kubernetes client.
        
        Args:
            k8s_config: Kubernetes runtime configuration
            
        Raises:
            Exception: If unable to load Kubernetes configuration
        """
        self.config = k8s_config
        self._load_config()
        self._core_v1_api: Optional[CoreV1Api] = None
        self._custom_objects_api: Optional[CustomObjectsApi] = None
    
    def _load_config(self) -> None:
        """
        Load Kubernetes configuration from kubeconfig or in-cluster.
        
        Raises:
            Exception: If configuration loading fails
        """
        try:
            if self.config.kubeconfig_path:
                # Load from kubeconfig file
                config.load_kube_config(config_file=self.config.kubeconfig_path)
            else:
                # Load in-cluster config (when running inside K8s)
                config.load_incluster_config()
        except Exception as e:
            raise Exception(f"Failed to load Kubernetes configuration: {e}") from e
    
    def get_core_v1_api(self) -> CoreV1Api:
        """
        Get CoreV1Api client instance.
        
        Returns:
            CoreV1Api: Kubernetes Core V1 API client
        """
        if self._core_v1_api is None:
            self._core_v1_api = client.CoreV1Api()
        return self._core_v1_api
    
    def get_custom_objects_api(self) -> CustomObjectsApi:
        """
        Get CustomObjectsApi client instance for CRD operations.
        
        Returns:
            CustomObjectsApi: Kubernetes Custom Objects API client
        """
        if self._custom_objects_api is None:
            self._custom_objects_api = client.CustomObjectsApi()
        return self._custom_objects_api
