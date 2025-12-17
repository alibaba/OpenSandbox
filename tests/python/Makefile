.PHONY: sync sync-dev test test-sandbox test-manager test-code lint fmt

sync:
	uv sync

sync-dev:
	uv sync --group dev

test:
	uv run pytest

test-sandbox:
	uv run pytest tests/test_sandbox_e2e.py

test-manager:
	uv run pytest tests/test_sandbox_manager_e2e.py

test-code:
	uv run pytest tests/test_code_interpreter_e2e.py

lint:
	uv run ruff check tests

fmt:
	uv run ruff format tests
