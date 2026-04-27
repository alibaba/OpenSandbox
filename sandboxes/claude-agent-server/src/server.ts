import { createApp } from './app.js'
import { config } from './lib/config.js'
import { logger } from './lib/logger.js'

const app = createApp()

app.listen(config.port, config.host, () => {
  logger.info(`claude-agent-server listening on http://${config.host}:${config.port}`)
})
