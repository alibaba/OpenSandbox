import { apiFetch } from './client.ts'
import type {
  CreatePoolRequest,
  ListPoolsResponse,
  PoolResponse,
  UpdatePoolRequest,
} from './types.ts'

export async function listPools(): Promise<ListPoolsResponse> {
  return apiFetch<ListPoolsResponse>('/pools')
}

export async function getPool(name: string): Promise<PoolResponse> {
  return apiFetch<PoolResponse>(`/pools/${name}`)
}

export async function createPool(req: CreatePoolRequest): Promise<PoolResponse> {
  return apiFetch<PoolResponse>('/pools', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function updatePool(name: string, req: UpdatePoolRequest): Promise<PoolResponse> {
  return apiFetch<PoolResponse>(`/pools/${name}`, {
    method: 'PUT',
    body: JSON.stringify(req),
  })
}

export async function deletePool(name: string): Promise<void> {
  return apiFetch<void>(`/pools/${name}`, { method: 'DELETE' })
}
