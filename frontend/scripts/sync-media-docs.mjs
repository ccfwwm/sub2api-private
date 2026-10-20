// The standalone docs and development fallback use exactly the same assets.
import { mkdir, copyFile } from 'node:fs/promises'
const root = new URL('../../',import.meta.url)
const target = new URL('frontend/public/media-docs/',root)
await mkdir(new URL('examples/',target),{recursive:true})
for(const name of ['index.html','app.mjs','styles.css'])await copyFile(new URL('docs-web/'+name,root),new URL(name,target))
await copyFile(new URL('docs/multimodal/imagevideo-content.json',root),new URL('content.json',target))
for(const name of ['imagevideo.py','imagevideo.mjs'])await copyFile(new URL('frontend/public/examples/'+name,root),new URL('examples/'+name,target))
