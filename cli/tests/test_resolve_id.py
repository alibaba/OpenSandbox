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

"""Tests for Docker-style sandbox ID prefix matching."""

from __future__ import annotations

from unittest.mock import MagicMock

import click
import pytest

from opensandbox_cli.client import ClientContext
from opensandbox_cli.output import OutputFormatter


def _make_sandbox_info(sandbox_id: str) -> MagicMock:
    """Create a mock SandboxInfo with given ID."""
    info = MagicMock()
    info.id = sandbox_id
    return info


def _make_paged_result(
    sandbox_ids: list[str], *, has_next_page: bool = False
) -> MagicMock:
    """Create a mock PagedSandboxInfos with pagination metadata."""
    result = MagicMock()
    result.sandbox_infos = [_make_sandbox_info(sid) for sid in sandbox_ids]
    result.pagination = MagicMock()
    result.pagination.has_next_page = has_next_page
    return result


def _make_client_context(
    sandbox_ids: list[str],
    *,
    pages: list[list[str]] | None = None,
) -> ClientContext:
    """Create a ClientContext with a mocked manager listing the given IDs.

    If *pages* is provided, each element is a separate page of sandbox IDs
    (useful for testing pagination).  Otherwise all IDs are in a single page.
    """
    ctx = ClientContext(
        resolved_config={
            "api_key": "test-key",
            "domain": "localhost:8080",
            "protocol": "http",
            "request_timeout": 30,
            "output_format": "json",
            "color": False,
            "default_image": None,
            "default_timeout": None,
        },
        output=OutputFormatter("json", color=False),
    )
    # Mock the manager
    mock_mgr = MagicMock()
    if pages is not None:
        side_effects = []
        for i, page_ids in enumerate(pages):
            has_next = i < len(pages) - 1
            side_effects.append(_make_paged_result(page_ids, has_next_page=has_next))
        mock_mgr.list_sandbox_infos.side_effect = side_effects
    else:
        mock_mgr.list_sandbox_infos.return_value = _make_paged_result(sandbox_ids)
    ctx._manager = mock_mgr
    return ctx


class TestResolveSandboxId:
    """Test Docker-style prefix matching for sandbox IDs."""

    def test_full_uuid_skips_listing(self) -> None:
        """A full UUID is returned directly without calling list."""
        ctx = _make_client_context([])
        full_id = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
        assert ctx.resolve_sandbox_id(full_id) == full_id
        # Manager should NOT have been called
        ctx._manager.list_sandbox_infos.assert_not_called()

    def test_unique_prefix_resolves(self) -> None:
        """A unique prefix returns the full matching ID."""
        ctx = _make_client_context([
            "abc123-def456-7890-abcd-000000000001",
            "xyz789-def456-7890-abcd-000000000002",
        ])
        result = ctx.resolve_sandbox_id("abc")
        assert result == "abc123-def456-7890-abcd-000000000001"

    def test_exact_match_among_multiple(self) -> None:
        """A prefix that uniquely matches one sandbox works."""
        ctx = _make_client_context([
            "sandbox-alpha-001",
            "sandbox-beta-002",
            "sandbox-gamma-003",
        ])
        result = ctx.resolve_sandbox_id("sandbox-a")
        assert result == "sandbox-alpha-001"

    def test_ambiguous_prefix_raises(self) -> None:
        """Multiple matches raises ClickException with helpful message."""
        ctx = _make_client_context([
            "abc-111",
            "abc-222",
            "abc-333",
        ])
        with pytest.raises(click.ClickException, match="Ambiguous ID prefix"):
            ctx.resolve_sandbox_id("abc")

    def test_ambiguous_error_shows_ids(self) -> None:
        """The ambiguous error lists the conflicting IDs."""
        ctx = _make_client_context(["abc-111", "abc-222"])
        with pytest.raises(click.ClickException) as exc_info:
            ctx.resolve_sandbox_id("abc")
        assert "abc-111" in str(exc_info.value)
        assert "abc-222" in str(exc_info.value)

    def test_no_match_raises(self) -> None:
        """No matches raises ClickException."""
        ctx = _make_client_context(["xyz-001", "xyz-002"])
        with pytest.raises(click.ClickException, match="No sandbox found"):
            ctx.resolve_sandbox_id("abc")

    def test_empty_sandbox_list_raises(self) -> None:
        """Empty sandbox list raises ClickException."""
        ctx = _make_client_context([])
        with pytest.raises(click.ClickException, match="No sandbox found"):
            ctx.resolve_sandbox_id("abc")

    def test_single_char_prefix(self) -> None:
        """Even a single character can match if unique."""
        ctx = _make_client_context([
            "a-sandbox-001",
            "b-sandbox-002",
        ])
        result = ctx.resolve_sandbox_id("a")
        assert result == "a-sandbox-001"

    def test_full_id_matches_exactly(self) -> None:
        """A non-UUID full ID still matches via prefix logic."""
        ctx = _make_client_context(["my-sandbox-123"])
        result = ctx.resolve_sandbox_id("my-sandbox-123")
        assert result == "my-sandbox-123"

    def test_more_than_five_ambiguous_shows_ellipsis(self) -> None:
        """When >5 matches, the error shows '...'."""
        ids = [f"sb-{i:03d}" for i in range(10)]
        ctx = _make_client_context(ids)
        with pytest.raises(click.ClickException) as exc_info:
            ctx.resolve_sandbox_id("sb-")
        assert "..." in str(exc_info.value)
        assert "10 sandboxes" in str(exc_info.value)

    # -- Pagination tests --

    def test_match_on_second_page(self) -> None:
        """A prefix that only appears on page 2 is still found."""
        ctx = _make_client_context(
            [],
            pages=[
                ["xyz-001", "xyz-002"],
                ["abc-999"],
            ],
        )
        result = ctx.resolve_sandbox_id("abc")
        assert result == "abc-999"

    def test_collision_across_pages(self) -> None:
        """Matches on different pages are detected as ambiguous."""
        ctx = _make_client_context(
            [],
            pages=[
                ["abc-001"],
                ["abc-002"],
            ],
        )
        with pytest.raises(click.ClickException, match="Ambiguous ID prefix"):
            ctx.resolve_sandbox_id("abc")

    def test_no_match_across_all_pages(self) -> None:
        """No match after exhausting all pages raises ClickException."""
        ctx = _make_client_context(
            [],
            pages=[
                ["xyz-001"],
                ["xyz-002"],
            ],
        )
        with pytest.raises(click.ClickException, match="No sandbox found"):
            ctx.resolve_sandbox_id("abc")
