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

"""
Factory for creating sandbox and WarmPool service instances.

This module provides factory functions keyed off application configuration
(see ``get_config()`` / ``SANDBOX_CONFIG_PATH``).
"""

import logging
from typing import Optional

from opensandbox_server.config import AppConfig, get_config
from opensandbox_server.services.docker import DockerSandboxService
from opensandbox_server.services.k8s import KubernetesSandboxService
from opensandbox_server.services.k8s.client import K8sClient
from opensandbox_server.services.k8s.pool_service import PoolService
from opensandbox_server.services.k8s.provider_factory import (
    PROVIDER_TYPE_BATCHSANDBOX,
    resolve_workload_provider_type,
)
from opensandbox_server.services.sandbox_service import SandboxService

logger = logging.getLogger(__name__)


def create_sandbox_service(
    service_type: Optional[str] = None,
    config: Optional[AppConfig] = None,
) -> SandboxService:
    """
    Create a sandbox service instance based on configuration.

    Args:
        service_type: Optional override for service implementation type.
        config: Optional application configuration. Defaults to global config.

    Returns:
        SandboxService: An instance of the configured sandbox service implementation.

    Raises:
        ValueError: If the configured service type is not supported.
    """
    active_config = config or get_config()
    selected_type = (service_type or active_config.runtime.type).lower()

    logger.info(f"Creating sandbox service with type: {selected_type}")

    # Service implementation registry
    # Add new implementations here as they are created
    implementations: dict[str, type[SandboxService]] = {
        "docker": DockerSandboxService,
        "kubernetes": KubernetesSandboxService,
        # Future implementations can be added here:
        # "containerd": ContainerdSandboxService,
    }

    if selected_type not in implementations:
        supported_types = ", ".join(implementations.keys())
        raise ValueError(
            f"Unsupported sandbox service type: {selected_type}. "
            f"Supported types: {supported_types}"
        )

    implementation_class = implementations[selected_type]
    return implementation_class(config=active_config)


def create_pool_service(config: Optional[AppConfig] = None) -> Optional[PoolService]:
    """
    Create the WarmPool CRUD service when the runtime is Kubernetes **and**
    ``kubernetes.workload_provider`` resolves to ``batchsandbox`` (WarmPool is
    only used with BatchSandbox workloads).

    Returns ``None`` for other runtimes, missing ``[kubernetes]``, or non-batchsandbox
    providers (e.g. ``agent-sandbox``). Uses a dedicated ``K8sClient`` (not the
    one held by ``KubernetesSandboxService``).
    """
    active_config = config or get_config()
    if active_config.runtime.type.lower() != "kubernetes":
        logger.debug(
            f"Pool service not created: runtime.type={active_config.runtime.type}"
        )
        return None
    if not active_config.kubernetes:
        logger.debug("Pool service not created: kubernetes config is missing")
        return None

    resolved_provider = resolve_workload_provider_type(
        active_config.kubernetes.workload_provider
    )
    if resolved_provider != PROVIDER_TYPE_BATCHSANDBOX:
        logger.debug(
            f"Pool service not created: workload_provider={resolved_provider!r} "
            f"(WarmPool requires {PROVIDER_TYPE_BATCHSANDBOX!r})"
        )
        return None

    logger.info("Creating pool service (Kubernetes WarmPool CRUD, batchsandbox)")
    k8s_client = K8sClient(active_config.kubernetes)
    return PoolService(
        k8s_client,
        namespace=active_config.kubernetes.namespace,
    )
