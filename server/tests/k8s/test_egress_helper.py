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
Unit tests for egress helper functions.
"""

import json

from src.api.schema import NetworkPolicy, NetworkRule
from src.services.k8s.egress_helper import (
    EGRESS_RULES_ENV,
    build_egress_sidecar_container,
    build_security_context_for_sandbox_container,
    build_ipv6_disable_sysctls,
)


class TestBuildEgressSidecarContainer:
    """Tests for build_egress_sidecar_container function."""

    def test_builds_container_with_basic_config(self):
        """Test that container is built with correct basic configuration."""
        egress_image = "opensandbox/egress:v1.0.0"
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[
                NetworkRule(action="allow", target="pypi.org"),
            ],
        )

        container = build_egress_sidecar_container(egress_image, network_policy)

        assert container["name"] == "egress"
        assert container["image"] == egress_image
        assert "env" in container
        assert "securityContext" in container

    def test_contains_egress_rules_environment_variable(self):
        """Test that container includes OPENSANDBOX_EGRESS_RULES environment variable."""
        egress_image = "opensandbox/egress:v1.0.0"
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[NetworkRule(action="allow", target="example.com")],
        )

        container = build_egress_sidecar_container(egress_image, network_policy)

        env_vars = container["env"]
        assert len(env_vars) == 1
        assert env_vars[0]["name"] == EGRESS_RULES_ENV
        assert env_vars[0]["value"] is not None

    def test_serializes_network_policy_correctly(self):
        """Test that network policy is correctly serialized to JSON."""
        egress_image = "opensandbox/egress:v1.0.0"
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[
                NetworkRule(action="allow", target="pypi.org"),
                NetworkRule(action="deny", target="*.malicious.com"),
            ],
        )

        container = build_egress_sidecar_container(egress_image, network_policy)

        env_value = container["env"][0]["value"]
        # Should be valid JSON
        policy_dict = json.loads(env_value)
        
        # Verify structure
        assert "defaultAction" in policy_dict  # by_alias=True converts default_action
        assert policy_dict["defaultAction"] == "deny"
        assert "egress" in policy_dict
        assert len(policy_dict["egress"]) == 2
        assert policy_dict["egress"][0]["action"] == "allow"
        assert policy_dict["egress"][0]["target"] == "pypi.org"
        assert policy_dict["egress"][1]["action"] == "deny"
        assert policy_dict["egress"][1]["target"] == "*.malicious.com"

    def test_handles_empty_egress_rules(self):
        """Test that empty egress rules are handled correctly."""
        egress_image = "opensandbox/egress:v1.0.0"
        network_policy = NetworkPolicy(
            default_action="allow",
            egress=[],
        )

        container = build_egress_sidecar_container(egress_image, network_policy)

        env_value = container["env"][0]["value"]
        policy_dict = json.loads(env_value)
        
        assert policy_dict["defaultAction"] == "allow"
        assert policy_dict["egress"] == []

    def test_handles_missing_default_action(self):
        """Test that missing default_action is handled (exclude_none=True)."""
        egress_image = "opensandbox/egress:v1.0.0"
        network_policy = NetworkPolicy(
            egress=[NetworkRule(action="allow", target="example.com")],
        )

        container = build_egress_sidecar_container(egress_image, network_policy)

        env_value = container["env"][0]["value"]
        policy_dict = json.loads(env_value)
        
        # defaultAction should be excluded if None (exclude_none=True)
        assert "defaultAction" not in policy_dict or policy_dict.get("defaultAction") is None
        assert "egress" in policy_dict

    def test_security_context_has_net_admin_capability(self):
        """Test that security context includes NET_ADMIN capability."""
        egress_image = "opensandbox/egress:v1.0.0"
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[],
        )

        container = build_egress_sidecar_container(egress_image, network_policy)

        security_context = container["securityContext"]
        assert "capabilities" in security_context
        assert "add" in security_context["capabilities"]
        assert "NET_ADMIN" in security_context["capabilities"]["add"]

    def test_container_spec_is_valid_kubernetes_format(self):
        """Test that returned container spec is in valid Kubernetes format."""
        egress_image = "opensandbox/egress:v1.0.0"
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[NetworkRule(action="allow", target="example.com")],
        )

        container = build_egress_sidecar_container(egress_image, network_policy)

        # Verify all required fields are present
        assert "name" in container
        assert "image" in container
        assert "env" in container
        assert "securityContext" in container
        
        # Verify env is a list of dicts with name/value
        assert isinstance(container["env"], list)
        assert len(container["env"]) > 0
        assert "name" in container["env"][0]
        assert "value" in container["env"][0]

    def test_handles_wildcard_domains(self):
        """Test that wildcard domains in egress rules are handled correctly."""
        egress_image = "opensandbox/egress:v1.0.0"
        network_policy = NetworkPolicy(
            default_action="deny",
            egress=[
                NetworkRule(action="allow", target="*.python.org"),
                NetworkRule(action="allow", target="pypi.org"),
            ],
        )

        container = build_egress_sidecar_container(egress_image, network_policy)

        env_value = container["env"][0]["value"]
        policy_dict = json.loads(env_value)
        
        assert len(policy_dict["egress"]) == 2
        assert policy_dict["egress"][0]["target"] == "*.python.org"
        assert policy_dict["egress"][1]["target"] == "pypi.org"


class TestBuildSecurityContextForMainContainer:
    """Tests for build_security_context_for_main_container function."""

    def test_returns_empty_dict_when_no_network_policy(self):
        """Test that empty dict is returned when network policy is disabled."""
        result = build_security_context_for_sandbox_container(has_network_policy=False)
        assert result == {}

    def test_drops_net_admin_when_network_policy_enabled(self):
        """Test that NET_ADMIN is dropped when network policy is enabled."""
        result = build_security_context_for_sandbox_container(has_network_policy=True)
        
        assert "capabilities" in result
        assert "drop" in result["capabilities"]
        assert "NET_ADMIN" in result["capabilities"]["drop"]


class TestBuildIpv6DisableSysctls:
    """Tests for build_ipv6_disable_sysctls function."""

    def test_returns_list_of_sysctls(self):
        """Test that function returns a list of sysctl configurations."""
        sysctls = build_ipv6_disable_sysctls()
        
        assert isinstance(sysctls, list)
        assert len(sysctls) == 3

    def test_contains_all_required_ipv6_disable_sysctls(self):
        """Test that all required IPv6 disable sysctls are present."""
        sysctls = build_ipv6_disable_sysctls()
        
        sysctl_names = {s["name"] for s in sysctls}
        expected_names = {
            "net.ipv6.conf.all.disable_ipv6",
            "net.ipv6.conf.default.disable_ipv6",
            "net.ipv6.conf.lo.disable_ipv6",
        }
        
        assert sysctl_names == expected_names

    def test_all_sysctls_have_value_one(self):
        """Test that all sysctls have value "1"."""
        sysctls = build_ipv6_disable_sysctls()
        
        for sysctl in sysctls:
            assert sysctl["value"] == "1"
            assert "name" in sysctl

    def test_sysctls_are_in_valid_kubernetes_format(self):
        """Test that sysctls are in valid Kubernetes format."""
        sysctls = build_ipv6_disable_sysctls()
        
        for sysctl in sysctls:
            assert isinstance(sysctl, dict)
            assert "name" in sysctl
            assert "value" in sysctl
            assert isinstance(sysctl["name"], str)
            assert isinstance(sysctl["value"], str)
