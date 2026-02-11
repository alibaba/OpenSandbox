# OpenSandbox Docs Site

This directory hosts the VitePress site for OpenSandbox.

## Local development

```bash
nvm use 22
cd docs
pnpm install
pnpm docs:dev
```

## Build

```bash
nvm use 22
cd docs
pnpm install
pnpm docs:build
```

## Notes

- Site content is generated from repository README and docs markdown files.
- Run `pnpm docs:sync` to regenerate the manifest and routed pages.
- Run `pnpm docs:spec` to regenerate `docs/public/api/spec-inline.js` from `specs/sandbox-lifecycle.yml`.
