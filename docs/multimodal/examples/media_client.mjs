// Node >=20. Single generation POST; persistent state interoperates with Python.
import { open, readFile, rename, unlink, stat } from 'node:fs/promises';
import { createHash, randomUUID } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';

class ApiError extends Error {
  constructor(status = 0, retryAfter = 0, code = 'transport_or_invalid_response') {
    super(`API ${code}; HTTP ${status}; keep state, never blindly resubmit`);
    Object.assign(this, { status, retryAfter, code });
  }
}
function canonical(value) {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === 'object') return Object.fromEntries(Object.keys(value).sort().map(k => [k, canonical(value[k])]));
  return value;
}
function encode(value) { return JSON.stringify(canonical(value)); }
function retryAfter(value) {
  if (!value) return 0;
  const n = Number(value);
  return Number.isFinite(n) ? Math.max(0, n) : Math.max(0, (Date.parse(value) - Date.now()) / 1000) || 0;
}
async function atomicJson(path, value, exclusive = false) {
  const temp = exclusive ? path : `${path}.${randomUUID()}.tmp`;
  const f = await open(temp, 'wx', 0o600);
  try { await f.writeFile(JSON.stringify(value, null, 2)); await f.sync(); } finally { await f.close(); }
  if (!exclusive) {
    try { await rename(temp, path); } finally { await unlink(temp).catch(() => {}); }
  }
}
async function locked(path, fn) {
  const lock = `${path}.lock`;
  const f = await open(lock, 'wx', 0o600);
  try { await f.writeFile(String(process.pid)); return await fn(); }
  finally { await f.close(); await unlink(lock); }
}
async function payload(path) {
  const value = JSON.parse((await readFile(path, 'utf8')).replace(/^\uFEFF/, ''));
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Payload must be an object');
  return value;
}
class Client {
  constructor() {
    this.base = (process.env.SUB2API_BASE_URL || '').replace(/\/+$/, '');
    const url = new URL(this.base);
    if (!['http:', 'https:'].includes(url.protocol) || url.pathname !== '/' || url.search || url.hash || url.username || url.password) throw new Error('Base must be origin without /v1 or credentials');
    if (url.protocol === 'http:' && !['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)) throw new Error('HTTPS required');
    this.key = process.env.SUB2API_API_KEY;
    if (!this.key?.trim()) throw new Error('Missing API key');
  }
  async response(method, path, body, extra = {}) {
    let r;
    try {
      r = await fetch(this.base + path, { method, body, headers: { Authorization: `Bearer ${this.key}`, ...extra }, redirect: 'manual', signal: AbortSignal.timeout(120000) });
    } catch { throw new ApiError(); }
    if (!r.ok) {
      await r.body?.cancel();
      throw new ApiError(r.status, retryAfter(r.headers.get('Retry-After')), 'http_error');
    }
    return r;
  }
  async json(method, path, data, extra = {}, raw) {
    if (data !== undefined) { raw = encode(data); extra = { ...extra, 'Content-Type': 'application/json' }; }
    const r = await this.response(method, path, raw, extra);
    const mime = (r.headers.get('Content-Type') || '').split(';')[0].trim().toLowerCase();
    if (mime !== 'application/json' && !mime.endsWith('+json')) { await r.body?.cancel(); throw new ApiError(r.status, 0, 'expected_json'); }
    const chunks = []; let size = 0;
    for await (const chunk of r.body) {
      size += chunk.length;
      if (size > 64 * 1024 * 1024) throw new ApiError(r.status, 0, 'json_too_large');
      chunks.push(chunk);
    }
    let value;
    try { value = JSON.parse(Buffer.concat(chunks).toString('utf8')); } catch { throw new ApiError(r.status, 0, 'invalid_json'); }
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new ApiError(r.status, 0, 'invalid_object');
    return value;
  }
  async download(path, output, kind) {
    const temp = `${output}.${randomUUID()}.part`;
    let file;
    try {
      const r = await this.response('GET', path);
      const mime = (r.headers.get('Content-Type') || '').split(';')[0].toLowerCase();
      const allowed = { video: ['video/mp4', 'application/octet-stream'], audio: ['audio/mpeg', 'audio/mp3', 'audio/wav', 'audio/x-wav', 'audio/mp4', 'audio/x-m4a', 'application/octet-stream'] };
      if (r.status !== 200 || !allowed[kind].includes(mime)) { await r.body?.cancel(); throw new ApiError(r.status, 0, 'expected_complete_media'); }
      file = await open(temp, 'wx', 0o600);
      let size = 0, prefix = Buffer.alloc(0);
      const hash = createHash('sha256');
      for await (const chunk of r.body) {
        size += chunk.length;
        if (size > 256 * 1024 * 1024) throw new ApiError(r.status, 0, 'download_limit');
        if (prefix.length < 16) prefix = Buffer.concat([prefix, chunk]).subarray(0, 16);
        hash.update(chunk);
        await file.writeFile(chunk);
      }
      const isMp4 = prefix.subarray(4, 8).toString() === 'ftyp';
      const valid = kind === 'video' ? isMp4 : isMp4 || ['ID3', 'RIFF'].some(x => prefix.subarray(0, x.length).toString() === x) || (prefix[0] === 255 && (prefix[1] & 224) === 224);
      if (!valid) throw new ApiError(r.status, 0, 'invalid_media_signature');
      const length = r.headers.get('Content-Length');
      if (length !== null && Number(length) !== size) throw new ApiError(r.status, 0, 'truncated_media');
      await file.sync(); await file.close(); file = undefined;
      await rename(temp, output);
      return { bytes: size, sha256: hash.digest('hex') };
    } finally { await file?.close(); await unlink(temp).catch(() => {}); }
  }
}
function parse() {
  const [command, ...tokens] = process.argv.slice(2);
  const args = { command, image: [] };
  const names = new Set(['payload', 'state', 'kind', 'output', 'model', 'endpoint', 'image', 'max-wait', 'poll-interval']);
  for (let i = 0; i < tokens.length; i += 2) {
    const key = tokens[i].replace(/^--/, '');
    if (!tokens[i].startsWith('--') || !names.has(key) || tokens[i + 1] === undefined) throw new Error('Invalid arguments');
    if (key === 'image') args.image.push(tokens[i + 1]); else args[key] = tokens[i + 1];
  }
  return args;
}
function required(args, ...names) { for (const n of names) if (!args[n]) throw new Error(`Missing --${n}`); }
async function submit(client, args) {
  required(args, 'payload', 'state');
  const data = await payload(args.payload), kind = args.kind || 'video';
  if (!['video', 'audio'].includes(kind)) throw new Error('kind video/audio required');
  const endpoint = kind === 'video' ? '/v1/videos' : '/v1/audio/generations';
  await locked(args.state, async () => {
    const state = { version: 1, base: client.base, kind, endpoint, request_hash: createHash('sha256').update(encode(data)).digest('hex'), idempotency_key: randomUUID(), task_id: null, status: 'pending_submission' };
    await atomicJson(args.state, state, true);
    try {
      const task = await client.json('POST', endpoint, data, { 'Idempotency-Key': state.idempotency_key });
      if (typeof task.id !== 'string' || !task.id.trim()) throw new ApiError(0, 0, 'missing_task_id');
      state.task_id = task.id;
      state.status = ['queued', 'in_progress', 'completed', 'failed', 'unknown'].includes(task.status) ? task.status : 'unknown';
      await atomicJson(args.state, state);
    } catch (e) {
      state.status = e instanceof ApiError && e.status >= 400 && e.status < 500 && e.status !== 408 ? 'rejected' : 'unknown';
      await atomicJson(args.state, state); throw e;
    }
    console.log(JSON.stringify({ id: state.task_id, status: state.status }));
    if (state.status === 'failed') throw new ApiError(0, 0, 'task_failed');
  });
}
async function resume(client, args) {
  required(args, 'state', 'output');
  const wait = Number(args['max-wait'] || 1200), poll = Number(args['poll-interval'] || 20);
  if (!Number.isFinite(wait) || wait <= 0 || !Number.isFinite(poll) || poll <= 0 || poll > 60) throw new Error('Invalid polling bounds');
  await locked(args.state, async () => {
    const state = await payload(args.state);
    if (state.version !== 1 || state.base !== client.base || !['audio', 'video'].includes(state.kind)) throw new Error('State version/base/kind mismatch');
    if (typeof state.task_id !== 'string' || !state.task_id.trim()) throw new Error('No task ID; inspect pending submission, never resubmit');
    if (state.status === 'failed') throw new ApiError(0, 0, 'task_failed');
    const root = state.kind === 'video' ? '/v1/videos/' : '/v1/audio/generations/';
    const path = root + encodeURIComponent(state.task_id);
    const deadline = Date.now() + wait * 1000;
    let interval = poll, nextAllowed = Date.now();
    while (Date.now() < deadline) {
      if (nextAllowed > Date.now()) { await delay(Math.min(60000, nextAllowed - Date.now(), Math.max(0, deadline - Date.now()))); continue; }
      try {
        const task = await client.json('GET', path);
        if (task.id !== state.task_id) throw new ApiError(0, 0, 'task_id_mismatch');
        state.status = ['queued', 'in_progress', 'completed', 'failed', 'unknown'].includes(task.status) ? task.status : 'unknown';
        await atomicJson(args.state, state);
        if (state.status === 'failed') throw new ApiError(0, 0, 'task_failed');
        if (state.status === 'completed') {
          state.download = await client.download(path + '/content', args.output, state.kind);
          await atomicJson(args.state, state);
          console.log(JSON.stringify({ id: state.task_id, status: state.status, ...state.download })); return;
        }
        interval = poll;
      } catch (e) {
        if (!(e instanceof ApiError) || ![0, 429, 502, 503, 504].includes(e.status) || ['task_failed', 'task_id_mismatch'].includes(e.code)) throw e;
        interval = Math.min(60, Math.max(interval * 2, e.retryAfter));
        nextAllowed = Date.now() + Math.max(interval, e.retryAfter) * 1000; continue;
      }
      nextAllowed = Date.now() + interval * 1000;
    }
    console.log('Polling window ended; ID preserved. Resume again; no generation POST was sent.');
  });
}
async function edit(client, args) {
  required(args, 'payload', 'output');
  if (!args.image.length) throw new Error('Need --image');
  const form = new FormData(), data = await payload(args.payload);
  for (const [key, value] of Object.entries(data)) {
    if (!['model', 'prompt', 'n', 'size', 'quality', 'response_format'].includes(key)) throw new Error('Unsupported edit field');
    form.append(key, String(value));
  }
  let total = 0;
  for (const [index, file] of args.image.entries()) {
    total += (await stat(file)).size;
    if (total > 64 * 1024 * 1024) throw new Error('Example upload cap 64 MiB; real model cap may be lower');
    const ext = file.split('.').at(-1).toLowerCase();
    const mime = { png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', webp: 'image/webp' }[ext];
    if (!mime) throw new Error('Upload PNG/JPEG/WebP');
    form.append('image[]', new Blob([await readFile(file)], { type: mime }), `reference-${index}.${ext}`);
  }
  const result = await client.json('POST', '/v1/images/edits', undefined, {}, form);
  await atomicJson(args.output, result);
  console.log('Response saved. Signed URLs are private. No automatic POST retries.');
}
async function main() {
  const args = parse(), client = new Client();
  if (args.command === 'submit') return submit(client, args);
  if (args.command === 'resume') return resume(client, args);
  if (args.command === 'edit') return edit(client, args);
  if (args.command === 'models') { console.log(JSON.stringify(await client.json('GET', '/v1/models'))); return; }
  if (args.command === 'estimate') { required(args, 'payload'); console.log(JSON.stringify(await client.json('POST', '/v1/sub2api/estimates', await payload(args.payload)))); return; }
  if (args.command === 'call') {
    required(args, 'endpoint', 'payload', 'output');
    if (!['images/generations', 'chat/completions', 'gemini'].includes(args.endpoint)) throw new Error('Invalid endpoint');
    if (args.endpoint === 'gemini') required(args, 'model');
    const path = args.endpoint === 'gemini' ? `/v1beta/models/${encodeURIComponent(args.model)}:generateContent` : `/v1/${args.endpoint}`;
    const result = await client.json('POST', path, await payload(args.payload));
    await atomicJson(args.output, result);
    console.log('Response saved. Signed URLs are private. No automatic POST retries.'); return;
  }
  throw new Error('Commands: models, estimate, submit, resume, call, edit. See README.md for flags.');
}
main().catch(e => { console.error(e instanceof ApiError ? e.message : `Client stopped (${e.name}); inspect inputs/state. No automatic resubmission.`); process.exitCode = 2; });
