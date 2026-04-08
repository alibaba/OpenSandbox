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

T = TypeVar("T", bound="PlatformSpec")


@_attrs_define
class PlatformSpec:
    """Runtime platform constraint used for scheduling/provisioning.

    This field is independent from `image` and expresses the expected target
    OS and CPU architecture for sandbox execution.

    Behavioral notes:
    - If omitted, runtime uses existing default behavior (backward compatible).
    - If provided and cannot be satisfied by runtime/template/pool constraints,
      request must fail explicitly.

        Attributes:
            os (str): Target operating system (for example `linux`). Example: linux.
            arch (str): Target CPU architecture (for example `amd64` or `arm64`). Example: arm64.
    """

    os: str
    arch: str

    def to_dict(self) -> dict[str, Any]:
        os = self.os

        arch = self.arch

        field_dict: dict[str, Any] = {}

        field_dict.update(
            {
                "os": os,
                "arch": arch,
            }
        )

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        d = dict(src_dict)
        os = d.pop("os")

        arch = d.pop("arch")

        platform_spec = cls(
            os=os,
            arch=arch,
        )

        return platform_spec
