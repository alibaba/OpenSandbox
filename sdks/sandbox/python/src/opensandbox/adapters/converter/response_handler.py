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
"""
Unified response handler for API calls.

Provides a centralized way to handle API responses, including:
1. Status code validation
2. Error response handling
3. Unified exception conversion

This eliminates the need to repeat response handling logic in each adapter method.
"""

import logging
from typing import Any, TypeVar

from opensandbox.exceptions import SandboxApiException

logger = logging.getLogger(__name__)


T = TypeVar("T")


def require_parsed(response_obj: Any, expected_type: type[T], operation_name: str) -> T:
    """
    Validate and return the parsed payload from an openapi-python-client response.

    Use this after `handle_api_error()` to enforce:
    - parsed payload must exist
    - parsed payload must match the expected type
    """
    status_code = getattr(response_obj, "status_code", 0)
    if hasattr(status_code, "value"):
        status_code = status_code.value

    parsed = getattr(response_obj, "parsed", None)
    if parsed is None:
        raise SandboxApiException(
            message=f"{operation_name} failed: empty response",
            status_code=status_code,
        )
    if not isinstance(parsed, expected_type):
        raise SandboxApiException(
            message=f"{operation_name} failed: unexpected response type",
            status_code=status_code,
        )
    return parsed


def handle_api_error(response_obj: Any, operation_name: str = "API call") -> None:
    """
    Check API response for errors and raise exception if needed.

    Call this before accessing response_obj.parsed to validate the response.

    Args:
        response_obj: The Response object from asyncio_detailed or sync_detailed
        operation_name: Name of the operation for error messages

    Raises:
        SandboxApiException: If the response indicates an error
    """
    status_code = response_obj.status_code
    if hasattr(status_code, "value"):
        status_code = status_code.value

    logger.debug(f"{operation_name} response: status={status_code}")

    if status_code >= 300:
        error_message = f"{operation_name} failed: HTTP {status_code}"

        if hasattr(response_obj, "parsed") and response_obj.parsed is not None:
            if hasattr(response_obj.parsed, "message"):
                error_message = (
                    f"{operation_name} failed: {response_obj.parsed.message}"
                )
            elif hasattr(response_obj.parsed, "code"):
                error_message = f"{operation_name} failed: {response_obj.parsed.code}"

        raise SandboxApiException(
            message=error_message,
            status_code=status_code,
        )
