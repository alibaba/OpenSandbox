import { Router } from 'express'

import {
  abortSession,
  createSessionBodySchema,
  execute,
  type ExecutePromptInput,
  forkSessionBodySchema,
  forkStoredSession,
  getMessagesQuerySchema,
  getSessionAgents,
  getSessionCommands,
  getSessionContext,
  getSessionModels,
  getStoredMessages,
  getStoredSession,
  listSessionsQuerySchema,
  listStoredSessions,
  patchModelBodySchema,
  patchPermissionModeBodySchema,
  patchSessionBodySchema,
  rewindSessionBodySchema,
  rewindSessionFiles,
  sdkSessionInfoToResponse,
  sendMessageBodySchema,
  sessionMessageToResponse,
  setSessionModel,
  setSessionPermissionMode,
  updateStoredSession,
} from '../lib/claude/session-service.js'
import { asyncHandler, HttpError } from '../lib/http/errors.js'
import { closeSse, openSse, requestAbortSignal, writeSseError, writeSseEvent } from '../lib/http/sse.js'

export const sessionsRouter = Router()

function sessionIdParam(value: string | string[] | undefined) {
  if (typeof value !== 'string' || value.length === 0) {
    throw new Error('Missing sessionId route parameter')
  }

  return value
}

function promptInput(
  value: {
    prompt: string
    includePartialMessages?: boolean | undefined
    options?: ExecutePromptInput['options']
  },
  extra: Record<string, unknown> = {},
): ExecutePromptInput {
  return {
    ...extra,
    prompt: value.prompt,
    ...(value.includePartialMessages !== undefined
      ? { includePartialMessages: value.includePartialMessages }
      : {}),
    ...(value.options !== undefined ? { options: value.options } : {}),
  }
}

sessionsRouter.get(
  '/sessions',
  asyncHandler(async (req, res) => {
    const input = listSessionsQuerySchema.parse(req.query)
    const sessions = await listStoredSessions(input)

    res.json({
      sessions: sessions.map(sdkSessionInfoToResponse),
    })
  }),
)

sessionsRouter.post(
  '/sessions',
  asyncHandler(async (req, res) => {
    const input = createSessionBodySchema.parse(req.body)

    if (input.stream) {
      openSse(res)

      try {
        const result = await execute(
          { ...promptInput(input), signal: requestAbortSignal(req, res) },
          (event) => writeSseEvent(res, event),
        )

        writeSseEvent(res, {
          event: 'session.completed',
          data: {
            sessionId: result.sessionId,
            subtype: result.result?.subtype ?? null,
          },
        })
      } catch (error) {
        writeSseError(res, error)
      } finally {
        closeSse(res)
      }

      return
    }

    const result = await execute(promptInput(input))

    res.status(201).json({
      sessionId: result.sessionId,
      result: result.result,
      events: result.events,
    })
  }),
)

sessionsRouter.get(
  '/sessions/:sessionId',
  asyncHandler(async (req, res) => {
    const session = await getStoredSession(sessionIdParam(req.params.sessionId), req.query.dir as string | undefined)

    res.json({
      session: {
        ...sdkSessionInfoToResponse(session),
        runtime: session.runtime,
      },
    })
  }),
)

sessionsRouter.patch(
  '/sessions/:sessionId',
  asyncHandler(async (req, res) => {
    const input = patchSessionBodySchema.parse(req.body)
    const session = await updateStoredSession(sessionIdParam(req.params.sessionId), input)

    res.json({
      session: {
        ...sdkSessionInfoToResponse(session),
        runtime: session.runtime,
      },
    })
  }),
)

sessionsRouter.get(
  '/sessions/:sessionId/messages',
  asyncHandler(async (req, res) => {
    const input = getMessagesQuerySchema.parse(req.query)
    const messages = await getStoredMessages(sessionIdParam(req.params.sessionId), input)

    res.json({
      messages: messages.map(sessionMessageToResponse),
    })
  }),
)

sessionsRouter.post(
  '/sessions/:sessionId/messages',
  asyncHandler(async (req, res) => {
    const input = sendMessageBodySchema.parse(req.body)
    const sessionId = sessionIdParam(req.params.sessionId)

    if (input.stream) {
      openSse(res)

      try {
        const result = await execute(
          {
            ...promptInput(input, {
              sessionId,
              ...(input.forkSession !== undefined ? { forkSession: input.forkSession } : {}),
            }),
            signal: requestAbortSignal(req, res),
          },
          (event) => writeSseEvent(res, event),
        )

        writeSseEvent(res, {
          event: 'session.completed',
          data: {
            sessionId: result.sessionId,
            subtype: result.result?.subtype ?? null,
          },
        })
      } catch (error) {
        writeSseError(res, error)
      } finally {
        closeSse(res)
      }

      return
    }

    const result = await execute(
      promptInput(input, {
        sessionId,
        ...(input.forkSession !== undefined ? { forkSession: input.forkSession } : {}),
      }),
    )

    res.json({
      sessionId: result.sessionId,
      result: result.result,
      events: result.events,
    })
  }),
)

sessionsRouter.post(
  '/sessions/:sessionId/abort',
  asyncHandler(async (req, res) => {
    const result = await abortSession(sessionIdParam(req.params.sessionId))
    res.json(result)
  }),
)

sessionsRouter.post(
  '/sessions/:sessionId/fork',
  asyncHandler(async (req, res) => {
    const input = forkSessionBodySchema.parse(req.body)
    const session = await forkStoredSession(sessionIdParam(req.params.sessionId), input)

    res.status(201).json({
      session: {
        ...sdkSessionInfoToResponse(session),
        runtime: session.runtime,
      },
    })
  }),
)

// ---------------------------------------------------------------------------
// File rewind (checkpointing)
// Requires the session to have been started with options.enableFileCheckpointing=true
// ---------------------------------------------------------------------------

sessionsRouter.post(
  '/sessions/:sessionId/rewind',
  asyncHandler(async (req, res) => {
    const input = rewindSessionBodySchema.parse(req.body)
    const result = await rewindSessionFiles(sessionIdParam(req.params.sessionId), input)
    res.json(result)
  }),
)

// ---------------------------------------------------------------------------
// Active-session introspection
// All endpoints below require a live active run (409 if idle, 404 if unknown)
// ---------------------------------------------------------------------------

sessionsRouter.get(
  '/sessions/:sessionId/commands',
  asyncHandler(async (req, res) => {
    const result = await getSessionCommands(sessionIdParam(req.params.sessionId))
    res.json(result)
  }),
)

sessionsRouter.get(
  '/sessions/:sessionId/models',
  asyncHandler(async (req, res) => {
    const result = await getSessionModels(sessionIdParam(req.params.sessionId))
    res.json(result)
  }),
)

sessionsRouter.get(
  '/sessions/:sessionId/agents',
  asyncHandler(async (req, res) => {
    const result = await getSessionAgents(sessionIdParam(req.params.sessionId))
    res.json(result)
  }),
)

sessionsRouter.get(
  '/sessions/:sessionId/context',
  asyncHandler(async (req, res) => {
    const result = await getSessionContext(sessionIdParam(req.params.sessionId))
    res.json(result)
  }),
)

sessionsRouter.get(
  '/sessions/:sessionId/subagents/:agentId/messages',
  asyncHandler(async (req, res) => {
    const input = getMessagesQuerySchema.parse(req.query)
    const messages = await getStoredMessages(sessionIdParam(req.params.agentId), input)
    res.json({
      messages: messages.map(sessionMessageToResponse),
    })
  }),
)

sessionsRouter.patch(
  '/sessions/:sessionId/model',
  asyncHandler(async (req, res) => {
    const input = patchModelBodySchema.parse(req.body)
    const result = await setSessionModel(sessionIdParam(req.params.sessionId), input.model)
    res.json(result)
  }),
)

sessionsRouter.patch(
  '/sessions/:sessionId/permission-mode',
  asyncHandler(async (req, res) => {
    const input = patchPermissionModeBodySchema.parse(req.body)
    const result = await setSessionPermissionMode(
      sessionIdParam(req.params.sessionId),
      input.permissionMode,
    )
    res.json(result)
  }),
)

// ---------------------------------------------------------------------------
// Subagent enumeration (no SDK API — remains 501)
// ---------------------------------------------------------------------------

sessionsRouter.get(
  '/sessions/:sessionId/subagents',
  asyncHandler(async () => {
    throw new HttpError(501, 'Subagent listing is not yet implemented')
  }),
)
