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

"""
Egress sidecar helper functions for Kubernetes workloads.

This module provides shared utilities for building egress sidecar containers
and related configurations that can be reused across different workload providers.
"""

import json
from typing import Dict, Any

from src.api.schema import NetworkPolicy

# Environment variable name for passing network policy to egress sidecar
EGRESS_RULES_ENV = "OPENSANDBOX_EGRESS_RULES"


def build_egress_sidecar_container(
    egress_image: str,
    network_policy: NetworkPolicy,
) -> Dict[str, Any]:
    """
    Build egress sidecar container specification for Kubernetes Pod.
    
    This function creates a container spec that can be added to a Pod's containers
    list. The sidecar container will:
    - Run the egress image
    - Receive network policy via OPENSANDBOX_EGRESS_RULES environment variable
    - Have NET_ADMIN capability to manage iptables
    
    Note: In Kubernetes, containers in the same Pod share the network namespace,
    so the main container can access the sidecar's ports (44772 for execd, 8080 for HTTP)
    via localhost without explicit port declarations.
    
    Important: IPv6 should be disabled at the Pod level (not container level) using
    build_ipv6_disable_sysctls() and adding the result to Pod's securityContext.sysctls.
    
    Args:
        egress_image: Container image for the egress sidecar
        network_policy: Network policy configuration to enforce
        
    Returns:
        Dict containing container specification compatible with Kubernetes Pod spec.
        This dict can be directly added to the Pod's containers list.
        
    Example:
        ```python
        sidecar = build_egress_sidecar_container(
            egress_image="opensandbox/egress:v1.0.0",
            network_policy=NetworkPolicy(
                default_action="deny",
                egress=[NetworkRule(action="allow", target="pypi.org")]
            )
        )
        pod_spec["containers"].append(sidecar)
        
        # Disable IPv6 at Pod level
        if "securityContext" not in pod_spec:
            pod_spec["securityContext"] = {}
        pod_spec["securityContext"]["sysctls"] = build_ipv6_disable_sysctls()
        ```
    """
    # Serialize network policy to JSON for environment variable
    policy_payload = json.dumps(
        network_policy.model_dump(by_alias=True, exclude_none=True)
    )
    
    # Build container specification
    container_spec: Dict[str, Any] = {
        "name": "egress",
        "image": egress_image,
        "env": [
            {
                "name": EGRESS_RULES_ENV,
                "value": policy_payload,
            }
        ],
        "securityContext": _build_security_context_for_egress(),
    }
    
    return container_spec


def _build_security_context_for_egress() -> Dict[str, Any]:
    """
    Build security context for egress sidecar container.
    
    The egress sidecar needs NET_ADMIN capability to manage iptables rules
    for network policy enforcement.
    
    This is an internal helper function used by build_egress_sidecar_container().
    
    Returns:
        Dict containing security context configuration with NET_ADMIN capability.
    """
    return {
        "capabilities": {
            "add": ["NET_ADMIN"],
        },
    }


def build_security_context_for_main_container(
    has_network_policy: bool,
) -> Dict[str, Any]:
    """
    Build security context for main sandbox container.
    
    When network policy is enabled, the main container should drop NET_ADMIN
    capability to prevent it from modifying network configuration. Only the
    egress sidecar should have NET_ADMIN.
    
    Args:
        has_network_policy: Whether network policy is enabled for this sandbox
        
    Returns:
        Dict containing security context configuration. If has_network_policy is True,
        includes NET_ADMIN in the drop list. Otherwise, returns empty dict.
    """
    if not has_network_policy:
        return {}
    
    return {
        "capabilities": {
            "drop": ["NET_ADMIN"],
        },
    }


def build_ipv6_disable_sysctls() -> list[Dict[str, str]]:
    """
    Build sysctls configuration to disable IPv6 in the Pod.
    
    When egress sidecar is used, IPv6 should be disabled in the shared network
    namespace to keep policy enforcement consistent. This matches the Docker
    implementation behavior.
    
    Returns:
        List of sysctl configurations to disable IPv6 at Pod level.
        
    Note:
        These sysctls need to be set at the Pod's securityContext level, not
        at the container level. The calling code should merge this into the
        Pod spec's securityContext.sysctls field.
    """
    return [
        {"name": "net.ipv6.conf.all.disable_ipv6", "value": "1"},
        {"name": "net.ipv6.conf.default.disable_ipv6", "value": "1"},
        {"name": "net.ipv6.conf.lo.disable_ipv6", "value": "1"},
    ]
