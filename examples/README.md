# OpenSandbox Examples

Examples for common OpenSandbox use cases. Each subdirectory contains runnable code and documentation.

## Integrations / Sandboxes
- [**echo**](echo): Minimal example using Ubuntu image to execute a simple echo command
- [**aio-sandbox**](aio-sandbox): Basic example for agent_sandbox
- [**code-interpreter**](code-interpreter): Code Interpreter SDK singleton example
- [**rl-training**](rl-training): Reinforcement learning training loop inside a sandbox
- [**claude-code**](claude-code): Call Claude (Anthropic) API/CLI within the sandbox
- [**iflow-cli**](iflow-cli): CLI invocation template for iFlow/custom HTTP LLM services
- [**gemini-cli**](gemini-cli): Call Google Gemini within the sandbox
- [**codex-cli**](codex-cli): Call OpenAI/Codex-like models within the sandbox
- [**desktop**](desktop): Launch VNC desktop (Xvfb + x11vnc) for VNC client connections
- [**playwright**](playwright): Launch headless browser (Playwright + Chromium) to scrape web content
- [**vscode**](vscode): Launch code-server (VS Code Web) to provide browser access
- [**chrome**](chrome): Launch headless Chromium with DevTools port exposed for remote debugging

## How to Run
- Set basic environment variables (e.g., `export SANDBOX_DOMAIN=...`, `export SANDBOX_API_KEY=...`)
- Add provider-specific variables as needed (e.g., `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `IFLOW_API_KEY`/`IFLOW_ENDPOINT`, etc.; model selection is optional)
- Navigate to the example directory and install dependencies: `pip install -r requirements.txt` (or refer to the Dockerfile in the directory)
- Then execute: `python main.py`
- To run in a container, build and run using the `Dockerfile` in the directory
- Summary: First set required environment variables via `export`, then run `python main.py` in the corresponding directory, or build/run the Docker image for that directory.
