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

import { afterAll, beforeAll, expect, test } from "vitest";

import { Sandbox, type ExecutionHandlers } from "@alibaba-group/opensandbox";

import {
  CodeInterpreter,
  SupportedLanguages,
} from "@alibaba-group/opensandbox-code-interpreter";

import {
  assertEndpointHasPort,
  assertRecentTimestampMs,
  createConnectionConfig,
  getSandboxImage,
} from "./base_e2e.ts";

let sandbox: Sandbox | null = null;
let ci: CodeInterpreter | null = null;

beforeAll(async () => {
  const connectionConfig = createConnectionConfig();

  sandbox = await Sandbox.create({
    connectionConfig,
    image: getSandboxImage(),
    entrypoint: ["/opt/opensandbox/code-interpreter.sh"],
    timeoutSeconds: 15 * 60,
    readyTimeoutSeconds: 60,
    metadata: { tag: "e2e-code-interpreter" },
    env: {
      E2E_TEST: "true",
      GO_VERSION: "1.25",
      JAVA_VERSION: "21",
      NODE_VERSION: "22",
      PYTHON_VERSION: "3.12",
      EXECD_LOG_FILE: "/tmp/opensandbox-e2e/logs/execd.log"
    },
    healthCheckPollingInterval: 200,
    volumes: [
        {
            name: "execd-log",
            host: { path: "/tmp/opensandbox-e2e/logs" },
            mountPath: "/tmp/opensandbox-e2e/logs",
            readOnly: false,
        },
    ],
  });

  ci = await CodeInterpreter.create(sandbox);
}, 10 * 60_000);

afterAll(async () => {
  if (!sandbox) return;
  try {
    await sandbox.kill();
  } catch {
    // ignore
  }
}, 5 * 60_000);

test("01 creation and basic functionality", async () => {
  if (!sandbox || !ci) throw new Error("not initialized");

  expect(ci.id).toBe(sandbox.id);
  expect(await sandbox.isHealthy()).toBe(true);

  const info = await sandbox.getInfo();
  expect(info.status.state).toBe("Running");

  const ep = await sandbox.getEndpoint(44772);
  assertEndpointHasPort(ep.endpoint, 44772);

  const metrics = await sandbox.getMetrics();
  assertRecentTimestampMs(metrics.timestamp);
});

test("01b context management: get/list/delete/deleteContexts", async () => {
  if (!ci) throw new Error("not initialized");

  const ctx = await ci.codes.createContext(SupportedLanguages.PYTHON);
  expect(ctx.id).toBeTruthy();
  expect(ctx.language).toBe("python");

  const got = await ci.codes.getContext(ctx.id!);
  expect(got.id).toBe(ctx.id);
  expect(got.language).toBe("python");

  const all = await ci.codes.listContexts();
  expect(all.some((c) => c.id === ctx.id)).toBe(true);

  const pyOnly = await ci.codes.listContexts(SupportedLanguages.PYTHON);
  expect(pyOnly.some((c) => c.id === ctx.id)).toBe(true);

  await ci.codes.deleteContext(ctx.id!);
  await expect(ci.codes.getContext(ctx.id!)).rejects.toBeTruthy();

  // Bulk cleanup should not throw.
  await ci.codes.deleteContexts(SupportedLanguages.PYTHON);
});

test("02 java code execution", async () => {
  if (!ci) throw new Error("not initialized");

  const javaCtx = await ci.codes.createContext(SupportedLanguages.JAVA);
  expect(javaCtx.id).toBeTruthy();
  expect(javaCtx.language).toBe("java");

  const stdout: string[] = [];
  const errors: string[] = [];
  const initIds: string[] = [];

  const handlers: ExecutionHandlers = {
    onStdout: (m) => {
      stdout.push(m.text);
    },
    onError: (e) => {
      errors.push(e.name);
    },
    onInit: (i) => {
      initIds.push(i.id);
    },
  };

  const r = await ci.codes.run(
    'System.out.println("Hello from Java!");\nint result = 2 + 2;\nSystem.out.println("2 + 2 = " + result);\nresult',
    { context: javaCtx, handlers }
  );
  expect(r.id).toBeTruthy();
  expect(r.error).toBeUndefined();
  expect(r.result[0]?.text).toBe("4");
  expect(initIds).toHaveLength(1);
  expect(errors).toHaveLength(0);
  expect(stdout.some((s) => s.includes("Hello from Java!"))).toBe(true);

  const err = await ci.codes.run("int x = 10 / 0; // ArithmeticException", {
    context: javaCtx,
  });
  expect(err.error).toBeTruthy();
  expect(err.error?.name).toBe("EvalException");
});

test("03 python code execution + direct language + persistence", async () => {
  if (!ci) throw new Error("not initialized");

  const direct = await ci.codes.run("result = 2 + 2\nresult", {
    language: SupportedLanguages.PYTHON,
  });
  expect(direct.error).toBeUndefined();
  expect(direct.result[0]?.text).toBe("4");

  const ctx = await ci.codes.createContext(SupportedLanguages.PYTHON);
  await ci.codes.run("x = 42", { context: ctx });
  const r = await ci.codes.run("result = x\nresult", { context: ctx });
  expect(r.result[0]?.text).toBe("42");

  const bad = await ci.codes.run("print(undefined_variable)", { context: ctx });
  expect(bad.error).toBeTruthy();
});

test("04 go and typescript execution (smoke)", async () => {
  if (!ci) throw new Error("not initialized");

  const goCtx = await ci.codes.createContext(SupportedLanguages.GO);
  const go = await ci.codes.run(
    'package main\nimport "fmt"\nfunc main() { fmt.Print("hi"); result := 2+2; fmt.Print(result) }',
    { context: goCtx }
  );
  expect(go.id).toBeTruthy();

  const tsCtx = await ci.codes.createContext(SupportedLanguages.TYPESCRIPT);
  const ts = await ci.codes.run(
    "console.log('Hello from TypeScript!');\nconst result: number = 2 + 2;\nresult",
    {
      context: tsCtx,
    }
  );
  expect(ts.id).toBeTruthy();
});

test("05 context isolation", async () => {
  if (!ci) throw new Error("not initialized");

  const python1 = await ci.codes.createContext(SupportedLanguages.PYTHON);
  const python2 = await ci.codes.createContext(SupportedLanguages.PYTHON);
  await ci.codes.run("secret_value1 = 'python1_secret'", { context: python1 });

  const ok = await ci.codes.run("result = secret_value1\nresult", {
    context: python1,
  });
  expect(ok.error).toBeUndefined();

  const bad = await ci.codes.run("result = secret_value1\nresult", {
    context: python2,
  });
  expect(bad.error).toBeTruthy();
  expect(bad.error?.name).toBe("NameError");
});

test("06 concurrent execution", async () => {
  if (!ci) throw new Error("not initialized");

  const py = await ci.codes.createContext(SupportedLanguages.PYTHON);
  const java = await ci.codes.createContext(SupportedLanguages.JAVA);
  const go = await ci.codes.createContext(SupportedLanguages.GO);

  const [r1, r2, r3] = await Promise.all([
    ci.codes.run(
      "import time\nfor i in range(3):\n  print(i)\n  time.sleep(0.1)",
      { context: py }
    ),
    ci.codes.run(
      "for (int i=0;i<3;i++){ System.out.println(i); try{Thread.sleep(100);}catch(Exception e){} }",
      { context: java }
    ),
    ci.codes.run(
      'package main\nimport "fmt"\nfunc main(){ for i:=0;i<3;i++{ fmt.Print(i) } }',
      { context: go }
    ),
  ]);

  expect(r1.id).toBeTruthy();
  expect(r2.id).toBeTruthy();
  expect(r3.id).toBeTruthy();
});

test("07 interrupt code execution + fake id", async () => {
  if (!ci) throw new Error("not initialized");
  const ci0 = ci;

  const ctx = await ci0.codes.createContext(SupportedLanguages.PYTHON);

  let initId: string | null = null;
  let runTask: Promise<unknown> | null = null;
  const initReceived = new Promise<void>((resolve) => {
    const handlers: ExecutionHandlers = {
      onInit: (i) => {
        initId = i.id;
        assertRecentTimestampMs(i.timestamp);
        resolve();
      },
    };

    runTask = ci0.codes.run(
      "import time\nfor i in range(100):\n  print(i)\n  time.sleep(0.2)",
      { context: ctx, handlers }
    );
  });

  await initReceived;
  if (!initId) throw new Error("missing init id");
  await ci0.codes.interrupt(initId);

  // Important: always await/catch the execution task to avoid Vitest reporting
  // unhandled rejections when the server closes the streaming connection.
  if (runTask) {
    try {
      await runTask;
    } catch {
      // Expected in some environments: interrupt may terminate the stream abruptly.
    }
  }

  await expect(ci0.codes.interrupt(`fake-${Date.now()}`)).rejects.toBeTruthy();
});
