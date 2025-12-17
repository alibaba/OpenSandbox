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
Sandbox service adapter implementation.

Implementation of SandboxService that adapts openapi-python-client generated API.
This adapter provides a clean abstraction layer between business logic and
the auto-generated API client, handling all model conversions and error mapping.
"""

import logging
from datetime import datetime, timedelta
from uuid import UUID

import httpx  # type: ignore[reportMissingImports]

from opensandbox.adapters.converter.exception_converter import (
    ExceptionConverter,
)
from opensandbox.adapters.converter.response_handler import (
    handle_api_error,
    require_parsed,
)
from opensandbox.adapters.converter.sandbox_model_converter import (
    SandboxModelConverter,
)
from opensandbox.api.lifecycle.types import UNSET
from opensandbox.config import ConnectionConfig
from opensandbox.models.sandboxes import (
    PagedSandboxInfos,
    SandboxCreateResponse,
    SandboxEndpoint,
    SandboxFilter,
    SandboxImageSpec,
    SandboxInfo,
)
from opensandbox.services.sandbox import Sandboxes

logger = logging.getLogger(__name__)


class SandboxesAdapter(Sandboxes):
    """
    Implementation of SandboxService that adapts openapi-python-client generated API.

    This adapter provides a clean abstraction layer between business logic and
    the sandbox management API, handling all model conversions and error mapping.

    The openapi-python-client generates functional APIs that support custom
    httpx.AsyncClient injection, allowing for fine-grained control over HTTP behavior.
    """

    def __init__(self, connection_config: ConnectionConfig) -> None:
        """
        Initialize the sandbox service adapter.

        Args:
            connection_config: Connection configuration (shared transport, headers, timeouts)
        """
        self.connection_config = connection_config
        from opensandbox.api.lifecycle import AuthenticatedClient

        api_key = self.connection_config.get_api_key()
        timeout_seconds = self.connection_config.request_timeout.total_seconds()
        timeout = httpx.Timeout(timeout_seconds)

        headers = {
            "User-Agent": self.connection_config.user_agent,
            **self.connection_config.headers,
        }
        if api_key:
            headers["OPEN-SANDBOX-API-KEY"] = api_key

        # Create client with custom auth header for OpenSandbox API
        self._client = AuthenticatedClient(
            base_url=self.connection_config.get_base_url(),
            token=api_key or "",
            prefix="",  # No prefix, just the token
            auth_header_name="OPEN-SANDBOX-API-KEY",  # Custom header name
            timeout=timeout,
        )

        # Inject httpx client (adapter-owned)
        self._httpx_client = httpx.AsyncClient(
            base_url=self.connection_config.get_base_url(),
            headers=headers,
            timeout=timeout,
            transport=self.connection_config.transport,
        )
        self._client.set_async_httpx_client(self._httpx_client)

    async def _get_client(self):
        """Return the authenticated client for lifecycle API."""
        return self._client

    async def create_sandbox(
        self,
        spec: SandboxImageSpec,
        entrypoint: list[str],
        env: dict[str, str],
        metadata: dict[str, str],
        timeout: timedelta,
        resource: dict[str, str],
    ) -> SandboxCreateResponse:
        """Create a new sandbox instance with the specified configuration."""
        logger.info(f"Creating sandbox with image: {spec.image}")

        try:
            from opensandbox.api.lifecycle.api.sandboxes import post_sandboxes

            create_request = SandboxModelConverter.to_api_create_sandbox_request(
                spec, entrypoint, env, metadata, timeout, resource
            )

            client = await self._get_client()
            response_obj = await post_sandboxes.asyncio_detailed(
                client=client,
                body=create_request,
            )

            handle_api_error(response_obj, "Create sandbox")

            from opensandbox.api.lifecycle.models import CreateSandboxResponse
            parsed = require_parsed(response_obj, CreateSandboxResponse, "Create sandbox")
            response = SandboxModelConverter.to_sandbox_create_response(parsed)
            logger.info(f"Successfully created sandbox: {response.id}")
            return response

        except Exception as e:
            logger.error(
                f"Failed to create sandbox with image: {spec.image}", exc_info=e
            )
            raise ExceptionConverter.to_sandbox_exception(e) from e

    async def get_sandbox_info(self, sandbox_id: UUID) -> SandboxInfo:
        """Retrieve detailed information about a sandbox."""
        logger.debug(f"Retrieving sandbox information: {sandbox_id}")

        try:
            from opensandbox.api.lifecycle.api.sandboxes import get_sandboxes_sandbox_id

            client = await self._get_client()
            response_obj = await get_sandboxes_sandbox_id.asyncio_detailed(
                client=client,
                sandbox_id=sandbox_id,
            )

            handle_api_error(response_obj, f"Get sandbox {sandbox_id}")

            from opensandbox.api.lifecycle.models import Sandbox
            parsed = require_parsed(response_obj, Sandbox, f"Get sandbox {sandbox_id}")
            return SandboxModelConverter.to_sandbox_info(parsed)

        except Exception as e:
            logger.error(f"Failed to get sandbox info: {sandbox_id}", exc_info=e)
            raise ExceptionConverter.to_sandbox_exception(e) from e

    async def list_sandboxes(self, filter: SandboxFilter) -> PagedSandboxInfos:
        """List sandboxes with optional filtering criteria."""
        logger.debug(f"Listing sandboxes with filter: {filter}")

        # Prepare metadata parameter similar to Kotlin SDK
        metadata = UNSET
        if filter.metadata:

            metadata_parts: list[str] = []
            for key, value in filter.metadata.items():
                metadata_parts.append(f"{key}={value}")
            metadata = "&".join(metadata_parts)

        try:
            from opensandbox.api.lifecycle.api.sandboxes import get_sandboxes
            from opensandbox.api.lifecycle.types import UNSET as API_UNSET

            client = await self._get_client()
            response_obj = await get_sandboxes.asyncio_detailed(
                client=client,
                state=filter.states if filter.states else API_UNSET,
                metadata=metadata,
                page=filter.page if filter.page is not None else API_UNSET,
                page_size=filter.page_size if filter.page_size is not None else API_UNSET,
            )

            handle_api_error(response_obj, "List sandboxes")

            from opensandbox.api.lifecycle.models import ListSandboxesResponse
            parsed = require_parsed(response_obj, ListSandboxesResponse, "List sandboxes")
            return SandboxModelConverter.to_paged_sandbox_infos(parsed)

        except Exception as e:
            logger.error("Failed to list sandboxes", exc_info=e)
            raise ExceptionConverter.to_sandbox_exception(e) from e

    async def get_sandbox_endpoint(
        self, sandbox_id: UUID, port: int
    ) -> SandboxEndpoint:
        """Get network endpoint information for a sandbox service."""
        logger.debug(f"Retrieving sandbox endpoint: {sandbox_id}, port {port}")

        try:
            from opensandbox.api.lifecycle.api.sandboxes import (
                get_sandboxes_sandbox_id_endpoints_port,
            )

            client = await self._get_client()
            response_obj = (
                await get_sandboxes_sandbox_id_endpoints_port.asyncio_detailed(
                    client=client,
                    sandbox_id=sandbox_id,
                    port=port,
                )
            )

            handle_api_error(
                response_obj, f"Get endpoint for sandbox {sandbox_id} port {port}"
            )

            from opensandbox.api.lifecycle.models import Endpoint
            parsed = require_parsed(response_obj, Endpoint, "Get endpoint")
            return SandboxModelConverter.to_sandbox_endpoint(parsed)

        except Exception as e:
            logger.error(
                f"Failed to retrieve sandbox endpoint for sandbox {sandbox_id}",
                exc_info=e,
            )
            raise ExceptionConverter.to_sandbox_exception(e) from e

    async def pause_sandbox(self, sandbox_id: UUID) -> None:
        """Pause a running sandbox while preserving its state."""
        logger.info(f"Pausing sandbox: {sandbox_id}")

        try:
            from opensandbox.api.lifecycle.api.sandboxes import (
                post_sandboxes_sandbox_id_pause,
            )

            client = await self._get_client()
            response_obj = await post_sandboxes_sandbox_id_pause.asyncio_detailed(
                client=client,
                sandbox_id=sandbox_id,
            )

            handle_api_error(response_obj, f"Pause sandbox {sandbox_id}")

            logger.info(f"Initiated pause for sandbox: {sandbox_id}")

        except Exception as e:
            logger.error(f"Failed to initiate pause sandbox: {sandbox_id}", exc_info=e)
            raise ExceptionConverter.to_sandbox_exception(e) from e

    async def resume_sandbox(self, sandbox_id: UUID) -> None:
        """Resume a previously paused sandbox."""
        logger.info(f"Resuming sandbox: {sandbox_id}")

        try:
            from opensandbox.api.lifecycle.api.sandboxes import (
                post_sandboxes_sandbox_id_resume,
            )

            client = await self._get_client()
            response_obj = await post_sandboxes_sandbox_id_resume.asyncio_detailed(
                client=client,
                sandbox_id=sandbox_id,
            )

            handle_api_error(response_obj, f"Resume sandbox {sandbox_id}")

            logger.info(f"Initiated resume for sandbox: {sandbox_id}")

        except Exception as e:
            logger.error(f"Failed initiate resume sandbox: {sandbox_id}", exc_info=e)
            raise ExceptionConverter.to_sandbox_exception(e) from e

    async def renew_sandbox_expiration(
        self, sandbox_id: UUID, new_expiration_time: datetime
    ) -> None:
        """Extend the expiration time of a sandbox."""
        logger.info(f"Renew sandbox {sandbox_id} expiration to {new_expiration_time}")

        try:
            from opensandbox.api.lifecycle.api.sandboxes import (
                post_sandboxes_sandbox_id_renew_expiration,
            )

            renew_request = SandboxModelConverter.to_api_renew_request(
                new_expiration_time
            )

            client = await self._get_client()
            response_obj = (
                await post_sandboxes_sandbox_id_renew_expiration.asyncio_detailed(
                    client=client,
                    sandbox_id=sandbox_id,
                    body=renew_request,
                )
            )

            handle_api_error(response_obj, f"Renew sandbox {sandbox_id} expiration")

            logger.info(f"Successfully renewed sandbox {sandbox_id} expiration")

        except Exception as e:
            logger.error(f"Failed to renew sandbox {sandbox_id} expiration", exc_info=e)
            raise ExceptionConverter.to_sandbox_exception(e) from e

    async def kill_sandbox(self, sandbox_id: UUID) -> None:
        """Permanently terminate a sandbox and clean up its resources."""
        logger.info(f"Terminating sandbox: {sandbox_id}")

        try:
            from opensandbox.api.lifecycle.api.sandboxes import (
                delete_sandboxes_sandbox_id,
            )

            client = await self._get_client()
            response_obj = await delete_sandboxes_sandbox_id.asyncio_detailed(
                client=client,
                sandbox_id=sandbox_id,
            )

            handle_api_error(response_obj, f"Kill sandbox {sandbox_id}")

            logger.info(f"Successfully terminated sandbox: {sandbox_id}")

        except Exception as e:
            logger.error(f"Failed to terminate sandbox: {sandbox_id}", exc_info=e)
            raise ExceptionConverter.to_sandbox_exception(e) from e
