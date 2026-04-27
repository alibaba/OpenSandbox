import { apiFetch } from './client.ts'
import type {
  CreateSandboxRequest,
  Endpoint,
  ListSandboxesResponse,
  RenewExpirationRequest,
  Sandbox,
} from './types.ts'

export async function listSandboxes(page = 1, pageSize = 50): Promise<ListSandboxesResponse> {
  return apiFetch<ListSandboxesResponse>(`/sandboxes?page=${page}&pageSize=${pageSize}`)
}

export async function getSandbox(id: string): Promise<Sandbox> {
  return apiFetch<Sandbox>(`/sandboxes/${id}`)
}

export async function createSandbox(req: CreateSandboxRequest): Promise<Sandbox> {
  return apiFetch<Sandbox>('/sandboxes', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function deleteSandbox(id: string): Promise<void> {
  return apiFetch<void>(`/sandboxes/${id}`, { method: 'DELETE' })
}

export async function pauseSandbox(id: string): Promise<void> {
  return apiFetch<void>(`/sandboxes/${id}/pause`, { method: 'POST' })
}

export async function resumeSandbox(id: string): Promise<void> {
  return apiFetch<void>(`/sandboxes/${id}/resume`, { method: 'POST' })
}

export async function renewSandbox(id: string, req: RenewExpirationRequest): Promise<void> {
  return apiFetch<void>(`/sandboxes/${id}/renew-expiration`, {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function getSandboxEndpoint(id: string, port = 3000): Promise<Endpoint> {
  return apiFetch<Endpoint>(`/sandboxes/${id}/endpoints/${port}`)
}
