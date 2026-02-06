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
Pydantic schemas for OpenSandbox Lifecycle API.

This module defines data models based on the OpenAPI specification
for request/response validation and serialization.
"""

from datetime import datetime
from typing import Dict, List, Optional

from pydantic import BaseModel, Field, RootModel, model_validator


# ============================================================================
# Image Specification
# ============================================================================

class ImageAuth(BaseModel):
    """
    Registry authentication credentials for private container registries.
    """
    username: str = Field(..., description="Registry username or service account")
    password: str = Field(..., description="Registry password or authentication token")


class ImageSpec(BaseModel):
    """
    Container image specification for sandbox provisioning.

    Supports public registry images and private registry images with authentication.
    """
    uri: str = Field(
        ...,
        description="Container image URI in standard format (e.g., 'python:3.11', 'gcr.io/my-project/app:v1.0')",
    )
    auth: Optional[ImageAuth] = Field(
        None,
        description="Registry authentication credentials (required for private registries)",
    )


# ============================================================================
# Resource Limits
# ============================================================================

class ResourceLimits(RootModel[Dict[str, str]]):
    """
    Runtime resource constraints as key-value pairs.

    Similar to Kubernetes resource specifications, allows flexible definition
    of resource limits. Common resource types include cpu, memory, and gpu.
    """
    root: Dict[str, str] = Field(
        default_factory=dict,
        example={"cpu": "500m", "memory": "512Mi", "gpu": "1"},
    )


class NetworkRule(BaseModel):
    """
    Egress rule: allow/deny a specific domain or wildcard.
    """

    action: str = Field(..., description="Whether to allow or deny matching targets (allow | deny).")
    target: str = Field(
        ...,
        description="FQDN or wildcard domain (e.g., 'example.com', '*.example.com').",
        min_length=1,
    )

    class Config:
        populate_by_name = True


class NetworkPolicy(BaseModel):
    """
    Egress network policy matching the sidecar /policy payload.
    """

    default_action: Optional[str] = Field(
        default=None,
        alias="defaultAction",
        description="Default action when no egress rule matches (allow | deny). If omitted, sidecar defaults to deny.",
    )
    egress: list[NetworkRule] = Field(
        default_factory=list,
        description="Ordered egress rules. Empty/omitted yields allow-all at startup.",
    )

    class Config:
        populate_by_name = True


# ============================================================================
# Volume Definitions
# ============================================================================


class Host(BaseModel):
    """
    Host path bind mount backend.

    Maps a directory on the host filesystem into the container.
    Only available when the runtime supports host mounts.

    Security note: Host paths are restricted by server-side allowlist.
    Users must specify paths under permitted prefixes.
    """

    path: str = Field(
        ...,
        description="Absolute path on the host filesystem to mount. Must start with '/'.",
        pattern=r"^/.*",
    )


class PVC(BaseModel):
    """
    Kubernetes PersistentVolumeClaim mount backend.

    References an existing PVC in the same namespace as the sandbox pod.
    Only available in Kubernetes runtime.
    """

    claim_name: str = Field(
        ...,
        alias="claimName",
        description="Name of the PersistentVolumeClaim in the same namespace.",
        pattern=r"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
        max_length=253,
    )

    class Config:
        populate_by_name = True


class Volume(BaseModel):
    """
    Storage mount definition for a sandbox.

    Each volume entry contains:
    - A unique name identifier
    - Exactly one backend struct (host, pvc, etc.) with backend-specific fields
    - Common mount settings (mountPath, readOnly, subPath)
    """

    name: str = Field(
        ...,
        description="Unique identifier for the volume within the sandbox.",
        pattern=r"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
        max_length=63,
    )
    host: Optional[Host] = Field(
        None,
        description="Host path bind mount backend.",
    )
    pvc: Optional[PVC] = Field(
        None,
        description="Kubernetes PersistentVolumeClaim mount backend.",
    )
    mount_path: str = Field(
        ...,
        alias="mountPath",
        description="Absolute path inside the container where the volume is mounted.",
        pattern=r"^/.*",
    )
    read_only: bool = Field(
        False,
        alias="readOnly",
        description="If true, the volume is mounted as read-only. Defaults to false (read-write).",
    )
    sub_path: Optional[str] = Field(
        None,
        alias="subPath",
        description="Optional subdirectory under the backend path to mount.",
    )

    class Config:
        populate_by_name = True

    @model_validator(mode="after")
    def validate_exactly_one_backend(self) -> "Volume":
        """Ensure exactly one backend type is specified."""
        backends = [self.host, self.pvc]
        specified = [b for b in backends if b is not None]
        if len(specified) == 0:
            raise ValueError("Exactly one backend (host, pvc) must be specified, but none was provided.")
        if len(specified) > 1:
            raise ValueError("Exactly one backend (host, pvc) must be specified, but multiple were provided.")
        return self


# ============================================================================
# Sandbox Status
# ============================================================================

class SandboxStatus(BaseModel):
    """
    Detailed status information with lifecycle state and transition details.
    """
    state: str = Field(
        ...,
        description="Current lifecycle state (Pending, Running, Pausing, Paused, Stopping, Terminated, Failed)",
    )
    reason: Optional[str] = Field(
        None,
        description="Short machine-readable reason code for the current state",
    )
    message: Optional[str] = Field(
        None,
        description="Human-readable message describing the current state or reason for state transition",
    )
    last_transition_at: Optional[datetime] = Field(
        None,
        alias="lastTransitionAt",
        description="Timestamp of the last state transition",
    )

    class Config:
        populate_by_name = True


# ============================================================================
# Sandbox Models
# ============================================================================

class CreateSandboxRequest(BaseModel):
    """
    Request to create a new sandbox from a container image.
    """
    image: ImageSpec = Field(..., description="Container image specification for the sandbox")
    timeout: int = Field(
        ...,
        ge=60,
        le=86400,
        description="Sandbox timeout in seconds (60-86400). The sandbox will automatically terminate after this duration.",
    )
    resource_limits: ResourceLimits = Field(
        ...,
        alias="resourceLimits",
        description="Runtime resource constraints for the sandbox instance",
    )
    env: Optional[Dict[str, Optional[str]]] = Field(
        None,
        description="Environment variables to inject into the sandbox runtime",
    )
    metadata: Optional[Dict[str, str]] = Field(
        None,
        description="Custom key-value metadata for management, filtering, and tagging",
    )
    entrypoint: List[str] = Field(
        ...,
        min_length=1,
        description="The command to execute as the sandbox's entry process",
        example=["python", "/app/main.py"],
    )
    network_policy: Optional[NetworkPolicy] = Field(
        None,
        alias="networkPolicy",
        description=(
            "Optional outbound network policy. Shape matches the egress sidecar /policy endpoint. "
            "Empty/omitted means allow-all until updated."
        ),
    )
    volumes: Optional[List[Volume]] = Field(
        None,
        description=(
            "Storage mounts for the sandbox. Each volume entry specifies a named backend-specific "
            "storage source and common mount settings. Exactly one backend type must be specified per volume entry."
        ),
    )
    extensions: Optional[Dict[str, str]] = Field(
        None,
        description="Opaque container for provider-specific or transient parameters not covered by the core API",
    )

    class Config:
        populate_by_name = True


class CreateSandboxResponse(BaseModel):
    """
    Response from creating a new sandbox.

    Contains essential information without image and updatedAt.
    """
    id: str = Field(..., description="Unique sandbox identifier")
    status: SandboxStatus = Field(..., description="Current lifecycle status and detailed state information")
    metadata: Optional[Dict[str, str]] = Field(None, description="Custom metadata from creation request")
    expires_at: datetime = Field(..., alias="expiresAt", description="Timestamp when sandbox will auto-terminate")
    created_at: datetime = Field(..., alias="createdAt", description="Sandbox creation timestamp")
    entrypoint: List[str] = Field(..., description="Entry process specification from creation request")

    class Config:
        populate_by_name = True


class Sandbox(BaseModel):
    """
    Runtime execution environment provisioned from a container image.

    This is the complete representation of the sandbox resource.
    """
    id: str = Field(..., description="Unique sandbox identifier")
    image: ImageSpec = Field(..., description="Container image specification used to provision this sandbox")
    status: SandboxStatus = Field(..., description="Current lifecycle status and detailed state information")
    metadata: Optional[Dict[str, str]] = Field(None, description="Custom metadata from creation request")
    entrypoint: List[str] = Field(..., description="The command to execute as the sandbox's entry process")
    expires_at: datetime = Field(..., alias="expiresAt", description="Timestamp when sandbox will auto-terminate")
    created_at: datetime = Field(..., alias="createdAt", description="Sandbox creation timestamp")

    class Config:
        populate_by_name = True


# ============================================================================
# List Sandboxes
# ============================================================================

class SandboxFilter(BaseModel):
    """
    Filtering criteria for listing sandboxes.
    """
    state: Optional[List[str]] = Field(
        None,
        min_length=1,
        description="Filter by lifecycle state (status.state) - supports OR logic",
    )
    metadata: Optional[Dict[str, str]] = Field(
        None,
        description="Filter by metadata key-value pairs (AND logic)",
    )


class PaginationRequest(BaseModel):
    """
    Pagination parameters for list requests.
    """
    page: int = Field(1, ge=1, description="Page number")
    page_size: int = Field(
        20,
        ge=1,
        le=200,
        alias="pageSize",
        description="Number of items per page",
    )

    class Config:
        populate_by_name = True


class ListSandboxesRequest(BaseModel):
    """
    Request body for complex listing queries.
    """
    filter: SandboxFilter = Field(
        default_factory=SandboxFilter,
        description="Filtering criteria (all conditions combined with AND logic)",
    )
    pagination: Optional[PaginationRequest] = Field(None, description="Pagination parameters")


class PaginationInfo(BaseModel):
    """
    Pagination metadata for list responses.
    """
    page: int = Field(..., ge=1, description="Current page number")
    page_size: int = Field(..., ge=1, alias="pageSize", description="Number of items per page")
    total_items: int = Field(..., ge=0, alias="totalItems", description="Total number of items matching the filter")
    total_pages: int = Field(..., ge=0, alias="totalPages", description="Total number of pages")
    has_next_page: bool = Field(..., alias="hasNextPage", description="Whether there are more pages after the current one")

    class Config:
        populate_by_name = True


class ListSandboxesResponse(BaseModel):
    """
    Paginated collection of sandboxes.
    """
    items: List[Sandbox] = Field(..., description="List of sandboxes")
    pagination: PaginationInfo = Field(..., description="Pagination metadata")


# ============================================================================
# Renew Expiration
# ============================================================================

class RenewSandboxExpirationRequest(BaseModel):
    """
    Request to renew sandbox expiration time.
    """
    expires_at: datetime = Field(
        ...,
        alias="expiresAt",
        description="New absolute expiration time in UTC (RFC 3339 format). Must be in the future.",
    )

    class Config:
        populate_by_name = True


class RenewSandboxExpirationResponse(BaseModel):
    """
    Response for renewing sandbox expiration.
    """
    expires_at: datetime = Field(
        ...,
        alias="expiresAt",
        description="The new absolute expiration time in UTC (RFC 3339 format)",
    )

    class Config:
        populate_by_name = True


# ============================================================================
# Endpoint
# ============================================================================

class Endpoint(BaseModel):
    """
    Endpoint for accessing a service running in the sandbox.
    """
    endpoint: str = Field(
        ...,
        description="Public endpoint string (host[:port]/path) exposed for the sandbox service",
    )


# ============================================================================
# Error Response
# ============================================================================

class ErrorResponse(BaseModel):
    """
    Standard error response for all non-2xx HTTP responses.

    HTTP status code indicates the error category; code and message provide details.
    """
    code: str = Field(
        ...,
        description="Machine-readable error code (e.g., INVALID_REQUEST, NOT_FOUND, INTERNAL_ERROR)",
    )
    message: str = Field(
        ...,
        description="Human-readable error message describing what went wrong and how to fix it",
    )
