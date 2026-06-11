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
from typing import Any, TypeVar

from attrs import define as _attrs_define

from ..models.bearer_credential_auth_type import BearerCredentialAuthType

T = TypeVar("T", bound="BearerCredentialAuth")


@_attrs_define
class BearerCredentialAuth:
    """
    Attributes:
        type_ (BearerCredentialAuthType):
        credential (str):
    """

    type_: BearerCredentialAuthType
    credential: str

    def to_dict(self) -> dict[str, Any]:
        type_ = self.type_.value

        credential = self.credential

        field_dict: dict[str, Any] = {}

        field_dict.update(
            {
                "type": type_,
                "credential": credential,
            }
        )

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        d = dict(src_dict)
        type_ = BearerCredentialAuthType(d.pop("type"))

        credential = d.pop("credential")

        bearer_credential_auth = cls(
            type_=type_,
            credential=credential,
        )

        return bearer_credential_auth
