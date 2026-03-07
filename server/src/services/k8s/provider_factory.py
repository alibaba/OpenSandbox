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
Factory for creating WorkloadProvider instances.
"""

import logging
from typing import Dict, Type, Optional

from src.config import AppConfig
from src.services.k8s.workload_provider import WorkloadProvider
from src.services.k8s.batchsandbox_provider import BatchSandboxProvider
from src.services.k8s.agent_sandbox_provider import AgentSandboxProvider
from src.services.k8s.client import K8sClient

logger = logging.getLogger(__name__)

# Provider type constants
PROVIDER_TYPE_BATCHSANDBOX = "batchsandbox"
PROVIDER_TYPE_AGENT_SANDBOX = "agent-sandbox"

# Registry of available workload providers
_PROVIDER_REGISTRY: Dict[str, Type[WorkloadProvider]] = {
    PROVIDER_TYPE_BATCHSANDBOX: BatchSandboxProvider,
    PROVIDER_TYPE_AGENT_SANDBOX: AgentSandboxProvider,
    # Future providers can be registered here:
    # "pod": PodProvider
}


def create_workload_provider(
    provider_type: str | None,
    k8s_client: K8sClient,
    app_config: Optional[AppConfig] = None,
) -> WorkloadProvider:
    """
    Create a WorkloadProvider instance based on the provider type.

    All provider-specific configuration is read from app_config; no separate
    config objects are passed.

    Args:
        provider_type: Type of provider (e.g., 'batchsandbox', 'agent-sandbox').
                      If None, uses the first registered provider.
        k8s_client: Kubernetes client instance.
        app_config: Application config; used by built-in providers for
                   kubernetes, agent_sandbox, and ingress settings.

    Returns:
        WorkloadProvider instance.

    Raises:
        ValueError: If provider_type is not supported or no providers are registered.
    """
    # Use first registered provider if not specified
    if provider_type is None:
        if not _PROVIDER_REGISTRY:
            raise ValueError(
                "No workload providers are registered. "
                "Cannot create a default provider."
            )
        provider_type = next(iter(_PROVIDER_REGISTRY.keys()))
        logger.info(f"No provider specified, using default: {provider_type}")

    provider_type_lower = provider_type.lower()

    if provider_type_lower not in _PROVIDER_REGISTRY:
        available = ", ".join(_PROVIDER_REGISTRY.keys())
        raise ValueError(
            f"Unsupported workload provider type '{provider_type}'. "
            f"Available providers: {available}"
        )

    provider_class = _PROVIDER_REGISTRY[provider_type_lower]
    logger.info(f"Creating workload provider: {provider_class.__name__}")

    # Built-in providers accept (k8s_client, app_config) and read config internally
    if provider_type_lower in (PROVIDER_TYPE_BATCHSANDBOX, PROVIDER_TYPE_AGENT_SANDBOX):
        return provider_class(k8s_client, app_config=app_config)

    # Custom/other providers may only accept k8s_client
    return provider_class(k8s_client)


def register_provider(name: str, provider_class: Type[WorkloadProvider]) -> None:
    """
    Register a custom WorkloadProvider implementation.
    
    This allows extending the system with custom provider implementations
    without modifying core code.
    
    Args:
        name: Provider name (used in configuration)
        provider_class: Provider class that implements WorkloadProvider
        
    Example:
        from my_module import CustomProvider
        register_provider("custom", CustomProvider)
    """
    if not issubclass(provider_class, WorkloadProvider):
        raise TypeError(
            f"Provider class must inherit from WorkloadProvider, "
            f"got {provider_class.__name__}"
        )
    
    name_lower = name.lower()
    if name_lower in _PROVIDER_REGISTRY:
        logger.warning(
            f"Overwriting existing provider registration: {name_lower}"
        )
    
    _PROVIDER_REGISTRY[name_lower] = provider_class
    logger.info(f"Registered workload provider: {name_lower} -> {provider_class.__name__}")


def list_available_providers() -> list[str]:
    """
    List all registered provider types.
    
    Returns:
        List of provider type names
    """
    return sorted(_PROVIDER_REGISTRY.keys())
