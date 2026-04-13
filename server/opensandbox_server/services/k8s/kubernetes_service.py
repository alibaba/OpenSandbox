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
    ImageSpec,
    ListSandboxesRequest,
    ListSandboxesResponse,
    PaginationInfo,
    RenewSandboxExpirationRequest,
    RenewSandboxExpirationResponse,
    Sandbox,
    SandboxStatus,
    PlatformSpec,
)
from opensandbox_server.config import AppConfig, EGRESS_MODE_DNS, get_config
from opensandbox_server.services.constants import (
    SANDBOX_EGRESS_AUTH_TOKEN_METADATA_KEY,
    SANDBOX_ID_LABEL,
    SANDBOX_MANUAL_CLEANUP_LABEL,
    SandboxErrorCodes,
)
from opensandbox_server.services.endpoint_auth import generate_egress_token
from opensandbox_server.services.endpoint_auth import build_egress_auth_headers, merge_endpoint_headers
from opensandbox_server.services.helpers import matches_filter
from opensandbox_server.services.extension_service import ExtensionService
from opensandbox_server.services.k8s.k8s_diagnostics import K8sDiagnosticsMixin
from opensandbox_server.services.sandbox_service import SandboxService
from opensandbox_server.services.validators import (
    calculate_expiration_or_raise,
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
from opensandbox_server.services.k8s.sandboxsnapshot_provider import SandboxSnapshotProvider

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
        
        # Ingress configuration (direct/gateway) if provided
        self.ingress_config = self.app_config.ingress

        self.namespace = self.app_config.kubernetes.namespace
        self.execd_image = runtime_config.execd_image
        
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

        # Initialize snapshot provider for pause/resume
        self.snapshot_provider = SandboxSnapshotProvider(self.k8s_client)

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
                status_info = self._normalize_create_status(
                    self.workload_provider.get_status(workload)
                )
                current_state = status_info["state"]
                current_message = status_info["message"]
                
                # Log state changes
                if current_state != last_state or current_message != last_message:
                    logger.info(
                        f"Sandbox {sandbox_id} state: {current_state} - {current_message}"
                    )
                    last_state = current_state
                    last_message = current_message
                
                # Check if Running or Allocated (IP assigned)
                if current_state in ("Running", "Allocated"):
                    return workload
                if self._is_unschedulable_status(status_info):
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
            
            # Wait before next poll
            await asyncio.sleep(poll_interval_seconds)
        
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

    @staticmethod
    def _normalize_create_status(status_info: Dict[str, Any]) -> Dict[str, Any]:
        if status_info.get("state") != "Allocated":
            return status_info

        return {
            **status_info,
            "state": "Running",
            "message": "Pod has IP assigned and sandbox is ready for requests",
        }

    @staticmethod
    def _is_unschedulable_status(status_info: Dict[str, Any]) -> bool:
        reason = str(status_info.get("reason") or "")
        return reason == "POD_PLATFORM_UNSCHEDULABLE"

    def _ensure_network_policy_support(self, request: CreateSandboxRequest) -> None:
        """
        Validate that network policy can be honored under the current runtime config.
        
        This validates that egress.image is configured when network_policy is provided.
        """
        # Common validation: egress.image must be configured
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
        # Validate request
        ensure_entrypoint(request.entrypoint)
        ensure_metadata_labels(request.metadata)
        ensure_platform_valid(request.platform)
        ensure_timeout_within_limit(
            request.timeout,
            self.app_config.server.max_sandbox_timeout_seconds,
        )
        self._ensure_network_policy_support(request)
        self._ensure_image_auth_support(request)
        
        # Generate sandbox ID
        sandbox_id = self.generate_sandbox_id()
        
        # Calculate expiration time (None = no TTL, manual cleanup only; same as Docker)
        created_at = datetime.now(timezone.utc)
        expires_at = None
        if request.timeout is not None:
            expires_at = calculate_expiration_or_raise(created_at, request.timeout)

        # Build labels
        labels = {
            SANDBOX_ID_LABEL: sandbox_id,
        }
        annotations: Dict[str, str] = {}
        if expires_at is None:
            labels[SANDBOX_MANUAL_CLEANUP_LABEL] = "true"
        
        # Add user metadata as labels
        if request.metadata:
            labels.update(request.metadata)
        
        # Extract resource limits
        resource_limits = {}
        if request.resource_limits and request.resource_limits.root:
            resource_limits = request.resource_limits.root
        
        try:
            egress_mode = (
                self.app_config.egress.mode
                if self.app_config.egress
                else EGRESS_MODE_DNS
            )
            # Get egress image if network policy is provided
            egress_image = None
            egress_auth_token = None
            if request.network_policy:
                egress_image = self.app_config.egress.image if self.app_config.egress else None
                egress_auth_token = generate_egress_token()
                annotations[SANDBOX_EGRESS_AUTH_TOKEN_METADATA_KEY] = egress_auth_token

            apply_access_renew_extend_seconds_to_mapping(annotations, request.extensions)

            # Validate volumes before creating workload
            ensure_volumes_valid(
                request.volumes,
                self.app_config.storage.allowed_host_paths or None,
            )
            
            # Create workload
            workload_info = self.workload_provider.create_workload(
                sandbox_id=sandbox_id,
                namespace=self.namespace,
                image_spec=request.image,
                entrypoint=request.entrypoint,
                env=request.env or {},
                resource_limits=resource_limits,
                labels=labels,
                annotations=annotations or None,
                expires_at=expires_at,
                execd_image=self.execd_image,
                extensions=request.extensions,
                network_policy=request.network_policy,
                egress_image=egress_image,
                egress_auth_token=egress_auth_token,
                egress_mode=egress_mode,
                volumes=request.volumes,
                platform=request.platform,
            )
            
            logger.info(
                "Created sandbox: id=%s, workload=%s",
                sandbox_id,
                workload_info.get("name"),
            )
            
            # Wait for Pod to be Running with IP
            try:
                workload = await self._wait_for_sandbox_ready(
                    sandbox_id=sandbox_id,
                    timeout_seconds=self.app_config.kubernetes.sandbox_create_timeout_seconds,
                    poll_interval_seconds=self.app_config.kubernetes.sandbox_create_poll_interval_seconds,
                )
                
                # Get final status
                status_info = self._normalize_create_status(
                    self.workload_provider.get_status(workload)
                )
                effective_platform = self._extract_platform_from_workload(workload)
                
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

        Aggregates state from both BatchSandbox and SnapshotSnapshot resources.

        Args:
            sandbox_id: Unique sandbox identifier
            
        Returns:
            Sandbox: Sandbox information
            
        Raises:
            HTTPException: If sandbox not found
        """
        try:
            # Get both BatchSandbox and Snapshot for state aggregation
            batchsandbox = self.workload_provider.get_workload(
                sandbox_id=sandbox_id,
                namespace=self.namespace,
            )
            snapshot = self.snapshot_provider.get_snapshot(sandbox_id, self.namespace)

            # Derive aggregated state
            state, reason, message = self._derive_sandbox_state(batchsandbox, snapshot)

            # Handle not found case
            if state == "NotFound":
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND,
                        "message": f"Sandbox '{sandbox_id}' not found",
                    },
                )

            # Build Sandbox from BatchSandbox if available
            if batchsandbox:
                sandbox = self._build_sandbox_from_workload(batchsandbox)
                # Override status with aggregated state
                sandbox.status.state = state
                sandbox.status.reason = reason
                sandbox.status.message = message
                return sandbox

            # Paused state: build from snapshot
            if snapshot:
                metadata = snapshot.get("metadata", {})
                labels = metadata.get("labels", {})
                creation_timestamp = metadata.get("creationTimestamp")

                # Extract user metadata; exclude both opensandbox system label namespaces
                user_metadata = {
                    k: v for k, v in labels.items()
                    if not k.startswith("opensandbox.io/") and not k.startswith("sandbox.opensandbox.io/")
                }

                # Get image URI from snapshot status.containerSnapshots (written by controller after push)
                snapshot_status = snapshot.get("status", {})
                container_snapshots = snapshot_status.get("containerSnapshots", [])
                if container_snapshots:
                    # Multi-container: use first container's image
                    image_uri = container_snapshots[0].get("imageUri", "unknown")
                else:
                    image_uri = "unknown"
                image_spec = ImageSpec(uri=image_uri) if image_uri else ImageSpec(uri="unknown")

                return Sandbox(
                    id=sandbox_id,
                    status=SandboxStatus(
                        state=state,
                        reason=reason,
                        message=message,
                        last_transition_at=creation_timestamp,
                    ),
                    created_at=creation_timestamp,
                    expires_at=None,
                    metadata=user_metadata if user_metadata else None,
                    image=image_spec,
                    entrypoint=[],
                )

            # Should not reach here due to NotFound check above
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail={
                    "code": SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND,
                    "message": f"Sandbox '{sandbox_id}' not found",
                },
            )

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
            sandboxes = [self._build_sandbox_from_workload(w) for w in workloads]

            # Include paused sandboxes (Ready snapshots without BatchSandbox)
            try:
                snapshots = self.snapshot_provider.list_snapshots(
                    namespace=self.namespace,
                    label_selector="sandbox.opensandbox.io/sandbox-id",
                )

                workload_ids = {
                    w.get("metadata", {}).get("labels", {}).get(SANDBOX_ID_LABEL) for w in workloads
                }

                for snap in snapshots:
                    snap_id = snap.get("spec", {}).get("sandboxId", "")
                    phase = snap.get("status", {}).get("phase", "")
                    if snap_id and phase == "Ready" and snap_id not in workload_ids:
                        metadata = snap.get("metadata", {})
                        labels = metadata.get("labels", {})
                        # Get image from status.containerSnapshots (written by controller after push)
                        snap_status = snap.get("status", {})
                        container_snapshots = snap_status.get("containerSnapshots", [])
                        if container_snapshots:
                            image_uri = container_snapshots[0].get("imageUri", "unknown")
                        else:
                            image_uri = "unknown"
                        user_metadata = {
                            k: v for k, v in labels.items()
                            if not k.startswith("opensandbox.io/") and not k.startswith("sandbox.opensandbox.io/")
                        }

                        paused_sandbox = Sandbox(
                            id=snap_id,
                            status=SandboxStatus(
                                state="Paused",
                                reason="SNAPSHOT_READY",
                                message="Sandbox paused",
                                last_transition_at=metadata.get("creationTimestamp"),
                            ),
                            created_at=metadata.get("creationTimestamp"),
                            expires_at=None,
                            metadata=user_metadata if user_metadata else None,
                            image=(
                                ImageSpec(uri=image_uri) if image_uri else ImageSpec(uri="unknown")
                            ),
                            entrypoint=[],
                        )
                        sandboxes.append(paused_sandbox)
            except Exception as e:
                logger.warning("Failed to list paused sandboxes from snapshots: %s", e)

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
        Delete sandbox by deleting BatchSandbox and SandboxSnapshot if they exist.

        Args:
            sandbox_id: Unique sandbox identifier

        Raises:
            HTTPException: 404 if neither BatchSandbox nor SandboxSnapshot exist
            HTTPException: 500 if deletion fails
        """
        # 1. Delete workload
        try:
            workload_deleted = self.workload_provider.delete_workload(sandbox_id, self.namespace)
            if workload_deleted:
                logger.info("Deleted BatchSandbox %s", sandbox_id)
            else:
                logger.debug("BatchSandbox %s not found", sandbox_id)
        except Exception as e:
            logger.error("Failed to delete BatchSandbox %s: %s", sandbox_id, e)
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_DELETE_FAILED,
                    "message": f"Failed to delete BatchSandbox '{sandbox_id}': {e}",
                },
            )

        # 2. Delete snapshot
        try:
            snapshot_deleted = self.snapshot_provider.delete_snapshot(sandbox_id, self.namespace)
            if snapshot_deleted:
                logger.info("Deleted SandboxSnapshot %s", sandbox_id)
            else:
                logger.debug("SandboxSnapshot %s not found", sandbox_id)
        except Exception as e:
            logger.error("Failed to delete SandboxSnapshot %s: %s", sandbox_id, e)
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_DELETE_FAILED,
                    "message": f"Failed to delete SandboxSnapshot '{sandbox_id}': {e}",
                },
            )

        # 3. Check if anything was deleted
        if not workload_deleted and not snapshot_deleted:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail={
                    "code": SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND,
                    "message": f"Sandbox '{sandbox_id}' not found",
                },
            )

    def pause_sandbox(self, sandbox_id: str) -> None:
        """
        Pause sandbox by creating SandboxSnapshot CR.

        The controller handles Pod discovery, commit, push, and BatchSandbox cleanup.
        """
        # 1. Get BatchSandbox
        batchsandbox = self.workload_provider.get_workload(sandbox_id, self.namespace)
        
        # 2. Check for existing snapshot (for already paused sandboxes)
        existing_snapshot = self.snapshot_provider.get_snapshot(sandbox_id, self.namespace)
        
        # If no BatchSandbox but snapshot exists, sandbox is already paused
        if not batchsandbox and existing_snapshot:
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail={
                    "code": SandboxErrorCodes.INVALID_STATE,
                    "message": f"Sandbox '{sandbox_id}' is already paused",
                },
            )
        
        # If neither BatchSandbox nor snapshot exists, sandbox not found
        if not batchsandbox:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail={
                    "code": SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND,
                    "message": f"Sandbox '{sandbox_id}' not found",
                },
            )

        # 3. Validate state
        workload_status = self._normalize_create_status(
            self.workload_provider.get_status(batchsandbox)
        )
        if workload_status["state"] != "Running":
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail={
                    "code": SandboxErrorCodes.INVALID_STATE,
                    "message": f"Cannot pause sandbox in state {workload_status['state']}",
                },
            )

        spec = batchsandbox.get("spec", {})
        if spec.get("replicas", 0) != 1:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.UNSUPPORTED_REPLICAS,
                    "message": "Pause only supports replicas=1",
                },
            )

        # 4. Get pause config from server config (not from BatchSandbox.pausePolicy which is removed)
        pause_config = self.app_config.pause if self.app_config.pause else None
        snapshot_registry = pause_config.snapshot_registry if pause_config else ""
        if not snapshot_registry:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.PAUSE_POLICY_NOT_CONFIGURED,
                    "message": "Pause is not configured: set pause.snapshot_registry in server config",
                },
            )

        # 5. Handle existing snapshot (re-pause or in-progress)
        # Reuse existing_snapshot from step 2

        if existing_snapshot:
            # Phase-based conflict detection
            phase = existing_snapshot.get("status", {}).get("phase")
            if phase in ("Pending", "Committing"):
                raise HTTPException(
                    status_code=status.HTTP_409_CONFLICT,
                    detail={
                        "code": SandboxErrorCodes.SNAPSHOT_IN_PROGRESS,
                        "message": "Snapshot already in progress",
                    },
                )

            # Re-pause: patch spec with latest config from server so rotated credentials
            # or updated registry settings take effect immediately (not stale from first pause).
            batch_sandbox_name = batchsandbox["metadata"]["name"]
            try:
                spec_patch = {
                    "sourceBatchSandboxName": batch_sandbox_name,
                    "action": "Pause",
                    "snapshotType": pause_config.snapshot_type if pause_config else "Rootfs",
                    "snapshotRegistry": snapshot_registry,
                    # removes stale values from the existing CR (null → field deleted).
                    "snapshotPushSecret": (pause_config.snapshot_push_secret or None) if pause_config else None,
                    "resumeImagePullSecret": (pause_config.resume_pull_secret or None) if pause_config else None,
                }
                self.snapshot_provider.patch_snapshot_spec(
                    snapshot_name=sandbox_id,
                    namespace=self.namespace,
                    spec_patch=spec_patch,
                )
                logger.info("Patched SandboxSnapshot %s for re-pause", sandbox_id)
            except Exception as e:
                logger.error("Failed to patch SandboxSnapshot for re-pause: %s", e)
                raise HTTPException(
                    status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                    detail={
                        "code": SandboxErrorCodes.K8S_API_ERROR,
                        "message": f"Failed to re-pause snapshot: {str(e)}",
                    },
                )
            return

        # 6. First pause: create SandboxSnapshot CR with pause config from server
        batch_sandbox_name = batchsandbox["metadata"]["name"]
        snapshot_spec = {
            "sandboxId": sandbox_id,
            "sourceBatchSandboxName": batch_sandbox_name,
            "action": "Pause",
            "snapshotType": pause_config.snapshot_type if pause_config else "Rootfs",
            "snapshotRegistry": snapshot_registry,
        }
        # Add optional secrets from config
        if pause_config:
            if pause_config.snapshot_push_secret:
                snapshot_spec["snapshotPushSecret"] = pause_config.snapshot_push_secret
            if pause_config.resume_pull_secret:
                snapshot_spec["resumeImagePullSecret"] = pause_config.resume_pull_secret

        snapshot_body = {
            "apiVersion": f"{SandboxSnapshotProvider.GROUP}/{SandboxSnapshotProvider.VERSION}",
            "kind": "SandboxSnapshot",
            "metadata": {
                "name": sandbox_id,
                "namespace": self.namespace,
                "labels": {
                    "sandbox.opensandbox.io/sandbox-id": sandbox_id,
                    # Copy user metadata labels from BatchSandbox so paused sandbox reads
                    **{
                        k: v
                        for k, v in batchsandbox.get("metadata", {}).get("labels", {}).items()
                        if not k.startswith("opensandbox.io/") and not k.startswith("sandbox.opensandbox.io/")
                    },
                },
            },
            "spec": snapshot_spec,
        }

        try:
            self.snapshot_provider.create_snapshot(self.namespace, snapshot_body)
            logger.info("Created SandboxSnapshot %s for pause", sandbox_id)
        except Exception as e:
            logger.error("Failed to create SandboxSnapshot: %s", e)
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_API_ERROR,
                    "message": f"Failed to create snapshot: {str(e)}",
                },
            )

    def resume_sandbox(self, sandbox_id: str) -> None:
        """
        Resume sandbox by patching SandboxSnapshot CR to trigger controller-driven resume.

        The controller watches for spec.action == "Resume" and
        creates the BatchSandbox from resumeTemplate.
        """
        # 1. Get SandboxSnapshot
        snapshot = self.snapshot_provider.get_snapshot(sandbox_id, self.namespace)
        if not snapshot:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail={
                    "code": SandboxErrorCodes.SNAPSHOT_NOT_FOUND,
                    "message": f"No snapshot found for sandbox {sandbox_id}",
                },
            )

        # 2. Validate snapshot is Ready
        phase = snapshot.get("status", {}).get("phase")
        if phase != "Ready":
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail={
                    "code": SandboxErrorCodes.SNAPSHOT_NOT_READY,
                    "message": f"Snapshot is in phase {phase}, cannot resume",
                },
            )

        # 3. Check BatchSandbox doesn't already exist
        existing = self.workload_provider.get_workload(sandbox_id, self.namespace)
        if existing:
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail={
                    "code": SandboxErrorCodes.INVALID_STATE,
                    "message": "BatchSandbox already exists, cannot resume",
                },
            )

        # 4. Signal resume intent with action="Resume"
        try:
            self.snapshot_provider.patch_snapshot_spec(
                snapshot_name=sandbox_id,
                namespace=self.namespace,
                spec_patch={"action": "Resume"},
            )
            logger.info(
                "Patched SandboxSnapshot %s with action=Resume to trigger resume",
                sandbox_id,
            )
        except Exception as e:
            logger.error("Failed to patch SandboxSnapshot for resume: %s", e)
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_API_ERROR,
                    "message": f"Failed to trigger resume: {e}",
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

            current_expiration = self.workload_provider.get_expiration(workload)
            if current_expiration is None:
                raise HTTPException(
                    status_code=status.HTTP_409_CONFLICT,
                    detail={
                        "code": SandboxErrorCodes.INVALID_EXPIRATION,
                        "message": f"Sandbox {sandbox_id} does not have automatic expiration enabled.",
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
            
            endpoint = self.workload_provider.get_endpoint_info(workload, port, sandbox_id)
            if not endpoint:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_POD_IP_NOT_AVAILABLE,
                        "message": "Pod IP is not yet available. The Pod may still be starting.",
                    },
                )
            self._attach_egress_auth_headers(endpoint, workload)
            return endpoint
            
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

    def _attach_egress_auth_headers(self, endpoint: Endpoint, workload: Any) -> None:
        token = self._get_egress_auth_token(workload)
        if not token:
            return

        endpoint.headers = merge_endpoint_headers(
            endpoint.headers,
            build_egress_auth_headers(token),
        )

    def _get_egress_auth_token(self, workload: Any) -> Optional[str]:
        if isinstance(workload, dict):
            metadata = workload.get("metadata", {})
            annotations = metadata.get("annotations", {}) or {}
            return annotations.get(SANDBOX_EGRESS_AUTH_TOKEN_METADATA_KEY)

        metadata = getattr(workload, "metadata", None)
        annotations = getattr(metadata, "annotations", None) or {}
        if isinstance(annotations, dict):
            return annotations.get(SANDBOX_EGRESS_AUTH_TOKEN_METADATA_KEY)
        return None

    def _derive_sandbox_state(
        self,
        batchsandbox: Optional[Dict[str, Any]],
        snapshot: Optional[Dict[str, Any]],
    ) -> tuple[str, str, str]:
        """
        Derive sandbox state from BatchSandbox and SnapshotSnapshot.

        Priority:
        1. If BatchSandbox exists and is running → return workload state
           (even if snapshot failed, the workload is still usable)
        2. If snapshot in progress → Pausing
        3. If no workload but snapshot ready → Paused
        4. If no workload and snapshot failed → Failed
        5. If nothing exists → NotFound

        Returns:
            Tuple of (state, reason, message)
        """
        snapshot_phase = None
        if snapshot:
            snapshot_phase = snapshot.get("status", {}).get("phase")

        # 1. BatchSandbox exists - check if it's the active workload
        if batchsandbox:
            annotations = batchsandbox.get("metadata", {}).get("annotations", {})
            is_resumed = annotations.get("sandbox.opensandbox.io/resumed-from-snapshot") == "true"
            status = self.workload_provider.get_status(batchsandbox)
            workload_state = status["state"]

            # Resumed sandbox in transient state (only Pending/Allocated, not terminal Failed)
            if is_resumed and workload_state in ("Pending", "Allocated"):
                return ("Resuming", "RESUMING", "Restoring from snapshot")

            # Snapshot in progress → Pausing (but workload still active)
            if snapshot_phase in ("Pending", "Committing"):
                return ("Pausing", f"SNAPSHOT_{snapshot_phase.upper()}", f"Snapshot is {snapshot_phase.lower()}")

            # Snapshot ready but BatchSandbox still exists → cleanup in progress
            if snapshot_phase == "Ready" and not is_resumed:
                return ("Pausing", "SNAPSHOT_READY_CLEANUP", "Releasing resources")

            # Normal workload state (Running, Pending, Failed, etc.)
            # This includes: snapshot Failed but workload still running → return workload state
            return (workload_state, status["reason"], status["message"])

        # 2. No BatchSandbox - derive state from snapshot only
        if snapshot:
            if snapshot_phase == "Ready":
                return ("Paused", "SNAPSHOT_READY", "Sandbox paused")
            if snapshot_phase in ("Pending", "Committing"):
                return ("Pausing", f"SNAPSHOT_{snapshot_phase.upper()}", f"Snapshot is {snapshot_phase.lower()}")
            if snapshot_phase == "Failed":
                return (
                    "Failed",
                    "SNAPSHOT_FAILED",
                    snapshot.get("status", {}).get("message", "Snapshot failed"),
                )

        # 3. Nothing exists
        return ("NotFound", "SANDBOX_NOT_FOUND", "Sandbox does not exist")

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
            template = spec.get("template") or spec.get("podTemplate") or {}
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
        
        image_spec = ImageSpec(uri=image_uri) if image_uri else ImageSpec(uri="unknown")
        platform_spec = self._extract_platform_from_workload(workload)

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
            platform=platform_spec,
        )

    def _extract_platform_from_workload(
        self,
        workload: Any,
    ) -> Optional[PlatformSpec]:
        if isinstance(workload, dict):
            spec = workload.get("spec", {})
            pod_spec = (
                spec.get("template", {}).get("spec")
                or spec.get("podTemplate", {}).get("spec")
                or {}
            )
        else:
            spec = getattr(workload, "spec", None)
            template = getattr(spec, "template", None)
            pod_template = getattr(spec, "pod_template", None)
            pod_spec = (
                getattr(template, "spec", None)
                or getattr(pod_template, "spec", None)
                or {}
            )

        node_selector = (
            pod_spec.get("nodeSelector", {})
            if isinstance(pod_spec, dict)
            else getattr(pod_spec, "node_selector", {}) or {}
        )
        if not isinstance(node_selector, dict):
            return None

        os_value = node_selector.get("kubernetes.io/os")
        arch_value = node_selector.get("kubernetes.io/arch")
        os_constraint = os_value if isinstance(os_value, str) and os_value else None
        arch_constraint = arch_value if isinstance(arch_value, str) and arch_value else None

        affinity = (
            pod_spec.get("affinity")
            if isinstance(pod_spec, dict)
            else getattr(pod_spec, "affinity", None)
        )
        if os_constraint is None:
            os_constraint = self._extract_platform_value_from_affinity(
                affinity,
                "kubernetes.io/os",
            )
        if arch_constraint is None:
            arch_constraint = self._extract_platform_value_from_affinity(
                affinity,
                "kubernetes.io/arch",
            )

        if os_constraint and arch_constraint:
            return PlatformSpec(os=os_constraint, arch=arch_constraint)
        return None

    @staticmethod
    def _extract_platform_from_affinity(affinity: Any) -> Optional[PlatformSpec]:
        os_value = KubernetesSandboxService._extract_platform_value_from_affinity(
            affinity,
            "kubernetes.io/os",
        )
        arch_value = KubernetesSandboxService._extract_platform_value_from_affinity(
            affinity,
            "kubernetes.io/arch",
        )
        if not os_value or not arch_value:
            return None
        return PlatformSpec(os=os_value, arch=arch_value)

    @staticmethod
    def _extract_platform_value_from_affinity(
        affinity: Any,
        key: str,
    ) -> Optional[str]:
        if affinity is None:
            return None
        node_affinity = (
            affinity.get("nodeAffinity")
            if isinstance(affinity, dict)
            else getattr(affinity, "node_affinity", None)
        )
        if node_affinity is None:
            return None
        required = (
            node_affinity.get("requiredDuringSchedulingIgnoredDuringExecution")
            if isinstance(node_affinity, dict)
            else getattr(
                node_affinity,
                "required_during_scheduling_ignored_during_execution",
                None,
            )
        )
        if required is None:
            return None
        terms = (
            required.get("nodeSelectorTerms", [])
            if isinstance(required, dict)
            else getattr(required, "node_selector_terms", []) or []
        )
        if not isinstance(terms, list) or not terms:
            return None

        inferred: Optional[str] = None
        for term in terms:
            expressions = (
                term.get("matchExpressions", [])
                if isinstance(term, dict)
                else getattr(term, "match_expressions", []) or []
            )
            if not isinstance(expressions, list):
                return None
            term_value: Optional[str] = None
            for expr in expressions:
                expr_key = (
                    expr.get("key")
                    if isinstance(expr, dict)
                    else getattr(expr, "key", None)
                )
                if expr_key != key:
                    continue
                operator = (
                    expr.get("operator")
                    if isinstance(expr, dict)
                    else getattr(expr, "operator", None)
                )
                values = (
                    expr.get("values", [])
                    if isinstance(expr, dict)
                    else getattr(expr, "values", []) or []
                )
                if operator != "In" or not isinstance(values, list) or len(values) != 1:
                    return None
                value = values[0]
                if not isinstance(value, str) or not value:
                    return None
                term_value = value
                break
            if term_value is None:
                return None
            if inferred is None:
                inferred = term_value
            elif inferred != term_value:
                return None
        return inferred

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
