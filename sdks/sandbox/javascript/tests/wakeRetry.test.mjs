// Copyright 2026 The OpenSandbox Authors
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

import {
  ConnectionConfig,
  SandboxApiException,
  SandboxWakeTimeoutException,
  SandboxError,
} from "../dist/index.js";
import {
  createWakeRetryFetch,
  parseRetryAfterMs,
  resolveWakeRetryAfterMs,
  isWakeTimeoutResponse,
  WAKE_MARKER_HEADER,
  WAKE_MARKER_VALUE,
  WAKE_DEFAULT_RETRY_AFTER_MS,
  RETRY_AFTER_CAP_MS,
} from "../dist/internal.js";

const POLICY = { enabled: true, maxAttempts: 5, overallDeadlineSeconds: 60 };

function markedResponse({
  status = 503,
  marker = WAKE_MARKER_VALUE,
  retryAfter = "1",
  body = { error: { code: "sandbox_wake_timeout", message: "restore in progress" } },
} = {}) {
  const headers = new Headers();
  if (marker != null) headers.set(WAKE_MARKER_HEADER, marker);
  if (retryAfter != null) headers.set("Retry-After", retryAfter);
  return new Response(body == null ? null : JSON.stringify(body), { status, headers });
}

function stubFetch(responses, { onRequest } = {}) {
  const calls = [];
  const fetch = async (input, init) => {
    calls.push({
      input,
      method: init?.method ?? "GET",
      body: init?.body,
    });
    onRequest?.(calls.length);
    const next = responses.shift();
    if (next instanceof Error) throw next;
    if (typeof next === "function") return next();
    return next;
  };
  return { fetch, calls };
}

test("wake retry: POST with marked 503 is retried and succeeds", async () => {
  const { fetch, calls } = stubFetch([
    markedResponse(),
    new Response(JSON.stringify({ ok: true }), { status: 200 }),
  ]);
  const wrapped = createWakeRetryFetch({ baseFetch: fetch, wakeRetry: POLICY });

  const res = await wrapped("http://example/v1/sandboxes", {
    method: "POST",
    body: JSON.stringify({ image: { uri: "python:3.12" } }),
    headers: { "content-type": "application/json" },
  });

  assert.equal(res.status, 200);
  assert.equal(calls.length, 2);
  assert.equal(calls[1].method, "POST");
  assert.equal(calls[1].body, calls[0].body);
});

test("wake retry: exhaustion throws SandboxWakeTimeoutException with attempts and waitedMs", async () => {
  const responses = Array.from({ length: 5 }, () => markedResponse());
  const { fetch, calls } = stubFetch(responses);
  const wrapped = createWakeRetryFetch({ baseFetch: fetch, wakeRetry: POLICY });

  await assert.rejects(
    wrapped("http://example/v1/sandboxes", {
      method: "POST",
      body: JSON.stringify({ x: 1 }),
    }),
    (err) => {
      assert.ok(err instanceof SandboxWakeTimeoutException);
      assert.equal(err.attempts, 5);
      assert.ok(err.waitedMs >= 0);
      assert.equal(err.statusCode, 503);
      assert.equal(err.error.code, SandboxError.WAKE_TIMEOUT);
      assert.equal(err.name, "SandboxWakeTimeoutException");
      assert.ok(err instanceof SandboxApiException);
      return true;
    }
  );
  assert.equal(calls.length, 5);
});

test("wake retry: Retry-After that exceeds the remaining budget stops before the next attempt", async () => {
  const { fetch, calls } = stubFetch([
    markedResponse({ retryAfter: "30" }),
    markedResponse({ retryAfter: "30" }),
  ]);
  const wrapped = createWakeRetryFetch({
    baseFetch: fetch,
    wakeRetry: { enabled: true, maxAttempts: 5, overallDeadlineSeconds: 10 },
  });

  await assert.rejects(
    wrapped("http://example/v1/sandboxes", { method: "POST", body: "x" }),
    (err) => {
      assert.ok(err instanceof SandboxWakeTimeoutException);
      assert.equal(err.attempts, 1);
      return true;
    }
  );
  // One attempt was made; its 30s wait already exceeds the 10s budget.
  assert.equal(calls.length, 1);
});

test("wake retry: maxAttempts bounds the retries", async () => {
  const { fetch, calls } = stubFetch(
    Array.from({ length: 3 }, () => markedResponse()),
  );
  const wrapped = createWakeRetryFetch({
    baseFetch: fetch,
    wakeRetry: { enabled: true, maxAttempts: 3, overallDeadlineSeconds: 60 },
  });
  await assert.rejects(
    wrapped("http://example/v1/x", { method: "GET" }),
    (err) => err.attempts === 3,
  );
  assert.equal(calls.length, 3);
});

test("wake retry: disabled policy returns the marked 503 untouched", async () => {
  const { fetch, calls } = stubFetch([markedResponse()]);
  const wrapped = createWakeRetryFetch({
    baseFetch: fetch,
    wakeRetry: { enabled: false, maxAttempts: 5, overallDeadlineSeconds: 60 },
  });
  const res = await wrapped("http://example/v1/x", { method: "POST", body: "x" });
  assert.equal(res.status, 503);
  assert.equal(calls.length, 1);
});

test("wake retry: non-replayable body is never retried", async () => {
  const { fetch, calls } = stubFetch([markedResponse()]);
  const wrapped = createWakeRetryFetch({ baseFetch: fetch, wakeRetry: POLICY });
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode("chunk"));
      controller.close();
    },
  });
  const res = await wrapped("http://example/v1/x", {
    method: "POST",
    body: stream,
  });
  assert.equal(res.status, 503);
  assert.equal(calls.length, 1);
});

test("wake retry: bare 503 is returned untouched for any method", async () => {
  const { fetch, calls } = stubFetch([new Response(null, { status: 503 })]);
  const wrapped = createWakeRetryFetch({ baseFetch: fetch, wakeRetry: POLICY });
  const res = await wrapped("http://example/v1/x", { method: "POST", body: "x" });
  assert.equal(res.status, 503);
  assert.equal(calls.length, 1);
});

test("wake retry: forged or wrong-value marker does not trigger retry", async () => {
  for (const marker of ["bogus", null]) {
    const { fetch, calls } = stubFetch([markedResponse({ marker })]);
    const wrapped = createWakeRetryFetch({ baseFetch: fetch, wakeRetry: POLICY });
    const res = await wrapped("http://example/v1/x", { method: "POST", body: "x" });
    assert.equal(res.status, 503);
    assert.equal(calls.length, 1, `marker=${marker}`);
  }
});

test("wake retry: HTTP-date Retry-After is honored", async () => {
  const soon = new Date(Date.now() + 50).toUTCString();
  const { fetch, calls } = stubFetch([
    markedResponse({ retryAfter: soon }),
    new Response(null, { status: 200 }),
  ]);
  const wrapped = createWakeRetryFetch({ baseFetch: fetch, wakeRetry: POLICY });
  const res = await wrapped("http://example/v1/x", { method: "POST", body: "x" });
  assert.equal(res.status, 200);
  assert.equal(calls.length, 2);
});

test("wake retry: missing Retry-After falls back to 1 second", async () => {
  const startedAt = Date.now();
  const { fetch } = stubFetch([
    markedResponse({ retryAfter: null }),
    new Response(null, { status: 200 }),
  ]);
  const wrapped = createWakeRetryFetch({ baseFetch: fetch, wakeRetry: POLICY });
  await wrapped("http://example/v1/x", { method: "POST", body: "x" });
  const waited = Date.now() - startedAt;
  assert.ok(waited >= WAKE_DEFAULT_RETRY_AFTER_MS - 50, `waited=${waited}`);
});

test("wake retry: Retry-After is capped at the operational ceiling", async () => {
  // The effective wait for a pathological header is the 60s ceiling, not the
  // raw 3600s — verified through the decision boundary: with a 59.9s budget
  // the capped 60s wait no longer fits, so the retry ends without another
  // attempt (a raw 3600s wait would end the same way, but resolveWakeRetryAfterMs
  // pins the capped value directly).
  const { fetch, calls } = stubFetch([markedResponse({ retryAfter: "3600" })]);
  const wrapped = createWakeRetryFetch({
    baseFetch: fetch,
    wakeRetry: { enabled: true, maxAttempts: 5, overallDeadlineSeconds: 59.9 },
  });
  await assert.rejects(
    wrapped("http://example/v1/x", { method: "POST", body: "x" }),
    (err) => err instanceof SandboxWakeTimeoutException,
  );
  assert.equal(calls.length, 1);

  assert.equal(resolveWakeRetryAfterMs(markedResponse({ retryAfter: "3600" })), RETRY_AFTER_CAP_MS);
  assert.equal(resolveWakeRetryAfterMs(markedResponse({ retryAfter: null })), WAKE_DEFAULT_RETRY_AFTER_MS);
  assert.equal(resolveWakeRetryAfterMs(markedResponse({ retryAfter: "2" })), 2000);
});

test("wake retry: null overall deadline removes the time bound (attempts still apply)", async () => {
  const { fetch } = stubFetch([
    markedResponse({ retryAfter: "2" }),
    new Response(null, { status: 200 }),
  ]);
  const wrapped = createWakeRetryFetch({
    baseFetch: fetch,
    wakeRetry: { enabled: true, maxAttempts: 5, overallDeadlineSeconds: null },
  });
  const res = await wrapped("http://example/v1/x", { method: "GET" });
  assert.equal(res.status, 200);
});

test("wake retry: caller abort during the wait stops retrying", async () => {
  const controller = new AbortController();
  const { fetch, calls } = stubFetch([markedResponse()]);
  const wrapped = createWakeRetryFetch({ baseFetch: fetch, wakeRetry: POLICY });
  const promise = wrapped("http://example/v1/x", {
    method: "POST",
    body: "x",
    signal: controller.signal,
  });
  setTimeout(() => controller.abort(new Error("client cancelled")), 10);
  await assert.rejects(promise, (err) => err.message === "client cancelled");
  assert.equal(calls.length, 1);
});

test("wake retry: non-503 errors are untouched", async () => {
  const { fetch, calls } = stubFetch([new Response("boom", { status: 500 })]);
  const wrapped = createWakeRetryFetch({ baseFetch: fetch, wakeRetry: POLICY });
  const res = await wrapped("http://example/v1/x", { method: "POST", body: "x" });
  assert.equal(res.status, 500);
  assert.equal(calls.length, 1);
});

test("wake retry: isWakeTimeoutResponse follows header-only recognition", () => {
  assert.ok(isWakeTimeoutResponse(markedResponse()));
  assert.ok(!isWakeTimeoutResponse(markedResponse({ marker: "other" })));
  assert.ok(!isWakeTimeoutResponse(new Response(null, { status: 503 })));
  assert.ok(!isWakeTimeoutResponse(markedResponse({ status: 500 })));
});

test("parseRetryAfterMs handles delta-seconds, HTTP-dates, and junk", () => {
  assert.equal(parseRetryAfterMs(null), null);
  assert.equal(parseRetryAfterMs(""), null);
  assert.equal(parseRetryAfterMs("soon"), null);
  assert.equal(parseRetryAfterMs("2"), 2000);
  assert.equal(parseRetryAfterMs("-3"), 0);
  assert.equal(parseRetryAfterMs("0"), 0);
  // Huge values are clamped to the overflow guard.
  assert.equal(parseRetryAfterMs("999999999999999"), 100 * 365 * 24 * 3600 * 1000);
  const future = Date.now() + 5000;
  const parsed = parseRetryAfterMs(new Date(future).toUTCString(), Date.now());
  assert.ok(parsed !== null && parsed > 4000 && parsed <= 5000);
  const past = parseRetryAfterMs(new Date(Date.now() - 5000).toUTCString(), Date.now());
  assert.equal(past, 0);
});

test("ConnectionConfig defaults enable the wake retry", () => {
  const config = new ConnectionConfig({ domain: "http://127.0.0.1:8080" });
  assert.deepEqual(config.wakeRetry, {
    enabled: true,
    maxAttempts: 5,
    overallDeadlineSeconds: 60,
  });
});

test("ConnectionConfig accepts explicit wake retry options and clones them", () => {
  const config = new ConnectionConfig({
    domain: "http://127.0.0.1:8080",
    wakeRetryEnabled: true,
    wakeMaxAttempts: 2,
    wakeOverallDeadlineSeconds: 5,
  });
  assert.deepEqual(config.wakeRetry, {
    enabled: true,
    maxAttempts: 2,
    overallDeadlineSeconds: 5,
  });
  const cloned = config.withCreateMetricsDisabled();
  assert.deepEqual(cloned.wakeRetry, config.wakeRetry);
  const unbounded = new ConnectionConfig({
    domain: "http://127.0.0.1:8080",
    wakeOverallDeadlineSeconds: null,
  });
  assert.equal(unbounded.wakeRetry.overallDeadlineSeconds, null);
});

test("ConnectionConfig rejects invalid wake retry options", () => {
  assert.throws(
    () => new ConnectionConfig({ wakeMaxAttempts: 0 }),
    /wakeMaxAttempts must be an integer >= 1/,
  );
  assert.throws(
    () => new ConnectionConfig({ wakeMaxAttempts: 1.5 }),
    /wakeMaxAttempts must be an integer >= 1/,
  );
  assert.throws(
    () => new ConnectionConfig({ wakeOverallDeadlineSeconds: 0 }),
    /wakeOverallDeadlineSeconds must be a positive number or null/,
  );
  assert.throws(
    () => new ConnectionConfig({ wakeOverallDeadlineSeconds: -1 }),
    /wakeOverallDeadlineSeconds must be a positive number or null/,
  );
});
