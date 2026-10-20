import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { resolve, sep, extname, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const content = () => readFile(resolve(root,'content.json')).catch(() => readFile(resolve(root,'../docs/multimodal/imagevideo-content.json')))
const mime = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.json': 'application/json; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.py': 'text/plain; charset=utf-8',
}

createServer(async (request, response) => {
  if (!['GET','HEAD'].includes(request.method)) {response.writeHead(405,{'allow':'GET, HEAD'}).end();return}
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
  if (pathname === '/api/v1/public-docs/imagevideo') {
    try {
      const sections=JSON.parse(await content())
      const body=JSON.stringify({code:0,data:{slug:'imagevideo',version:13,content:sections}})
      response.writeHead(200,{'content-type':mime['.json'],'cache-control':'no-cache','x-content-type-options':'nosniff'})
      response.end(request.method==='HEAD'?undefined:body)
    }catch{response.writeHead(503).end()}
    return
  }
  // Nginx strips /docs/; direct container checks retain it.
  pathname=pathname.replace(/^\/docs\//,'/')
  if (pathname === '/imagevideo') pathname='/index.html'
  if (pathname === '/imagevideo/') {response.writeHead(302,{'location':'/docs/imagevideo','cache-control':'no-store'}).end();return}
  const file = resolve(root, `.${pathname}`)
  if (file !== root && !file.startsWith(root + sep)) {
    response.writeHead(404).end('not found\n')
    return
  }

  try {
    const body = pathname==='/content.json' ? await content() : await readFile(file === root || pathname.endsWith('/') ? `${file.replace(/\/$/, '')}/index.html` : file).catch(error=>{
      if(['/examples/imagevideo.py','/examples/imagevideo.mjs'].includes(pathname))return readFile(resolve(root,'../frontend/public'+pathname))
      throw error
    })
    response.writeHead(200, {
      'content-type': mime[extname(file)] || 'application/octet-stream',
      'cache-control': 'no-store',
      'x-content-type-options': 'nosniff',
      'referrer-policy': 'strict-origin-when-cross-origin',
    })
    response.end(request.method==='HEAD'?undefined:body)
  } catch {
    response.writeHead(404).end('not found\n')
  }
}).listen(Number(process.env.PORT || 8080), '0.0.0.0')
