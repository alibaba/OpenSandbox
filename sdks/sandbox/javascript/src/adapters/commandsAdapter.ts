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

import type { ExecdClient } from "../openapi/execdClient.js";
import { throwOnOpenApiFetchError } from "./openapiError.js";
import { parseJsonEventStream } from "./sse.js";
import type { paths as ExecdPaths } from "../api/execd.js";
import type {
  CommandExecution,
  CommandLogs,
  CommandStatus,
  RunCommandOpts,
  ServerStreamEvent,
} from "../models/execd.js";
import type { ExecdCommands } from "../services/execdCommands.js";
import type { ExecutionHandlers } from "../models/execution.js";
import { ExecutionEventDispatcher } from "../models/executionEventDispatcher.js";

function joinUrl(baseUrl: string, pathname: string): string {
  const base = baseUrl.endsWith("/") ? baseUrl.slice(0, -1) : baseUrl;
  const path = pathname.startsWith("/") ? pathname : `/${pathname}`;
  return `${base}${path}`;
}

type ApiRunCommandRequest =
  ExecdPaths["/command"]["post"]["requestBody"]["content"]["application/json"];
type ApiCommandStatusOk =
  ExecdPaths["/command/status/{id}"]["get"]["responses"][200]["content"]["application/json"];
type ApiCommandLogsOk =
  ExecdPaths["/command/{id}/logs"]["get"]["responses"][200]["content"]["text/plain"];

function toRunCommandRequest(command: string, opts?: RunCommandOpts): ApiRunCommandRequest {
  const body: ApiRunCommandRequest = {
    command,
    cwd: opts?.workingDirectory,
    background: !!opts?.background,
  };
  if (opts?.timeoutSeconds != null) {
    body.timeout = Math.round(opts.timeoutSeconds * 1000);
  }
  return body;
}

function parseOptionalDate(value: unknown, field: string): Date | undefined {
  if (value == null) return undefined;
  if (value instanceof Date) return value;
  if (typeof value !== "string") {
    throw new Error(`Invalid ${field}: expected ISO string, got ${typeof value}`);
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    throw new Error(`Invalid ${field}: ${value}`);
  }
  return parsed;
}

export interface CommandsAdapterOptions {
  /**
   * Must match the baseUrl used by the ExecdClient.
   */
  baseUrl: string;
  fetch?: typeof fetch;
  headers?: Record<string, string>;
}

export class CommandsAdapter implements ExecdCommands {
  private readonly fetch: typeof fetch;

  constructor(
    private readonly client: ExecdClient,
    private readonly opts: CommandsAdapterOptions,
  ) {
    this.fetch = opts.fetch ?? fetch;
  }

  async interrupt(sessionId: string): Promise<void> {
    const { error, response } = await this.client.DELETE("/command", {
      params: { query: { id: sessionId } },
    });
    throwOnOpenApiFetchError({ error, response }, "Interrupt command failed");
  }

  async getCommandStatus(commandId: string): Promise<CommandStatus> {
    const { data, error, response } = await this.client.GET("/command/status/{id}", {
      params: { path: { id: commandId } },
    });
    throwOnOpenApiFetchError({ error, response }, "Get command status failed");
    const ok = data as ApiCommandStatusOk | undefined;
    if (!ok || typeof ok !== "object") {
      throw new Error("Get command status failed: unexpected response shape");
    }
    return {
      id: ok.id,
      content: ok.content,
      running: ok.running,
      exitCode: ok.exit_code ?? null,
      error: ok.error,
      startedAt: parseOptionalDate(ok.started_at, "startedAt"),
      finishedAt: parseOptionalDate(ok.finished_at, "finishedAt") ?? null,
    };
  }

  async getBackgroundCommandLogs(commandId: string, cursor?: number): Promise<CommandLogs> {
    const { data, error, response } = await this.client.GET("/command/{id}/logs", {
      params: { path: { id: commandId }, query: cursor == null ? {} : { cursor } },
      parseAs: "text",
    });
    throwOnOpenApiFetchError({ error, response }, "Get command logs failed");
    const ok = data as ApiCommandLogsOk | undefined;
    if (typeof ok !== "string") {
      throw new Error("Get command logs failed: unexpected response shape");
    }
    const cursorHeader = response.headers.get("EXECD-COMMANDS-TAIL-CURSOR");
    const parsedCursor = (cursorHeader != null && cursorHeader !== "") ? Number(cursorHeader) : undefined;
    return {
      content: ok,
      cursor: Number.isFinite(parsedCursor ?? NaN) ? parsedCursor : undefined,
    };
  }

  async *runStream(
    command: string,
    opts?: RunCommandOpts,
    signal?: AbortSignal,
  ): AsyncIterable<ServerStreamEvent> {
    const url = joinUrl(this.opts.baseUrl, "/command");
    const body = JSON.stringify(toRunCommandRequest(command, opts));

    const res = await this.fetch(url, {
      method: "POST",
      headers: {
        "accept": "text/event-stream",
        "content-type": "application/json",
        ...(this.opts.headers ?? {}),
      },
      body,
      signal,
    });

    for await (const ev of parseJsonEventStream<ServerStreamEvent>(res, { fallbackErrorMessage: "Run command failed" })) {
      yield ev;
    }
  }

  async run(
    command: string,
    opts?: RunCommandOpts,
    handlers?: ExecutionHandlers,
    signal?: AbortSignal,
  ): Promise<CommandExecution> {
    const execution: CommandExecution = {
      logs: { stdout: [], stderr: [] },
      result: [],
    };
    const dispatcher = new ExecutionEventDispatcher(execution, handlers);
    for await (const ev of this.runStream(command, opts, signal)) {
      // Keep legacy behavior: if server sends "init" with empty id, preserve previous id.
      if (ev.type === "init" && (ev.text ?? "") === "" && execution.id) {
        (ev as any).text = execution.id;
      }
      await dispatcher.dispatch(ev as any);
    }

    return execution;
  }
}