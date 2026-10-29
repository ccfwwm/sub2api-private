import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ImageVideoDocsView from '../public/ImageVideoDocsView.vue'
import { imagevideoMarkdown, imagevideoModels, imagevideoSections, sectionMarkdown, groupMarkdown } from '@/data/imagevideoDocs'

describe('Public media docs', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.unstubAllEnvs()
    localStorage.removeItem('media-docs-theme')
  })
  it('uses 28 exact unique models and validates JSON and cURL JSON bodies', () => {
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
    expect(groupMarkdown('视频生成')).toContain(sectionMarkdown(imagevideoSections.find(section => section.id === 'grok-video-image')!))
    expect(imagevideoSections.some(s => s.id === 'clients')).toBe(true)
  })
  it('documents canonical video materials without exposing supplier configuration', () => {
    const video = imagevideoSections.find(section => section.id === 'video') ?? imagevideoSections.find(section => section.blocks.some(block => block.kind === 'code' && block.label === '视频 URL 参考请求'))!
    const markdown = sectionMarkdown(video)
    expect(markdown).toContain('images / videos / audios')
    expect(markdown).not.toMatch(/yunfei|v6k|adapter|request_format|template_ref|reference_images|reference_videos|reference_audios/i)
    const examples = video.blocks.filter(block => block.kind === 'code' && block.language === 'json')
    expect(examples.length).toBeGreaterThan(2)
    for (const block of examples) {
      if (block.kind !== 'code') continue
      const payload = JSON.parse(block.text)
      expect(Object.keys(payload).some(key => /^(?:provider|adapter|request_format|template_ref|reference_)/.test(key))).toBe(false)
    }
    const frames = examples.find(block => block.kind === 'code' && block.label === '首尾帧请求')!
    if (frames.kind !== 'code') throw new Error('Missing frame request')
    expect(JSON.parse(frames.text).images.map((image: { role: string }) => image.role)).toEqual(['first_frame', 'last_frame'])
  })
  it.each([true, false])('opens standalone docs as the top-level page (dev=%s), preserving links', (dev) => {
    vi.stubEnv('DEV', dev)
    const replace = vi.fn()
    vi.stubGlobal('window', { location: { search: '?from=home', hash: '#gpt-edit', replace } })
    const w = mount(ImageVideoDocsView)
    const destination = (dev ? '/media-docs/index.html' : '/docs/imagevideo') + '?from=home#gpt-edit'
    expect(w.find('iframe').exists()).toBe(false)
    expect(replace).toHaveBeenCalledWith(destination)
    expect(w.get('a').attributes('href')).toBe(destination)
    w.unmount()
  })
})
