import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { get, put, post } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({ apiClient: { get, put, post } }))

import ImageVideoDocsEditorView from '../ImageVideoDocsEditorView.vue'

const content = [{ id: 'start', group: 'Guide', navTitle: 'Start', title: 'Start', subtitle: 'Intro', blocks: [{ kind: 'text', text: 'Hello' }] }]
const state = { published_version: 3, published_content: content, draft_version: 4, draft_content: content }

describe('media docs editor concurrency', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
    post.mockReset()
    get.mockImplementation((url: string) => Promise.resolve({ data: url.endsWith('/history') ? [] : state }))
  })
  afterEach(() => vi.restoreAllMocks())

  it('publishes with the version loaded into the editor', async () => {
    post.mockResolvedValue({ data: { version: 5 } })
    const wrapper = mount(ImageVideoDocsEditorView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })
    await flushPromises()
    await wrapper.get('button.bg-emerald-600').trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/api/v1/admin/docs/imagevideo/publish', {
      expected_version: 4,
      content,
    })
    wrapper.unmount()
  })

  it('does not publish the old draft after a version conflict', async () => {
    post.mockRejectedValue(new Error('document version conflict'))
    const wrapper = mount(ImageVideoDocsEditorView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })
    await flushPromises()
    await wrapper.get('button.bg-emerald-600').trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('发布失败')
    expect(get).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('keeps draft save separate from publish and previews through docs-web', async () => {
    put.mockResolvedValue({ data: { version: 5 } })
    const wrapper = mount(ImageVideoDocsEditorView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })
    await flushPromises()
    await wrapper.get('button.bg-primary-600').trigger('click')
    await flushPromises()
    expect(put).toHaveBeenCalledWith('/api/v1/admin/docs/imagevideo/draft', {
      expected_version: 4,
      content,
    })
    expect(post).not.toHaveBeenCalled()
    const previewButton = wrapper.findAll('button').find(button => button.text() === '预览草稿')
    expect(previewButton).toBeDefined()
    await previewButton!.trigger('click')
    expect(wrapper.find('iframe').attributes('src')).toBe('/docs/imagevideo?preview=1')
    wrapper.unmount()
  })
})
