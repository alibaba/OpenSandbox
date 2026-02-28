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

import {
  DEFAULT_ENTRYPOINT,
  DEFAULT_EXECD_PORT,
  DEFAULT_HEALTH_CHECK_POLLING_INTERVAL_MILLIS,
  DEFAULT_READY_TIMEOUT_SECONDS,
  DEFAULT_RESOURCE_LIMITS,
  DEFAULT_TIMEOUT_SECONDS,
} from "./core/constants.js";
import { ConnectionConfig, type ConnectionConfigOptions } from "./config/connection.js";
import type { SandboxFiles } from "./services/filesystem.js";
import { createDefaultAdapterFactory } from "./factory/defaultAdapterFactory.js";
import type { AdapterFactory } from "./factory/adapterFactory.js";

import type { Sandboxes } from "./services/sandboxes.js";
import type { ExecdCommands } from "./services/execdCommands.js";
import type { ExecdHealth } from "./services/execdHealth.js";
import type { ExecdMetrics } from "./services/execdMetrics.js";
import type {
  CreateSandboxRequest,
  Endpoint,
  NetworkPolicy,
  RenewSandboxExpirationResponse,
  SandboxId,
  SandboxInfo,
  Volume,
} from "./models/sandboxes.js";
import { SandboxReadyTimeoutException } from "./core/exceptions.js";

export interface SandboxCreateOptions {
  /**
   * Connection configuration for calling the OpenSandbox Lifecycle API and the sandbox's execd API.
   */
  connectionConfig?: ConnectionConfig | ConnectionConfigOptions;
  /**
   * Advanced override: inject a custom adapter factory (custom transports, dependency injection).
   */
  adapterFactory?: AdapterFactory;

  /**
   * Container image uri, e.g. `python:3.11`
   */
  image:
    | string
    | { uri: string; auth?: { username: string; password: string } };

  /**
   * Entrypoint command for the sandbox (defaults to tail -f /dev/null).
   */
  entrypoint?: string[];
  /**
   * Environment variables to inject into the sandbox runtime.
   */
  env?: Record<string, string>;
  /**
   * Custom metadata tags (used for filtering/management).
   */
  metadata?: Record<string, string>;
  /**
   * Optional outbound network policy for the sandbox.
   * If provided without defaultAction, defaults to "deny".
   */
  networkPolicy?: NetworkPolicy;
  /**
   * Optional list of volume mounts for persistent storage.
   * Each volume specifies a backend (host path or PVC) and mount configuration.
   */
  volumes?: Volume[];
  /**
   * Opaque extension parameters passed through to the server as-is.
   */
  extensions?: Record<string, string>;

  /**
   * Resource limits applied to the sandbox container.
   *
   * This is forwarded to the Lifecycle API as `resourceLimits`.
   */
  resource?: Record<string, string>;
  /**
   * Sandbox timeout in seconds.
   */
  timeoutSeconds?: number;

  /**
   * Skip readiness checks during create/connect.
   *
   * When true, the SDK will not wait for lifecycle state `Running` or perform the health check.
   * The returned sandbox instance may not be ready yet.
   */
  skipHealthCheck?: boolean;
  /**
   * Optional custom readiness check used by {@link Sandbox.waitUntilReady}.
   *
   * If provided, the SDK will call this function during readiness checks instead of
   * using the default `execd` ping check.
   */
  healthCheck?: (sbx: Sandbox) => boolean | Promise<boolean>;
  readyTimeoutSeconds?: number;
  healthCheckPollingInterval?: number;
}

export interface SandboxConnectOptions {
  /**
   * Connection configuration for calling the OpenSandbox APIs.
   */
  connectionConfig?: ConnectionConfig | ConnectionConfigOptions;
  /**
   * Advanced override: inject a custom adapter factory (custom transports, dependency injection).
   */
  adapterFactory?: AdapterFactory;
  /**
   * ID of the existing sandbox to connect to.
   */
  sandboxId: SandboxId;

  /**
   * Skip readiness checks after connecting.
   */
  skipHealthCheck?: boolean;
  /**
   * Optional custom readiness check used by {@link Sandbox.waitUntilReady}.
   */
  healthCheck?: (sbx: Sandbox) => boolean | Promise<boolean>;
  /**
   * Max time to wait for readiness.
   */
  readyTimeoutSeconds?: number;
  /**
   * Polling interval for readiness checks (milliseconds).
   */
  healthCheckPollingInterval?: number;
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

function toImageSpec(
  image: SandboxCreateOptions["image"]
): CreateSandboxRequest["image"] {
  if (typeof image === "string") return { uri: image };
  return { uri: image.uri, auth: image.auth };
}

export class Sandbox {
  readonly id: SandboxId;
  readonly connectionConfig: ConnectionConfig;

  /**
   * Lifecycle (sandbox management) service.
   */
  readonly sandboxes: Sandboxes;

  /**
   * Execd services.
   */
  readonly commands: ExecdCommands;
  /**
   * High-level filesystem facade (JS-friendly).
   */
  readonly files: SandboxFiles;
  readonly health: ExecdHealth;
  readonly metrics: ExecdMetrics;

  /**
   * Internal state kept out of the public instance shape.
   *
   * This avoids nominal typing issues when multiple copies of the SDK exist in a dependency graph.
   */
  private static readonly _priv = new WeakMap<
    Sandbox,
    {
      adapterFactory: AdapterFactory;
      lifecycleBaseUrl: string;
      execdBaseUrl: string;
    }
  >();

  private constructor(opts: {
    id: SandboxId;
    connectionConfig: ConnectionConfig;
    adapterFactory: AdapterFactory;
    lifecycleBaseUrl: string;
    execdBaseUrl: string;
    sandboxes: Sandboxes;
    commands: ExecdCommands;
    files: SandboxFiles;
    health: ExecdHealth;
    metrics: ExecdMetrics;
  }) {
    this.id = opts.id;
    this.connectionConfig = opts.connectionConfig;
    Sandbox._priv.set(this, {
      adapterFactory: opts.adapterFactory,
      lifecycleBaseUrl: opts.lifecycleBaseUrl,
      execdBaseUrl: opts.execdBaseUrl,
    });

    this.sandboxes = opts.sandboxes;
    this.commands = opts.commands;
    this.files = opts.files;
    this.health = opts.health;
    this.metrics = opts.metrics;
  }

  static async create(opts: SandboxCreateOptions): Promise<Sandbox> {
    const baseConnectionConfig =
      opts.connectionConfig instanceof ConnectionConfig
        ? opts.connectionConfig
        : new ConnectionConfig(opts.connectionConfig);
    const connectionConfig = baseConnectionConfig.withTransportIfMissing();
    const lifecycleBaseUrl = connectionConfig.getBaseUrl();
    const adapterFactory = opts.adapterFactory ?? createDefaultAdapterFactory();

    let sandboxes: Sandboxes;
    try {
      sandboxes = adapterFactory.createLifecycleStack({
        connectionConfig,
        lifecycleBaseUrl,
      }).sandboxes;
    } catch (err) {
      await connectionConfig.closeTransport();
      throw err;
    }

    // Validate volumes: exactly one backend must be specified per volume
    if (opts.volumes) {
      for (const vol of opts.volumes) {
        const backendsSpecified = [vol.host, vol.pvc].filter((b) => b !== undefined).length;
        if (backendsSpecified === 0) {
          throw new Error(
            `Volume '${vol.name}' must specify exactly one backend (host, pvc), but none was provided.`
          );
        }
        if (backendsSpecified > 1) {
          throw new Error(
            `Volume '${vol.name}' must specify exactly one backend (host, pvc), but multiple were provided.`
          );
        }
      }
    }

    const req: CreateSandboxRequest = {
      image: toImageSpec(opts.image),
      entrypoint: opts.entrypoint ?? DEFAULT_ENTRYPOINT,
      timeout: Math.floor(opts.timeoutSeconds ?? DEFAULT_TIMEOUT_SECONDS),
      resourceLimits: opts.resource ?? DEFAULT_RESOURCE_LIMITS,
      env: opts.env ?? {},
      metadata: opts.metadata ?? {},
      networkPolicy: opts.networkPolicy
        ? {
            ...opts.networkPolicy,
            defaultAction: opts.networkPolicy.defaultAction ?? "deny",
          }
        : undefined,
      volumes: opts.volumes,
      extensions: opts.extensions ?? {},
    };

    let sandboxId: SandboxId | undefined;
    try {
      const created = await sandboxes.createSandbox(req);
      sandboxId = created.id as SandboxId;

      const endpoint = await sandboxes.getSandboxEndpoint(
        sandboxId,
        DEFAULT_EXECD_PORT,
        connectionConfig.useServerProxy
      );
      const execdBaseUrl = `${connectionConfig.protocol}://${endpoint.endpoint}`;

      const { commands, files, health, metrics } =
        adapterFactory.createExecdStack({
          connectionConfig,
          execdBaseUrl,
          endpointHeaders: endpoint.headers,
        });

      const sbx = new Sandbox({
        id: sandboxId,
        connectionConfig,
        adapterFactory,
        lifecycleBaseUrl,
        execdBaseUrl,
        sandboxes,
        commands,
        files,
        health,
        metrics,
      });

      if (!(opts.skipHealthCheck ?? false)) {
        await sbx.waitUntilReady({
          readyTimeoutSeconds:
            opts.readyTimeoutSeconds ?? DEFAULT_READY_TIMEOUT_SECONDS,
          pollingIntervalMillis:
            opts.healthCheckPollingInterval ??
            DEFAULT_HEALTH_CHECK_POLLING_INTERVAL_MILLIS,
          healthCheck: opts.healthCheck,
        });
      }

      return sbx;
    } catch (err) {
      if (sandboxId) {
        try {
          await sandboxes.deleteSandbox(sandboxId);
        } catch {
          // Ignore cleanup failure; surface original error.
        }
      }
      await connectionConfig.closeTransport();
      throw err;
    }
  }

  static async connect(opts: SandboxConnectOptions): Promise<Sandbox> {
    const baseConnectionConfig =
      opts.connectionConfig instanceof ConnectionConfig
        ? opts.connectionConfig
        : new ConnectionConfig(opts.connectionConfig);
    const connectionConfig = baseConnectionConfig.withTransportIfMissing();
    const adapterFactory = opts.adapterFactory ?? createDefaultAdapterFactory();
    const lifecycleBaseUrl = connectionConfig.getBaseUrl();

    let sandboxes: Sandboxes;
    try {
      sandboxes = adapterFactory.createLifecycleStack({
        connectionConfig,
        lifecycleBaseUrl,
      }).sandboxes;
    } catch (err) {
      await connectionConfig.closeTransport();
      throw err;
    }

    try {
      const endpoint = await sandboxes.getSandboxEndpoint(
        opts.sandboxId,
        DEFAULT_EXECD_PORT,
        connectionConfig.useServerProxy
      );
      const execdBaseUrl = `${connectionConfig.protocol}://${endpoint.endpoint}`;
      const { commands, files, health, metrics } =
        adapterFactory.createExecdStack({
          connectionConfig,
          execdBaseUrl,
          endpointHeaders: endpoint.headers,
        });

      const sbx = new Sandbox({
        id: opts.sandboxId,
        connectionConfig,
        adapterFactory,
        lifecycleBaseUrl,
        execdBaseUrl,
        sandboxes,
        commands,
        files,
        health,
        metrics,
      });

      if (!(opts.skipHealthCheck ?? false)) {
        await sbx.waitUntilReady({
          readyTimeoutSeconds:
            opts.readyTimeoutSeconds ?? DEFAULT_READY_TIMEOUT_SECONDS,
          pollingIntervalMillis:
            opts.healthCheckPollingInterval ??
            DEFAULT_HEALTH_CHECK_POLLING_INTERVAL_MILLIS,
          healthCheck: opts.healthCheck,
        });
      }

      return sbx;
    } catch (err) {
      await connectionConfig.closeTransport();
      throw err;
    }
  }

  async getInfo(): Promise<SandboxInfo> {
    return await this.sandboxes.getSandbox(this.id);
  }

  async isHealthy(): Promise<boolean> {
    try {
      return await this.health.ping();
    } catch {
      return false;
    }
  }

  async getMetrics() {
    return await this.metrics.getMetrics();
  }

  async pause(): Promise<void> {
    await this.sandboxes.pauseSandbox(this.id);
  }

  /**
   * Resume a paused sandbox and return a fresh, connected Sandbox instance.
   *
   * After resume, the execd endpoint may change, so this method returns a new
   * {@link Sandbox} instance with a refreshed execd base URL.
   */
  async resume(
    opts: {
      skipHealthCheck?: boolean;
      readyTimeoutSeconds?: number;
      healthCheckPollingInterval?: number;
    } = {}
  ): Promise<Sandbox> {
    await this.sandboxes.resumeSandbox(this.id);
    return await Sandbox.connect({
      sandboxId: this.id,
      connectionConfig: this.connectionConfig,
      adapterFactory: Sandbox._priv.get(this)!.adapterFactory,
      skipHealthCheck: opts.skipHealthCheck ?? false,
      readyTimeoutSeconds: opts.readyTimeoutSeconds,
      healthCheckPollingInterval: opts.healthCheckPollingInterval,
    });
  }

  /**
   * Resume a paused sandbox by id, then connect to its execd endpoint.
   */
  static async resume(opts: SandboxConnectOptions): Promise<Sandbox> {
    const baseConnectionConfig =
      opts.connectionConfig instanceof ConnectionConfig
        ? opts.connectionConfig
        : new ConnectionConfig(opts.connectionConfig);
    const adapterFactory = opts.adapterFactory ?? createDefaultAdapterFactory();
    const resumeConnectionConfig = baseConnectionConfig.withTransportIfMissing();
    const lifecycleBaseUrl = resumeConnectionConfig.getBaseUrl();

    let sandboxes: Sandboxes;
    try {
      sandboxes = adapterFactory.createLifecycleStack({
        connectionConfig: resumeConnectionConfig,
        lifecycleBaseUrl,
      }).sandboxes;
      await sandboxes.resumeSandbox(opts.sandboxId);
    } catch (err) {
      await resumeConnectionConfig.closeTransport();
      throw err;
    }

    await resumeConnectionConfig.closeTransport();
    return await Sandbox.connect({ ...opts, connectionConfig: baseConnectionConfig, adapterFactory });
  }

  async kill(): Promise<void> {
    await this.sandboxes.deleteSandbox(this.id);
  }

  /**
   * Release any client-side resources (e.g. Node.js HTTP agents) owned by this Sandbox instance.
   */
  async close(): Promise<void> {
    await this.connectionConfig.closeTransport();
  }

  /**
   * Renew expiration by setting expiresAt to now + timeoutSeconds.
   */
  async renew(timeoutSeconds: number): Promise<RenewSandboxExpirationResponse> {
    const expiresAt = new Date(
      Date.now() + timeoutSeconds * 1000
    ).toISOString();
    return await this.sandboxes.renewSandboxExpiration(this.id, { expiresAt });
  }

  /**
   * Get sandbox endpoint for a port (STRICT: no scheme), e.g. "localhost:44772" or "domain/route/.../44772".
   */
  async getEndpoint(port: number): Promise<Endpoint> {
    return await this.sandboxes.getSandboxEndpoint(
      this.id,
      port,
      this.connectionConfig.useServerProxy
    );
  }

  /**
   * Get absolute endpoint URL with scheme (convenience for HTTP clients).
   */
  async getEndpointUrl(port: number): Promise<string> {
    const ep = await this.getEndpoint(port);
    return `${this.connectionConfig.protocol}://${ep.endpoint}`;
  }

  async waitUntilReady(opts: {
    readyTimeoutSeconds: number;
    pollingIntervalMillis: number;
    healthCheck?: (sbx: Sandbox) => boolean | Promise<boolean>;
  }): Promise<void> {
    const deadline = Date.now() + opts.readyTimeoutSeconds * 1000;

    // Wait until execd becomes reachable and passes health check.
    while (true) {
      if (Date.now() > deadline) {
        throw new SandboxReadyTimeoutException({
          message: `Sandbox not ready: timed out waiting for health check (timeoutSeconds=${opts.readyTimeoutSeconds})`,
        });
      }
      try {
        if (opts.healthCheck) {
          const ok = await opts.healthCheck(this);
          if (ok) return;
        } else {
          const ok = await this.health.ping();
          if (ok) return;
        }
      } catch {
        // ignore and retry
      }
      await sleep(opts.pollingIntervalMillis);
    }
  }
}