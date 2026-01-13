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

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar

from attrs import define as _attrs_define

from ..types import UNSET, Unset

if TYPE_CHECKING:
    from ..models.image_spec_auth import ImageSpecAuth


T = TypeVar("T", bound="ImageSpec")


@_attrs_define
class ImageSpec:
    """Container image specification for sandbox provisioning.

    Supports public registry images and private registry images with authentication.

        Attributes:
            uri (str): Container image URI in standard format.

                Examples:
                  - "python:3.11" (Docker Hub)
                  - "ubuntu:22.04"
                  - "gcr.io/my-project/model-server:v1.0"
                  - "private-registry.company.com:5000/app:latest"
            auth (ImageSpecAuth | Unset): Registry authentication credentials (required for private registries)
    """

    uri: str
    auth: ImageSpecAuth | Unset = UNSET

    def to_dict(self) -> dict[str, Any]:
        uri = self.uri

        auth: dict[str, Any] | Unset = UNSET
        if not isinstance(self.auth, Unset):
            auth = self.auth.to_dict()

        field_dict: dict[str, Any] = {}

        field_dict.update(
            {
                "uri": uri,
            }
        )
        if auth is not UNSET:
            field_dict["auth"] = auth

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.image_spec_auth import ImageSpecAuth

        d = dict(src_dict)
        uri = d.pop("uri")

        _auth = d.pop("auth", UNSET)
        auth: ImageSpecAuth | Unset
        if isinstance(_auth, Unset):
            auth = UNSET
        else:
            auth = ImageSpecAuth.from_dict(_auth)

        image_spec = cls(
            uri=uri,
            auth=auth,
        )

        return image_spec
