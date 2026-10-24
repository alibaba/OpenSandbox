// Copyright 2026 The OpenSandbox Authors
// SPDX-License-Identifier: Apache-2.0

import type { Context } from '@deepseek-ai/cordis';
import { ShellExecutor } from '@deepseek-ai/dsh-shell';
import { FileSystem } from '@deepseek-ai/dsh-fs';
import type { NativeRoute } from './owner.js';

/** Local composition readiness only. It neither probes nor creates a sandbox. */
export interface NativeReadiness {
  readonly preset: 'opensandbox';
  assertReady(): void;
}
declare module '@deepseek-ai/cordis' {
  interface Context { opensandboxNativeReady: NativeReadiness; }
}

/** Native host-SPI service; the owned core provider remains lazy and session-local. */
export class NativeShell extends ShellExecutor {
  readonly #route: NativeRoute;
  constructor(ctx: Context, config: { route: NativeRoute }) { super(ctx); this.#route = config.route; }

  resolve: ShellExecutor['resolve'] = request => {
    this.#route.assertActive();
    const providers = this.#route.peekProviders();
    if (!providers) throw new Error('OpenSandbox execution has not acquired its session provider.');
    return providers.shell.resolve(request);
  };
  execute: ShellExecutor['execute'] = async request => {
    const { shell } = await this.#route.acquire(request.signal);
    this.#route.assertActive();
    const execution = await shell.execute(request);
    this.#route.assertActive();
    return execution;
  };
}

/** Every target is owned by the underlying session provider, never a host mapping. */
export class NativeFileSystem extends FileSystem {
  readonly #route: NativeRoute;
  constructor(ctx: Context, config: { route: NativeRoute }) { super(ctx); this.#route = config.route; }
  #current() {
    this.#route.assertActive();
    const providers = this.#route.peekProviders();
    if (!providers) throw new Error('OpenSandbox filesystem has not acquired its session provider.');
    return providers.fs;
  }
  async #with<T>(signal: AbortSignal | undefined, operation: (fs: NonNullable<ReturnType<NativeRoute['peekProviders']>>['fs']) => Promise<T>): Promise<T> {
    const { fs } = await this.#route.acquire(signal);
    this.#route.assertActive();
    const value = await operation(fs);
    this.#route.assertActive();
    return value;
  }
  resolve: FileSystem['resolve'] = (path, options) => this.#with(options?.signal, fs => fs.resolve(path, options));
  processPath: FileSystem['processPath'] = target => this.#current().processPath(target);
  processPathFromHostPath: FileSystem['processPathFromHostPath'] = () => undefined;
  fileUrl: FileSystem['fileUrl'] = target => this.#current().fileUrl(target);
  contains: FileSystem['contains'] = (parent, child) => this.#current().contains(parent, child);
  stat: FileSystem['stat'] = (target, signal) => this.#with(signal, fs => fs.stat(target, signal));
  lstat: FileSystem['lstat'] = (path, options, signal) => this.#with(signal, fs => fs.lstat(path, options, signal));
  readText: FileSystem['readText'] = (target, signal) => this.#with(signal, fs => fs.readText(target, signal));
  streamText: FileSystem['streamText'] = (target, signal) => this.#with(signal, fs => fs.streamText(target, signal));
  readBytes: FileSystem['readBytes'] = (target, signal, maxBytes) => this.#with(signal, fs => fs.readBytes(target, signal, maxBytes));
  readByteRange: FileSystem['readByteRange'] = (target, range, signal) => this.#with(signal, fs => fs.readByteRange(target, range, signal));
  listDir: FileSystem['listDir'] = (target, signal) => this.#with(signal, fs => fs.listDir(target, signal));
  writeText: FileSystem['writeText'] = (target, text, expected, signal, policy) => this.#with(signal, fs => fs.writeText(target, text, expected, signal, policy));
  editText: FileSystem['editText'] = (target, edit, expected, signal, policy) => this.#with(signal, fs => fs.editText(target, edit, expected, signal, policy));
}

/** Native Web needs an fs service, but its host preview and desktop APIs must not read files. */
export class DeniedHostFileSystem extends FileSystem {
  #deny(): never { throw new Error('Host filesystem access is disabled in the OpenSandbox profile.'); }
  resolve: FileSystem['resolve'] = async () => this.#deny();
  processPath: FileSystem['processPath'] = () => this.#deny();
  processPathFromHostPath: FileSystem['processPathFromHostPath'] = () => undefined;
  fileUrl: FileSystem['fileUrl'] = () => this.#deny();
  contains: FileSystem['contains'] = () => this.#deny();
  stat: FileSystem['stat'] = async () => this.#deny();
  lstat: FileSystem['lstat'] = async () => this.#deny();
  readText: FileSystem['readText'] = async () => this.#deny();
  streamText: FileSystem['streamText'] = async () => this.#deny();
  readBytes: FileSystem['readBytes'] = async () => this.#deny();
  readByteRange: FileSystem['readByteRange'] = async () => this.#deny();
  listDir: FileSystem['listDir'] = async () => this.#deny();
  writeText: FileSystem['writeText'] = async () => this.#deny();
  editText: FileSystem['editText'] = async () => this.#deny();
}

/** Controller plumbing for text-only prompts, with no upload storage or host file transport. */
export function installDeniedFileUploads(ctx: Context): void {
  const deny = (): never => { throw new Error('File uploads are disabled in the OpenSandbox profile.'); };
  ctx.provide('fileUploads', {
    registerAgentResolver: () => () => {},
    upload: async () => deny(),
    uploadStream: async () => deny(),
    resolve: () => undefined,
    bindPrompt: (_agent: unknown, receiptIds: readonly unknown[]) => {
      if (receiptIds.length) deny();
      return { commit() {}, [Symbol.dispose]() {} };
    },
    retirePrompt() {},
  });
}
