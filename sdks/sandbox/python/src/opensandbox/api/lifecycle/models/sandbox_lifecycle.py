#
# Copyright 2026 The OpenSandbox Authors
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
    from ..models.idle_policy import IdlePolicy
    from ..models.lifecycle_hook import LifecycleHook
    from ..models.periodic_lifecycle_hook import PeriodicLifecycleHook


T = TypeVar("T", bound="SandboxLifecycle")


@_attrs_define
class SandboxLifecycle:
    """Extensible container for sandbox lifecycle configuration. All fields
    are optional. Future lifecycle events are added as new optional
    fields without changing the semantics of existing fields.

    `preStart` and `periodic` are hooks executed inside the sandbox by
    execd. `idlePolicy` is control-plane policy (OSEP-0024): it tells the
    server how to treat the sandbox's state machine and never reaches the
    guest.

        Attributes:
            pre_start (LifecycleHook | Unset): A lifecycle command executed directly as an argv array. No implicit shell
                expansion is performed. Use an explicit shell command such as
                `["sh", "-c", "..."]` when shell syntax is required.
            periodic (list[PeriodicLifecycleHook] | Unset): Scheduled hooks run by execd while the sandbox is running. Runs
                of
                the same named hook never overlap; a scheduled run is skipped when
                its previous run is still active.
            idle_policy (IdlePolicy | Unset): Idle behavior declared by the sandbox (OSEP-0024). The policy is
                desired state: the server validates it and persists it as the
                reserved `sandbox.opensandbox.io/idle-policy` Sandbox CR annotation.

                With `idleTimeoutSeconds`, the sandbox is auto-paused after that many
                seconds without ingress-observed activity. Without it, the policy is
                wake-only: the sandbox is never auto-paused, but a (manually) paused
                sandbox wakes on access. A missing, malformed, or invalid policy is
                treated exactly as absent — never as an implicit opt-in.

                Constraints: `idleTimeoutSeconds` must not exceed the server's
                configured activity TTL (default 1800), so a present-but-stale
                activity key always implies idle-by-value. Enabling deployment flags
                never changes the behavior of sandboxes that do not declare a policy.
    """

    pre_start: LifecycleHook | Unset = UNSET
    periodic: list[PeriodicLifecycleHook] | Unset = UNSET
    idle_policy: IdlePolicy | Unset = UNSET

    def to_dict(self) -> dict[str, Any]:
        pre_start: dict[str, Any] | Unset = UNSET
        if not isinstance(self.pre_start, Unset):
            pre_start = self.pre_start.to_dict()

        periodic: list[dict[str, Any]] | Unset = UNSET
        if not isinstance(self.periodic, Unset):
            periodic = []
            for periodic_item_data in self.periodic:
                periodic_item = periodic_item_data.to_dict()
                periodic.append(periodic_item)

        idle_policy: dict[str, Any] | Unset = UNSET
        if not isinstance(self.idle_policy, Unset):
            idle_policy = self.idle_policy.to_dict()

        field_dict: dict[str, Any] = {}

        field_dict.update({})
        if pre_start is not UNSET:
            field_dict["preStart"] = pre_start
        if periodic is not UNSET:
            field_dict["periodic"] = periodic
        if idle_policy is not UNSET:
            field_dict["idlePolicy"] = idle_policy

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.idle_policy import IdlePolicy
        from ..models.lifecycle_hook import LifecycleHook
        from ..models.periodic_lifecycle_hook import PeriodicLifecycleHook

        d = dict(src_dict)
        _pre_start = d.pop("preStart", UNSET)
        pre_start: LifecycleHook | Unset
        if isinstance(_pre_start, Unset):
            pre_start = UNSET
        else:
            pre_start = LifecycleHook.from_dict(_pre_start)

        _periodic = d.pop("periodic", UNSET)
        periodic: list[PeriodicLifecycleHook] | Unset = UNSET
        if _periodic is not UNSET:
            periodic = []
            for periodic_item_data in _periodic:
                periodic_item = PeriodicLifecycleHook.from_dict(periodic_item_data)

                periodic.append(periodic_item)

        _idle_policy = d.pop("idlePolicy", UNSET)
        idle_policy: IdlePolicy | Unset
        if isinstance(_idle_policy, Unset):
            idle_policy = UNSET
        else:
            idle_policy = IdlePolicy.from_dict(_idle_policy)

        sandbox_lifecycle = cls(
            pre_start=pre_start,
            periodic=periodic,
            idle_policy=idle_policy,
        )

        return sandbox_lifecycle
