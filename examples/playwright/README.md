# Playwright Example

Access web pages in headless mode using Playwright + Chromium in OpenSandbox to scrape title/body snippets.

## Build the Playwright Sandbox Image

The Dockerfile in this directory builds a sandbox image with Playwright and Chromium pre-installed:

```shell
cd examples/playwright
docker build -t opensandbox/playwright:latest .
```

This image includes:
- Playwright Python package
- Chromium browser binaries
- Node.js and npm (for Playwright MCP)
- Non-root user (playwright) for security

## Start OpenSandbox server [local]

Pre-pull the Playwright image:

```shell
docker pull sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/playwright:latest
```

Start the local OpenSandbox server:

```shell
git clone git@github.com:alibaba/OpenSandbox.git
cd OpenSandbox/server
cp example.config.toml ~/.sandbox.toml
uv sync
uv run python -m src.main
```

## Create and Accessing the Playwright Sandbox

```shell
# Install OpenSandbox package
uv pip install --index-url https://test.pypi.org/simple/ --extra-index-url https://pypi.org/simple opensandbox

uv run python examples/playwright/main.py
```

The script launches Chromium in headless mode to access the target URL, prints title/body snippets, and saves a full-page screenshot to `/home/playwright/screenshot.png` inside the sandbox. It also downloads the screenshot to the local working directory as `./screenshot.png`. Uses the prebuilt Playwright image by default.

![Playwright screenshot](./screenshot.png)

## References
- [Playwright](https://playwright.dev/)
