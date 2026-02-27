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
from typing import Dict, Any, List, Optional

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
            egress_image="opensandbox/egress:v1.0.1",
            network_policy=NetworkPolicy(
                default_action="deny",
                egress=[NetworkRule(action="allow", target="pypi.org")]
            )
        )
        pod_spec["containers"].append(sidecar)
        
        # Disable IPv6 at Pod level (extends existing sysctls)
        if "securityContext" not in pod_spec:
            pod_spec["securityContext"] = {}
        existing_sysctls = pod_spec["securityContext"].get("sysctls")
        new_sysctls = build_ipv6_disable_sysctls()
        pod_spec["securityContext"]["sysctls"] = _merge_sysctls(
            existing_sysctls, new_sysctls
        )
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


def build_security_context_for_sandbox_container(
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


def _merge_sysctls(
    existing_sysctls: Optional[List[Dict[str, str]]],
    new_sysctls: List[Dict[str, str]],
) -> List[Dict[str, str]]:
    """
    Merge new sysctls into existing sysctls, avoiding duplicates.
    
    If a sysctl with the same name already exists, the new value will
    override the existing one (last write wins).
    
    Args:
        existing_sysctls: Existing sysctls list or None
        new_sysctls: New sysctls to merge in
        
    Returns:
        Merged list of sysctls with no duplicate names
    """
    if not existing_sysctls:
        return new_sysctls.copy()
    
    # Create a dict to track sysctls by name (for deduplication)
    sysctls_dict: Dict[str, str] = {}
    
    # First, add existing sysctls
    for sysctl in existing_sysctls:
        if isinstance(sysctl, dict) and "name" in sysctl:
            sysctls_dict[sysctl["name"]] = sysctl.get("value", "")
    
    # Then, add/override with new sysctls
    for sysctl in new_sysctls:
        if isinstance(sysctl, dict) and "name" in sysctl:
            sysctls_dict[sysctl["name"]] = sysctl.get("value", "")
    
    # Convert back to list format
    return [{"name": name, "value": value} for name, value in sysctls_dict.items()]


def apply_egress_to_spec(
    pod_spec: Dict[str, Any],
    containers: List[Dict[str, Any]],
    network_policy: Optional[NetworkPolicy],
    egress_image: Optional[str],
) -> None:
    """
    Apply egress sidecar configuration to Pod spec.
    
    This function adds the egress sidecar container to the containers list
    and configures IPv6 disable sysctls at the Pod level when network policy
    is provided. Existing sysctls are preserved and merged with the new ones.
    
    Args:
        pod_spec: Pod specification dict (will be modified in place)
        containers: List of container dicts (will be modified in place)
        network_policy: Optional network policy configuration
        egress_image: Optional egress sidecar image
        
    Example:
        ```python
        containers = [main_container_dict]
        pod_spec = {"containers": containers, ...}
        
        apply_egress_to_spec(
            pod_spec=pod_spec,
            containers=containers,
            network_policy=network_policy,
            egress_image=egress_image,
        )
        ```
        
    Note:
        This function extends existing sysctls rather than overwriting them.
        If a sysctl with the same name already exists, the egress-related
        sysctls will override it (last write wins).
    """
    if not network_policy or not egress_image:
        return
    
    # Build and add egress sidecar container
    sidecar_container = build_egress_sidecar_container(
        egress_image=egress_image,
        network_policy=network_policy,
    )
    containers.append(sidecar_container)
    
    # Disable IPv6 at Pod level, merging with existing sysctls
    if "securityContext" not in pod_spec:
        pod_spec["securityContext"] = {}
    
    existing_sysctls = pod_spec["securityContext"].get("sysctls")
    new_sysctls = build_ipv6_disable_sysctls()
    pod_spec["securityContext"]["sysctls"] = _merge_sysctls(
        existing_sysctls, new_sysctls
    )


def build_security_context_from_dict(
    security_context_dict: Dict[str, Any],
) -> Optional[Any]:
    """
    Convert security context dict to V1SecurityContext object.
    
    This is a helper function to convert the dict returned by
    build_security_context_for_sandbox_container() into a Kubernetes
    V1SecurityContext object that can be used in V1Container.
    
    Args:
        security_context_dict: Security context configuration dict
        
    Returns:
        V1SecurityContext object or None if dict is empty
        
    Example:
        ```python
        from kubernetes.client import V1Container
        
        security_context_dict = build_security_context_for_sandbox_container(True)
        security_context = build_security_context_from_dict(security_context_dict)
        
        container = V1Container(
            name="sandbox",
            security_context=security_context,
        )
        ```
    """
    if not security_context_dict:
        return None
    
    from kubernetes.client import V1SecurityContext, V1Capabilities
    
    capabilities = None
    if "capabilities" in security_context_dict:
        caps_dict = security_context_dict["capabilities"]
        add_caps = caps_dict.get("add", [])
        drop_caps = caps_dict.get("drop", [])
        capabilities = V1Capabilities(
            add=add_caps if add_caps else None,
            drop=drop_caps if drop_caps else None,
        )
    
    return V1SecurityContext(capabilities=capabilities)


def serialize_security_context_to_dict(
    security_context: Optional[Any],
) -> Optional[Dict[str, Any]]:
    """
    Serialize V1SecurityContext to dict format for CRD.
    
    This function converts a V1SecurityContext object (from V1Container)
    into a dict format that can be used in Kubernetes CRD specifications.
    
    Args:
        security_context: V1SecurityContext object or None
        
    Returns:
        Dict representation of security context or None
        
    Example:
        ```python
        container_dict = {
            "name": container.name,
            "image": container.image,
        }
        
        if container.security_context:
            container_dict["securityContext"] = serialize_security_context_to_dict(
                container.security_context
            )
        ```
    """
    if not security_context:
        return None
    
    result: Dict[str, Any] = {}
    
    if security_context.capabilities:
        caps: Dict[str, Any] = {}
        if security_context.capabilities.add:
            caps["add"] = security_context.capabilities.add
        if security_context.capabilities.drop:
            caps["drop"] = security_context.capabilities.drop
        if caps:
            result["capabilities"] = caps
    
    return result if result else None


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
