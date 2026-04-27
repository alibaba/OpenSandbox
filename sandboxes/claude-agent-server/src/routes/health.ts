import { Router } from 'express'

import { config } from '../lib/config.js'

export const healthRouter = Router()

healthRouter.get('/health', (_req, res) => {
  res.json({
    healthy: true,
    service: 'claude-agent-server',
    host: config.host,
    port: config.port,
    timestamp: new Date().toISOString(),
  })
})
