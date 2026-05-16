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

from opensandbox_server.extensions import (
    apply_access_renew_extend_seconds_to_mapping,
    apply_extensions_to_annotations,
)
from opensandbox_server.extensions.keys import ACCESS_RENEW_EXTEND_SECONDS_METADATA_KEY
from opensandbox_server.api.schema import (
    CreateSandboxRequest,
    CreateSandboxResponse,
    Endpoint,
    ListSandboxesRequest,
    ListSandboxesResponse,
    PatchSandboxMetadataRequest,
    RenewSandboxExpirationRequest,
    RenewSandboxExpirationResponse,
    Sandbox,
    SandboxStatus,
)
from opensandbox_server.config import AppConfig, INGRESS_MODE_GATEWAY, SecureAccessConfig, get_config
from opensandbox_server.services.constants import (
    SANDBOX_ID_LABEL,
    SANDBOX_MANAGED_VOLUMES_LABEL,
    SandboxErrorCodes,
)
from opensandbox_server.services.endpoint_auth import generate_egress_token, generate_secure_access_token
from opensandbox_server.services.extension_service import ExtensionService
from opensandbox_server.services.helpers import format_ingress_endpoint
from opensandbox_server.services.k8s.create_helpers import _build_create_workload_context
from opensandbox_server.services.k8s.error_helpers import _build_k8s_api_error
from opensandbox_server.services.k8s.k8s_diagnostics import K8sDiagnosticsMixin
from opensandbox_server.services.k8s.endpoint_resolver import _attach_egress_auth_headers, _attach_secure_access_headers
from opensandbox_server.services.k8s.list_helpers import _build_list_sandboxes_response
from opensandbox_server.services.k8s.status_helpers import (
    _is_unschedulable_status,
    _normalize_create_status,
)
from opensandbox_server.services.k8s.workload_mapper import (
    _build_sandbox_from_workload,
    _extract_platform_from_workload,
)
from opensandbox_server.services.signing import (
    build_canonical_bytes,
    compute_signature,
    encode_expires_b36,
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
from opensandbox_server.services.snapshot_restore import resolve_sandbox_image_from_request

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
                    await asyncio.sleep(poll_interval_seconds)
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

    def _ensure_secure_access_support(self, request: CreateSandboxRequest) -> None:
        """Validate that secure access can be enforced for the configured exposure mode."""
        if not request.secure_access:
            return
        if self.ingress_config and self.ingress_config.mode == INGRESS_MODE_GATEWAY:
            return
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail={
                "code": SandboxErrorCodes.INVALID_PARAMETER,
                "message": (
                    "secureAccess is currently supported only for Kubernetes sandboxes exposed "
                    "through ingress.mode='gateway'. Configure ingress gateway mode or disable secureAccess."
                ),
            },
        )

    def _ensure_pvc_volumes(self, volumes: list, sandbox_id: str) -> list[str]:
        """
        Ensure that PVC volumes exist before creating the workload.

        For each volume with a ``pvc`` backend, check whether the
        PersistentVolumeClaim already exists in the target namespace.
        If not, create it using the provisioning hints from the PVC model.
        Auto-created PVCs are labeled with ``opensandbox.io/volume-managed-by=server``
        and ``opensandbox.io/id=<sandbox_id>`` only when the caller opts into
        cleanup via ``deleteOnSandboxTermination=true`` — that label pair drives
        deletion in ``_cleanup_managed_pvcs``. Pre-existing PVCs and PVCs auto-
        created without the opt-in are never deleted by the server.

        Returns the list of claim names that were freshly created with the
        managed-by labels in this call. The caller uses this list to attach
        ``ownerReferences`` to the workload CR once it is created, so
        controller-driven CR deletion (TTL expiry, cascade delete) also
        garbage-collects the PVC. PVCs that were already present, opt-out
        PVCs, and PVCs we failed to create are excluded from the list.

        Degrades gracefully: if the service account lacks RBAC permissions
        for PVC operations (403), the check is skipped and volume resolution
        is left to the kubelet at pod scheduling time.
        """
        from kubernetes.client import V1PersistentVolumeClaim, V1ObjectMeta
        from kubernetes.client import ApiException

        default_size = self.app_config.storage.volume_default_size

        # Multiple Volume entries may legitimately mount the same PVC at
        # different paths, but their provisioning flags must agree —
        # otherwise the first wins and a later opt-in leaks or a later
        # opt-out is unexpectedly deleted. Reject 400 up front before any
        # side effects.
        flags_by_claim: dict[str, tuple[bool, bool]] = {}
        for vol in volumes:
            if vol.pvc is None:
                continue
            key = (bool(vol.pvc.create_if_not_exists), bool(vol.pvc.delete_on_sandbox_termination))
            prior = flags_by_claim.setdefault(vol.pvc.claim_name, key)
            if prior != key:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail={
                        "code": SandboxErrorCodes.INVALID_PARAMETER,
                        "message": (
                            f"Conflicting provisioning flags for PVC '{vol.pvc.claim_name}': "
                            f"createIfNotExists/deleteOnSandboxTermination must match across all "
                            f"mounts of the same claim."
                        ),
                    },
                )

        managed_pvcs: list[str] = []
        seen_claims: set[str] = set()
        for vol in volumes:
            if vol.pvc is None or not vol.pvc.create_if_not_exists:
                continue
            claim_name = vol.pvc.claim_name
            if claim_name in seen_claims:
                continue
            seen_claims.add(claim_name)

            try:
                existing = self.k8s_client.get_pvc(self.namespace, claim_name)
            except ApiException as e:
                if e.status == 403:
                    logger.warning(
                        f"No RBAC permission to read PVC '{claim_name}', skipping auto-create. "
                        "Grant 'get' and 'create' on 'persistentvolumeclaims' to enable."
                    )
                    return managed_pvcs  # Skip all remaining PVCs — same SA, same permissions
                raise
            if existing is not None:
                logger.debug(f"PVC '{claim_name}' already exists in namespace '{self.namespace}'")
                continue

            storage = vol.pvc.storage or default_size
            access_modes = vol.pvc.access_modes or ["ReadWriteOnce"]
            storage_class = vol.pvc.storage_class  # None = cluster default

            is_managed = bool(vol.pvc.delete_on_sandbox_termination)
            pvc_labels: dict[str, str] = {}
            if is_managed:
                pvc_labels[SANDBOX_MANAGED_VOLUMES_LABEL] = "server"
                pvc_labels[SANDBOX_ID_LABEL] = sandbox_id

            pvc_body = V1PersistentVolumeClaim(
                metadata=V1ObjectMeta(
                    name=claim_name,
                    namespace=self.namespace,
                    labels=pvc_labels or None,
                ),
                spec={
                    "accessModes": access_modes,
                    "resources": {"requests": {"storage": storage}},
                },
            )
            if storage_class is not None:
                pvc_body.spec["storageClassName"] = storage_class

            try:
                self.k8s_client.create_pvc(self.namespace, pvc_body)
                logger.info(
                    f"Auto-created PVC '{claim_name}' (size={storage}, class={storage_class or '<default>'}) "
                    f"in namespace '{self.namespace}'"
                )
                if is_managed:
                    managed_pvcs.append(claim_name)
            except ApiException as e:
                if e.status == 409:
                    # Race condition: another request created it between our check and create.
                    # Don't add to managed_pvcs — whoever created it owns it.
                    logger.info(f"PVC '{claim_name}' was created concurrently, proceeding")
                elif e.status == 403:
                    logger.warning(
                        f"No RBAC permission to create PVC '{claim_name}', skipping. "
                        "The PVC must be pre-created or RBAC must be updated."
                    )
                elif e.status in (400, 422):
                    # Invalid PVC spec from user-provided hints
                    # (e.g. accessModes, storage). These are client errors,
                    # not retryable server faults.
                    raise HTTPException(
                        status_code=status.HTTP_400_BAD_REQUEST,
                        detail={
                            "code": SandboxErrorCodes.INVALID_PARAMETER,
                            "message": f"Invalid PVC spec for '{claim_name}': {e.reason}",
                        },
                    ) from e
                else:
                    logger.error(f"Failed to create PVC '{claim_name}': {e}")
                    raise HTTPException(
                        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                        detail={
                            "code": SandboxErrorCodes.INTERNAL_ERROR,
                            "message": f"Failed to auto-create PVC '{claim_name}': {e.reason}",
                        },
                    ) from e
        return managed_pvcs

    def _attach_pvc_owner_references(
        self,
        claim_names: list[str],
        workload_info: dict,
    ) -> None:
        """
        Patch each PVC to set ``ownerReferences`` pointing at the just-created
        workload CR. K8s garbage collection then deletes the PVC whenever the
        CR is deleted — by ``delete_sandbox``, by TTL expiry handled in the
        controller, or by any other cascade.

        Best-effort: failures are logged but never propagate. The label-based
        ``_cleanup_managed_pvcs`` path remains as a fallback for the
        ``delete_sandbox`` API.
        """
        if not claim_names:
            return
        owner_uid = workload_info.get("uid")
        owner_name = workload_info.get("name")
        owner_api_version = workload_info.get("apiVersion")
        owner_kind = workload_info.get("kind")
        if not (owner_uid and owner_name and owner_api_version and owner_kind):
            logger.warning(
                "Workload provider did not return full owner reference info "
                "(name/uid/apiVersion/kind); skipping PVC ownerReference patch. "
                "PVC cleanup on controller-driven CR deletion may not run."
            )
            return

        owner_ref = {
            "apiVersion": owner_api_version,
            "kind": owner_kind,
            "name": owner_name,
            "uid": owner_uid,
            # blockOwnerDeletion=False so PVC delete failures don't stall the
            # CR delete; controller is the source of truth.
            "blockOwnerDeletion": False,
            # controller=False — we don't claim ownership semantics beyond GC.
            "controller": False,
        }
        patch_body = {"metadata": {"ownerReferences": [owner_ref]}}

        for name in claim_names:
            try:
                self.k8s_client.patch_pvc(self.namespace, name, patch_body)
                logger.debug(
                    f"sandbox={owner_name} | attached ownerReference {owner_kind}/{owner_name} to PVC '{name}'"
                )
            except Exception as e:
                logger.warning(
                    f"sandbox={owner_name} | failed to attach ownerReference to PVC '{name}': {e}. "
                    f"Label-based cleanup on delete_sandbox will still run; "
                    f"controller-driven (TTL) cleanup may not."
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
        request = resolve_sandbox_image_from_request(request)
        ensure_entrypoint(request.entrypoint or [])
        ensure_metadata_labels(request.metadata)
        ensure_platform_valid(request.platform)
        ensure_timeout_within_limit(
            request.timeout,
            self.app_config.server.max_sandbox_timeout_seconds,
        )
        self._ensure_secure_access_support(request)
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
            secure_access_token_factory=generate_secure_access_token,
        )
        
        # Tracks whether we have side effects (auto-created PVCs) that must
        # be swept by the finally clause if the request fails before returning.
        # Set eagerly *before* the call so a partial failure inside
        # _ensure_pvc_volumes (some PVCs created, others not) still triggers
        # cleanup of the ones we managed to label.
        managed_pvcs_may_exist = False
        # Set to True the moment ``create_workload`` returns; cleared only when
        # the workload is confirmed gone (success path, or rollback
        # ``delete_workload`` returns without raising). The ``finally`` clause
        # must not sweep PVCs while the CR is still alive — that would leave a
        # live workload referencing missing storage. Mirrors the semantics of
        # ``delete_sandbox`` which skips PVC cleanup unless the workload was
        # deleted (or already gone).
        workload_left_alive = False
        created_managed_pvcs: list[str] = []
        try:
            apply_access_renew_extend_seconds_to_mapping(context.annotations, request.extensions)
            apply_extensions_to_annotations(context.annotations, request.extensions)

            ensure_volumes_valid(
                request.volumes,
                self.app_config.storage.allowed_host_paths,
            )


            # Auto-create PVCs that don't exist yet
            if request.volumes:
                managed_pvcs_may_exist = True
                created_managed_pvcs = self._ensure_pvc_volumes(request.volumes, sandbox_id)

            # Create workload — once this returns, the CR exists and any
            # rollback must delete it before we can sweep its PVCs.
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
            workload_left_alive = True

            logger.info(
                "Created sandbox: id=%s, workload=%s",
                sandbox_id,
                workload_info.get("name"),
            )

            # Attach ownerReferences so K8s GC removes PVCs whenever the CR is
            # deleted — including TTL expiry handled by the controller, which
            # never invokes our delete_sandbox API and so bypasses
            # _cleanup_managed_pvcs.
            self._attach_pvc_owner_references(created_managed_pvcs, workload_info)

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

                response = CreateSandboxResponse(
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
                # Reached success — the caller now owns the sandbox lifecycle
                # and any PVCs we created. delete_sandbox is responsible for
                # the eventual cleanup.
                managed_pvcs_may_exist = False
                workload_left_alive = False
                return response

            except HTTPException as e:
                try:
                    logger.error(f"Creation failed, cleaning up sandbox {sandbox_id}: {e}")
                    self.workload_provider.delete_workload(sandbox_id, self.namespace)
                    workload_left_alive = False
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
        finally:
            if managed_pvcs_may_exist:
                if workload_left_alive:
                    # Rollback's delete_workload failed (or never ran for a
                    # path that flipped the flag) — the CR is still in the
                    # cluster, possibly with pods that need the PVCs. Skip the
                    # sweep so we don't yank storage from a live workload;
                    # the ownerReference attached after create_workload will
                    # let K8s GC reclaim the PVCs once the CR is eventually
                    # removed (TTL, manual delete, or a delete_sandbox retry).
                    logger.warning(
                        f"sandbox={sandbox_id} | skipping managed-PVC cleanup: "
                        f"workload rollback did not confirm deletion; "
                        f"ownerReference GC will reclaim PVCs when the CR is removed"
                    )
                else:
                    # Best-effort: the caller can't sweep these because the create
                    # API returned no sandbox id. _cleanup_managed_pvcs is scoped
                    # to PVCs labeled with this sandbox_id, so it can't touch
                    # anything else.
                    self._cleanup_managed_pvcs(sandbox_id)

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
        except HTTPException as e:
            # Workload not found (404) still triggers managed-PVC cleanup so a
            # retry can sweep orphans; other errors leave the workload in place,
            # so we must not delete its PVCs.
            if e.status_code == status.HTTP_404_NOT_FOUND:
                self._cleanup_managed_pvcs(sandbox_id)
            raise
        except Exception as e:
            logger.error(f"Error deleting sandbox {sandbox_id}: {e}")
            raise _build_k8s_api_error("delete sandbox", e) from e

        self._cleanup_managed_pvcs(sandbox_id)

    def _cleanup_managed_pvcs(self, sandbox_id: str) -> None:
        """
        Delete PVCs that were auto-created for this sandbox.

        Only PVCs labeled with ``opensandbox.io/volume-managed-by=server`` and
        ``opensandbox.io/id=<sandbox_id>`` are removed; user-managed PVCs are
        never touched. Errors are logged but never propagate — workload
        deletion has already succeeded and PVC cleanup is best-effort.

        Runs after workload deletion so the kubelet has dropped the
        ``kubernetes.io/pvc-protection`` finalizer; otherwise the PVC would
        stay in the ``Terminating`` state until pod teardown completes
        (Kubernetes handles that case correctly, but immediate removal is
        cleaner when the pod is already gone).
        """
        from kubernetes.client import ApiException

        selector = (
            f"{SANDBOX_MANAGED_VOLUMES_LABEL}=server,"
            f"{SANDBOX_ID_LABEL}={sandbox_id}"
        )
        try:
            pvcs = self.k8s_client.list_pvcs(self.namespace, label_selector=selector)
        except ApiException as e:
            if e.status == 403:
                logger.debug(
                    f"No RBAC permission to list PVCs, skipping managed-PVC cleanup for sandbox {sandbox_id}"
                )
                return
            logger.warning(
                f"Failed to list managed PVCs for sandbox {sandbox_id}: {e}"
            )
            return
        except Exception as e:
            logger.warning(
                f"Failed to list managed PVCs for sandbox {sandbox_id}: {e}"
            )
            return

        for pvc in pvcs:
            metadata = getattr(pvc, "metadata", None)
            name = getattr(metadata, "name", None) if metadata is not None else None
            if not name:
                continue
            try:
                self.k8s_client.delete_pvc(self.namespace, name)
                logger.info(
                    f"sandbox={sandbox_id} | deleted managed PVC '{name}' in namespace '{self.namespace}'"
                )
            except ApiException as e:
                if e.status == 403:
                    logger.warning(
                        f"sandbox={sandbox_id} | no RBAC permission to delete PVC '{name}', skipping"
                    )
                    return  # Same SA — no point trying the rest
                logger.warning(
                    f"sandbox={sandbox_id} | failed to delete managed PVC '{name}': {e}"
                )
            except Exception as e:
                logger.warning(
                    f"sandbox={sandbox_id} | failed to delete managed PVC '{name}': {e}"
                )
    
    def pause_sandbox(self, sandbox_id: str) -> None:
        """
        Pause sandbox by delegating to the workload provider.
        """
        try:
            self.workload_provider.pause_sandbox(sandbox_id, self.namespace)
        except NotImplementedError:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.INVALID_STATE,
                    "message": "Pause is not supported for this sandbox type",
                },
            )
        except ValueError as e:
            msg = str(e)
            if "not found" in msg:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND,
                        "message": msg,
                    },
                )
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail={
                    "code": SandboxErrorCodes.INVALID_STATE,
                    "message": msg,
                },
            )
        except Exception as e:
            logger.error("Failed to pause sandbox %s: %s", sandbox_id, e)
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_API_ERROR,
                    "message": f"Failed to pause sandbox: {e}",
                },
            )

    def resume_sandbox(self, sandbox_id: str) -> None:
        """
        Resume sandbox by delegating to the workload provider.
        """
        try:
            self.workload_provider.resume_sandbox(sandbox_id, self.namespace)
        except NotImplementedError:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.INVALID_STATE,
                    "message": "Resume is not supported for this sandbox type",
                },
            )
        except ValueError as e:
            msg = str(e)
            if "not found" in msg:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND,
                        "message": msg,
                    },
                )
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail={
                    "code": SandboxErrorCodes.INVALID_STATE,
                    "message": msg,
                },
            )
        except Exception as e:
            logger.error("Failed to resume sandbox %s: %s", sandbox_id, e)
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail={
                    "code": SandboxErrorCodes.K8S_API_ERROR,
                    "message": f"Failed to resume sandbox: {e}",
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

    def patch_sandbox_metadata(self, sandbox_id: str, patch: PatchSandboxMetadataRequest) -> Sandbox:
        """Patch sandbox metadata via JSON Merge Patch (RFC 7396). Does not restart the sandbox."""
        workload = _get_workload_or_404(
            self.workload_provider,
            self.namespace,
            sandbox_id,
        )

        if isinstance(workload, dict):
            labels = dict(workload.get("metadata", {}).get("labels") or {})
            name = workload["metadata"]["name"]
        else:
            labels = dict(getattr(workload.metadata, "labels", None) or {})
            name = workload.metadata.name

        new_labels = self._apply_metadata_patch(labels, patch)

        try:
            self.workload_provider.patch_labels(
                name=name,
                namespace=self.namespace,
                labels=new_labels,
            )
        except Exception as e:
            logger.error("Error patching labels for sandbox %s: %s", sandbox_id, e)
            raise _build_k8s_api_error("patch sandbox labels", e) from e

        updated = _get_workload_or_404(
            self.workload_provider,
            self.namespace,
            sandbox_id,
        )
        return _build_sandbox_from_workload(updated, self.workload_provider)

    def get_endpoint(
        self,
        sandbox_id: str,
        port: int,
        resolve_internal: bool = False,
        expires: Optional[int] = None,
    ) -> Endpoint:
        """
        Get sandbox access endpoint.

        Args:
            sandbox_id: Unique sandbox identifier
            port: Port number
            resolve_internal: Ignored for Kubernetes (always returns Pod IP)
            expires: Unix epoch seconds for a signed route token.
                Requires ingress gateway mode with secure_access keys configured.

        Returns:
            Endpoint: Endpoint information

        Raises:
            HTTPException: If endpoint not available or signed routes unsupported
        """
        self.validate_port(port)

        if expires is not None:
            if expires < 0:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail={
                        "code": SandboxErrorCodes.INVALID_PARAMETER,
                        "message": "expires must be a non-negative Unix timestamp (uint64).",
                    },
                )
            now = int(time.time())
            if expires <= now:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail={
                        "code": SandboxErrorCodes.INVALID_PARAMETER,
                        "message": f"expires ({expires}) must be greater than current time ({now}).",
                    },
                )
            if expires > 18446744073709551615:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail={
                        "code": SandboxErrorCodes.INVALID_PARAMETER,
                        "message": "expires exceeds uint64 maximum value.",
                    },
                )

        try:
            workload = _get_workload_or_404(
                self.workload_provider,
                self.namespace,
                sandbox_id,
            )

            if expires is not None:
                endpoint = self._build_signed_endpoint(sandbox_id, port, expires)
            else:
                endpoint = self.workload_provider.get_endpoint_info(workload, port, sandbox_id)

            if not endpoint:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_POD_IP_NOT_AVAILABLE,
                        "message": "Pod IP is not yet available. The Pod may still be starting.",
                    },
                )
            if expires is None:
                _attach_secure_access_headers(endpoint, workload)
            _attach_egress_auth_headers(endpoint, workload)
            return endpoint

        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error getting endpoint for {sandbox_id}:{port}: {e}")
            raise _build_k8s_api_error("get endpoint", e) from e

    def _build_signed_endpoint(self, sandbox_id: str, port: int, expires: int) -> Endpoint:
        """Build a signed ingress endpoint per OSEP-0011."""
        secure_cfg = self._get_secure_access_config()

        expires_b36 = encode_expires_b36(expires)
        secret = secure_cfg.get_active_secret_bytes()
        active_key = secure_cfg.active_key
        canonical = build_canonical_bytes(sandbox_id, port, expires_b36)
        signature = compute_signature(secret, active_key, canonical)

        endpoint = format_ingress_endpoint(
            self.ingress_config, sandbox_id, port,
            expires_b36=expires_b36, signature=signature,
        )
        if endpoint is None:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.INVALID_PARAMETER,
                    "message": (
                        "Signed routes are only available when ingress is in gateway mode. "
                        "Configure ingress gateway or omit the expires parameter."
                    ),
                },
            )
        return endpoint

    def _get_secure_access_config(self) -> SecureAccessConfig:
        """Return the secure_access config or raise 400 if not configured."""
        if not self.ingress_config or self.ingress_config.mode != INGRESS_MODE_GATEWAY:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.INVALID_PARAMETER,
                    "message": (
                        "Signed routes require ingress.mode = 'gateway'. "
                        "Configure ingress gateway or omit the expires parameter."
                    ),
                },
            )
        secure = self.ingress_config.secure_access
        if secure is None:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail={
                    "code": SandboxErrorCodes.INVALID_PARAMETER,
                    "message": (
                        "Signed routes require ingress.secure_access to be configured "
                        "with signing keys. Configure secure_access or omit the expires parameter."
                    ),
                },
            )
        return secure
