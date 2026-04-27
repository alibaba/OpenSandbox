import { createParser } from 'eventsource-parser'
import { apiFetchSSE } from './client.ts'

export interface SseEvent {
  type: string
  data: unknown
}

export type SseHandler = (event: SseEvent) => void
export type SseDoneHandler = () => void
export type SseErrorHandler = (err: Error) => void
export type SseSessionIdHandler = (sessionId: string) => void

/**
 * Post a prompt to claude-agent-server via the opensandbox proxy and stream SSE events.
 *
 * - If sessionId is null, creates a new session via POST /sessions with options.cwd set.
 *   The sessionId is extracted from the SSE event data and reported via onSessionId.
 * - If sessionId is provided, continues the existing session via POST /sessions/{sessionId}/messages.
 *
 * Returns an AbortController so the caller can cancel the stream.
 */
export function streamSession(
  sandboxId: string,
  port: number,
  sessionId: string | null,
  prompt: string,
  cwd: string,
  permissionMode: string,
  onEvent: SseHandler,
  onSessionId: SseSessionIdHandler,
  onDone: SseDoneHandler,
  onError: SseErrorHandler,
): AbortController {
  const controller = new AbortController()

  void (async () => {
    const basePath = `/sandboxes/${sandboxId}/proxy/${port}`
    const path = sessionId
      ? `${basePath}/sessions/${sessionId}/messages`
      : `${basePath}/sessions`

    const body = sessionId
      ? { prompt, stream: true, options: { permissionMode } }
      : { prompt, stream: true, options: { cwd, permissionMode } }

    let res: Response
    try {
      res = await apiFetchSSE(path, body)
    } catch (err) {
      onError(err instanceof Error ? err : new Error(String(err)))
      return
    }

    if (!res.ok) {
      onError(new Error(`HTTP ${res.status} from session endpoint`))
      return
    }

    const responseBody = res.body
    if (!responseBody) {
      onError(new Error('Response body is empty'))
      return
    }

    let sessionIdReported = sessionId !== null

    const parser = createParser({
      onEvent(event) {
        let data: unknown
        try {
          data = event.data === '[DONE]' ? null : (JSON.parse(event.data) as unknown)
        } catch {
          data = event.data
        }

        // Extract sessionId from the first event that carries it
        if (!sessionIdReported && data !== null && typeof data === 'object') {
          const record = data as Record<string, unknown>
          if (typeof record.sessionId === 'string') {
            sessionIdReported = true
            onSessionId(record.sessionId)
          }
        }

        onEvent({ type: event.event ?? 'message', data })
      },
    })

    const reader = responseBody.getReader()
    const decoder = new TextDecoder()

    try {
      while (true) {
        const { done, value } = await reader.read()
        if (controller.signal.aborted) break
        if (done) break
        parser.feed(decoder.decode(value, { stream: true }))
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        onError(err instanceof Error ? err : new Error(String(err)))
        return
      }
    }

    onDone()
  })()

  return controller
}
