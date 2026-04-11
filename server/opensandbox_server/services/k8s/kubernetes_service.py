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

import asyncio
import logging
import time
from datetime import datetime, timezone
from typing import Optional, Dict, Any

from fastapi import HTTPException, status

from opensandbox_server.extensions import apply_access_renew_extend_seconds_to_mapping
from opensandbox_server.extensions.keys import ACCESS_RENEW_EXTEND_SECONDS_METADATA_KEY
from opensandbox_server.api.schema import (
    CreateSandboxRequest,
    CreateSandboxResponse,
    Endpoint,
    ListSandboxesRequest,
    ListSandboxesResponse,
    RenewSandboxExpirationRequest,
    RenewSandboxExpirationResponse,
    Sandbox,
    SandboxStatus,
)
from opensandbox_server.config import AppConfig, get_config
from opensandbox_server.services.constants import (
    SANDBOX_ID_LABEL,
    SandboxErrorCodes,
)
from opensandbox_server.services.endpoint_auth import generate_egress_token
from opensandbox_server.services.extension_service import ExtensionService
from opensandbox_server.services.k8s.create_helpers import _build_create_workload_context
from opensandbox_server.services.k8s.error_helpers import _build_k8s_api_error
from opensandbox_server.services.k8s.k8s_diagnostics import K8sDiagnosticsMixin
from opensandbox_server.services.k8s.endpoint_resolver import _attach_egress_auth_headers
from opensandbox_server.services.k8s.list_helpers import _build_list_sandboxes_response
from opensandbox_server.services.k8s.status_helpers import (
    _is_unschedulable_status,
    _normalize_create_status,
)
from opensandbox_server.services.k8s.workload_mapper import (
    _build_sandbox_from_workload,
    _extract_platform_from_workload,
)
from opensandbox_server.services.k8s.workload_access import (
    _delete_workload_or_404,
    _get_workload_or_404,
)
from opensandbox_server.services.sandbox_service import SandboxService
from opensandbox_server.services.validators import (
    ensure_entrypoint,
    ensure_egress_configured,
    ensure_future_expiration,
    ensure_metadata_labels,
    ensure_platform_valid,
    ensure_timeout_within_limit,
    ensure_volumes_valid,
)
from opensandbox_server.services.k8s.client import K8sClient
from opensandbox_server.services.k8s.provider_factory import create_workload_provider

logger = logging.getLogger(__name__)


class KubernetesSandboxService(K8sDiagnosticsMixin, SandboxService, ExtensionService):
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
        
        self.ingress_config = self.app_config.ingress

        self.namespace = self.app_config.kubernetes.namespace
        self.execd_image = runtime_config.execd_image
        
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
        
        provider_type = self.app_config.kubernetes.workload_provider
        try:
            self.workload_provider = create_workload_provider(
                provider_type=provider_type,
                k8s_client=self.k8s_client,
                app_config=self.app_config,
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
    
    async def _wait_for_sandbox_ready(
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
                workload = self.workload_provider.get_workload(
                    sandbox_id=sandbox_id,
                    namespace=self.namespace,
                )
                
                if not workload:
                    logger.debug(f"Workload not found yet for sandbox {sandbox_id}")
                    time.sleep(poll_interval_seconds)
                    continue
                
                status_info = _normalize_create_status(
                    self.workload_provider.get_status(workload)
                )
                current_state = status_info["state"]
                current_message = status_info["message"]
                
                if current_state != last_state or current_message != last_message:
                    logger.info(
                        f"Sandbox {sandbox_id} state: {current_state} - {current_message}"
                    )
                    last_state = current_state
                    last_message = current_message
                
                if current_state in ("Running", "Allocated"):
                    return workload
                if _is_unschedulable_status(status_info):
                    raise HTTPException(
                        status_code=status.HTTP_400_BAD_REQUEST,
                        detail={
                            "code": SandboxErrorCodes.INVALID_PARAMETER,
                            "message": (
                                f"Sandbox {sandbox_id} is unschedulable: "
                                f"{current_message or status_info.get('reason') or 'no scheduler details'}"
                            ),
                        },
                    )
                
            except HTTPException:
                raise
            except Exception as e:
                logger.warning(
                    f"Error checking sandbox {sandbox_id} status: {e}",
                    exc_info=True
                )
            
            await asyncio.sleep(poll_interval_seconds)
        
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

    def _ensure_network_policy_support(self, request: CreateSandboxRequest) -> None:
        """
        Validate that network policy can be honored under the current runtime config.
        
        This validates that egress.image is configured when network_policy is provided.
        """
        ensure_egress_configured(request.network_policy, self.app_config.egress)

    def _ensure_image_auth_support(self, request: CreateSandboxRequest) -> None:
        """
        Validate image auth support for the current workload provider.

        Raises HTTP 400 if the provider does not support per-request image auth.
        """
        if request.image.auth is None:
            return
        if self.workload_provider.supports_image_auth():
            return
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail={
                "code": SandboxErrorCodes.INVALID_PARAMETER,
                "message": (
                    "image.auth is not supported by the current workload provider. "
                    "Use imagePullSecrets via Kubernetes ServiceAccount or sandbox template."
                ),
            },
        )

    async def create_sandbox(self, request: CreateSandboxRequest) -> CreateSandboxResponse:
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
        ensure_entrypoint(request.entrypoint)
        ensure_metadata_labels(request.metadata)
        ensure_platform_valid(request.platform)
        ensure_timeout_within_limit(
            request.timeout,
            self.app_config.server.max_sandbox_timeout_seconds,
        )
        self._ensure_network_policy_support(request)
        self._ensure_image_auth_support(request)
        
        sandbox_id = self.generate_sandbox_id()
        
        created_at = datetime.now(timezone.utc)
        context = _build_create_workload_context(
            app_config=self.app_config,
            request=request,
            sandbox_id=sandbox_id,
            created_at=created_at,
            egress_token_factory=generate_egress_token,
        )
        
        try:
            apply_access_renew_extend_seconds_to_mapping(context.annotations, request.extensions)

            ensure_volumes_valid(
                request.volumes,
                self.app_config.storage.allowed_host_paths or None,
            )
            
            workload_info = self.workload_provider.create_workload(
                sandbox_id=sandbox_id,
                namespace=self.namespace,
                image_spec=request.image,
                entrypoint=request.entrypoint,
                env=request.env or {},
                resource_limits=context.resource_limits,
                labels=context.labels,
                annotations=context.annotations or None,
                expires_at=context.expires_at,
                execd_image=self.execd_image,
                extensions=request.extensions,
                network_policy=request.network_policy,
                egress_image=context.egress_image,
                egress_auth_token=context.egress_auth_token,
                egress_mode=context.egress_mode,
                volumes=request.volumes,
                platform=request.platform,
            )
            
            logger.info(
                "Created sandbox: id=%s, workload=%s",
                sandbox_id,
                workload_info.get("name"),
            )
            
            try:
                workload = await self._wait_for_sandbox_ready(
                    sandbox_id=sandbox_id,
                    timeout_seconds=self.app_config.kubernetes.sandbox_create_timeout_seconds,
                    poll_interval_seconds=self.app_config.kubernetes.sandbox_create_poll_interval_seconds,
                )
                
                status_info = _normalize_create_status(
                    self.workload_provider.get_status(workload)
                )
                effective_platform = _extract_platform_from_workload(workload)
                
                return CreateSandboxResponse(
                    id=sandbox_id,
                    status=SandboxStatus(
                        state=status_info["state"],
                        reason=status_info["reason"],
                        message=status_info["message"],
                        last_transition_at=status_info["last_transition_at"],
                    ),
                    created_at=created_at,
                    expires_at=context.expires_at,
                    metadata=request.metadata,
                    entrypoint=request.entrypoint,
                    platform=effective_platform or request.platform,
                )
                
            except HTTPException as e:
                try:
                    logger.error(f"Creation failed, cleaning up sandbox {sandbox_id}: {e}")
                    self.workload_provider.delete_workload(sandbox_id, self.namespace)
                except Exception as cleanup_ex:
                    logger.error(f"Failed to cleanup sandbox {sandbox_id}", exc_info=cleanup_ex)
                raise
            
        except HTTPException:
            raise
        except ValueError as e:
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
            workload = _get_workload_or_404(
                self.workload_provider,
                self.namespace,
                sandbox_id,
            )
            return _build_sandbox_from_workload(workload, self.workload_provider)
            
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error getting sandbox {sandbox_id}: {e}")
            raise _build_k8s_api_error("get sandbox", e) from e
    
    def list_sandboxes(self, request: ListSandboxesRequest) -> ListSandboxesResponse:
        """
        List sandboxes with filtering and pagination.
        
        Args:
            request: List request with filters and pagination
            
        Returns:
            ListSandboxesResponse: Paginated list of sandboxes
        """
        try:
            label_selector = SANDBOX_ID_LABEL
            workloads = self.workload_provider.list_workloads(
                namespace=self.namespace,
                label_selector=label_selector,
            )
            sandboxes = [
                _build_sandbox_from_workload(w, self.workload_provider)
                for w in workloads
            ]
            
            return _build_list_sandboxes_response(sandboxes, request)
            
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
            _delete_workload_or_404(
                self.workload_provider,
                self.namespace,
                sandbox_id,
            )
            logger.info(f"Deleted sandbox: {sandbox_id}")

        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error deleting sandbox {sandbox_id}: {e}")
            raise _build_k8s_api_error("delete sandbox", e) from e
    
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

    def get_access_renew_extend_seconds(self, sandbox_id: str) -> Optional[int]:
        workload = self.workload_provider.get_workload(
            sandbox_id=sandbox_id,
            namespace=self.namespace,
        )
        if not workload:
            return None
        if isinstance(workload, dict):
            annotations = workload.get("metadata", {}).get("annotations") or {}
        else:
            md = getattr(workload, "metadata", None)
            raw_ann = getattr(md, "annotations", None) if md else None
            annotations = raw_ann if isinstance(raw_ann, dict) else {}
        raw = annotations.get(ACCESS_RENEW_EXTEND_SECONDS_METADATA_KEY)
        if raw is None or not str(raw).strip():
            return None
        try:
            return int(str(raw).strip())
        except ValueError:
            return None

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
        new_expiration = ensure_future_expiration(request.expires_at)
        
        try:
            workload = _get_workload_or_404(
                self.workload_provider,
                self.namespace,
                sandbox_id,
            )

            current_expiration = self.workload_provider.get_expiration(workload)
            if current_expiration is None:
                raise HTTPException(
                    status_code=status.HTTP_409_CONFLICT,
                    detail={
                        "code": SandboxErrorCodes.INVALID_EXPIRATION,
                        "message": f"Sandbox {sandbox_id} does not have automatic expiration enabled.",
                    },
                )

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
            raise _build_k8s_api_error("renew expiration", e) from e
    
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
            workload = _get_workload_or_404(
                self.workload_provider,
                self.namespace,
                sandbox_id,
            )
            
            endpoint = self.workload_provider.get_endpoint_info(workload, port, sandbox_id)
            if not endpoint:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_POD_IP_NOT_AVAILABLE,
                        "message": "Pod IP is not yet available. The Pod may still be starting.",
                    },
                )
            _attach_egress_auth_headers(endpoint, workload)
            return endpoint
            
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error getting endpoint for {sandbox_id}:{port}: {e}")
            raise _build_k8s_api_error("get endpoint", e) from e

