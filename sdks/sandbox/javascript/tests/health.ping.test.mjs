import assert from "node:assert/strict";
import test from "node:test";

import { HealthAdapter } from "../dist/internal.js";

function createAdapter(status, body = "") {
  const fetchImpl = async () => new Response(body, { status });
  const client = {
    GET: (_path, _opts) =>
      fetchImpl().then((response) => ({ error: undefined, response })),
  };
  return new HealthAdapter(client);
}

test("HealthAdapter.ping returns true for 200 with no body", async () => {
  const adapter = createAdapter(200, "");
  const result = await adapter.ping();
  assert.equal(result, true);
});

test("HealthAdapter.ping passes parseAs to avoid JSON parse on empty body", async () => {
  let capturedOpts;
  const client = {
    GET: async (_path, opts) => {
      capturedOpts = opts;
      return { error: undefined, response: new Response("", { status: 200 }) };
    },
  };
  const adapter = new HealthAdapter(client);
  await adapter.ping();
  // parseAs must be set to any non-"json" value ("text" or "stream") so that
  // openapi-fetch does not attempt JSON.parse on the empty /ping response body.
  assert.ok(
    capturedOpts?.parseAs === "text" || capturedOpts?.parseAs === "stream",
    `expected parseAs to be "text" or "stream", got ${capturedOpts?.parseAs}`,
  );
});

test("HealthAdapter.ping throws on non-200 error response", async () => {
  const client = {
    GET: async (_path, _opts) => ({
      error: { code: "SOME_ERROR", message: "execd unreachable" },
      response: new Response(null, { status: 502 }),
    }),
  };
  const adapter = new HealthAdapter(client);
  await assert.rejects(() => adapter.ping(), /execd unreachable/);
});
