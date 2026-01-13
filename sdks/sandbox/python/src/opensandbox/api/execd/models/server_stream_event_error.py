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
from typing import Any, TypeVar, cast

from attrs import define as _attrs_define
from attrs import field as _attrs_field

from ..types import UNSET, Unset

T = TypeVar("T", bound="ServerStreamEventError")


@_attrs_define
class ServerStreamEventError:
    """Execution error details if an error occurred

    Attributes:
        ename (str | Unset): Error name/type Example: NameError.
        evalue (str | Unset): Error value/message Example: name 'undefined_var' is not defined.
        traceback (list[str] | Unset): Stack trace lines Example: ['Traceback (most recent call last):', '  File
            "<stdin>", line 1, in <module>', "NameError: name 'undefined_var' is not defined"].
    """

    ename: str | Unset = UNSET
    evalue: str | Unset = UNSET
    traceback: list[str] | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        ename = self.ename

        evalue = self.evalue

        traceback: list[str] | Unset = UNSET
        if not isinstance(self.traceback, Unset):
            traceback = self.traceback

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update({})
        if ename is not UNSET:
            field_dict["ename"] = ename
        if evalue is not UNSET:
            field_dict["evalue"] = evalue
        if traceback is not UNSET:
            field_dict["traceback"] = traceback

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        d = dict(src_dict)
        ename = d.pop("ename", UNSET)

        evalue = d.pop("evalue", UNSET)

        traceback = cast(list[str], d.pop("traceback", UNSET))

        server_stream_event_error = cls(
            ename=ename,
            evalue=evalue,
            traceback=traceback,
        )

        server_stream_event_error.additional_properties = d
        return server_stream_event_error

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
