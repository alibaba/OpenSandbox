## OpenSandbox Python SDK – E2E Tests (uv)

This folder is a standalone e2e test project managed by **uv**.

### Setup

```bash
cd tests/e2e/python
uv sync
```

### Run tests

```bash
uv run pytest
```

Run a specific suite:

```bash
uv run pytest tests/test_sandbox_e2e.py
```

### Notes about asyncio + shared Sandbox

These tests may reuse a single Sandbox instance across multiple test cases for speed.
To avoid `RuntimeError: Event loop is closed`, pytest-asyncio is configured to use a
**session-scoped event loop** in `pyproject.toml`.

### Handy shortcuts

```bash
make sync
make test
make test-sandbox
make lint
make fmt
```
