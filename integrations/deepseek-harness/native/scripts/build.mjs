// Copyright 2026 The OpenSandbox Authors
// SPDX-License-Identifier: Apache-2.0

import { execFileSync } from 'node:child_process';
import { cp, mkdir, rm } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { root, stage } from './stage.mjs';

await stage();
const check = process.argv.includes('--check');
if (!check) await rm(new URL('../dist/', import.meta.url), {recursive: true, force: true});
execFileSync(process.execPath, [createRequire(import.meta.url).resolve('typescript/bin/tsc'),
  '-p', check ? 'tsconfig.json' : 'tsconfig.build.json', ...(check ? ['--noEmit'] : [])], {cwd: root, stdio: 'inherit'});
if (!check) {
  await mkdir(new URL('../dist/remote/', import.meta.url), {recursive: true});
  await cp(new URL('../../remote/fs_helper.py', import.meta.url), new URL('../dist/remote/fs_helper.py', import.meta.url));
}
