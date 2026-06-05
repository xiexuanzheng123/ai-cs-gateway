#!/usr/bin/env node
import { execFileSync } from 'node:child_process'

const defaults = {
  remoteHost: 'pc-2ze5525j3u9xw1vy0.rwlb.rds.aliyuncs.com',
  remotePort: '3306',
  remoteUser: 'sobot_ai',
  remotePassword: 'sobot_ai_202696eF',
  remoteDatabase: 'sobot_ai',
  gatewayBaseUrl: 'http://localhost:8080',
  limit: 10,
  offset: 0,
  batchSize: 100,
  ids: '',
}

const args = parseArgs(process.argv.slice(2))
const config = { ...defaults, ...args }

const existingKnowledge = await fetchKnowledgeMap(config.gatewayBaseUrl)
const summary = {
  total: 0,
  created: 0,
  updated: 0,
  failed: 0,
  failures: [],
}

let currentOffset = Number(config.offset) || 0
const idList = String(config.ids || '')
  .split(',')
  .map((item) => Number(item.trim()))
  .filter((item) => Number.isInteger(item) && item > 0)
const endOffset = currentOffset + (idList.length > 0 ? idList.length : Number(config.limit) || 10)
const batchSize = Math.max(1, Math.min(Number(config.batchSize) || 100, 500))

while (currentOffset < endOffset) {
  const limit = Math.min(batchSize, endOffset - currentOffset)
  const batchIds = idList.length > 0 ? idList.slice(currentOffset, currentOffset + limit) : []
  const remoteDocuments = fetchRemoteDocuments({ ...config, limit, offset: currentOffset, ids: batchIds.join(',') })
  if (remoteDocuments.length === 0) break
  console.log(`[batch] offset=${currentOffset} size=${remoteDocuments.length}`)
  for (const document of remoteDocuments) {
    summary.total++
    const input = mapDocumentToKnowledgeInput(document)
    const existing = existingKnowledge.get(input.knowledge_id)
    try {
      if (existing) {
        await callGateway(`${config.gatewayBaseUrl}/api/customer-service/admin/knowledge/${existing.id}`, {
          method: 'PUT',
          body: input,
        })
        summary.updated++
      } else {
        const created = await callGateway(`${config.gatewayBaseUrl}/api/customer-service/admin/knowledge`, {
          method: 'POST',
          body: input,
        })
        existingKnowledge.set(input.knowledge_id, created)
        summary.created++
      }
      console.log(`[ok] ${input.knowledge_id} ${input.title}`)
    } catch (error) {
      summary.failed++
      summary.failures.push({
        document_id: document.id,
        knowledge_id: input.knowledge_id,
        error: error instanceof Error ? error.message : String(error),
      })
      console.error(`[failed] ${input.knowledge_id} ${summary.failures.at(-1).error}`)
    }
  }
  currentOffset += remoteDocuments.length
  if (remoteDocuments.length < limit) break
}

console.log(JSON.stringify(summary, null, 2))

function parseArgs(argv) {
  const out = {}
  for (const arg of argv) {
    if (!arg.startsWith('--')) continue
    const [rawKey, rawValue = ''] = arg.slice(2).split('=')
    const key = rawKey.replace(/-([a-z])/g, (_, char) => char.toUpperCase())
    out[key] = rawValue
  }
  if (out.limit !== undefined) out.limit = Number(out.limit)
  if (out.offset !== undefined) out.offset = Number(out.offset)
  if (out.batchSize !== undefined) out.batchSize = Number(out.batchSize)
  return out
}

function fetchRemoteDocuments(config) {
  const ids = String(config.ids || '')
    .split(',')
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isInteger(item) && item > 0)
  const idCondition = ids.length > 0 ? `  AND id IN (${ids.join(',')})\n` : ''
  const sql = `
SELECT JSON_OBJECT(
  'id', id,
  'title', COALESCE(NULLIF(TRIM(title), ''), NULLIF(TRIM(question), '')),
  'question', question,
  'answer', answer,
  'category', COALESCE(NULLIF(TRIM(category), ''), 'general'),
  'owner', COALESCE(NULLIF(TRIM(creator), ''), NULLIF(TRIM(applicant), ''), NULLIF(TRIM(created_by), ''), 'remote_sync')
) AS payload
FROM documents
WHERE deleted_at IS NULL
  AND review_status = 1
  AND COALESCE(TRIM(question), '') <> ''
  AND COALESCE(TRIM(answer), '') <> ''
${idCondition}
ORDER BY updated_at DESC, id DESC
LIMIT ${Number(config.limit) || 10}
OFFSET ${Number(config.offset) || 0};`

  const stdout = execFileSync(
    'docker',
    [
      'exec',
      '-i',
      'ai-cs-mysql',
      'mysql',
      '--default-character-set=utf8mb4',
      '--batch',
      '--raw',
      '--skip-column-names',
      '-h',
      config.remoteHost,
      '-P',
      String(config.remotePort),
      '-u',
      config.remoteUser,
      `-p${config.remotePassword}`,
      config.remoteDatabase,
      '-e',
      sql,
    ],
    { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] },
  )

  return stdout
    .trim()
    .split('\n')
    .filter(Boolean)
    .map((line) => JSON.parse(line))
}

async function fetchKnowledgeMap(gatewayBaseUrl) {
  const payload = await callGateway(`${gatewayBaseUrl}/api/customer-service/admin/knowledge`)
  const map = new Map()
  for (const item of payload.knowledge ?? []) {
    map.set(item.knowledge_id, item)
  }
  return map
}

function mapDocumentToKnowledgeInput(document) {
  return {
    knowledge_id: `kb_doc_${document.id}`,
    title: document.title || document.question,
    content: document.answer,
    category: document.category || 'general',
    owner: document.owner || 'remote_sync',
    version: 'v1',
    status: 'published',
  }
}

async function callGateway(url, options = {}) {
  const response = await fetch(url, {
    method: options.method || 'GET',
    headers: {
      'Content-Type': 'application/json',
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  })
  if (!response.ok) {
    const text = await response.text()
    throw new Error(`${response.status} ${text}`)
  }
  return response.json()
}
