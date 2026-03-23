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

import pytest
from fastapi import HTTPException

from src.extensions import (
    ACCESS_RENEW_EXTEND_SECONDS_KEY,
    ACCESS_RENEW_EXTEND_SECONDS_MAX,
    ACCESS_RENEW_EXTEND_SECONDS_METADATA_KEY,
    ACCESS_RENEW_EXTEND_SECONDS_MIN,
    apply_access_renew_extend_seconds_to_mapping,
    validate_extensions,
)


class TestValidateCreateSandboxExtensionsAccessRenewExtendSeconds:
    """access.renew.extend.seconds in [300, 86400] when present."""

    def test_omitted_extensions_ok(self):
        validate_extensions(None)

    def test_extensions_without_key_ok(self):
        validate_extensions({"other": "x"})

    def test_boundary_min_ok(self):
        validate_extensions(
            {ACCESS_RENEW_EXTEND_SECONDS_KEY: str(ACCESS_RENEW_EXTEND_SECONDS_MIN)}
        )

    def test_boundary_max_ok(self):
        validate_extensions(
            {ACCESS_RENEW_EXTEND_SECONDS_KEY: str(ACCESS_RENEW_EXTEND_SECONDS_MAX)}
        )

    def test_typical_value_ok(self):
        validate_extensions({ACCESS_RENEW_EXTEND_SECONDS_KEY: "1800"})

    def test_whitespace_trimmed_ok(self):
        validate_extensions({ACCESS_RENEW_EXTEND_SECONDS_KEY: "  1800  "})

    def test_below_min_rejected(self):
        with pytest.raises(HTTPException) as exc:
            validate_extensions(
                {ACCESS_RENEW_EXTEND_SECONDS_KEY: str(ACCESS_RENEW_EXTEND_SECONDS_MIN - 1)}
            )
        assert exc.value.status_code == 400

    def test_above_max_rejected(self):
        with pytest.raises(HTTPException) as exc:
            validate_extensions(
                {ACCESS_RENEW_EXTEND_SECONDS_KEY: str(ACCESS_RENEW_EXTEND_SECONDS_MAX + 1)}
            )
        assert exc.value.status_code == 400

    def test_non_integer_string_rejected(self):
        with pytest.raises(HTTPException) as exc:
            validate_extensions({ACCESS_RENEW_EXTEND_SECONDS_KEY: "abc"})
        assert exc.value.status_code == 400

    def test_empty_string_rejected(self):
        with pytest.raises(HTTPException) as exc:
            validate_extensions({ACCESS_RENEW_EXTEND_SECONDS_KEY: ""})
        assert exc.value.status_code == 400


class TestAccessRenewExtendSecondsStorage:
    def test_apply_to_mapping_with_mixed_extension_keys(self):
        m: dict[str, str] = {}
        apply_access_renew_extend_seconds_to_mapping(
            m,
            {"other": "x", ACCESS_RENEW_EXTEND_SECONDS_KEY: "3600"},
        )
        assert m == {ACCESS_RENEW_EXTEND_SECONDS_METADATA_KEY: "3600"}

    def test_apply_to_mapping_sets_default_key(self):
        m: dict[str, str] = {}
        apply_access_renew_extend_seconds_to_mapping(
            m, {ACCESS_RENEW_EXTEND_SECONDS_KEY: "1200"}
        )
        assert m == {ACCESS_RENEW_EXTEND_SECONDS_METADATA_KEY: "1200"}

    def test_apply_to_mapping_noop_when_key_absent(self):
        m: dict[str, str] = {"x": "1"}
        apply_access_renew_extend_seconds_to_mapping(m, {"poolRef": "p"})
        assert m == {"x": "1"}
