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
import type { PtySession, PtySessionStatus } from "../models/execd.js";
import type { ExecdPty } from "../services/execdPty.js";

export class PtyAdapter implements ExecdPty {
  constructor(private readonly client: ExecdClient) {}

  async createSession(opts?: { cwd?: string; command?: string }): Promise<PtySession> {
    const { data, error, response } = await this.client.POST("/pty", {
      body: { cwd: opts?.cwd, command: opts?.command },
    });
    throwOnOpenApiFetchError({ error, response }, "Create PTY session failed");
    return { sessionId: data!.session_id };
  }

  async getSession(sessionId: string): Promise<PtySessionStatus> {
    const { data, error, response } = await this.client.GET("/pty/{sessionId}", {
      params: { path: { sessionId } },
    });
    throwOnOpenApiFetchError({ error, response }, "Get PTY session failed");
    return {
      sessionId: data!.session_id,
      running: data!.running,
      outputOffset: data!.output_offset,
    };
  }

  async deleteSession(sessionId: string): Promise<void> {
    // DELETE returns 200 with an empty body; avoid JSON-parsing the empty response.
    const { error, response } = await this.client.DELETE("/pty/{sessionId}", {
      params: { path: { sessionId } },
      parseAs: "stream",
    });
    throwOnOpenApiFetchError({ error, response }, "Delete PTY session failed");
  }
}
