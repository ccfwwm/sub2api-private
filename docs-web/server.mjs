import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { resolve, sep } from 'node:path'

const root = '/app'
const mime = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
}

createServer(async (request, response) => {
  if (request.url === '/health') {
    response.writeHead(200, { 'content-type': 'text/plain; charset=utf-8', 'cache-control': 'no-store' })
    response.end('ok\n')
    return
  }

  let pathname
  try {
    pathname = decodeURIComponent(new URL(request.url || '/', 'http://localhost').pathname)
  } catch {
    response.writeHead(400).end('bad request\n')
    return
  }
  const file = resolve(root, `.${pathname}`)
  if (file !== root && !file.startsWith(root + sep)) {
    response.writeHead(404).end('not found\n')
    return
  }

  try {
    const body = await readFile(file === root || pathname.endsWith('/') ? `${file.replace(/\/$/, '')}/index.html` : file)
    response.writeHead(200, {
      'content-type': mime[file.slice(file.lastIndexOf('.'))] || 'application/octet-stream',
      'cache-control': 'no-store',
      'x-content-type-options': 'nosniff',
      'referrer-policy': 'strict-origin-when-cross-origin',
    })
    response.end(body)
  } catch {
    if (request.method === 'GET' || request.method === 'HEAD') {
      try {
        const body = await readFile(`${root}/index.html`)
        response.writeHead(200, { 'content-type': mime['.html'], 'cache-control': 'no-store', 'x-content-type-options': 'nosniff' })
        response.end(body)
      } catch {
        response.writeHead(404).end('not found\n')
      }
      return
    }
    response.writeHead(404).end('not found\n')
  }
}).listen(8080, '0.0.0.0')
