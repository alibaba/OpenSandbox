# OpenSandbox Examples

Examples for common OpenSandbox use cases. Each subdirectory contains runnable code and documentation.

## Integrations / Sandboxes
- 🧰 [**aio-sandbox**](aio-sandbox): All-in-one sandbox setup using OpenSandbox SDK and agent-sandbox
- <img src="https://kubernetes.io/icons/favicon-32.png" alt="Kubernetes" height="16" /> [**agent-sandbox**](agent-sandbox): Create a kubernetes-sigs/agent-sandbox instance and run a command
- 🧪 [**code-interpreter**](code-interpreter): Code Interpreter SDK singleton example
- 💾 [**host-volume-mount**](host-volume-mount): Mount host directories into sandboxes (read-write, read-only, subpath)
- 🎯 [**rl-training**](rl-training): Reinforcement learning training loop inside a sandbox
- <img src="https://img.shields.io/badge/-%20-D97757?logo=claude&logoColor=white&style=flat-square" alt="Claude" height="16" /> [**claude-code**](claude-code): Call Claude (Anthropic) API/CLI within the sandbox
- <img src="https://cli.iflow.cn/img/favicon.ico" alt="iFlow" width="16" /> [**iflow-cli**](iflow-cli): CLI invocation template for iFlow/custom HTTP LLM services
- <img src="https://geminicli.com/favicon.ico" alt="Google Gemini" height="16" /> [**gemini-cli**](gemini-cli): Call Google Gemini within the sandbox
- <img src="https://developers.openai.com/favicon.png" alt="OpenAI" height="16" /> [**codex-cli**](codex-cli): Call OpenAI/Codex-like models within the sandbox
- <img src="https://img.shields.io/badge/-%20-1C3C3C?logo=langgraph&logoColor=white&style=flat-square" alt="LangGraph" height="16" /> [**langgraph**](langgraph): LangGraph agent orchestrating sandbox lifecycle + tools
- <img src="https://google.github.io/adk-docs/assets/agent-development-kit.png" alt="Google ADK" height="16" /> [**google-adk**](google-adk): Google ADK agent calling OpenSandbox tools
- 🦞 [**openclaw**](openclaw): Run an OpenClaw Gateway inside a sandbox
- 🖥️ [**desktop**](desktop): Launch VNC desktop (Xvfb + x11vnc) for VNC client connections
- <img src="https://playwright.dev/img/playwright-logo.svg" alt="Playwright" width="16" /> [**playwright**](playwright): Launch headless browser (Playwright + Chromium) to scrape web content
- <img src="https://code.visualstudio.com/assets/favicon.ico" alt="VS Code" height="16" /> [**vscode**](vscode): Launch code-server (VS Code Web) to provide browser access
- <img src="https://www.google.com/chrome/static/images/chrome-logo.svg" alt="Google Chrome" height="16" /> [**chrome**](chrome): Launch headless Chromium with DevTools port exposed for remote debugging

## How to Run
- Set basic environment variables (e.g., `export SANDBOX_DOMAIN=...`, `export SANDBOX_API_KEY=...`)
- Add provider-specific variables as needed (e.g., `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `IFLOW_API_KEY`/`IFLOW_ENDPOINT`, etc.; model selection is optional)
- Navigate to the example directory and install dependencies: `pip install -r requirements.txt` (or refer to the Dockerfile in the directory)
- Then execute: `python main.py`
- To run in a container, build and run using the `Dockerfile` in the directory
- Summary: First set required environment variables via `export`, then run `python main.py` in the corresponding directory, or build/run the Docker image for that directory.
