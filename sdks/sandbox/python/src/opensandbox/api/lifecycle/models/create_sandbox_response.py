#
# Copyright 2026 Alibaba Group Holding Ltd.
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

from __future__ import annotations

import datetime
from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar, cast

from attrs import define as _attrs_define
from attrs import field as _attrs_field
from dateutil.parser import isoparse

from ..types import UNSET, Unset

if TYPE_CHECKING:
    from ..models.create_sandbox_response_metadata import CreateSandboxResponseMetadata
    from ..models.sandbox_status import SandboxStatus


T = TypeVar("T", bound="CreateSandboxResponse")


@_attrs_define
class CreateSandboxResponse:
    """Response from creating a new sandbox. Contains essential information without image and updatedAt.

    Attributes:
        id (str): Unique sandbox identifier
        status (SandboxStatus): Detailed status information with lifecycle state and transition details
        expires_at (datetime.datetime): Timestamp when sandbox will auto-terminate
        created_at (datetime.datetime): Sandbox creation timestamp
        entrypoint (list[str]): Entry process specification from creation request
        metadata (CreateSandboxResponseMetadata | Unset): Custom metadata from creation request
    """

    id: str
    status: SandboxStatus
    expires_at: datetime.datetime
    created_at: datetime.datetime
    entrypoint: list[str]
    metadata: CreateSandboxResponseMetadata | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        id = self.id

        status = self.status.to_dict()

        expires_at = self.expires_at.isoformat()

        created_at = self.created_at.isoformat()

        entrypoint = self.entrypoint

        metadata: dict[str, Any] | Unset = UNSET
        if not isinstance(self.metadata, Unset):
            metadata = self.metadata.to_dict()

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update(
            {
                "id": id,
                "status": status,
                "expiresAt": expires_at,
                "createdAt": created_at,
                "entrypoint": entrypoint,
            }
        )
        if metadata is not UNSET:
            field_dict["metadata"] = metadata

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.create_sandbox_response_metadata import CreateSandboxResponseMetadata
        from ..models.sandbox_status import SandboxStatus

        d = dict(src_dict)
        id = d.pop("id")

        status = SandboxStatus.from_dict(d.pop("status"))

        expires_at = isoparse(d.pop("expiresAt"))

        created_at = isoparse(d.pop("createdAt"))

        entrypoint = cast(list[str], d.pop("entrypoint"))

        _metadata = d.pop("metadata", UNSET)
        metadata: CreateSandboxResponseMetadata | Unset
        if isinstance(_metadata, Unset):
            metadata = UNSET
        else:
            metadata = CreateSandboxResponseMetadata.from_dict(_metadata)

        create_sandbox_response = cls(
            id=id,
            status=status,
            expires_at=expires_at,
            created_at=created_at,
            entrypoint=entrypoint,
            metadata=metadata,
        )

        create_sandbox_response.additional_properties = d
        return create_sandbox_response

    @property
    def additional_keys(self) -> list[str]:
        return list(self.additional_properties.keys())

    def __getitem__(self, key: str) -> Any:
        return self.additional_properties[key]

    def __setitem__(self, key: str, value: Any) -> None:
        self.additional_properties[key] = value

    def __delitem__(self, key: str) -> None:
        del self.additional_properties[key]

    def __contains__(self, key: str) -> bool:
        return key in self.additional_properties
