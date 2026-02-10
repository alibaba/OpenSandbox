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

import type {
  CreateSandboxRequest,
  CreateSandboxResponse,
  Endpoint,
  ListSandboxesParams,
  ListSandboxesResponse,
  RenewSandboxExpirationRequest,
  RenewSandboxExpirationResponse,
  SandboxId,
  SandboxInfo,
} from "../models/sandboxes.js";

export interface Sandboxes {
  createSandbox(req: CreateSandboxRequest): Promise<CreateSandboxResponse>;
  getSandbox(sandboxId: SandboxId): Promise<SandboxInfo>;
  listSandboxes(params?: ListSandboxesParams): Promise<ListSandboxesResponse>;
  deleteSandbox(sandboxId: SandboxId): Promise<void>;

  pauseSandbox(sandboxId: SandboxId): Promise<void>;
  resumeSandbox(sandboxId: SandboxId): Promise<void>;

  renewSandboxExpiration(
    sandboxId: SandboxId,
    req: RenewSandboxExpirationRequest,
  ): Promise<RenewSandboxExpirationResponse>;

  getSandboxEndpoint(
    sandboxId: SandboxId,
    port: number,
    useServerProxy?: boolean
  ): Promise<Endpoint>;
}