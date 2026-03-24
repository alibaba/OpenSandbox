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
Unit tests for AgentSandboxTemplateManager.
"""

import pytest
import yaml

from opensandbox_server.services.k8s.agent_sandbox_template import AgentSandboxTemplateManager


class TestAgentSandboxTemplateManager:
    """AgentSandboxTemplateManager unit tests"""

    def test_load_valid_yaml_template_successfully(self, tmp_path):
        """
        Test case: Verify loading valid YAML template
        """
        template_file = tmp_path / "valid_template.yaml"
        template_content = {
            "metadata": {"annotations": {"test": "value"}},
            "spec": {"podTemplate": {"spec": {"nodeSelector": {"env": "test"}}}},
        }
        template_file.write_text(yaml.dump(template_content))

        manager = AgentSandboxTemplateManager(str(template_file))

        assert manager._template == template_content
        assert manager.template_file_path == str(template_file)

    def test_load_nonexistent_file_raises_error(self):
        """
        Test case: Verify FileNotFoundError raised when file doesn't exist
        """
        with pytest.raises(FileNotFoundError) as exc_info:
            AgentSandboxTemplateManager("/path/to/nonexistent.yaml")

        assert "not found" in str(exc_info.value)

    def test_load_invalid_yaml_raises_error(self, tmp_path):
        """
        Test case: Verify RuntimeError raised with invalid YAML file
        """
        template_file = tmp_path / "invalid.yaml"
        template_file.write_text("invalid: yaml: [missing: bracket")

        with pytest.raises(RuntimeError) as exc_info:
            AgentSandboxTemplateManager(str(template_file))

        assert "Failed to load" in str(exc_info.value)

    def test_load_non_dict_yaml_raises_error(self, tmp_path):
        """
        Test case: Verify ValueError raised when YAML content is not a dict
        """
        template_file = tmp_path / "list.yaml"
        template_file.write_text("- item1\n- item2")

        with pytest.raises(ValueError) as exc_info:
            AgentSandboxTemplateManager(str(template_file))

        assert "must be a YAML object" in str(exc_info.value)
        assert "got list" in str(exc_info.value)

    def test_init_without_template_file_creates_empty_manager(self):
        """
        Test case: Verify empty manager created without template file
        """
        manager = AgentSandboxTemplateManager(None)

        assert manager._template is None
        assert manager.template_file_path is None

    def test_deep_merge_runtime_overrides_template(self):
        """
        Test case: Verify runtime values override template values
        """
        base = {"spec": {"replicas": 1, "shutdownTime": "old"}}
        override = {"spec": {"shutdownTime": "new"}}

        result = AgentSandboxTemplateManager._deep_merge(base, override)

        assert result == {"spec": {"replicas": 1, "shutdownTime": "new"}}

    def test_deep_merge_preserves_template_only_fields(self):
        """
        Test case: Verify template-only fields are preserved
        """
        base = {
            "spec": {
                "podTemplate": {
                    "spec": {
                        "nodeSelector": {"env": "prod"},
                        "tolerations": [{"key": "test"}],
                    }
                }
            }
        }
        override = {"spec": {"replicas": 1}}

        result = AgentSandboxTemplateManager._deep_merge(base, override)

        assert result["spec"]["replicas"] == 1
        assert result["spec"]["podTemplate"]["spec"]["nodeSelector"] == {"env": "prod"}
        assert result["spec"]["podTemplate"]["spec"]["tolerations"] == [{"key": "test"}]

    def test_deep_merge_nested_dicts_recursively(self):
        """
        Test case: Verify nested dicts are merged recursively
        """
        base = {"metadata": {"annotations": {"a": "1", "b": "2"}}}
        override = {"metadata": {"annotations": {"b": "3", "c": "4"}}}

        result = AgentSandboxTemplateManager._deep_merge(base, override)

        expected = {"metadata": {"annotations": {"a": "1", "b": "3", "c": "4"}}}
        assert result == expected

    def test_deep_merge_replaces_lists_not_merges(self):
        """
        Test case: Verify lists are replaced not merged
        """
        base = {"spec": {"tolerations": [{"key": "a"}]}}
        override = {"spec": {"tolerations": [{"key": "b"}]}}

        result = AgentSandboxTemplateManager._deep_merge(base, override)

        assert result == {"spec": {"tolerations": [{"key": "b"}]}}

    def test_deep_merge_none_values_do_not_override(self):
        """
        Test case: Verify None values don't override existing values
        """
        base = {"spec": {"shutdownTime": "2024-12-31"}}
        override = {"spec": {"shutdownTime": None}}

        result = AgentSandboxTemplateManager._deep_merge(base, override)

        assert result == {"spec": {"shutdownTime": "2024-12-31"}}

    def test_deep_copy_creates_independent_copies(self):
        """
        Test case: Verify deep copy creates independent copies
        """
        original = {
            "nested": {"list": [1, 2, 3], "dict": {"key": "value"}},
        }

        copy = AgentSandboxTemplateManager._deep_copy(original)

        copy["nested"]["list"].append(4)
        copy["nested"]["dict"]["key"] = "new_value"

        assert original["nested"]["list"] == [1, 2, 3]
        assert original["nested"]["dict"]["key"] == "value"

    def test_get_base_template_returns_copy(self, tmp_path):
        """
        Test case: Verify get_base_template returns a copy
        """
        template_file = tmp_path / "template.yaml"
        template_content = {"spec": {"replicas": 1}}
        template_file.write_text(yaml.dump(template_content))

        manager = AgentSandboxTemplateManager(str(template_file))

        template1 = manager.get_base_template()
        template2 = manager.get_base_template()

        assert template1 == template2
        assert template1 is not template2

    def test_get_base_template_returns_empty_dict_when_no_template(self):
        """
        Test case: Verify empty dict returned when no template
        """
        manager = AgentSandboxTemplateManager(None)

        assert manager.get_base_template() == {}
