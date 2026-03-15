import assert from "node:assert/strict";
import test from "node:test";

import {
  DEFAULT_TIMEOUT_SECONDS,
  Sandbox,
} from "../dist/index.js";

function createAdapterFactory() {
  const recordedRequests = [];
  const sandboxes = {
    async createSandbox(req) {
      recordedRequests.push(req);
      return { id: "sandbox-test-id", expiresAt: null };
    },
    async getSandbox() {
      throw new Error("not implemented");
    },
    async listSandboxes() {
      throw new Error("not implemented");
    },
    async deleteSandbox() {},
    async pauseSandbox() {},
    async resumeSandbox() {},
    async renewSandboxExpiration() {
      throw new Error("not implemented");
    },
    async getSandboxEndpoint() {
      return { endpoint: "127.0.0.1:5000", headers: {} };
    },
  };

  const adapterFactory = {
    createLifecycleStack() {
      return { sandboxes };
    },
    createExecdStack() {
      return {
        commands: {},
        files: {},
        health: {},
        metrics: {},
      };
    },
  };

  return { adapterFactory, recordedRequests };
}

test("Sandbox.create omits timeout when timeoutSeconds is null", async () => {
  const { adapterFactory, recordedRequests } = createAdapterFactory();

  await Sandbox.create({
    adapterFactory,
    connectionConfig: { domain: "http://127.0.0.1:8080" },
    image: "python:3.12",
    timeoutSeconds: null,
    skipHealthCheck: true,
  });

  assert.equal(recordedRequests.length, 1);
  assert.ok(!Object.hasOwn(recordedRequests[0], "timeout"));
});

test("Sandbox.create floors finite timeoutSeconds", async () => {
  const { adapterFactory, recordedRequests } = createAdapterFactory();

  await Sandbox.create({
    adapterFactory,
    connectionConfig: { domain: "http://127.0.0.1:8080" },
    image: "python:3.12",
    timeoutSeconds: 61.9,
    skipHealthCheck: true,
  });

  assert.equal(recordedRequests.length, 1);
  assert.equal(recordedRequests[0].timeout, 61);
});

test("Sandbox.create uses the default timeout when timeoutSeconds is undefined", async () => {
  const { adapterFactory, recordedRequests } = createAdapterFactory();

  await Sandbox.create({
    adapterFactory,
    connectionConfig: { domain: "http://127.0.0.1:8080" },
    image: "python:3.12",
    skipHealthCheck: true,
  });

  assert.equal(recordedRequests.length, 1);
  assert.equal(recordedRequests[0].timeout, DEFAULT_TIMEOUT_SECONDS);
});

test("Sandbox.create rejects non-finite timeoutSeconds", async () => {
  for (const timeoutSeconds of [Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY]) {
    const { adapterFactory } = createAdapterFactory();
    await assert.rejects(
      Sandbox.create({
        adapterFactory,
        connectionConfig: { domain: "http://127.0.0.1:8080" },
        image: "python:3.12",
        timeoutSeconds,
        skipHealthCheck: true,
      }),
      /timeoutSeconds must be a finite number/
    );
  }
});
