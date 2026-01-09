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
Abstract workload provider interface for Kubernetes resources.
"""

from abc import ABC, abstractmethod
from datetime import datetime
from typing import Dict, List, Any, Optional

from src.api.schema import ImageSpec


class WorkloadProvider(ABC):
    """
    Abstract interface for managing Kubernetes workload resources.
    
    This abstraction allows supporting different K8s resource types
    (Pod, Job, StatefulSet, etc.) with a unified interface.
    """
    
    @abstractmethod
    def create_workload(
        self,
        sandbox_id: str,
        namespace: str,
        image_spec: ImageSpec,
        entrypoint: List[str],
        env: Dict[str, str],
        resource_limits: Dict[str, str],
        labels: Dict[str, str],
        expires_at: datetime,
        execd_image: str,
        extensions: Optional[Dict[str, str]] = None,
    ) -> Dict[str, Any]:
        """
        Create a new workload resource.
        
        Args:
            sandbox_id: Unique sandbox identifier
            namespace: Kubernetes namespace
            image_spec: Container image specification
            entrypoint: Container entrypoint command
            env: Environment variables
            resource_limits: Resource limits (cpu, memory)
            labels: Labels to apply to the workload
            expires_at: Expiration time
            execd_image: execd daemon image
            extensions: General extension field for passing additional configuration.
                This is a flexible field for various use cases (e.g., ``poolRef`` for pool-based creation).
            
        Returns:
            Dict containing workload metadata (name, uid, etc.)
            
        Raises:
            ApiException: If creation fails
        """
        pass
    
    @abstractmethod
    def get_workload(self, sandbox_id: str, namespace: str) -> Optional[Any]:
        """
        Get workload by sandbox ID.
        
        Args:
            sandbox_id: Unique sandbox identifier
            namespace: Kubernetes namespace
            
        Returns:
            Workload object or None if not found
        """
        pass
    
    @abstractmethod
    def delete_workload(self, sandbox_id: str, namespace: str) -> None:
        """
        Delete a workload resource.
        
        Args:
            sandbox_id: Unique sandbox identifier
            namespace: Kubernetes namespace
            
        Raises:
            ApiException: If deletion fails
        """
        pass
    
    @abstractmethod
    def list_workloads(self, namespace: str, label_selector: str) -> List[Any]:
        """
        List workloads matching label selector.
        
        Args:
            namespace: Kubernetes namespace
            label_selector: Label selector query
            
        Returns:
            List of workload objects
        """
        pass
    
    @abstractmethod
    def update_expiration(self, sandbox_id: str, namespace: str, expires_at: datetime) -> None:
        """
        Update workload expiration time.
        
        Args:
            sandbox_id: Unique sandbox identifier
            namespace: Kubernetes namespace
            expires_at: New expiration time
            
        Raises:
            Exception: If update fails
        """
        pass
    
    @abstractmethod
    def get_expiration(self, workload: Any) -> Optional[datetime]:
        """
        Get expiration time from workload.
        
        Args:
            workload: Workload object
            
        Returns:
            Expiration datetime or None if not set
        """
        pass
    
    @abstractmethod
    def get_status(self, workload: Any) -> Dict[str, Any]:
        """
        Get status from workload object.
        
        Args:
            workload: Workload object
            
        Returns:
            Dict with state, reason, message, last_transition_at
        """
        pass
    
    @abstractmethod
    def get_endpoint_info(self, workload: Any, port: int) -> Optional[str]:
        """
        Get endpoint information from workload.
        
        Args:
            workload: Workload object
            port: Port number
            
        Returns:
            Endpoint string (e.g., "10.244.0.5:8080") or None if not available
        """
        pass
