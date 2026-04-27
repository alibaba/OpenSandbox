import { loadSettings } from './client.ts'
import { ApiRequestError } from './client.ts'

async function fetchText(path: string): Promise<string> {
  const settings = loadSettings()
  if (!settings.serverUrl) {
    throw new ApiRequestError(0, 'NO_SERVER', 'Server URL not configured.')
  }
  const url = `${settings.serverUrl.replace(/\/$/, '')}${path}`
  const headers: Record<string, string> = {}
  if (settings.authToken) {
    headers['OPEN-SANDBOX-API-KEY'] = settings.authToken
  }
  const res = await fetch(url, { headers })
  if (!res.ok) {
    throw new ApiRequestError(res.status, 'HTTP_ERROR', `HTTP ${res.status}`)
  }
  return res.text()
}

export async function getSandboxLogs(id: string, tail = 200): Promise<string> {
  return fetchText(`/sandboxes/${id}/diagnostics/logs?tail=${tail}`)
}

export async function getSandboxInspect(id: string): Promise<string> {
  return fetchText(`/sandboxes/${id}/diagnostics/inspect`)
}

export async function getSandboxEvents(id: string, limit = 50): Promise<string> {
  return fetchText(`/sandboxes/${id}/diagnostics/events?limit=${limit}`)
}

export async function getSandboxDiagnosticsSummary(id: string): Promise<string> {
  return fetchText(`/sandboxes/${id}/diagnostics/summary`)
}
