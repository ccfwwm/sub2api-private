// The standalone docs and development fallback use exactly the same assets.
import { mkdir, copyFile, readFile, writeFile } from 'node:fs/promises'
const root = new URL('../../',import.meta.url)
const target = new URL('frontend/public/media-docs/',root)
await mkdir(new URL('examples/',target),{recursive:true})
const html = await readFile(new URL('docs-web/index.html',root),'utf8')
await writeFile(new URL('index.html',target),html.replace('href="/docs/styles.css"','href="./styles.css"').replace('src="/docs/app.mjs"','src="./app.mjs"'))
for(const name of ['app.mjs','styles.css'])await copyFile(new URL('docs-web/'+name,root),new URL(name,target))
await copyFile(new URL('docs/multimodal/imagevideo-content.json',root),new URL('content.json',target))
await copyFile(new URL('docs-web/general-content.json',root),new URL('general-content.json',target))
await copyFile(new URL('docs-web/knowledge-content.json',root),new URL('knowledge-content.json',target))
for(const name of ['imagevideo.py','imagevideo.mjs'])await copyFile(new URL('frontend/public/examples/'+name,root),new URL('examples/'+name,target))
