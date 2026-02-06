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

/**
 * Domain models for sandbox lifecycle.
 *
 * IMPORTANT:
 * - These are NOT OpenAPI-generated types.
 * - They are intentionally stable and JS-friendly.
 *
 * The internal OpenAPI schemas may change frequently; adapters map responses into these models.
 */

export type SandboxId = string;

export interface ImageAuth extends Record<string, unknown> {
  username?: string;
  password?: string;
  token?: string;
}

export interface ImageSpec {
  uri: string;
  auth?: ImageAuth;
}

export type ResourceLimits = Record<string, string>;

export type NetworkRuleAction = "allow" | "deny";

export interface NetworkRule extends Record<string, unknown> {
  /**
   * Whether to allow or deny matching targets.
   */
  action: NetworkRuleAction;
  /**
   * FQDN or wildcard domain (e.g., "example.com", "*.example.com").
   * IP/CIDR is not supported in the egress MVP.
   */
  target: string;
}

export interface NetworkPolicy extends Record<string, unknown> {
  /**
   * Default action when no egress rule matches. Defaults to "deny".
   */
  defaultAction?: NetworkRuleAction;
  /**
   * List of egress rules evaluated in order.
   */
  egress?: NetworkRule[];
}

// ============================================================================
// Volume Models
// ============================================================================

/**
 * Host path bind mount backend.
 *
 * Maps a directory on the host filesystem into the container.
 * Only available when the runtime supports host mounts.
 */
export interface Host extends Record<string, unknown> {
  /**
   * Absolute path on the host filesystem to mount.
   */
  path: string;
}

/**
 * Kubernetes PersistentVolumeClaim mount backend.
 *
 * References an existing PVC in the same namespace as the sandbox pod.
 * Only available in Kubernetes runtime.
 */
export interface PVC extends Record<string, unknown> {
  /**
   * Name of the PersistentVolumeClaim in the same namespace.
   */
  claimName: string;
}

/**
 * Storage mount definition for a sandbox.
 *
 * Each volume entry contains:
 * - A unique name identifier
 * - Exactly one backend (host, pvc) with backend-specific fields
 * - Common mount settings (mountPath, readOnly, subPath)
 */
export interface Volume extends Record<string, unknown> {
  /**
   * Unique identifier for the volume within the sandbox.
   */
  name: string;
  /**
   * Host path bind mount backend (mutually exclusive with pvc).
   */
  host?: Host;
  /**
   * Kubernetes PVC mount backend (mutually exclusive with host).
   */
  pvc?: PVC;
  /**
   * Absolute path inside the container where the volume is mounted.
   */
  mountPath: string;
  /**
   * If true, the volume is mounted as read-only. Defaults to false (read-write).
   */
  readOnly?: boolean;
  /**
   * Optional subdirectory under the backend path to mount.
   */
  subPath?: string;
}

export type SandboxState =
  | "Creating"
  | "Running"
  | "Pausing"
  | "Paused"
  | "Resuming"
  | "Deleting"
  | "Deleted"
  | "Error"
  | string;

export interface SandboxStatus extends Record<string, unknown> {
  state: SandboxState;
  reason?: string;
  message?: string;
}

export interface SandboxInfo extends Record<string, unknown> {
  id: SandboxId;
  image: ImageSpec;
  entrypoint: string[];
  metadata?: Record<string, string>;
  status: SandboxStatus;
  /**
   * Sandbox creation time.
   */
  createdAt: Date;
  /**
   * Sandbox expiration time (server-side TTL).
   */
  expiresAt: Date;
}

export interface CreateSandboxRequest extends Record<string, unknown> {
  image: ImageSpec;
  entrypoint: string[];
  /**
   * Timeout in seconds (server semantics).
   */
  timeout: number;
  resourceLimits: ResourceLimits;
  env?: Record<string, string>;
  metadata?: Record<string, string>;
  /**
   * Optional outbound network policy for the sandbox.
   */
  networkPolicy?: NetworkPolicy;
  /**
   * Optional list of volume mounts for persistent storage.
   */
  volumes?: Volume[];
  extensions?: Record<string, unknown>;
}

export interface CreateSandboxResponse extends Record<string, unknown> {
  id: SandboxId;
  status: SandboxStatus;
  metadata?: Record<string, string>;
  /**
   * Sandbox expiration time after creation.
   */
  expiresAt: Date;
  /**
   * Sandbox creation time.
   */
  createdAt: Date;
  entrypoint: string[];
}

export interface PaginationInfo extends Record<string, unknown> {
  page: number;
  pageSize: number;
  totalItems: number;
  totalPages: number;
  hasNextPage: boolean;
}

export interface ListSandboxesResponse extends Record<string, unknown> {
  items: SandboxInfo[];
  pagination?: PaginationInfo;
}

export interface RenewSandboxExpirationRequest {
  expiresAt: string;
}

export interface RenewSandboxExpirationResponse extends Record<string, unknown> {
  /**
   * Updated expiration time (if the server returns it).
   */
  expiresAt?: Date;
}

export interface Endpoint extends Record<string, unknown> {
  endpoint: string;
}

export interface ListSandboxesParams {
  /**
   * Filter by lifecycle state (the API supports multiple `state` query params).
   * Example: `{ states: ["Running", "Paused"] }`
   */
  states?: string[];
  /**
   * Filter by metadata key-value pairs.
   * NOTE: This will be encoded to a single `metadata` query parameter as described in the spec.
   */
  metadata?: Record<string, string>;
  page?: number;
  pageSize?: number;
};