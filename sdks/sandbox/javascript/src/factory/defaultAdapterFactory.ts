// Copyright 2026 Alibaba Group Holding Ltd.
// 
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// 
//     http://www.apache.org/licenses/LICENSE-2.0
// 
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { createExecdClient } from "../openapi/execdClient.js";
import { createLifecycleClient } from "../openapi/lifecycleClient.js";

import { CommandsAdapter } from "../adapters/commandsAdapter.js";
import { FilesystemAdapter } from "../adapters/filesystemAdapter.js";
import { HealthAdapter } from "../adapters/healthAdapter.js";
import { MetricsAdapter } from "../adapters/metricsAdapter.js";
import { SandboxesAdapter } from "../adapters/sandboxesAdapter.js";

import type { AdapterFactory, CreateExecdStackOptions, CreateLifecycleStackOptions, ExecdStack, LifecycleStack } from "./adapterFactory.js";

export class DefaultAdapterFactory implements AdapterFactory {
  createLifecycleStack(opts: CreateLifecycleStackOptions): LifecycleStack {
    const lifecycleClient = createLifecycleClient({
      baseUrl: opts.lifecycleBaseUrl,
      apiKey: opts.connectionConfig.apiKey,
      headers: opts.connectionConfig.headers,
      fetch: opts.connectionConfig.fetch,
    });
    const sandboxes = new SandboxesAdapter(lifecycleClient);
    return { sandboxes };
  }

  createExecdStack(opts: CreateExecdStackOptions): ExecdStack {
    const headers: Record<string, string> = {
      ...(opts.connectionConfig.headers ?? {}),
      ...(opts.endpointHeaders ?? {}),
    };
    const execdClient = createExecdClient({
      baseUrl: opts.execdBaseUrl,
      headers,
      fetch: opts.connectionConfig.fetch,
    });

    const health = new HealthAdapter(execdClient);
    const metrics = new MetricsAdapter(execdClient);
    const files = new FilesystemAdapter(execdClient, {
      baseUrl: opts.execdBaseUrl,
      fetch: opts.connectionConfig.fetch,
      headers,
    });
    const commands = new CommandsAdapter(execdClient, {
      baseUrl: opts.execdBaseUrl,
      fetch: opts.connectionConfig.sseFetch,
      headers,
    });

    return {
      commands,
      files,
      health,
      metrics,
    };
  }
}

export function createDefaultAdapterFactory(): AdapterFactory {
  return new DefaultAdapterFactory();
}