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

from __future__ import annotations

import tempfile
from pathlib import Path
from unittest.mock import MagicMock

import pytest
from pydantic import ValidationError

from opensandbox_server.tenants import load_tenants_config
from opensandbox_server.tenants.loader import TenantLoader
from opensandbox_server.tenants.models import TenantEntry, TenantsConfig


def _write_toml(content: str, dir: Path) -> Path:
    p = dir / "tenants.toml"
    p.write_text(content)
    return p


# ---------------------------------------------------------------------------
# TenantEntry
# ---------------------------------------------------------------------------

def test_tenant_entry_constructs():
    e = TenantEntry(name="t1", namespace="ns1", api_keys=["k1", "k2"])
    assert e.name == "t1"
    assert e.namespace == "ns1"
    assert e.api_keys == ["k1", "k2"]


# ---------------------------------------------------------------------------
# TenantsConfig
# ---------------------------------------------------------------------------

def test_tenants_config_valid():
    cfg = TenantsConfig(
        entries=[
            TenantEntry(name="a", namespace="ns-a", api_keys=["ka1"]),
            TenantEntry(name="b", namespace="ns-b", api_keys=["kb1", "kb2"]),
        ]
    )
    assert len(cfg.entries) == 2


def test_tenants_config_rejects_duplicate_keys_across_entries():
    with pytest.raises(ValidationError, match="Duplicate api_key"):
        TenantsConfig(
            entries=[
                TenantEntry(name="a", namespace="ns-a", api_keys=["shared"]),
                TenantEntry(name="b", namespace="ns-b", api_keys=["shared"]),
            ]
        )


def test_tenants_config_rejects_duplicate_keys_within_same_entry():
    """A single entry with duplicate api_keys — still rejected."""
    with pytest.raises(ValidationError, match="Duplicate api_key"):
        TenantsConfig(
            entries=[
                TenantEntry(name="a", namespace="ns-a", api_keys=["dup", "dup"]),
            ]
        )


def test_tenants_config_empty_entries_ok():
    cfg = TenantsConfig(entries=[])
    assert cfg.entries == []


# ---------------------------------------------------------------------------
# load_tenants_config
# ---------------------------------------------------------------------------

def test_load_returns_none_when_file_absent():
    cfg = load_tenants_config("/tmp/nonexistent/tenants.toml")
    assert cfg is None


def test_load_parses_valid_file():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = _write_toml(
            """\
[[tenants]]
name = "team-a"
namespace = "ns-a"
api_keys = ["key1", "key2"]

[[tenants]]
name = "team-b"
namespace = "ns-b"
api_keys = ["key3"]
""",
            Path(tmpdir),
        )
        cfg = load_tenants_config(p)
        assert cfg is not None
        assert len(cfg.entries) == 2
        assert cfg.entries[0].name == "team-a"
        assert cfg.entries[0].namespace == "ns-a"
        assert cfg.entries[0].api_keys == ["key1", "key2"]
        assert cfg.entries[1].name == "team-b"
        assert cfg.entries[1].namespace == "ns-b"


def test_load_raises_on_duplicate_keys():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = _write_toml(
            """\
[[tenants]]
name = "a"
namespace = "ns-a"
api_keys = ["same-key"]

[[tenants]]
name = "b"
namespace = "ns-b"
api_keys = ["same-key"]
""",
            Path(tmpdir),
        )
        with pytest.raises(ValidationError, match="Duplicate api_key"):
            load_tenants_config(p)


def test_load_raises_on_missing_required_field():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = _write_toml(
            """\
[[tenants]]
name = "a"
# missing namespace and api_keys
""",
            Path(tmpdir),
        )
        with pytest.raises((ValidationError, TypeError, KeyError)):
            load_tenants_config(p)


# ---------------------------------------------------------------------------
# validate_namespaces
# ---------------------------------------------------------------------------

def test_validate_namespaces_all_present():
    cfg = TenantsConfig(
        entries=[
            TenantEntry(name="a", namespace="ns-a", api_keys=["k1"]),
        ]
    )
    mock_client = MagicMock()
    mock_client.get_core_v1_api().read_namespace.return_value = MagicMock()
    missing = __import__(
        "opensandbox_server.tenants", fromlist=["validate_namespaces"]
    ).validate_namespaces(cfg, mock_client)
    assert missing == []


def test_validate_namespaces_some_missing():
    cfg = TenantsConfig(
        entries=[
            TenantEntry(name="a", namespace="ns-a", api_keys=["k1"]),
            TenantEntry(name="b", namespace="ns-b", api_keys=["k2"]),
        ]
    )
    mock_client = MagicMock()

    def _read_ns(name):
        if name == "ns-b":
            raise RuntimeError("not found")
        return MagicMock()

    mock_client.get_core_v1_api().read_namespace.side_effect = _read_ns
    from opensandbox_server.tenants import validate_namespaces

    missing = validate_namespaces(cfg, mock_client)
    assert missing == ["ns-b"]


# ---------------------------------------------------------------------------
# TenantLoader
# ---------------------------------------------------------------------------

def _write_tenants_toml(dir: Path, content: str) -> Path:
    p = dir / "tenants.toml"
    p.write_text(content)
    return p


TWO_TENANTS = """\
[[tenants]]
name = "alpha"
namespace = "ns-alpha"
api_keys = ["key-a1", "key-a2"]

[[tenants]]
name = "beta"
namespace = "ns-beta"
api_keys = ["key-b1"]
"""


def test_loader_initial_load():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = _write_tenants_toml(Path(tmpdir), TWO_TENANTS)
        loader = TenantLoader(p)
        try:
            entry = loader.lookup("key-a1")
            assert entry is not None
            assert entry.name == "alpha"
            assert entry.namespace == "ns-alpha"

            entry = loader.lookup("key-b1")
            assert entry is not None
            assert entry.name == "beta"

            assert loader.lookup("unknown-key") is None
            assert loader.tenant_count == 2
        finally:
            loader.stop()


def test_loader_reload_adds_new_key():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = _write_tenants_toml(Path(tmpdir), TWO_TENANTS)
        loader = TenantLoader(p)
        try:
            assert loader.lookup("key-c1") is None

            # Overwrite file with a third tenant
            p.write_text(
                TWO_TENANTS
                + """
[[tenants]]
name = "gamma"
namespace = "ns-gamma"
api_keys = ["key-c1"]
"""
            )
            loader._reload()

            entry = loader.lookup("key-c1")
            assert entry is not None
            assert entry.name == "gamma"
            assert loader.tenant_count == 3
        finally:
            loader.stop()


def test_loader_reload_removes_tenant():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = _write_tenants_toml(Path(tmpdir), TWO_TENANTS)
        loader = TenantLoader(p)
        try:
            assert loader.lookup("key-b1") is not None

            p.write_text(
                """\
[[tenants]]
name = "alpha"
namespace = "ns-alpha"
api_keys = ["key-a1", "key-a2"]
"""
            )
            loader._reload()

            assert loader.lookup("key-b1") is None
            assert loader.lookup("key-a1") is not None
            assert loader.tenant_count == 1
        finally:
            loader.stop()


def test_loader_reload_clears_all():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = _write_tenants_toml(Path(tmpdir), TWO_TENANTS)
        loader = TenantLoader(p)
        try:
            assert loader.lookup("key-a1") is not None

            p.write_text("")  # empty file, no [[tenants]]
            loader._reload()

            assert loader.lookup("key-a1") is None
            assert loader.tenant_count == 0
        finally:
            loader.stop()


def test_loader_reload_keeps_old_entries_on_bad_file():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = _write_tenants_toml(Path(tmpdir), TWO_TENANTS)
        loader = TenantLoader(p)
        try:
            assert loader.lookup("key-a1") is not None

            p.write_text("not valid toml {{{")
            try:
                loader._reload()
            except Exception:
                pass
            # Old entries still intact because reload failed before lock swap
            assert loader.lookup("key-a1") is not None
        finally:
            loader.stop()


def test_loader_reload_rejects_duplicate_keys():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = _write_tenants_toml(Path(tmpdir), TWO_TENANTS)
        loader = TenantLoader(p)
        try:
            p.write_text(
                """\
[[tenants]]
name = "x"
namespace = "nx"
api_keys = ["dup"]

[[tenants]]
name = "y"
namespace = "ny"
api_keys = ["dup"]
"""
            )
            with pytest.raises(ValueError, match="Duplicate api_key"):
                loader._reload()
            # Old entries preserved
            assert loader.lookup("key-a1") is not None
        finally:
            loader.stop()


def test_loader_stop_stops_watcher():
    with tempfile.TemporaryDirectory() as tmpdir:
        p = _write_tenants_toml(Path(tmpdir), TWO_TENANTS)
        loader = TenantLoader(p)
        loader.stop()
        assert loader._stop_event.is_set()
