# OpenSandbox AGENTS

Use this file as the root router for the monorepo. Prefer the nearest `AGENTS.md` in the directory tree for task-specific instructions.

## Repository Map

- `server/`: lifecycle control plane and server tests
- `components/execd/`: in-sandbox execution daemon
- `sdks/`: language SDKs and generated clients
- `specs/`: public API contracts
- `components/`, `cli/`, `docs/`, `kubernetes/`, `sandboxes/`: runtime, tooling, and deployment surfaces

## Routing

- For `server/**`, or lifecycle server behavior, sandbox creation flow, or user-visible server config, read `server/AGENTS.md`.
- For `sdks/**`, or SDK generation, handwritten adapters, or cross-language SDK alignment, read `sdks/AGENTS.md`.
- For `specs/**`, or API contract, schema, or example changes, read `specs/AGENTS.md`.
- For cross-cutting changes spanning spec, server, and SDKs, start with `specs/AGENTS.md` and then read affected consumer guides.
- For areas without a local `AGENTS.md`, use the nearest `README.md`, `DEVELOPMENT.md`, and CI workflow as the next source of truth.

## Guardrails

Always:

- Keep changes focused on the user request.
- Treat `specs/*` as public contract sources.
- Keep spec, implementation, SDKs, docs, examples, config, and CLI behavior aligned when user-visible behavior changes.
- When changing `specs/*`, also update or verify affected server, SDK, docs, and release outputs when practical.
- Prefer additive, backward-compatible changes for public interfaces.
- Regenerate derived outputs when the source-of-truth file changes.
- Update tests when behavior changes or bugs are fixed.
- Mention unrun or blocked verification in the final handoff.
- Prefer file-scoped or package-scoped checks before full-suite validation.

Ask first:

- Breaking public API, SDK, config, protocol, or CLI changes
- Intentional drift between a public contract and its implementation
- User-visible config or behavior changes without a clear migration story

Never:

- Edit generated output as the only fix.
- Mix unrelated component work into the same change.

## Review Focus

- Prioritize breaking changes in specs, SDK interfaces, config, CLI behavior, and protocols.
- Flag protocol changes that are unnecessary, inconsistent, or hard to implement.
- Flag changes that break source-of-truth boundaries or intended layering.
- Call out missing tests and compatibility risks explicitly.
