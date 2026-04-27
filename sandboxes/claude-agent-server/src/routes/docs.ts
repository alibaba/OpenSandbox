import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { Router } from 'express'
import swaggerUi from 'swagger-ui-express'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

// Resolve path works for both ts (src/routes/) and compiled js (dist/routes/)
const docPath = join(__dirname, '../../docs/openapi.json')
const spec = JSON.parse(readFileSync(docPath, 'utf8')) as Record<string, unknown>

export const docsRouter = Router()

// Raw OpenAPI spec
docsRouter.get('/openapi.json', (_req, res) => {
  res.json(spec)
})

// Swagger UI
docsRouter.use('/docs', swaggerUi.serve)
docsRouter.get('/docs', swaggerUi.setup(spec, { explorer: false }))
