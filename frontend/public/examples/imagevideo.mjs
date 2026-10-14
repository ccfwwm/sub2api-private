// Node.js 20+. Server-side only: keep API keys out of browser bundles.
import { readFile, writeFile, rename, mkdir } from 'node:fs/promises'
import { randomUUID } from 'node:crypto'
import { pathToFileURL } from 'node:url'
import { join, extname } from 'node:path'

export class Client {
  constructor(base = process.env.MEDIA_BASE_URL || 'https://code.aicodeme.xyz', key = process.env.MEDIA_API_KEY) {
    this.base = base.replace(/\/$/, '')
    this.key = key
    const u = new URL(this.base)
    if (!key || u.username || u.password || u.pathname !== '/' || u.search || u.hash || !['http:', 'https:'].includes(u.protocol)) throw new Error('Missing key or invalid site origin')
    if (u.protocol === 'http:' && !['localhost', '127.0.0.1', '[::1]'].includes(u.hostname)) throw new Error('Use HTTPS')
  }
  async request(method, path, payload, extra = {}) {
    const url = new URL(path, this.base + '/')
    if (url.origin !== this.base) throw new Error('Artifact URL must belong to the API origin')
    const headers = { Authorization: `Bearer ${this.key}`, ...extra }
    const multipart = payload instanceof FormData
    if (payload !== undefined && !multipart) headers['Content-Type'] = 'application/json'
    const r = await fetch(url, { method, headers, body: payload === undefined ? undefined : multipart ? payload : JSON.stringify(payload), redirect: 'error', signal: AbortSignal.timeout(200000) })
    if (!r.ok) throw new Error(`HTTP ${r.status}; retain task state; do not blindly resubmit`)
    return r
  }
  async json(...args) { return (await this.request(...args)).json() }
  async download(artifact, directory, index) {
    if (artifact.available === false) throw new Error('Artifact unavailable')
    const r = await this.request('GET', artifact.content_url)
    const mime = r.headers.get('content-type')?.split(';')[0].trim()
    const extensions = { 'image/png': '.png', 'image/jpeg': '.jpg', 'image/webp': '.webp', 'video/mp4': '.mp4', 'audio/mpeg': '.mp3', 'audio/mp3': '.mp3', 'audio/wav': '.wav', 'audio/x-wav': '.wav', 'audio/mp4': '.m4a', 'audio/ogg': '.ogg', 'audio/flac': '.flac', 'application/octet-stream': '.bin' }
    if (!extensions[mime]) throw new Error(`Unexpected artifact MIME: ${mime}`)
    const path = join(directory, `${index}${extensions[mime]}`)
    await writeFile(path, Buffer.from(await r.arrayBuffer()))
    return path
  }
}

async function save(path, state, exclusive = false) {
  if (exclusive) return writeFile(path, JSON.stringify(state, null, 2), { flag: 'wx' })
  await writeFile(path + '.tmp', JSON.stringify(state, null, 2))
  await rename(path + '.tmp', path)
}

export async function submit(client, endpoint, payload, statePath, images = []) {
  if (images.length) {
    if (endpoint !== '/v1/images/edits' || images.length > 2) throw new Error('Image uploads require /v1/images/edits and 1-2 files')
    const form = new FormData()
    for (const [name, value] of Object.entries(payload)) form.append(name, typeof value === 'string' ? value : JSON.stringify(value))
    for (const path of images) {
      const mime = { '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg', '.webp': 'image/webp' }[extname(path).toLowerCase()]
      if (!mime) throw new Error('Use PNG, JPEG or WebP references')
      form.append('image[]', new Blob([await readFile(path)], { type: mime }), 'reference' + extname(path))
    }
    payload = form
  }
  const state = { base: client.base, idempotency_key: randomUUID(), task_id: null, status: 'submitting' }
  await save(statePath, state, true)
  // Never automatically retry POST. State persists if the connection fails.
  const result = await client.json('POST', endpoint, payload, { 'Idempotency-Key': state.idempotency_key })
  state.task_id = result.task_id || result.id
  state.status = result.status || 'submitted'
  await save(statePath, state)
  if (typeof state.task_id !== 'string' || !state.task_id.trim()) throw new Error('Missing task ID; reconcile, do not resubmit')
  return state
}

export async function resume(client, statePath, directory, interval = 20, maxWait = 3600) {
  const state = JSON.parse(await readFile(statePath, 'utf8'))
  if (state.base !== client.base || !state.task_id) throw new Error('Origin mismatch or no task ID; reconcile')
  const deadline = Date.now() + maxWait * 1000
  const path = `/v1/tasks/${encodeURIComponent(state.task_id)}`
  while (Date.now() < deadline) {
    const task = await client.json('GET', path)
    if (task.id !== state.task_id) throw new Error('Task ID mismatch')
    state.status = task.status
    await save(statePath, state)
    if (task.status === 'completed') {
      const { artifacts } = await client.json('GET', path + '/artifacts')
      if (!artifacts.length) throw new Error('Completed task has no artifacts yet; retain ID and reconcile delivery')
      await mkdir(directory, { recursive: true })
      state.files = []
      for (const [i, artifact] of artifacts.entries()) state.files.push(await client.download(artifact, directory, i))
      await save(statePath, state)
      return state.files
    }
    if (!['queued', 'in_progress'].includes(task.status)) throw new Error(`Task ${task.status}; retain ID for reconciliation`)
    await new Promise(resolve => setTimeout(resolve, interval * 1000))
  }
  throw new Error('Polling ended; resume again using the same state file')
}

async function main() {
  const [command, ...rest] = process.argv.slice(2)
  const args = {}
  const images = []
  for (let i = 0; i < rest.length; i += 2) {
    const name = rest[i].replace(/^--/, '')
    if (name === 'image') images.push(rest[i + 1])
    else args[name] = rest[i + 1]
  }
  const client = new Client()
  const state = args.state || 'media-task.json'
  if (command === 'submit' && args.payload) {
    console.log(await submit(client, args.endpoint || '/v1/videos', JSON.parse((await readFile(args.payload, 'utf8')).replace(/^\uFEFF/, '')), state, images))
  } else if (command === 'resume') {
    const interval = Number(args.interval || 20), maxWait = Number(args['max-wait'] || 3600)
    if (!(interval > 0 && maxWait > 0)) throw new Error('interval and max-wait must be positive')
    console.log(await resume(client, state, args.output || 'media-output', interval, maxWait))
  } else throw new Error('Usage: node imagevideo.mjs submit --payload request.json --state task.json | resume --state task.json --output results')
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) main().catch(e => { console.error(e.message); process.exitCode = 1 })
