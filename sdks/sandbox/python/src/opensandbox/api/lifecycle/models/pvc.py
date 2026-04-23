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
            create_if_not_exists (bool): When true (default), auto-create the volume if it does not exist.
            delete_on_sandbox_termination (bool): When true, auto-created Docker volume is removed on sandbox
                deletion. Ignored for Kubernetes PVCs.
            storage_class (str | None): Kubernetes StorageClass for auto-created PVCs. Null means cluster default.
                Ignored for Docker.
            storage (str | None): Storage capacity request for auto-created PVCs (e.g. "1Gi"). Ignored for Docker.
            access_modes (list[str] | None): Access modes for auto-created PVCs (e.g. ["ReadWriteOnce"]). Ignored
                for Docker.
    """

    claim_name: str
    create_if_not_exists: bool = True
    delete_on_sandbox_termination: bool = False
    storage_class: str | None = None
    storage: str | None = None
    access_modes: list[str] | None = None

    def to_dict(self) -> dict[str, Any]:
        claim_name = self.claim_name
        create_if_not_exists = self.create_if_not_exists
        delete_on_sandbox_termination = self.delete_on_sandbox_termination
        storage_class = self.storage_class
        storage = self.storage
        access_modes = self.access_modes

        field_dict: dict[str, Any] = {}

        field_dict.update(
            {
                "claimName": claim_name,
                "createIfNotExists": create_if_not_exists,
                "deleteOnSandboxTermination": delete_on_sandbox_termination,
            }
        )
        if storage_class is not None:
            field_dict["storageClass"] = storage_class
        if storage is not None:
            field_dict["storage"] = storage
        if access_modes is not None:
            field_dict["accessModes"] = access_modes

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        d = dict(src_dict)
        claim_name = d.pop("claimName")
        create_if_not_exists = d.pop("createIfNotExists", True)
        delete_on_sandbox_termination = d.pop("deleteOnSandboxTermination", False)
        storage_class = d.pop("storageClass", None)
        storage = d.pop("storage", None)
        access_modes = d.pop("accessModes", None)

        pvc = cls(
            claim_name=claim_name,
            create_if_not_exists=create_if_not_exists,
            delete_on_sandbox_termination=delete_on_sandbox_termination,
            storage_class=storage_class,
            storage=storage,
            access_modes=access_modes,
        )

        return pvc
