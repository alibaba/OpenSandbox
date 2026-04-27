const SETTINGS_KEY = 'opensandbox_settings'

export interface Settings {
  serverUrl: string
  authToken: string
}

export function loadSettings(): Settings {
  try {
    const raw = localStorage.getItem(SETTINGS_KEY)
    if (raw) return JSON.parse(raw) as Settings
  } catch {
    // ignore parse errors
  }
  return { serverUrl: '', authToken: '' }
}

export function saveSettings(settings: Settings): void {
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings))
}

// Track whether the last request succeeded for the connection indicator
let lastRequestOk: boolean | null = null
const listeners: Array<(ok: boolean | null) => void> = []

export function getConnectionStatus(): boolean | null {
  return lastRequestOk
}

export function onConnectionStatusChange(cb: (ok: boolean | null) => void): () => void {
  listeners.push(cb)
  return () => {
    const idx = listeners.indexOf(cb)
    if (idx !== -1) listeners.splice(idx, 1)
  }
}

function setConnectionStatus(ok: boolean): void {
  lastRequestOk = ok
  listeners.forEach((cb) => cb(ok))
}

export class ApiRequestError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message)
    this.name = 'ApiRequestError'
  }
}

export async function apiFetch<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const settings = loadSettings()
  if (!settings.serverUrl) {
    throw new ApiRequestError(0, 'NO_SERVER', 'Server URL not configured. Open Settings to configure.')
  }

  const url = `${settings.serverUrl.replace(/\/$/, '')}${path}`
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  }
  if (settings.authToken) {
    headers['OPEN-SANDBOX-API-KEY'] = settings.authToken
  }

  let res: Response
  try {
    res = await fetch(url, { ...options, headers })
  } catch (err) {
    setConnectionStatus(false)
    throw new ApiRequestError(0, 'NETWORK_ERROR', `Network error: ${String(err)}`)
  }

  setConnectionStatus(res.ok)

  if (!res.ok) {
    let code = 'HTTP_ERROR'
    let message = `HTTP ${res.status}`
    try {
      const body = await res.json() as { code?: string; message?: string }
      code = body.code ?? code
      message = body.message ?? message
    } catch {
      // ignore
    }
    throw new ApiRequestError(res.status, code, message)
  }

  // 204 No Content
  if (res.status === 204) return undefined as T

  return res.json() as Promise<T>
}

/** Build a fetch request for SSE — returns the raw Response so the caller can stream it. */
export async function apiFetchSSE(
  path: string,
  body: unknown,
): Promise<Response> {
  const settings = loadSettings()
  if (!settings.serverUrl) {
    throw new ApiRequestError(0, 'NO_SERVER', 'Server URL not configured.')
  }

  const url = `${settings.serverUrl.replace(/\/$/, '')}${path}`
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
  }
  if (settings.authToken) {
    headers['OPEN-SANDBOX-API-KEY'] = settings.authToken
  }

  const res = await fetch(url, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  })

  setConnectionStatus(res.ok)
  return res
}
