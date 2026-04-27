[Skip to main content](https://code.claude.com/docs/en/agent-sdk/hosting#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Deployment

Hosting the Agent SDK

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Hosting Requirements](https://code.claude.com/docs/en/agent-sdk/hosting#hosting-requirements)
- [Container-Based Sandboxing](https://code.claude.com/docs/en/agent-sdk/hosting#container-based-sandboxing)
- [System Requirements](https://code.claude.com/docs/en/agent-sdk/hosting#system-requirements)
- [Understanding the SDK Architecture](https://code.claude.com/docs/en/agent-sdk/hosting#understanding-the-sdk-architecture)
- [Sandbox Provider Options](https://code.claude.com/docs/en/agent-sdk/hosting#sandbox-provider-options)
- [Production Deployment Patterns](https://code.claude.com/docs/en/agent-sdk/hosting#production-deployment-patterns)
- [Pattern 1: Ephemeral Sessions](https://code.claude.com/docs/en/agent-sdk/hosting#pattern-1-ephemeral-sessions)
- [Pattern 2: Long-Running Sessions](https://code.claude.com/docs/en/agent-sdk/hosting#pattern-2-long-running-sessions)
- [Pattern 3: Hybrid Sessions](https://code.claude.com/docs/en/agent-sdk/hosting#pattern-3-hybrid-sessions)
- [Pattern 4: Single Containers](https://code.claude.com/docs/en/agent-sdk/hosting#pattern-4-single-containers)
- [FAQ](https://code.claude.com/docs/en/agent-sdk/hosting#faq)
- [How do I communicate with my sandboxes?](https://code.claude.com/docs/en/agent-sdk/hosting#how-do-i-communicate-with-my-sandboxes)
- [What is the cost of hosting a container?](https://code.claude.com/docs/en/agent-sdk/hosting#what-is-the-cost-of-hosting-a-container)
- [When should I shut down idle containers vs. keeping them warm?](https://code.claude.com/docs/en/agent-sdk/hosting#when-should-i-shut-down-idle-containers-vs-keeping-them-warm)
- [How often should I update the Claude Code CLI?](https://code.claude.com/docs/en/agent-sdk/hosting#how-often-should-i-update-the-claude-code-cli)
- [How do I monitor container health and agent performance?](https://code.claude.com/docs/en/agent-sdk/hosting#how-do-i-monitor-container-health-and-agent-performance)
- [How long can an agent session run before timing out?](https://code.claude.com/docs/en/agent-sdk/hosting#how-long-can-an-agent-session-run-before-timing-out)
- [Next Steps](https://code.claude.com/docs/en/agent-sdk/hosting#next-steps)

The Claude Agent SDK differs from traditional stateless LLM APIs in that it maintains conversational state and executes commands in a persistent environment. This guide covers the architecture, hosting considerations, and best practices for deploying SDK-based agents in production.

For security hardening beyond basic sandboxing (including network controls, credential management, and isolation options), see [Secure Deployment](https://code.claude.com/docs/en/agent-sdk/secure-deployment).

## [​](https://code.claude.com/docs/en/agent-sdk/hosting\#hosting-requirements)  Hosting Requirements

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#container-based-sandboxing)  Container-Based Sandboxing

For security and isolation, the SDK should run inside a sandboxed container environment. This provides process isolation, resource limits, network control, and ephemeral filesystems.The SDK also supports [programmatic sandbox configuration](https://code.claude.com/docs/en/agent-sdk/typescript#sandbox-settings) for command execution.

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#system-requirements)  System Requirements

Each SDK instance requires:

- **Runtime dependencies**  - Python 3.10+ for the Python SDK, or Node.js 18+ for the TypeScript SDK
  - Both SDK packages bundle a native Claude Code binary for the host platform, so no separate Claude Code or Node.js install is needed for the spawned CLI
- **Resource allocation**  - Recommended: 1GiB RAM, 5GiB of disk, and 1 CPU (vary this based on your task as needed)
- **Network access**  - Outbound HTTPS to `api.anthropic.com`
  - Optional: Access to MCP servers or external tools

## [​](https://code.claude.com/docs/en/agent-sdk/hosting\#understanding-the-sdk-architecture)  Understanding the SDK Architecture

Unlike stateless API calls, the Claude Agent SDK operates as a **long-running process** that:

- **Executes commands** in a persistent shell environment
- **Manages file operations** within a working directory
- **Handles tool execution** with context from previous interactions

## [​](https://code.claude.com/docs/en/agent-sdk/hosting\#sandbox-provider-options)  Sandbox Provider Options

Several providers specialize in secure container environments for AI code execution:

- **[Modal Sandbox](https://modal.com/docs/guide/sandbox)** \- [demo implementation](https://modal.com/docs/examples/claude-slack-gif-creator)
- **[Cloudflare Sandboxes](https://github.com/cloudflare/sandbox-sdk)**
- **[Daytona](https://www.daytona.io/)**
- **[E2B](https://e2b.dev/)**
- **[Fly Machines](https://fly.io/docs/machines/)**
- **[Vercel Sandbox](https://vercel.com/docs/functions/sandbox)**

For self-hosted options (Docker, gVisor, Firecracker) and detailed isolation configuration, see [Isolation Technologies](https://code.claude.com/docs/en/agent-sdk/secure-deployment#isolation-technologies).

## [​](https://code.claude.com/docs/en/agent-sdk/hosting\#production-deployment-patterns)  Production Deployment Patterns

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#pattern-1-ephemeral-sessions)  Pattern 1: Ephemeral Sessions

Create a new container for each user task, then destroy it when complete.Best for one-off tasks, the user may still interact with the AI while the task is completing, but once completed the container is destroyed.**Examples:**

- Bug Investigation & Fix: Debug and resolve a specific issue with relevant context
- Invoice Processing: Extract and structure data from receipts/invoices for accounting systems
- Translation Tasks: Translate documents or content batches between languages
- Image/Video Processing: Apply transformations, optimizations, or extract metadata from media files

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#pattern-2-long-running-sessions)  Pattern 2: Long-Running Sessions

Maintain persistent container instances for long running tasks. Often times running _multiple_ Claude Agent processes inside of the container based on demand.Best for proactive agents that take action without the users input, agents that serve content or agents that process high amounts of messages.**Examples:**

- Email Agent: Monitors incoming emails and autonomously triages, responds, or takes actions based on content
- Site Builder: Hosts custom websites per user with live editing capabilities served through container ports
- High-Frequency Chat Bots: Handles continuous message streams from platforms like Slack where rapid response times are critical

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#pattern-3-hybrid-sessions)  Pattern 3: Hybrid Sessions

Ephemeral containers that are hydrated with history and state, possibly from a database or from the SDK’s session resumption features.Best for containers with intermittent interaction from the user that kicks off work and spins down when the work is completed but can be continued.**Examples:**

- Personal Project Manager: Helps manage ongoing projects with intermittent check-ins, maintains context of tasks, decisions, and progress
- Deep Research: Conducts multi-hour research tasks, saves findings and resumes investigation when user returns
- Customer Support Agent: Handles support tickets that span multiple interactions, loads ticket history and customer context

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#pattern-4-single-containers)  Pattern 4: Single Containers

Run multiple Claude Agent SDK processes in one global container.Best for agents that must collaborate closely together. This is likely the least popular pattern because you will have to prevent agents from overwriting each other.**Examples:**

- **Simulations**: Agents that interact with each other in simulations such as video games.

## [​](https://code.claude.com/docs/en/agent-sdk/hosting\#faq)  FAQ

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#how-do-i-communicate-with-my-sandboxes)  How do I communicate with my sandboxes?

When hosting in containers, expose ports to communicate with your SDK instances. Your application can expose HTTP/WebSocket endpoints for external clients while the SDK runs internally within the container.

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#what-is-the-cost-of-hosting-a-container)  What is the cost of hosting a container?

The dominant cost of serving agents is the tokens; containers vary based on what you provision, but a minimum cost is roughly 5 cents per hour running.

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#when-should-i-shut-down-idle-containers-vs-keeping-them-warm)  When should I shut down idle containers vs. keeping them warm?

This is likely provider dependent, different sandbox providers will let you set different criteria for idle timeouts after which a sandbox might spin down.
You will want to tune this timeout based on how frequent you think user response might be.

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#how-often-should-i-update-the-claude-code-cli)  How often should I update the Claude Code CLI?

The Claude Code CLI is versioned with semver, so any breaking changes will be versioned.

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#how-do-i-monitor-container-health-and-agent-performance)  How do I monitor container health and agent performance?

Since containers are just servers the same logging infrastructure you use for the backend will work for containers.

### [​](https://code.claude.com/docs/en/agent-sdk/hosting\#how-long-can-an-agent-session-run-before-timing-out)  How long can an agent session run before timing out?

An agent session will not timeout, but consider setting a ‘maxTurns’ property to prevent Claude from getting stuck in a loop.

## [​](https://code.claude.com/docs/en/agent-sdk/hosting\#next-steps)  Next Steps

- [Secure Deployment](https://code.claude.com/docs/en/agent-sdk/secure-deployment) \- Network controls, credential management, and isolation hardening
- [TypeScript SDK - Sandbox Settings](https://code.claude.com/docs/en/agent-sdk/typescript#sandbox-settings) \- Configure sandbox programmatically
- [Sessions Guide](https://code.claude.com/docs/en/agent-sdk/sessions) \- Learn about session management
- [Permissions](https://code.claude.com/docs/en/agent-sdk/permissions) \- Configure tool permissions
- [Cost Tracking](https://code.claude.com/docs/en/agent-sdk/cost-tracking) \- Monitor API usage
- [MCP Integration](https://code.claude.com/docs/en/agent-sdk/mcp) \- Extend with custom tools

Was this page helpful?

YesNo

[Todo Lists](https://code.claude.com/docs/en/agent-sdk/todo-tracking) [Securely deploying AI agents](https://code.claude.com/docs/en/agent-sdk/secure-deployment)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.