import { createParser } from 'eventsource-parser'
import { apiFetchSSE } from './client.ts'

export interface SseEvent {
  type: string
  data: unknown
}

export type SseHandler = (event: SseEvent) => void
export type SseDoneHandler = () => void
export type SseErrorHandler = (err: Error) => void

/**
 * Post a prompt to claude-agent-server via the opensandbox proxy and stream SSE events.
 * Returns an AbortController so the caller can cancel the stream.
 */
export function streamSession(
  sandboxId: string,
  port: number,
  prompt: string,
  onEvent: SseHandler,
  onDone: SseDoneHandler,
  onError: SseErrorHandler,
): AbortController {
  const controller = new AbortController()

  void (async () => {
    let res: Response
    try {
      res = await apiFetchSSE(`/sandboxes/${sandboxId}/proxy/${port}/sessions`, {
        prompt,
        stream: true,
      })
    } catch (err) {
      onError(err instanceof Error ? err : new Error(String(err)))
      return
    }

    if (!res.ok) {
      onError(new Error(`HTTP ${res.status} from session endpoint`))
      return
    }

    const body = res.body
    if (!body) {
      onError(new Error('Response body is empty'))
      return
    }

    const parser = createParser({
      onEvent(event) {
        try {
          const data = event.data === '[DONE]' ? null : (JSON.parse(event.data) as unknown)
          onEvent({ type: event.event ?? 'message', data })
        } catch {
          onEvent({ type: event.event ?? 'message', data: event.data })
        }
      },
    })

    const reader = body.getReader()
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
