
import unittest
import re
from unittest.mock import patch, MagicMock, mock_open
from pathlib import Path
import sys

# Add scripts directory to sys.path to allow importing manage_sandbox_version
sys.path.append(str(Path(__file__).parent))

import manage_sandbox_version

class TestManageSandboxVersion(unittest.TestCase):

    def test_get_pattern_basic(self):
        pattern = manage_sandbox_version.get_pattern("opensandbox/execd")
        
        # Test matches
        self.assertTrue(pattern.search("image: opensandbox/execd:v1.0.0"))
        self.assertTrue(pattern.search('"opensandbox/execd:v1.2.3"'))
        
        # Test group capturing
        match = pattern.search("opensandbox/execd:v1.0.0")
        self.assertEqual(match.group(2), "v1.0.0")
        self.assertIsNone(match.group(1))

    def test_get_pattern_registry_prefix(self):
        pattern = manage_sandbox_version.get_pattern("opensandbox/execd")
        
        # Simple registry
        text = "registry.example.com/opensandbox/execd:v1.0.0"
        match = pattern.search(text)
        self.assertTrue(match)
        self.assertEqual(match.group(1), "registry.example.com/")
        self.assertEqual(match.group(2), "v1.0.0")

        # Complex registry with port and multi-level path
        text = "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/execd:v1.0.5"
        match = pattern.search(text)
        self.assertTrue(match)
        self.assertEqual(match.group(1), "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/")
        self.assertEqual(match.group(2), "v1.0.5")

    def test_get_pattern_ignores(self):
        pattern = manage_sandbox_version.get_pattern("opensandbox/execd")
        
        # Should ignore these tags
        self.assertFalse(pattern.search("opensandbox/execd:latest"))
        self.assertFalse(pattern.search("opensandbox/execd:local"))
        self.assertFalse(pattern.search("opensandbox/execd:dev"))
        self.assertFalse(pattern.search("opensandbox/execd:test"))
        self.assertFalse(pattern.search("opensandbox/execd:${TAG}"))
        self.assertFalse(pattern.search("opensandbox/execd:$TAG"))

    def test_should_process_file(self):
        # Valid files
        self.assertTrue(manage_sandbox_version.should_process_file(Path("README.md")))
        self.assertTrue(manage_sandbox_version.should_process_file(Path("script.py")))
        self.assertTrue(manage_sandbox_version.should_process_file(Path("config.yaml")))
        
        # Invalid files
        self.assertFalse(manage_sandbox_version.should_process_file(Path(".hidden")))
        self.assertFalse(manage_sandbox_version.should_process_file(Path("image.png")))
        self.assertFalse(manage_sandbox_version.should_process_file(Path("text.txt")))
        
        # Should ignore the script itself
        script_path = Path(manage_sandbox_version.__file__)
        self.assertFalse(manage_sandbox_version.should_process_file(script_path))

    @patch('manage_sandbox_version.PROJECT_ROOT', Path("/mock/root"))
    @patch('manage_sandbox_version.COMPONENTS')
    def test_get_current_version(self, mock_components):
        mock_version_file = MagicMock()
        mock_version_file.exists.return_value = True
        mock_version_file.read_text.return_value = "v1.2.3\n"
        
        mock_components.__getitem__.return_value = {
            "version_file": mock_version_file
        }
        
        version = manage_sandbox_version.get_current_version("test-component")
        self.assertEqual(version, "v1.2.3")

    @patch('manage_sandbox_version.scan_files')
    @patch('manage_sandbox_version.get_current_version')
    @patch('manage_sandbox_version.COMPONENTS')
    def test_verify_component_mismatch(self, mock_components, mock_get_version, mock_scan_files):
        # Setup
        mock_get_version.return_value = "v2.0.0"
        mock_components.__getitem__.return_value = {"image": "test/image"}
        
        # Mock file content with mismatching version
        mock_file = MagicMock()
        mock_file.read_text.return_value = "image: test/image:v1.0.0"
        mock_file.relative_to.return_value = Path("test/file.md")
        mock_scan_files.return_value = [mock_file]
        
        # Run
        errors = manage_sandbox_version.verify_component("test-component")
        
        # Assert
        self.assertEqual(len(errors), 1)
        self.assertIn("Found v1.0.0, expected v2.0.0", errors[0])

    @patch('manage_sandbox_version.scan_files')
    @patch('manage_sandbox_version.get_current_version')
    @patch('manage_sandbox_version.COMPONENTS')
    def test_verify_component_match(self, mock_components, mock_get_version, mock_scan_files):
        # Setup
        mock_get_version.return_value = "v2.0.0"
        mock_components.__getitem__.return_value = {"image": "test/image"}
        
        # Mock file content with matching version
        mock_file = MagicMock()
        mock_file.read_text.return_value = "image: test/image:v2.0.0"
        mock_scan_files.return_value = [mock_file]
        
        # Run
        errors = manage_sandbox_version.verify_component("test-component")
        
        # Assert
        self.assertEqual(len(errors), 0)

    @patch('manage_sandbox_version.scan_files')
    @patch('manage_sandbox_version.get_current_version')
    @patch('manage_sandbox_version.COMPONENTS')
    def test_update_component(self, mock_components, mock_get_version, mock_scan_files):
        # Setup
        mock_get_version.return_value = "v2.0.0"
        mock_components.__getitem__.return_value = {"image": "test/image"}
        
        # Mock file with old version
        mock_file = MagicMock()
        original_content = "some config\nimage: test/image:v1.0.0\nend"
        mock_file.read_text.return_value = original_content
        mock_file.relative_to.return_value = Path("test/config.yaml")
        mock_scan_files.return_value = [mock_file]
        
        # Run
        with patch('builtins.print'): # suppress print output
            manage_sandbox_version.update_component("test-component")
        
        # Assert
        expected_content = "some config\nimage: test/image:v2.0.0\nend"
        mock_file.write_text.assert_called_once_with(expected_content, encoding="utf-8")

    @patch('manage_sandbox_version.scan_files')
    @patch('manage_sandbox_version.get_current_version')
    @patch('manage_sandbox_version.COMPONENTS')
    def test_update_component_preserves_prefix(self, mock_components, mock_get_version, mock_scan_files):
        # Setup
        mock_get_version.return_value = "v2.0.0"
        mock_components.__getitem__.return_value = {"image": "test/image"}
        
        # Mock file with registry prefix
        mock_file = MagicMock()
        original_content = "image: my.registry.com/test/image:v1.0.0"
        mock_file.read_text.return_value = original_content
        mock_file.relative_to.return_value = Path("test/config.yaml")
        mock_scan_files.return_value = [mock_file]
        
        # Run
        with patch('builtins.print'):
            manage_sandbox_version.update_component("test-component")
        
        # Assert
        expected_content = "image: my.registry.com/test/image:v2.0.0"
        mock_file.write_text.assert_called_once_with(expected_content, encoding="utf-8")

if __name__ == '__main__':
    unittest.main()
