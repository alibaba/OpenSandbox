---
name: file-operations
description: Use OpenSandbox file commands to read, write, upload, download, search, replace, move, delete, and inspect files or directories inside a sandbox. Trigger when users want exact sandbox file manipulation commands instead of generic shell guidance.
---

# OpenSandbox File Operations

Manipulate sandbox files with `osb file` commands. Choose the operation mode first, then use the matching verification step. Do not mix sandbox-internal edits with host-to-sandbox transfer commands casually.

## When To Use

- the user wants to read or write a file inside a sandbox
- the user needs to upload a local file into a sandbox or download a sandbox file back to the host
- the user wants to search, replace, move, chmod, or inspect paths
- the user needs directory creation or cleanup

## Operation Modes

Treat these as distinct categories:

- sandbox-only content operations
  `cat`, `write`, `replace`, `mv`, `mkdir`, `rm`, `rmdir`, `info`, `chmod`
- host-to-sandbox transfer
  `upload`
- sandbox-to-host transfer
  `download`
- discovery before modification
  `search`, then `info`

If the path is uncertain, search first. If the file boundary crosses between host and sandbox, use `upload` or `download` instead of `write` or `cat`.

## Golden Paths

Write and verify inside the sandbox:

```bash
osb file write <sandbox-id> /workspace/app.txt -c "hello"
osb file cat <sandbox-id> /workspace/app.txt
```

Upload from host and verify in the sandbox:

```bash
osb file upload <sandbox-id> ./local.txt /workspace/local.txt
osb file cat <sandbox-id> /workspace/local.txt
```

Search before editing:

```bash
osb file search <sandbox-id> /workspace --pattern "*.py"
osb file info <sandbox-id> /workspace/main.py
```

## Sandbox-Only File Edits

Read and write:

```bash
osb file cat <sandbox-id> /path/to/file
osb file write <sandbox-id> /path/to/file -c "hello"
```

Use `write` with `-c/--content` when the new content is known directly. If the content should come from stdin, omit `-c` and pipe or paste the content into the command.

Edit existing content:

```bash
osb file replace <sandbox-id> /path/to/file --old old --new new
osb file mv <sandbox-id> /old/path /new/path
```

Prefer `replace` for small text substitutions and `mv` for rename/path changes. Do not rewrite a full file when a targeted replace is enough.

Create directories:

```bash
osb file mkdir <sandbox-id> /workspace/output
osb file mkdir <sandbox-id> /workspace/a /workspace/b --mode 755
```

## Host <-> Sandbox Transfer

Host to sandbox:

```bash
osb file upload <sandbox-id> ./local.txt /remote/path/local.txt
```

Sandbox to host:

```bash
osb file download <sandbox-id> /remote/path/result.json ./result.json
```

Rules:

- use `upload` when the source file is on the host
- use `download` when the destination should be written to the host filesystem
- use `write` and `cat` only when the operation stays entirely inside the sandbox

## Metadata and Permissions

Inspect metadata:

```bash
osb file info <sandbox-id> /path/to/file
osb file info <sandbox-id> /path/one /path/two
```

Search by pattern:

```bash
osb file search <sandbox-id> /workspace --pattern "*.py"
```

Set permissions:

```bash
osb file chmod <sandbox-id> /path/to/script --mode 0755
osb file chmod <sandbox-id> /path/to/file --mode 0644 --owner root --group root
```

Use `info` after `chmod` when the user needs to confirm mode, ownership, or timestamps changed as expected.

## Destructive Operations

Delete files or directories only after verifying the target path:

```bash
osb file info <sandbox-id> /workspace/tmp.txt
osb file rm <sandbox-id> /workspace/tmp.txt
```

```bash
osb file search <sandbox-id> /workspace --pattern "old-*"
osb file rmdir <sandbox-id> /workspace/old-dir
```

Rules:

- prefer `info` when the exact path is known
- prefer `search` when the path is uncertain
- do not suggest `rm` or `rmdir` until the target has been verified
- after `mv`, `rm`, or `rmdir`, verify the new state with `info` or `search`

## Failure Semantics

- `upload` and `download` have host filesystem side effects; treat them as cross-boundary operations
- `download` writes to the local path immediately, so be explicit about the destination
- permission or ownership failures are usually path/runtime permission issues, not a reason to switch away from `osb file`
- if multiple file commands fail unexpectedly, check sandbox health before assuming a file-command bug

## Response Pattern

Structure the answer as:

1. exact `osb file` command
2. which operation mode it uses
3. the next verification command if the workflow continues

Keep command examples concrete and ready to paste.

## Minimal Closed Loops

Write and verify:

```bash
osb file write <sandbox-id> /workspace/app.txt -c "hello"
osb file cat <sandbox-id> /workspace/app.txt
```

Upload and verify:

```bash
osb file upload <sandbox-id> ./local.txt /workspace/local.txt
osb file cat <sandbox-id> /workspace/local.txt
```

Replace and verify:

```bash
osb file replace <sandbox-id> /workspace/app.txt --old hello --new world
osb file cat <sandbox-id> /workspace/app.txt
```

Change permissions and inspect:

```bash
osb file chmod <sandbox-id> /workspace/script.sh --mode 0755
osb file info <sandbox-id> /workspace/script.sh
```

Create a directory and inspect it:

```bash
osb file mkdir <sandbox-id> /workspace/output
osb file info <sandbox-id> /workspace/output
```

Move a file and verify the new path:

```bash
osb file mv <sandbox-id> /workspace/app.txt /workspace/archive/app.txt
osb file info <sandbox-id> /workspace/archive/app.txt
```

Delete and verify removal:

```bash
osb file info <sandbox-id> /workspace/tmp.txt
osb file rm <sandbox-id> /workspace/tmp.txt
osb file search <sandbox-id> /workspace --pattern "tmp.txt"
```

## Best Practices

- prefer `search` before modification when the path is not certain
- prefer `info` before destructive actions
- prefer `replace` over full rewrites for small text changes
- prefer `upload` and `download` only for host boundary crossings
- prefer explicit verification after `mv`, `chmod`, `rm`, and `rmdir`
