// ============================================================
// Shared types mirroring opensandbox_server/api/schema.py
// ============================================================

export interface SandboxStatus {
  state: string
  reason?: string
  message?: string
  lastTransitionAt?: string
}

export interface ImageAuth {
  username: string
  password: string
}

export interface ImageSpec {
  uri: string
  auth?: ImageAuth
}

export interface PlatformSpec {
  os: string
  arch: string
}

export interface NetworkRule {
  action: 'allow' | 'deny'
  target: string
}

export interface NetworkPolicy {
  defaultAction?: 'allow' | 'deny' | null
  egress?: NetworkRule[]
}

export interface VolumeHost {
  path: string
}

export interface VolumePVC {
  claimName: string
  createIfNotExists?: boolean
  deleteOnSandboxTermination?: boolean
  storageClass?: string | null
  storage?: string | null
  accessModes?: string[] | null
}

export interface VolumeOSSFS {
  bucket: string
  endpoint: string
  version?: '1.0' | '2.0'
  options?: string[] | null
  accessKeyId?: string | null
  accessKeySecret?: string | null
}

export interface Volume {
  name: string
  mountPath: string
  readOnly?: boolean
  subPath?: string | null
  host?: VolumeHost | null
  pvc?: VolumePVC | null
  ossfs?: VolumeOSSFS | null
}

export interface Sandbox {
  id: string
  image: ImageSpec
  status: SandboxStatus
  metadata?: Record<string, string>
  env?: Record<string, string>
  entrypoint: string[]
  expiresAt?: string
  createdAt: string
}

export interface PaginationInfo {
  page: number
  pageSize: number
  totalItems: number
  totalPages: number
  hasNextPage: boolean
}

export interface ListSandboxesResponse {
  items: Sandbox[]
  pagination: PaginationInfo
}

export interface CreateSandboxRequest {
  image: ImageSpec
  resourceLimits: Record<string, string>
  entrypoint: string[]
  platform?: PlatformSpec | null
  timeout?: number | null
  env?: Record<string, string> | null
  metadata?: Record<string, string> | null
  networkPolicy?: NetworkPolicy | null
  secureAccess?: boolean
  volumes?: Volume[] | null
  extensions?: Record<string, string> | null
}

export interface RenewExpirationRequest {
  expiresAt: string
}

export interface Endpoint {
  endpoint: string
  headers?: Record<string, string>
}

// ============================================================
// Pool types
// ============================================================

export interface PoolCapacitySpec {
  bufferMax: number
  bufferMin: number
  poolMax: number
  poolMin: number
}

export interface PoolStatus {
  total: number
  allocated: number
  available: number
  revision: string
}

export interface PoolResponse {
  name: string
  capacitySpec: PoolCapacitySpec
  status?: PoolStatus
  createdAt?: string
}

export interface ListPoolsResponse {
  items: PoolResponse[]
}

export interface CreatePoolRequest {
  name: string
  template: Record<string, unknown>
  capacitySpec: PoolCapacitySpec
}

export interface UpdatePoolRequest {
  capacitySpec: PoolCapacitySpec
}

// ============================================================
// Error
// ============================================================

export interface ApiError {
  code: string
  message: string
}

// ============================================================
// Console / SSE session types
// ============================================================

export type MessageRole = 'user' | 'assistant' | 'tool_call' | 'tool_result' | 'system'

export interface ChatMessage {
  id: string
  role: MessageRole
  content: string
  timestamp: string
  toolName?: string
  isStreaming?: boolean
}
