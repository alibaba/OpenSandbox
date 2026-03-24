# Generic Release Automation

This repository uses tag-driven publish workflows. The script below standardizes:

- canonical tag creation for each release target
- release note generation from previous release to current commit
- GitHub Release create/update

Script path:

- `scripts/release/create-release.sh`

## Supported Targets

- `js/sandbox`
- `js/code-interpreter`
- `python/sandbox`
- `python/code-interpreter`
- `python/mcp/sandbox`
- `java/sandbox`
- `java/code-interpreter`
- `csharp/sandbox`
- `csharp/code-interpreter`
- `cli`
- `server`
- `docker/execd`
- `docker/code-interpreter`
- `docker/ingress`
- `docker/egress`
- `k8s/controller`
- `k8s/task-executor`
- `helm/opensandbox-controller`
- `helm/opensandbox-server`
- `helm/opensandbox`

## Tag Rules

The script aligns with existing workflow triggers:

- v-prefixed tags:
  - `<target>/v<version>` for SDK/CLI/Server targets
  - examples: `js/sandbox/v1.0.5`, `server/v0.2.0`
- plain suffix tags:
  - `<target>/<version>` for docker/k8s/helm targets
  - examples: `docker/execd/v0.3.0`, `helm/opensandbox/0.1.0`

## Release Notes Format

Generated notes follow `docs/RELEASE_NOTE_TEMPLATE.md` sections:

- `## What's New`
- `### ✨ Features`
- `### 🐛 Bug Fixes`
- `### ⚠️ Breaking Changes`
- `### 📦 Misc`
- `## 👥 Contributors`

Commit categorization:

- `feat:` -> Features
- `fix:` -> Bug Fixes
- `BREAKING CHANGE` or `type!:` -> Breaking Changes
- everything else -> Misc

## Usage

```bash
scripts/release/create-release.sh --target <target> --version <version> [options]
```

Required:

- `--target`
- `--version`

Options:

- `--from-tag <tag>`: explicit previous release boundary
- `--path <path>`: append custom path filter (repeatable)
- `--no-path-filter`: disable default target path scope and use whole range
- `--initial-release`: allow no previous tag; use full history
- `--dry-run`: render computed tag/range/notes without side effects
- `--push`: push created tag to origin

## Path Filtering Strategy

By default, each target only includes commits from target-related paths to reduce noise.

Examples:

- `js/sandbox` -> `sdks/sandbox/javascript` + `specs/sandbox-lifecycle.yml`
- `server` -> `server` + `specs/sandbox-lifecycle.yml`
- `docker/egress` -> `components/egress`
- `helm/opensandbox` -> `kubernetes/charts/opensandbox`

Override behavior:

- Add extra scope with `--path`:
  - `--path docs/` or `--path specs/execd-api.yaml`
- Disable default scope with `--no-path-filter`:
  - falls back to the entire commit range (`from..HEAD`)

## Common Examples

Dry-run JavaScript SDK release:

```bash
scripts/release/create-release.sh --target js/sandbox --version 1.0.5 --dry-run
```

Server release with tag push:

```bash
scripts/release/create-release.sh --target server --version 0.2.0 --push
```

Component image release:

```bash
scripts/release/create-release.sh --target docker/execd --version v0.3.0 --push
```

Helm chart release:

```bash
scripts/release/create-release.sh --target helm/opensandbox --version 0.1.0 --push
```

## Safety Defaults

- The script creates/updates GitHub Release only when not in `--dry-run`.
- Tag push is opt-in (`--push`), preventing accidental workflow trigger.
- If previous tag cannot be found, script fails unless `--from-tag` or `--initial-release` is provided.

## GitHub Actions Entry

You can trigger the same flow in GitHub Actions from:

- `.github/workflows/release-generic.yml`

Inputs exposed in the workflow dispatch form:

- `target`
- `version`
- `from_tag` (optional)
- `initial_release` (boolean)
- `push_tag` (boolean)
- `dry_run` (boolean, default `true`)

Recommended first run in UI:

- set `dry_run=true`
- keep `push_tag=false`
- verify the generated release notes preview in logs
- rerun with `dry_run=false` and `push_tag=true` when confirmed
