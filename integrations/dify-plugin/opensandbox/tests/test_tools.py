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

"""Unit tests for OpenSandbox Dify plugin tools.

These tests directly invoke the tool logic without going through Dify,
providing coverage for the actual sandbox operations.
"""

from __future__ import annotations

import os
import unittest
from unittest.mock import MagicMock, patch

# Skip tests if OpenSandbox server is not available
OPENSANDBOX_URL = os.environ.get("OPENSANDBOX_BASE_URL", "http://localhost:8080")
OPENSANDBOX_KEY = os.environ.get("OPENSANDBOX_API_KEY", "test-key")


class MockRuntime:
    """Mock Dify plugin runtime."""

    def __init__(self, credentials: dict):
        self.credentials = credentials


class TestToolsUnit(unittest.TestCase):
    """Unit tests for tool utility functions."""

    def test_normalize_domain(self):
        from tools.utils import normalize_domain

        self.assertEqual(normalize_domain("http://localhost:8080"), "http://localhost:8080")
        self.assertEqual(normalize_domain("http://localhost:8080/"), "http://localhost:8080")
        self.assertEqual(normalize_domain("http://localhost:8080/v1"), "http://localhost:8080")
        self.assertEqual(normalize_domain("http://localhost:8080/v1/"), "http://localhost:8080")

    def test_build_connection_config_from_credentials(self):
        from tools.utils import build_connection_config

        with patch.dict(os.environ, {}, clear=True):
            config = build_connection_config({
                "opensandbox_base_url": "http://test:8080",
                "opensandbox_api_key": "test-key",
            })
            self.assertEqual(config.domain, "http://test:8080")
            self.assertEqual(config.api_key, "test-key")

    def test_build_connection_config_from_env(self):
        from tools.utils import build_connection_config

        with patch.dict(os.environ, {
            "OPENSANDBOX_BASE_URL": "http://env:8080",
            "OPENSANDBOX_API_KEY": "env-key",
        }):
            config = build_connection_config({})
            self.assertEqual(config.domain, "http://env:8080")
            self.assertEqual(config.api_key, "env-key")

    def test_build_connection_config_missing_url(self):
        from tools.utils import build_connection_config

        with patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(ValueError) as ctx:
                build_connection_config({})
            self.assertIn("opensandbox_base_url", str(ctx.exception))

    def test_parse_optional_json_valid(self):
        from tools.utils import parse_optional_json

        result = parse_optional_json('{"key": "value"}', "test")
        self.assertEqual(result, {"key": "value"})

    def test_parse_optional_json_empty(self):
        from tools.utils import parse_optional_json

        self.assertIsNone(parse_optional_json(None, "test"))
        self.assertIsNone(parse_optional_json("", "test"))

    def test_parse_optional_json_invalid(self):
        from tools.utils import parse_optional_json

        with self.assertRaises(ValueError):
            parse_optional_json("not json", "test")


class TestToolsIntegration(unittest.TestCase):
    """Integration tests that require a running OpenSandbox server."""

    @classmethod
    def setUpClass(cls):
        """Check if OpenSandbox server is available."""
        import requests

        try:
            resp = requests.get(f"{OPENSANDBOX_URL}/health", timeout=5)
            cls.server_available = resp.status_code == 200
        except Exception:
            cls.server_available = False

        if not cls.server_available:
            print(f"Skipping integration tests: OpenSandbox server not available at {OPENSANDBOX_URL}")

    def setUp(self):
        if not self.server_available:
            self.skipTest("OpenSandbox server not available")

    def test_sandbox_lifecycle(self):
        """Test full sandbox lifecycle: create -> run -> kill."""
        from datetime import timedelta

        from opensandbox.sync.sandbox import SandboxSync
        from opensandbox.config.connection_sync import ConnectionConfigSync

        config = ConnectionConfigSync(domain=OPENSANDBOX_URL, api_key=OPENSANDBOX_KEY)

        # Create sandbox
        sandbox = SandboxSync.create(
            "python:3.11-slim",
            timeout=timedelta(seconds=60),
            ready_timeout=timedelta(seconds=30),
            connection_config=config,
        )
        self.assertIsNotNone(sandbox.id)
        print(f"Created sandbox: {sandbox.id}")

        try:
            # Run command
            result = sandbox.commands.run("echo opensandbox-test")
            # Check no error occurred
            self.assertIsNone(result.error, f"Execution error: {result.error}")
            # Check stdout contains expected output
            stdout_text = "".join(msg.text or "" for msg in result.logs.stdout)
            self.assertIn("opensandbox-test", stdout_text)
            print(f"Command output: {stdout_text}")

        finally:
            # Kill sandbox
            sandbox.kill()
            print("Sandbox killed")


if __name__ == "__main__":
    unittest.main()