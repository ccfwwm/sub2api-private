import seedSections from '../../../docs/multimodal/imagevideo-content.json'
import pythonClient from '../../public/examples/imagevideo.py?raw'
import javascriptClient from '../../public/examples/imagevideo.mjs?raw'

/** Public media documentation metadata. The reviewed JSON is the single source
 * used by the independent docs container and the frontend fallback. */
export const docsTitle = '词元互联图片、视频与音乐接口文档'
export const docsVersion = 'v1.2.15 · 2026-10-21'
export const apiOrigin = 'https://code.aicodeme.xyz'

export type DocBlock =
  | { kind: 'heading'; text: string }
  | { kind: 'endpoint'; method: string; path: string }
  | { kind: 'text'; text: string }
  | { kind: 'code'; language: string; text: string; label: string }
  | { kind: 'table'; headers: string[]; rows: string[][] }
export interface DocSection { id: string; group: string; navTitle: string; title: string; subtitle: string; blocks: DocBlock[] }

export const imagevideoSections = seedSections as DocSection[]

const modelTable = imagevideoSections.find(section => section.id === 'models')?.blocks.find(
  block => block.kind === 'table' && block.headers[0] === '模型 ID'
) as Extract<DocBlock, { kind: 'table' }> | undefined

export const imagevideoModels: { id: string; type: string; limits: string }[] = (modelTable?.rows ?? []).map(row => ({
  id: row[0],
  type: row[1],
  limits: row[2]
}))

export function sectionMarkdown(section: DocSection): string {
  return `## ${section.title}\n\n${section.subtitle}\n\n` + section.blocks.map(block => {
    if (block.kind === 'text') return block.text
    if (block.kind === 'heading') return `### ${block.text}`
    if (block.kind === 'endpoint') return `**${block.method}** ${apiOrigin}${block.path}`
    if (block.kind === 'code') return `### ${block.label}\n\n\`\`\`${block.language}\n${block.text}\n\`\`\``
    const row = (cells: string[]) => '| ' + cells.map(cell => cell.replace(/\|/g, '\\|').replace(/\n/g, '<br>')).join(' | ') + ' |'
    return [row(block.headers), row(block.headers.map(() => '---')), ...block.rows.map(row)].join('\n')
  }).join('\n\n')
}

export const imagevideoMarkdown = (sections: DocSection[] = imagevideoSections, version = docsVersion) =>
  `# ${docsTitle}\n\n版本：${version}\n\n根地址：${apiOrigin}\n\n` + sections.map(sectionMarkdown).join('\n\n')

export const imagevideoGroups = [...new Set(imagevideoSections.map(section => section.group))]
  .map(title => ({ title, sections: imagevideoSections.filter(section => section.group === title) }))
export const groupMarkdown = (title: string) => imagevideoSections.filter(section => section.group === title).map(sectionMarkdown).join('\n\n')

// Examples are included in the export source so copied documentation
// and downloadable files always use the same client implementation.
export { pythonClient, javascriptClient }
