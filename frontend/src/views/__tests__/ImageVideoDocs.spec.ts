import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ImageVideoDocsView from '../public/ImageVideoDocsView.vue'
import { imagevideoMarkdown, imagevideoModels, imagevideoSections, sectionMarkdown, groupMarkdown } from '@/data/imagevideoDocs'

describe('Public media docs', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.removeItem('media-docs-theme')
  })
  it('uses 26 exact unique models and validates JSON and cURL JSON bodies', () => {
    expect(imagevideoModels).toHaveLength(28)
    expect(new Set(imagevideoModels.map(m => m.id)).size).toBe(28)
    expect(imagevideoModels.some(m => m.id === 'MinimaxH3-2K')).toBe(true)
    for (const section of imagevideoSections) for (const block of section.blocks) {
      if (block.kind !== 'code') continue
      if (block.language === 'json') expect(() => JSON.parse(block.text)).not.toThrow()
      if (block.label === 'cURL') {
        expect(block.text).not.toMatch(/\n\+/)
        const body = block.text.match(/-d '([\s\S]*)'$/)?.[1]
        if (body) {
          const payload = JSON.parse(body)
          const model = payload.model || payload.request?.model
          if (model) expect(imagevideoModels.map(m => m.id)).toContain(model)
        }
      }
    }
    const markdown = imagevideoMarkdown()
    expect(markdown).not.toMatch(/yunfei|v6k|doc\.yunfei|www\.v6k/i)
    expect(markdown).not.toMatch(/待验证|待渠道验证|能力待配置|不承诺|尚无成功承诺|编辑素材能力/)
    for (const section of imagevideoSections) expect(markdown).toContain(sectionMarkdown(section))
    expect(markdown).toContain('https://code.aicodeme.xyz/v1beta/models/')
    const edit = imagevideoSections.find(s => s.id === 'gpt-edit')!
    expect(sectionMarkdown(edit)).toContain('/v1/images/edits')
    expect(sectionMarkdown(edit)).toContain("-F 'image=@reference.png'")
    expect(sectionMarkdown(edit)).toContain("-F 'image[]=@scene.png'")
    expect(groupMarkdown('GPT 图片')).toContain(sectionMarkdown(edit))
    expect(imagevideoSections.some(s => s.id === 'clients')).toBe(true)
  })
  it('uses the standalone repository docs fallback without draft communication', () => {
    const w = mount(ImageVideoDocsView)
    expect(w.get('iframe').attributes('src')).toBe('/media-docs/index.html')
    expect(w.get('iframe').attributes('src')).not.toContain('preview')
    w.unmount()
  })
})
