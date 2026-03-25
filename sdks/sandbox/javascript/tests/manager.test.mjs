import assert from "node:assert/strict";
import test from "node:test";

import { ConnectionConfig, SandboxManager } from "../dist/index.js";

function createSandboxesStub() {
  const calls = [];
  const sandboxes = {
    async listSandboxes(filter) {
      calls.push({ method: "listSandboxes", args: [filter] });
      return { items: [{ id: "sbx-1" }] };
    },
    async getSandbox(sandboxId) {
      calls.push({ method: "getSandbox", args: [sandboxId] });
      return { id: sandboxId };
    },
    async deleteSandbox(sandboxId) {
      calls.push({ method: "deleteSandbox", args: [sandboxId] });
    },
    async pauseSandbox(sandboxId) {
      calls.push({ method: "pauseSandbox", args: [sandboxId] });
    },
    async resumeSandbox(sandboxId) {
      calls.push({ method: "resumeSandbox", args: [sandboxId] });
    },
    async renewSandboxExpiration(sandboxId, body) {
      calls.push({ method: "renewSandboxExpiration", args: [sandboxId, body] });
    },
  };
  return { sandboxes, calls };
}

test("SandboxManager delegates lifecycle operations and closes its transport", async () => {
  const { sandboxes, calls } = createSandboxesStub();
  const connectionConfig = new ConnectionConfig({ domain: "http://127.0.0.1:8080" });
  let closeCalls = 0;
  connectionConfig.closeTransport = async () => {
    closeCalls += 1;
  };
  connectionConfig.withTransportIfMissing = () => connectionConfig;

  const manager = SandboxManager.create({
    connectionConfig,
    adapterFactory: {
      createLifecycleStack() {
        return { sandboxes };
      },
    },
  });

  const list = await manager.listSandboxInfos({
    states: ["Running"],
    metadata: { team: "sdk" },
    page: 2,
    pageSize: 5,
  });
  assert.equal(list.items[0].id, "sbx-1");

  const info = await manager.getSandboxInfo("sbx-42");
  assert.equal(info.id, "sbx-42");

  await manager.pauseSandbox("sbx-42");
  await manager.resumeSandbox("sbx-42");
  await manager.killSandbox("sbx-42");
  await manager.renewSandbox("sbx-42", 30);
  await manager.close();

  assert.deepEqual(
    calls.map((entry) => entry.method),
    [
      "listSandboxes",
      "getSandbox",
      "pauseSandbox",
      "resumeSandbox",
      "deleteSandbox",
      "renewSandboxExpiration",
    ],
  );
  assert.deepEqual(calls[0].args[0], {
    states: ["Running"],
    metadata: { team: "sdk" },
    page: 2,
    pageSize: 5,
  });
  assert.equal(calls[5].args[0], "sbx-42");
  assert.ok(typeof calls[5].args[1].expiresAt === "string");
  assert.ok(Number.isFinite(Date.parse(calls[5].args[1].expiresAt)));
  assert.equal(closeCalls, 1);
});
