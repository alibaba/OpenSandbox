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

T = TypeVar("T", bound="PVC")


@_attrs_define
class PVC:
    """Platform-managed named volume backend. A runtime-neutral abstraction
    for referencing a pre-existing, platform-managed named volume.

    - Kubernetes: maps to a PersistentVolumeClaim in the same namespace.
    - Docker: maps to a Docker named volume (created via `docker volume create`).

    The volume must already exist on the target platform before sandbox
    creation.

        Attributes:
            claim_name (str): Name of the volume on the target platform.
                In Kubernetes this is the PVC name; in Docker this is the named
                volume name. Must be a valid DNS label.
    """

    claim_name: str

    def to_dict(self) -> dict[str, Any]:
        claim_name = self.claim_name

        field_dict: dict[str, Any] = {}

        field_dict.update(
            {
                "claimName": claim_name,
            }
        )

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        d = dict(src_dict)
        claim_name = d.pop("claimName")

        pvc = cls(
            claim_name=claim_name,
        )

        return pvc
