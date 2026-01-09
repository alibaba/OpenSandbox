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
Kubernetes-based implementation of SandboxService.

This module provides a Kubernetes implementation of the sandbox service interface,
using Kubernetes resources for sandbox lifecycle management.
"""

import logging
import time
from datetime import datetime, timedelta, timezone
from typing import Optional, Dict, Any

from fastapi import HTTPException, status

from src.api.schema import (
    CreateSandboxRequest,
    CreateSandboxResponse,
    Endpoint,
    ListSandboxesRequest,
    ListSandboxesResponse,
    PaginationInfo,
    RenewSandboxExpirationRequest,
    RenewSandboxExpirationResponse,
    Sandbox,
    SandboxStatus,
)
from src.config import AppConfig, get_config
from src.services.constants import (
    SANDBOX_ID_LABEL,
    SandboxErrorCodes,
)
from src.services.helpers import matches_filter
from src.services.sandbox_service import SandboxService
from src.services.validators import (
    ensure_entrypoint,
    ensure_future_expiration,
    ensure_metadata_labels,
)
from src.services.k8s.client import K8sClient
from src.services.k8s.provider_factory import create_workload_provider

logger = logging.getLogger(__name__)


class KubernetesSandboxService(SandboxService):
    """
    Kubernetes-based implementation of SandboxService.
    
    This class implements sandbox lifecycle operations using Kubernetes resources.
    """
    
    def __init__(self, config: Optional[AppConfig] = None):
        """
        Initialize Kubernetes sandbox service.
        
        Args:
            config: Application configuration
            
        Raises:
            HTTPException: If initialization fails
        """
        self.app_config = config or get_config()
        runtime_config = self.app_config.runtime
        
        if runtime_config.type != "kubernetes":
            raise ValueError("KubernetesSandboxService requires runtime.type = 'kubernetes'")
        
        if not self.app_config.kubernetes:
            raise ValueError("Kubernetes configuration is required")
        
        self.namespace = self.app_config.kubernetes.namespace
        self.execd_image = runtime_config.execd_image
        self.service_account = self.app_config.kubernetes.service_account
        
        # Initialize Kubernetes client
        try:
            self.k8s_client = K8sClient(self.app_config.kubernetes)
            logger.info("Kubernetes client initialized successfully")
        except Exception as e:
            logger.error(f"Failed to initialize Kubernetes client: {e}")
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail={
                    "code": SandboxErrorCodes.K8S_INITIALIZATION_ERROR,
                    "message": f"Failed to initialize Kubernetes client: {str(e)}",
                },
            ) from e
        
        # Initialize workload provider
        provider_type = self.app_config.kubernetes.workload_provider
        try:
            self.workload_provider = create_workload_provider(
                provider_type=provider_type,
                k8s_client=self.k8s_client,
                k8s_config=self.app_config.kubernetes,
            )
            logger.info(
                f"Initialized workload provider: {self.workload_provider.__class__.__name__}"
            )
        except ValueError as e:
            logger.error(f"Failed to create workload provider: {e}")
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail={
                    "code": SandboxErrorCodes.K8S_INITIALIZATION_ERROR,
                    "message": f"Invalid workload provider configuration: {str(e)}",
                },
            ) from e
        
        logger.info(
            "KubernetesSandboxService initialized: namespace=%s, execd_image=%s",
            self.namespace,
            self.execd_image,
        )
    
    def _wait_for_sandbox_ready(
        self,
        sandbox_id: str,
        timeout_seconds: int = 60,
        poll_interval_seconds: float = 1.0,
    ) -> Dict[str, Any]:
        """
        Wait for Pod to be Running and have an IP address.
        
        Args:
            sandbox_id: Sandbox ID
            timeout_seconds: Maximum time to wait in seconds
            poll_interval_seconds: Time between polling attempts
            
        Returns:
            Workload dict when Pod is Running with IP
            
        Raises:
            HTTPException: If timeout or Pod fails
        """
        logger.info(
            f"Waiting for sandbox {sandbox_id} to be Running with IP (timeout: {timeout_seconds}s)"
        )
        
        start_time = time.time()
        last_state = None
        last_message = None
        
        while time.time() - start_time < timeout_seconds:
            try:
                # Get current workload status
                workload = self.workload_provider.get_workload(
                    sandbox_id=sandbox_id,
                    namespace=self.namespace,
                )
                
                if not workload:
                    logger.debug(f"Workload not found yet for sandbox {sandbox_id}")
                    time.sleep(poll_interval_seconds)
                    continue
                
                # Get status
                status_info = self.workload_provider.get_status(workload)
                current_state = status_info["state"]
                current_message = status_info["message"]
                
                # Log state changes
                if current_state != last_state or current_message != last_message:
                    logger.info(
                        f"Sandbox {sandbox_id} state: {current_state} - {current_message}"
                    )
                    last_state = current_state
                    last_message = current_message
                
                # Check if Failed
                if current_state == "Failed":
                    raise HTTPException(
                        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                        detail={
                            "code": SandboxErrorCodes.K8S_POD_FAILED,
                            "message": f"Pod failed: {current_message}",
                        },
                    )
                
                # Check if Running
                if current_state == "Running":
                    return workload
                
            except HTTPException:
                raise
            except Exception as e:
                logger.warning(
                    f"Error checking sandbox {sandbox_id} status: {e}",
                    exc_info=True
                )
            
            # Wait before next poll
            time.sleep(poll_interval_seconds)
        
        # Timeout
        elapsed = time.time() - start_time
        raise HTTPException(
            status_code=status.HTTP_504_GATEWAY_TIMEOUT,
            detail={
                "code": SandboxErrorCodes.K8S_POD_READY_TIMEOUT,
                "message": (
                    f"Timeout waiting for sandbox {sandbox_id} to be Running with IP. "
                    f"Elapsed: {elapsed:.1f}s, Last state: {last_state}"
                ),
            },
        )
    
    def create_sandbox(self, request: CreateSandboxRequest) -> CreateSandboxResponse:
        """
        Create a new sandbox using Kubernetes Pod.
        
        Wait for the Pod to be Running and have an IP address before returning.
        
        Args:
            request: Sandbox creation request.
            
        Returns:
            CreateSandboxResponse: Created sandbox information with Running state
            
        Raises:
            HTTPException: If creation fails, timeout, or invalid parameters
        """
        # Validate request
        ensure_entrypoint(request.entrypoint)
        ensure_metadata_labels(request.metadata)
        
        # Generate sandbox ID
        sandbox_id = self.generate_sandbox_id()
        
        # Calculate expiration time
        created_at = datetime.now(timezone.utc)
        expires_at = created_at + timedelta(seconds=request.timeout)
        
        # Build labels
        labels = {
            SANDBOX_ID_LABEL: sandbox_id,
        }
        
        # Add user metadata as labels
        if request.metadata:
            labels.update(request.metadata)
        
        # Extract resource limits
        resource_limits = {}
        if request.resource_limits and request.resource_limits.root:
            resource_limits = request.resource_limits.root
        
        try:
            # Create workload
            workload_info = self.workload_provider.create_workload(
                sandbox_id=sandbox_id,
                namespace=self.namespace,
                image_spec=request.image,
                entrypoint=request.entrypoint,
                env=request.env or {},
                resource_limits=resource_limits,
                labels=labels,
                expires_at=expires_at,
                execd_image=self.execd_image,
                extensions=request.extensions,
            )
            
            logger.info(
                "Created sandbox: id=%s, workload=%s",
                sandbox_id,
                workload_info.get("name"),
            )
            
            # Wait for Pod to be Running with IP
            try:
                workload = self._wait_for_sandbox_ready(
                    sandbox_id=sandbox_id,
                    timeout_seconds=60,
                    poll_interval_seconds=1.0,
                )
                
                # Get final status
                status_info = self.workload_provider.get_status(workload)
                
                # Build and return response with Running state
                return CreateSandboxResponse(
                    id=sandbox_id,
                    status=SandboxStatus(
                        state=status_info["state"],
                        reason=status_info["reason"],
                        message=status_info["message"],
                        last_transition_at=status_info["last_transition_at"],
                    ),
                    created_at=created_at,
                    expires_at=expires_at,
                    metadata=request.metadata,
                    image=request.image,
                    entrypoint=request.entrypoint,
                )
                
            except HTTPException:
                # Clean up on failure
                try:
                    logger.warning(f"Creation failed, cleaning up sandbox: {sandbox_id}")
                    self.workload_provider.delete_workload(sandbox_id, self.namespace)
                except Exception as cleanup_ex:
                    logger.error(f"Failed to cleanup sandbox {sandbox_id}", exc_info=cleanup_ex)
                raise
            
        except HTTPException:
            raise
        except ValueError as e:
            # Handle parameter validation errors from provider
            logger.error(f"Invalid parameters for sandbox creation: {e}")
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.INVALID_PARAMETER,
                    "message": str(e),
                },
            ) from e
        except Exception as e:
            logger.error(f"Error creating sandbox: {e}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_API_ERROR,
                    "message": f"Failed to create sandbox: {str(e)}",
                },
            ) from e
    
    def get_sandbox(self, sandbox_id: str) -> Sandbox:
        """
        Get sandbox by ID.
        
        Args:
            sandbox_id: Unique sandbox identifier
            
        Returns:
            Sandbox: Sandbox information
            
        Raises:
            HTTPException: If sandbox not found
        """
        try:
            workload = self.workload_provider.get_workload(
                sandbox_id=sandbox_id,
                namespace=self.namespace,
            )
            
            if not workload:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND,
                        "message": f"Sandbox '{sandbox_id}' not found",
                    },
                )
            
            return self._build_sandbox_from_workload(workload)
            
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error getting sandbox {sandbox_id}: {e}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_API_ERROR,
                    "message": f"Failed to get sandbox: {str(e)}",
                },
            ) from e
    
    def list_sandboxes(self, request: ListSandboxesRequest) -> ListSandboxesResponse:
        """
        List sandboxes with filtering and pagination.
        
        Args:
            request: List request with filters and pagination
            
        Returns:
            ListSandboxesResponse: Paginated list of sandboxes
        """
        try:
            # Build label selector
            label_selector = SANDBOX_ID_LABEL
            
            # List all workloads
            workloads = self.workload_provider.list_workloads(
                namespace=self.namespace,
                label_selector=label_selector,
            )
            
            # Convert to Sandbox objects
            sandboxes = [
                self._build_sandbox_from_workload(w) for w in workloads
            ]
            
            # Apply filters
            filtered = self._apply_filters(sandboxes, request.filter)
            
            # Sort by creation time (newest first)
            filtered.sort(key=lambda s: s.created_at or datetime.min, reverse=True)
            
            # Apply pagination
            total_items = len(filtered)
            page = request.pagination.page
            page_size = request.pagination.page_size
            
            start_idx = (page - 1) * page_size
            end_idx = start_idx + page_size
            paginated_items = filtered[start_idx:end_idx]
            
            total_pages = (total_items + page_size - 1) // page_size
            has_next = page < total_pages
            
            return ListSandboxesResponse(
                items=paginated_items,
                pagination=PaginationInfo(
                    page=page,
                    page_size=page_size,
                    total_items=total_items,
                    total_pages=total_pages,
                    has_next_page=has_next,
                ),
            )
            
        except Exception as e:
            logger.error(f"Error listing sandboxes: {e}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_API_ERROR,
                    "message": f"Failed to list sandboxes: {str(e)}",
                },
            ) from e
    
    def delete_sandbox(self, sandbox_id: str) -> None:
        """
        Delete a sandbox.
        
        Args:
            sandbox_id: Unique sandbox identifier
            
        Raises:
            HTTPException: If deletion fails
        """
        try:
            self.workload_provider.delete_workload(
                sandbox_id=sandbox_id,
                namespace=self.namespace,
            )
            
            logger.info(f"Deleted sandbox: {sandbox_id}")
            
        except Exception as e:
            if "not found" in str(e).lower():
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND,
                        "message": f"Sandbox '{sandbox_id}' not found",
                    },
                ) from e
            
            logger.error(f"Error deleting sandbox {sandbox_id}: {e}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_API_ERROR,
                    "message": f"Failed to delete sandbox: {str(e)}",
                },
            ) from e
    
    def pause_sandbox(self, sandbox_id: str) -> None:
        """
        Pause sandbox (not supported in Kubernetes).
        
        Args:
            sandbox_id: Unique sandbox identifier
            
        Raises:
            HTTPException: Always raises 501 Not Implemented
        """
        raise HTTPException(
            status_code=status.HTTP_501_NOT_IMPLEMENTED,
            detail={
                "code": SandboxErrorCodes.API_NOT_SUPPORTED,
                "message": "Pause operation is not supported in Kubernetes runtime",
            },
        )
    
    def resume_sandbox(self, sandbox_id: str) -> None:
        """
        Resume sandbox (not supported in Kubernetes).
        
        Args:
            sandbox_id: Unique sandbox identifier
            
        Raises:
            HTTPException: Always raises 501 Not Implemented
        """
        raise HTTPException(
            status_code=status.HTTP_501_NOT_IMPLEMENTED,
            detail={
                "code": SandboxErrorCodes.API_NOT_SUPPORTED,
                "message": "Resume operation is not supported in Kubernetes runtime",
            },
        )
    
    def renew_expiration(
        self,
        sandbox_id: str,
        request: RenewSandboxExpirationRequest,
    ) -> RenewSandboxExpirationResponse:
        """
        Renew sandbox expiration time.
        
        Updates both the BatchSandbox spec.expireTime and label for consistency.
        
        Args:
            sandbox_id: Unique sandbox identifier
            request: Renewal request with new expiration time
            
        Returns:
            RenewSandboxExpirationResponse: Updated expiration time
            
        Raises:
            HTTPException: If renewal fails
        """
        # Validate future expiration
        new_expiration = ensure_future_expiration(request.expires_at)
        
        try:
            # Verify sandbox exists
            workload = self.workload_provider.get_workload(
                sandbox_id=sandbox_id,
                namespace=self.namespace,
            )
            
            if not workload:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND,
                        "message": f"Sandbox '{sandbox_id}' not found",
                    },
                )
            
            # Update BatchSandbox spec.expireTime field
            self.workload_provider.update_expiration(
                sandbox_id=sandbox_id,
                namespace=self.namespace,
                expires_at=new_expiration,
            )
            
            logger.info(
                f"Renewed sandbox {sandbox_id} expiration to {new_expiration}"
            )
            
            return RenewSandboxExpirationResponse(
                expires_at=new_expiration
            )
            
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error renewing expiration for {sandbox_id}: {e}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_API_ERROR,
                    "message": f"Failed to renew expiration: {str(e)}",
                },
            ) from e
    
    def get_endpoint(
        self,
        sandbox_id: str,
        port: int,
        resolve_internal: bool = False,
    ) -> Endpoint:
        """
        Get sandbox access endpoint.
        
        Args:
            sandbox_id: Unique sandbox identifier
            port: Port number
            resolve_internal: Ignored for Kubernetes (always returns Pod IP)
            
        Returns:
            Endpoint: Endpoint information
            
        Raises:
            HTTPException: If endpoint not available
        """
        self.validate_port(port)
        
        try:
            workload = self.workload_provider.get_workload(
                sandbox_id=sandbox_id,
                namespace=self.namespace,
            )
            
            if not workload:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND,
                        "message": f"Sandbox '{sandbox_id}' not found",
                    },
                )
            
            endpoint_str = self.workload_provider.get_endpoint_info(workload, port)
            
            if not endpoint_str:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_POD_IP_NOT_AVAILABLE,
                        "message": "Pod IP is not yet available. The Pod may still be starting.",
                    },
                )
            
            return Endpoint(endpoint=endpoint_str)
            
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error getting endpoint for {sandbox_id}:{port}: {e}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_API_ERROR,
                    "message": f"Failed to get endpoint: {str(e)}",
                },
            ) from e
    
    def _build_sandbox_from_workload(self, workload: Any) -> Sandbox:
        """
        Build Sandbox object from Kubernetes workload.
        
        Args:
            workload: Kubernetes workload object (V1Pod or dict for CRD)
            
        Returns:
            Sandbox: Sandbox object
        """
        # Handle both dict (CRD) and object (Pod) formats
        if isinstance(workload, dict):
            metadata = workload.get("metadata", {})
            spec = workload.get("spec", {})
            labels = metadata.get("labels", {})
            creation_timestamp = metadata.get("creationTimestamp")
        else:
            metadata = workload.metadata
            spec = workload.spec
            labels = metadata.labels or {}
            creation_timestamp = metadata.creation_timestamp
        
        sandbox_id = labels.get(SANDBOX_ID_LABEL, "")
        
        # Get expiration from provider
        expires_at = self.workload_provider.get_expiration(workload)
        
        # Get status
        status_info = self.workload_provider.get_status(workload)
        
        # Extract metadata (filter out system labels)
        user_metadata = {
            k: v for k, v in labels.items()
            if not k.startswith("opensandbox.io/")
        }
        
        # Get image and entrypoint from spec
        image_uri = ""
        entrypoint = []
        
        if isinstance(workload, dict):
            # For CRD, extract from template
            template = spec.get("template", {})
            pod_spec = template.get("spec", {})
            containers = pod_spec.get("containers", [])
            if containers:
                container = containers[0]
                image_uri = container.get("image", "")
                entrypoint = container.get("command", [])
        else:
            # For Pod object
            if hasattr(spec, 'containers') and spec.containers:
                container = spec.containers[0]
                image_uri = container.image or ""
                entrypoint = container.command or []
        
        # Create ImageSpec object
        from src.api.schema import ImageSpec
        image_spec = ImageSpec(uri=image_uri) if image_uri else ImageSpec(uri="unknown")
        
        return Sandbox(
            id=sandbox_id,
            status=SandboxStatus(
                state=status_info["state"],
                reason=status_info["reason"],
                message=status_info["message"],
                last_transition_at=status_info["last_transition_at"],
            ),
            created_at=creation_timestamp,
            expires_at=expires_at,
            metadata=user_metadata if user_metadata else None,
            image=image_spec,
            entrypoint=entrypoint,
        )
    
    def _apply_filters(self, sandboxes: list[Sandbox], filter_spec: Any) -> list[Sandbox]:
        """
        Apply filters to sandbox list.
        
        Args:
            sandboxes: List of sandboxes
            filter_spec: Filter specification
            
        Returns:
            Filtered list of sandboxes
        """
        if not filter_spec:
            return sandboxes
        
        filtered = []
        for sandbox in sandboxes:
            if matches_filter(sandbox, filter_spec):
                filtered.append(sandbox)
        
        return filtered
