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
from attrs import field as _attrs_field

T = TypeVar("T", bound="PaginationInfo")


@_attrs_define
class PaginationInfo:
    """Pagination metadata for list responses

    Attributes:
        page (int): Current page number
        page_size (int): Number of items per page
        total_items (int): Total number of items matching the filter
        total_pages (int): Total number of pages
        has_next_page (bool): Whether there are more pages after the current one
    """

    page: int
    page_size: int
    total_items: int
    total_pages: int
    has_next_page: bool
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        page = self.page

        page_size = self.page_size

        total_items = self.total_items

        total_pages = self.total_pages

        has_next_page = self.has_next_page

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update(
            {
                "page": page,
                "pageSize": page_size,
                "totalItems": total_items,
                "totalPages": total_pages,
                "hasNextPage": has_next_page,
            }
        )

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        d = dict(src_dict)
        page = d.pop("page")

        page_size = d.pop("pageSize")

        total_items = d.pop("totalItems")

        total_pages = d.pop("totalPages")

        has_next_page = d.pop("hasNextPage")

        pagination_info = cls(
            page=page,
            page_size=page_size,
            total_items=total_items,
            total_pages=total_pages,
            has_next_page=has_next_page,
        )

        pagination_info.additional_properties = d
        return pagination_info

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
