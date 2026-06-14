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

import assert from "node:assert/strict";
import test from "node:test";

import { PtyAdapter, createExecdClient } from "../dist/internal.js";

test("PtyAdapter.createSession posts to /pty and maps session_id", async () => {
  const pty = new PtyAdapter(createExecdClient({
    baseUrl: "http://execd.test",
    async fetch(request) {
      assert.equal(new URL(request.url).pathname, "/pty");
      assert.equal(request.method, "POST");
      return Response.json({ session_id: "sess-123" }, { status: 201 });
    },
  }));

  const session = await pty.createSession({ cwd: "/tmp", command: "bash" });
  assert.equal(session.sessionId, "sess-123");
});

test("PtyAdapter.getSession maps running and output_offset", async () => {
  const pty = new PtyAdapter(createExecdClient({
    baseUrl: "http://execd.test",
    async fetch(request) {
      assert.equal(new URL(request.url).pathname, "/pty/sess-123");
      return Response.json(
        { session_id: "sess-123", running: true, output_offset: 4096 },
        { status: 200 },
      );
    },
  }));

  const status = await pty.getSession("sess-123");
  assert.equal(status.sessionId, "sess-123");
  assert.equal(status.running, true);
  assert.equal(status.outputOffset, 4096);
});

test("PtyAdapter.deleteSession issues a DELETE", async () => {
  let method;
  const pty = new PtyAdapter(createExecdClient({
    baseUrl: "http://execd.test",
    async fetch(request) {
      method = request.method;
      assert.equal(new URL(request.url).pathname, "/pty/sess-123");
      return new Response("", { status: 200 });
    },
  }));

  await pty.deleteSession("sess-123");
  assert.equal(method, "DELETE");
});
