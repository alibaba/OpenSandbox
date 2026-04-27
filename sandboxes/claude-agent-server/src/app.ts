import express from 'express'
import { randomUUID } from 'node:crypto'
import { pinoHttp } from 'pino-http'

import { config } from './lib/config.js'
import { errorHandler, HttpError } from './lib/http/errors.js'
import { logger } from './lib/logger.js'
import { docsRouter } from './routes/docs.js'
import { healthRouter } from './routes/health.js'
import { sessionsRouter } from './routes/sessions.js'

export function createApp() {
  const app = express()

  app.use(express.json({ limit: '1mb' }))

  app.use(pinoHttp({
    logger,
    genReqId(req, res) {
      const id = randomUUID()
      res.setHeader('x-request-id', id)
      return id
    },
    customSuccessObject(req, res, val) {
      return {
        method: req.method,
        path: req.url,
        statusCode: res.statusCode,
        responseTimeMs: val.responseTime as number,
        requestId: req.id as string,
      }
    },
    customErrorObject(req, res, _err, val) {
      return {
        method: req.method,
        path: req.url,
        statusCode: res.statusCode,
        responseTimeMs: val.responseTime as number,
        requestId: req.id as string,
      }
    },
  }))

  // Docs routes are public — mount before auth middleware
  app.use(docsRouter)

  app.use((req, _res, next) => {
    if (!config.authToken) {
      next()
      return
    }

    const authHeader = req.header('authorization')
    if (authHeader === `Bearer ${config.authToken}`) {
      next()
      return
    }

    next(new HttpError(401, 'Unauthorized'))
  })

  app.use(healthRouter)
  app.use(sessionsRouter)

  app.use((_req, _res, next) => {
    next(new HttpError(404, 'Route not found'))
  })

  app.use(errorHandler)

  return app
}
