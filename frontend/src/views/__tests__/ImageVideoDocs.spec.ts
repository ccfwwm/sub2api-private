import { describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import ImageVideoDocsView from '../public/ImageVideoDocsView.vue'
import { imagevideoMarkdown, imagevideoModels, imagevideoSections, sectionMarkdown } from '@/data/imagevideoDocs'

describe('Public media docs', () => {
  it('uses 26 exact unique models and validates JSON and cURL JSON bodies', () => {
    expect(imagevideoModels).toHaveLength(26)
    expect(new Set(imagevideoModels.map(m => m.id)).size).toBe(26)
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
    for (const section of imagevideoSections) expect(markdown).toContain(sectionMarkdown(section))
    expect(markdown).toContain('https://code.aicodeme.xyz/v1beta/models/')
  })
  it('copies the same content source and provides a collapsible mobile directory', async () => {
    vi.stubGlobal('IntersectionObserver', class { observe() {} disconnect() {} })
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const w = mount(ImageVideoDocsView, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await w.findAll('button').find(b => b.text() === '复制完整 Markdown ↗')!.trigger('click')
    expect(writeText).toHaveBeenLastCalledWith(imagevideoMarkdown())
    await w.get('.media-doc-mobile button').trigger('click')
    expect(w.get('aside').classes()).toContain('open')
    await w.get('nav a').trigger('click')
    expect(w.get('aside').classes()).not.toContain('open')
    w.unmount(); vi.unstubAllGlobals()
  })
})
