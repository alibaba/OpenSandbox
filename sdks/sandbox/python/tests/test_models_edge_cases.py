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
Additional tests for sandbox models covering edge cases.

This module provides test coverage for edge cases and validation scenarios
not fully covered by the main test suite.
"""
import pytest

from opensandbox.models.sandboxes import (
    NetworkPolicy,
    NetworkRule,
    SandboxImageAuth,
    SandboxImageSpec,
)


# ============================================================================
# SandboxImageAuth Tests
# ============================================================================


def test_sandbox_image_auth_strips_whitespace() -> None:
    """Test that whitespace-only usernames and passwords are rejected."""
    with pytest.raises(ValueError, match="blank"):
        SandboxImageAuth(username="   ", password="valid_pass")
    with pytest.raises(ValueError, match="blank"):
        SandboxImageAuth(username="valid_user", password="   ")


def test_sandbox_image_auth_valid_credentials() -> None:
    """Test that valid credentials are accepted."""
    auth = SandboxImageAuth(username="user", password="pass")
    assert auth.username == "user"
    assert auth.password == "pass"


def test_sandbox_image_auth_unicode_credentials() -> None:
    """Test that unicode credentials are handled correctly."""
    auth = SandboxImageAuth(username="用户", password="пароль")
    assert auth.username == "用户"
    assert auth.password == "пароль"


# ============================================================================
# SandboxImageSpec Tests
# ============================================================================


def test_sandbox_image_spec_with_auth() -> None:
    """Test creating image spec with authentication."""
    auth = SandboxImageAuth(username="user", password="pass")
    spec = SandboxImageSpec(image="private.registry.com/image:tag", auth=auth)
    assert spec.image == "private.registry.com/image:tag"
    assert spec.auth is not None
    assert spec.auth.username == "user"
    assert spec.auth.password == "pass"


def test_sandbox_image_spec_without_auth() -> None:
    """Test creating image spec without authentication."""
    spec = SandboxImageSpec(image="public.registry.com/image:tag")
    assert spec.image == "public.registry.com/image:tag"
    assert spec.auth is None


def test_sandbox_image_spec_positional_arg() -> None:
    """Test positional argument for image."""
    spec = SandboxImageSpec("python:3.11")
    assert spec.image == "python:3.11"


def test_sandbox_image_spec_empty_string() -> None:
    """Test that empty string image is rejected."""
    with pytest.raises(ValueError, match="blank"):
        SandboxImageSpec("")


def test_sandbox_image_spec_whitespace_only() -> None:
    """Test that whitespace-only image is rejected."""
    with pytest.raises(ValueError, match="blank"):
        SandboxImageSpec("   \t\n  ")


def test_sandbox_image_spec_complex_image_names() -> None:
    """Test various valid image name formats."""
    valid_names = [
        "ubuntu:22.04",
        "python:3.11-slim",
        "my-registry.com/image:tag",
        "docker.io/library/nginx:latest",
        "gcr.io/project/image@sha256:abc123",
        "registry:5000/image:v1.0.0",
    ]
    for name in valid_names:
        spec = SandboxImageSpec(name)
        assert spec.image == name


# ============================================================================
# NetworkRule Tests
# ============================================================================


def test_network_rule_valid_allow() -> None:
    """Test creating valid allow rule."""
    rule = NetworkRule(action="allow", target="example.com")
    assert rule.action == "allow"
    assert rule.target == "example.com"


def test_network_rule_valid_deny() -> None:
    """Test creating valid deny rule."""
    rule = NetworkRule(action="deny", target="*.evil.com")
    assert rule.action == "deny"
    assert rule.target == "*.evil.com"


def test_network_rule_empty_target() -> None:
    """Test that empty target is rejected."""
    with pytest.raises(ValueError, match="blank"):
        NetworkRule(action="allow", target="")


def test_network_rule_whitespace_target() -> None:
    """Test that whitespace-only target is rejected."""
    with pytest.raises(ValueError, match="blank"):
        NetworkRule(action="allow", target="   ")


def test_network_rule_wildcard_targets() -> None:
    """Test various valid wildcard target patterns."""
    targets = [
        "*.example.com",
        "api.*.example.com",
        "*.github.com",
        "pypi.org",
        "*.pypi.org",
        "192.168.1.1",
    ]
    for target in targets:
        rule = NetworkRule(action="allow", target=target)
        assert rule.target == target


# ============================================================================
# NetworkPolicy Tests
# ============================================================================


def test_network_policy_default_action() -> None:
    """Test that default action is 'deny'."""
    policy = NetworkPolicy()
    assert policy.default_action == "deny"
    assert policy.egress is None


def test_network_policy_custom_default_action() -> None:
    """Test setting custom default action."""
    policy = NetworkPolicy(default_action="allow")
    assert policy.default_action == "allow"


def test_network_policy_with_egress_rules() -> None:
    """Test policy with egress rules."""
    rules = [
        NetworkRule(action="allow", target="pypi.org"),
        NetworkRule(action="deny", target="*.evil.com"),
    ]
    policy = NetworkPolicy(egress=rules)
    assert policy.egress is not None
    assert len(policy.egress) == 2
    assert policy.egress[0].target == "pypi.org"
    assert policy.egress[1].target == "*.evil.com"


def test_network_policy_empty_egress_list() -> None:
    """Test policy with empty egress list."""
    policy = NetworkPolicy(egress=[])
    assert policy.egress == []


def test_network_policy_serialization() -> None:
    """Test that policy serializes correctly with aliases."""
    policy = NetworkPolicy(
        default_action="allow",
        egress=[NetworkRule(action="allow", target="example.com")],
    )
    dumped = policy.model_dump(by_alias=True, mode="json")
    assert "defaultAction" in dumped
    assert dumped["defaultAction"] == "allow"
    assert "egress" in dumped
    assert len(dumped["egress"]) == 1


# ============================================================================
# Integration Tests
# ============================================================================


def test_sandbox_image_spec_with_auth_serialization() -> None:
    """Test serialization of image spec with auth."""
    auth = SandboxImageAuth(username="user", password="pass")
    spec = SandboxImageSpec(image="private.registry.com/image:tag", auth=auth)
    dumped = spec.model_dump(by_alias=True, mode="json")
    assert dumped["image"] == "private.registry.com/image:tag"
    assert dumped["auth"]["username"] == "user"
    assert dumped["auth"]["password"] == "pass"


def test_network_policy_complex_scenario() -> None:
    """Test a realistic network policy scenario."""
    policy = NetworkPolicy(
        default_action="deny",
        egress=[
            NetworkRule(action="allow", target="pypi.org"),
            NetworkRule(action="allow", target="*.pythonhosted.org"),
            NetworkRule(action="allow", target="github.com"),
            NetworkRule(action="deny", target="*.facebook.com"),
        ],
    )
    assert policy.default_action == "deny"
    assert len(policy.egress) == 4
    
    # Verify allow rules
    allow_rules = [r for r in policy.egress if r.action == "allow"]
    deny_rules = [r for r in policy.egress if r.action == "deny"]
    
    assert len(allow_rules) == 3
    assert len(deny_rules) == 1
    assert deny_rules[0].target == "*.facebook.com"
