#
# Copyright 2025 Alibaba Group Holding Ltd.
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

from datetime import datetime, timedelta

import pytest

from opensandbox.adapters.converter.exception_converter import (
    ExceptionConverter,
    parse_sandbox_error,
)
from opensandbox.adapters.converter.execution_converter import (
    ExecutionConverter,
)
from opensandbox.adapters.converter.filesystem_model_converter import (
    FilesystemModelConverter,
)
from opensandbox.adapters.converter.metrics_model_converter import (
    MetricsModelConverter,
)
from opensandbox.adapters.converter.response_handler import handle_api_error
from opensandbox.adapters.converter.sandbox_model_converter import (
    SandboxModelConverter,
)
from opensandbox.exceptions import (
    InvalidArgumentException,
    SandboxApiException,
    SandboxInternalException,
)
from opensandbox.models.execd import RunCommandOpts
from opensandbox.models.sandboxes import SandboxImageSpec


def test_parse_sandbox_error_from_json_bytes() -> None:
    err = parse_sandbox_error(b'{"code":"X","message":"m"}')
    assert err is not None
    assert err.code == "X"
    assert err.message == "m"


def test_parse_sandbox_error_from_plain_text_string() -> None:
    err = parse_sandbox_error("not-json")
    assert err is not None
    assert err.code == "UNEXPECTED_RESPONSE"
    assert err.message == "not-json"


def test_handle_api_error_raises_with_parsed_message() -> None:
    class Parsed:
        message = "bad request"

    class Resp:
        status_code = 400
        parsed = Parsed()

    with pytest.raises(SandboxApiException) as ei:
        handle_api_error(Resp(), "Op")
    assert "bad request" in str(ei.value)


def test_handle_api_error_noop_on_success() -> None:
    class Resp:
        status_code = 200
        parsed = None

    handle_api_error(Resp(), "Op")


def test_exception_converter_maps_common_types() -> None:
    se = ExceptionConverter.to_sandbox_exception(ValueError("x"))
    assert isinstance(se, InvalidArgumentException)

    se2 = ExceptionConverter.to_sandbox_exception(OSError("x"))
    assert isinstance(se2, SandboxInternalException)


def test_execution_converter_to_api_run_command_request() -> None:
    from opensandbox.api.execd.types import UNSET

    api = ExecutionConverter.to_api_run_command_request("echo hi", RunCommandOpts())
    d = api.to_dict()
    assert d["command"] == "echo hi"
    assert "cwd" not in d

    api2 = ExecutionConverter.to_api_run_command_request(
        "echo hi",
        RunCommandOpts(working_directory="/tmp"),
    )
    d2 = api2.to_dict()
    assert d2["cwd"] == "/tmp"
    # background defaults to False in domain opts; when False we omit it from the API request.
    assert d2.get("background", UNSET) is UNSET


def test_filesystem_and_metrics_converters() -> None:
    from datetime import datetime, timezone

    from opensandbox.api.execd.models import FileInfo, Metrics

    fi = FileInfo(
        path="/a",
        mode=644,
        owner="u",
        group="g",
        size=1,
        modified_at=datetime(2025, 1, 1, tzinfo=timezone.utc),
        created_at=datetime(2025, 1, 1, tzinfo=timezone.utc),
    )
    entry = FilesystemModelConverter.to_entry_info(fi)
    assert entry.path == "/a"

    api_metrics = Metrics(cpu_count=1.0, cpu_used_pct=2.0, mem_total_mib=3.0, mem_used_mib=4.0, timestamp=5)
    m = MetricsModelConverter.to_sandbox_metrics(api_metrics)
    assert m.cpu_used_percentage == 2.0


def test_sandbox_model_converter_to_api_create_request_and_renew_tz() -> None:
    from datetime import timezone

    spec = SandboxImageSpec("python:3.11")
    req = SandboxModelConverter.to_api_create_sandbox_request(
        spec=spec,
        entrypoint=["/bin/sh"],
        env={},
        metadata={},
        timeout=timedelta(seconds=3),
        resource={"cpu": "100m"},
        extensions={},
    )
    d = req.to_dict()
    assert d["image"]["uri"] == "python:3.11"
    assert d["timeout"] == 3
    assert "env" not in d
    assert "metadata" not in d

    renew = SandboxModelConverter.to_api_renew_request(datetime(2025, 1, 1))
    assert renew.expires_at.tzinfo is timezone.utc
