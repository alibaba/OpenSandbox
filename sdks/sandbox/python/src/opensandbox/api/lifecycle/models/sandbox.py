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
    from ..models.image_spec import ImageSpec
    from ..models.sandbox_metadata import SandboxMetadata
    from ..models.sandbox_status import SandboxStatus


T = TypeVar("T", bound="Sandbox")


@_attrs_define
class Sandbox:
    """Runtime execution environment provisioned from a container image

    Attributes:
        id (str): Unique sandbox identifier
        image (ImageSpec): Container image specification for sandbox provisioning.

            Supports public registry images and private registry images with authentication.
        status (SandboxStatus): Detailed status information with lifecycle state and transition details
        entrypoint (list[str]): The command to execute as the sandbox's entry process.
            Always present in responses since entrypoint is required in creation requests.
        expires_at (datetime.datetime): Timestamp when sandbox will auto-terminate
        created_at (datetime.datetime): Sandbox creation timestamp
        metadata (SandboxMetadata | Unset): Custom metadata from creation request
    """

    id: str
    image: ImageSpec
    status: SandboxStatus
    entrypoint: list[str]
    expires_at: datetime.datetime
    created_at: datetime.datetime
    metadata: SandboxMetadata | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        id = self.id

        image = self.image.to_dict()

        status = self.status.to_dict()

        entrypoint = self.entrypoint

        expires_at = self.expires_at.isoformat()

        created_at = self.created_at.isoformat()

        metadata: dict[str, Any] | Unset = UNSET
        if not isinstance(self.metadata, Unset):
            metadata = self.metadata.to_dict()

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update(
            {
                "id": id,
                "image": image,
                "status": status,
                "entrypoint": entrypoint,
                "expiresAt": expires_at,
                "createdAt": created_at,
            }
        )
        if metadata is not UNSET:
            field_dict["metadata"] = metadata

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.image_spec import ImageSpec
        from ..models.sandbox_metadata import SandboxMetadata
        from ..models.sandbox_status import SandboxStatus

        d = dict(src_dict)
        id = d.pop("id")

        image = ImageSpec.from_dict(d.pop("image"))

        status = SandboxStatus.from_dict(d.pop("status"))

        entrypoint = cast(list[str], d.pop("entrypoint"))

        expires_at = isoparse(d.pop("expiresAt"))

        created_at = isoparse(d.pop("createdAt"))

        _metadata = d.pop("metadata", UNSET)
        metadata: SandboxMetadata | Unset
        if isinstance(_metadata, Unset):
            metadata = UNSET
        else:
            metadata = SandboxMetadata.from_dict(_metadata)

        sandbox = cls(
            id=id,
            image=image,
            status=status,
            entrypoint=entrypoint,
            expires_at=expires_at,
            created_at=created_at,
            metadata=metadata,
        )

        sandbox.additional_properties = d
        return sandbox

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
