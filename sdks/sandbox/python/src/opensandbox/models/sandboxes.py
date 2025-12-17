#
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
#
"""
Sandbox-related data models.

Models for sandbox creation, configuration, status, and lifecycle management.
"""

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator


class SandboxImageAuth(BaseModel):
    """
    Authentication credentials for container registries.
    """

    username: str = Field(description="Registry username")
    password: str = Field(description="Registry password or access token")

    @field_validator("username")
    @classmethod
    def username_must_not_be_empty(cls, v: str) -> str:
        if not v.strip():
            raise ValueError("Username cannot be blank")
        return v

    @field_validator("password")
    @classmethod
    def password_must_not_be_empty(cls, v: str) -> str:
        if not v.strip():
            raise ValueError("Password cannot be blank")
        return v


class SandboxImageSpec(BaseModel):
    """
    Specification for a sandbox container image.

    Usage:
        # Simple creation with just image
        spec = SandboxImageSpec("python:3.11")

        # With private registry auth
        spec = SandboxImageSpec(
            "my-registry.com/image:tag",
            auth=SandboxImageAuth(username="user", password="pass")
        )
    """

    image: str = Field(
        description="Image reference (e.g., 'ubuntu:22.04', 'python:3.11')"
    )
    auth: SandboxImageAuth | None = Field(
        default=None, description="Authentication for private registries"
    )

    def __init__(
        self, image: str | None = None, *, auth: SandboxImageAuth | None = None, **data: object
    ) -> None:
        """
        Initialize SandboxImageSpec.

        Args:
            image: Container image reference (positional or keyword)
            auth: Optional authentication for private registries
        """
        if image is not None:
            data["image"] = image
        if auth is not None:
            data["auth"] = auth
        super().__init__(**data)

    @field_validator("image")
    @classmethod
    def image_must_not_be_empty(cls, v: str) -> str:
        if not v.strip():
            raise ValueError("Image cannot be blank")
        return v


class SandboxStatus(BaseModel):
    """
    Status information for a sandbox.
    """

    state: str = Field(
        description="Current state (e.g., RUNNING, PENDING, PAUSED, TERMINATED)"
    )
    reason: str | None = Field(
        default=None, description="Short reason code for current state"
    )
    message: str | None = Field(
        default=None, description="Human-readable status message"
    )
    last_transition_at: datetime | None = Field(
        default=None,
        description="Timestamp of last state transition",
        alias="last_transition_at",
    )

    model_config = ConfigDict(populate_by_name=True)


class SandboxInfo(BaseModel):
    """
    Detailed information about a sandbox instance.
    """

    id: UUID = Field(description="Unique identifier of the sandbox")
    status: SandboxStatus = Field(description="Current status of the sandbox")
    entrypoint: list[str] = Field(
        description="Command line arguments used to start the sandbox"
    )
    expires_at: datetime = Field(
        description="Scheduled termination timestamp", alias="expires_at"
    )
    created_at: datetime = Field(description="Creation timestamp", alias="created_at")
    image: SandboxImageSpec | None = Field(
        default=None, description="Image specification used to create sandbox"
    )
    metadata: dict[str, str] | None = Field(default=None, description="Custom metadata")

    model_config = ConfigDict(populate_by_name=True)


class SandboxCreateResponse(BaseModel):
    """
    Response returned when a sandbox is created.
    """

    id: UUID = Field(description="Unique identifier of the newly created sandbox")


class SandboxEndpoint(BaseModel):
    """
    Connection endpoint information for a sandbox.
    """

    endpoint: str = Field(description="Sandbox connection endpoint")


class PaginationInfo(BaseModel):
    """
    Pagination metadata.
    """

    page: int = Field(description="Current page number (0-indexed)")
    page_size: int = Field(description="Number of items per page", alias="page_size")
    total_items: int = Field(
        description="Total number of items across all pages", alias="total_items"
    )
    total_pages: int = Field(description="Total number of pages", alias="total_pages")
    has_next_page: bool = Field(
        description="True if there is a next page available", alias="has_next_page"
    )

    model_config = ConfigDict(populate_by_name=True)


class PagedSandboxInfos(BaseModel):
    """
    A paginated list of sandbox information.
    """

    sandbox_infos: list[SandboxInfo] = Field(
        description="List of sandbox details for current page", alias="sandbox_infos"
    )
    pagination: PaginationInfo = Field(description="Pagination metadata")

    model_config = ConfigDict(populate_by_name=True)


class SandboxFilter(BaseModel):
    """
    Filter criteria for listing sandboxes.
    """

    states: list[str] | None = Field(
        default=None, description="Filter by sandbox states"
    )
    metadata: dict[str, str] | None = Field(
        default=None, description="Filter by metadata key-value pairs"
    )
    page_size: int | None = Field(
        default=None, description="Number of items per page", alias="page_size"
    )
    page: int | None = Field(default=None, description="Page number (0-indexed)")

    @field_validator("page_size")
    @classmethod
    def page_size_must_be_positive(cls, v: int | None) -> int | None:
        if v is not None and v <= 0:
            raise ValueError("Page size must be positive")
        return v

    @field_validator("page")
    @classmethod
    def page_must_be_non_negative(cls, v: int | None) -> int | None:
        if v is not None and v < 0:
            raise ValueError("Page must be non-negative")
        return v

    model_config = ConfigDict(populate_by_name=True)


class SandboxMetrics(BaseModel):
    """
    Real-time resource usage metrics for a sandbox.
    """

    cpu_count: float = Field(
        description="Number of CPU cores available/allocated", alias="cpu_count"
    )
    cpu_used_percentage: float = Field(
        description="Current CPU usage as percentage (0.0 - 100.0)",
        alias="cpu_used_percentage",
    )
    memory_total_in_mib: float = Field(
        description="Total memory available in Mebibytes", alias="memory_total_in_mib"
    )
    memory_used_in_mib: float = Field(
        description="Memory currently used in Mebibytes", alias="memory_used_in_mib"
    )
    timestamp: int = Field(
        description="Timestamp of metric collection (Unix epoch milliseconds)"
    )

    model_config = ConfigDict(populate_by_name=True)
