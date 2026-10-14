// Export from the exact data source rendered by /docs/imagevideo.
// Run from frontend: node scripts/export-imagevideo-docs.mjs [output.md]
import { createServer } from 'vite'
import { mkdir, writeFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { createServer as createHttpServer } from 'node:http'

const output = resolve(process.argv[2] || '../output/imagevideo/接口调用文档.md')
// Attach HMR to an unbound server so an active dev preview keeps its port.
const server = await createServer({ configFile: false, cacheDir: resolve('../output/imagevideo/vite-export-cache'), server: { middlewareMode: true, hmr: { server: createHttpServer() } }, optimizeDeps: { noDiscovery: true, include: [] } })
try {
  const { imagevideoMarkdown } = await server.ssrLoadModule('/src/data/imagevideoDocs.ts')
  await mkdir(dirname(output), { recursive: true })
  await writeFile(output, imagevideoMarkdown(), 'utf8')
  console.log(output)
} finally {
  await server.close()
}
