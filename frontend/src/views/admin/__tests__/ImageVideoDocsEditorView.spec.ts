import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { AxiosError, type AxiosAdapter, type InternalAxiosRequestConfig } from 'axios'
import { apiClient } from '@/api/client'
import ImageVideoDocsEditorView from '../ImageVideoDocsEditorView.vue'

vi.mock('@/i18n', () => ({ getLocale: () => 'zh-CN' }))

const content = [{ id: 'start', group: 'Guide', navTitle: 'Start', title: 'Start', subtitle: 'Intro', blocks: [{ kind: 'text', text: 'Hello' }] }]
const documentState = { published_version: 3, published_content: content, draft_version: 4, draft_content: content }
const history = [{ version: 3, action: 'publish', created_at: '2026-10-15T10:00:00Z' }]
const editorPath = '/api/v1/admin/docs/imagevideo'
const originalAdapter = apiClient.defaults.adapter
const wrappers: ReturnType<typeof mount>[] = []
let requests: { method: string; path: string; body: unknown; authorization: unknown }[]
let failWith: { method: string; status: number; message: string } | null

function mountEditor() {
  const wrapper = mount(ImageVideoDocsEditorView, { attachTo: document.body, global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })
  wrappers.push(wrapper)
  return wrapper
}

describe('media docs editor requests', () => {
  beforeEach(() => {
    requests = []
    failWith = null
    localStorage.setItem('auth_token', 'editor-test-token')
    const adapter: AxiosAdapter = async (config: InternalAxiosRequestConfig) => {
      const path = new URL(apiClient.getUri(config), window.location.origin).pathname
      const method = config.method ?? 'get'
      requests.push({ method, path, body: config.data ? JSON.parse(config.data) : null, authorization: config.headers.get('Authorization') })
      const validPath = [editorPath, `${editorPath}/history`, `${editorPath}/draft`, `${editorPath}/publish`, `${editorPath}/rollback`].includes(path)
      const failure = !validPath ? { status: 404, message: 'Not found' } : failWith?.method === method ? failWith : null
      const response = { status: failure?.status ?? 200, statusText: failure ? 'Error' : 'OK', headers: {}, config, data: failure ? { message: failure.message } : { code: 0, data: method !== 'get' ? { version: 5 } : path.endsWith('/history') ? history : documentState } }
      if (failure) throw new AxiosError(failure.message, 'ERR_BAD_RESPONSE', config, undefined, response)
      return response
    }
    apiClient.defaults.adapter = adapter
  })
  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    apiClient.defaults.adapter = originalAdapter
    localStorage.removeItem('auth_token')
    vi.restoreAllMocks()
  })

  it('loads content and history through the authenticated client with one API prefix', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    expect(requests.map(request => request.path)).toEqual([editorPath, `${editorPath}/history`])
    expect(requests.every(request => request.authorization === 'Bearer editor-test-token')).toBe(true)
    expect(wrapper.text()).toContain('已发布 v3 · 草稿版本 4')
    expect(JSON.parse((wrapper.get('textarea').element as HTMLTextAreaElement).value)).toEqual(content)
    expect(wrapper.text()).toContain('v3 · publish')
  })

  it('publishes with the version loaded into the editor', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('button.bg-emerald-600').trigger('click')
    await flushPromises()
    expect(requests.find(request => request.method === 'post')).toMatchObject({ path: `${editorPath}/publish`, body: { expected_version: 4, content } })
    expect(wrapper.text()).toContain('新文档版本已发布')
  })

  it('keeps the edited content on a version conflict and displays the API message', async () => {
    failWith = { method: 'post', status: 409, message: 'document version conflict' }
    const wrapper = mountEditor()
    await flushPromises()
    const edited = [{ ...content[0], title: 'Unsaved edit' }]
    await wrapper.get('textarea').setValue(JSON.stringify(edited))
    await wrapper.get('button.bg-emerald-600').trigger('click')
    await flushPromises()
    expect(requests.filter(request => request.method === 'post')).toHaveLength(1)
    expect(requests.filter(request => request.method === 'get')).toHaveLength(2)
    expect(wrapper.get('[role="status"]').text()).toContain('document version conflict')
    expect(wrapper.text()).not.toContain('[object Object]')
    expect(JSON.parse((wrapper.get('textarea').element as HTMLTextAreaElement).value)).toEqual(edited)
  })

  it('saves a draft separately from publishing and previews through docs-web', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('button.bg-primary-600').trigger('click')
    await flushPromises()
    expect(requests.find(request => request.method === 'put')).toMatchObject({ path: `${editorPath}/draft`, body: { expected_version: 4, content } })
    expect(requests.some(request => request.method === 'post')).toBe(false)
    await wrapper.findAll('button').find(button => button.text() === '预览草稿')!.trigger('click')
    const frame = wrapper.get('iframe')
    expect(frame.attributes('src')).toBe('/docs/imagevideo?preview=1')
    const postMessage = vi.spyOn((frame.element as HTMLIFrameElement).contentWindow!, 'postMessage')
    await frame.trigger('load')
    expect(postMessage).toHaveBeenCalledWith({ type: 'imagevideo-docs-preview', content, version: 4 }, window.location.origin)
  })

  it('rolls back with the current draft version', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '回滚到此内容')!.trigger('click')
    await flushPromises()
    expect(requests.find(request => request.method === 'post')).toMatchObject({ path: `${editorPath}/rollback`, body: { expected_version: 4, version: 3 } })
  })

  it('displays a readable load failure and disables editing until retry succeeds', async () => {
    failWith = { method: 'get', status: 503, message: 'document unavailable' }
    const wrapper = mountEditor()
    await flushPromises()
    expect(wrapper.get('[role="status"]').text()).toBe('读取失败：document unavailable')
    expect(wrapper.get('textarea').attributes('disabled')).toBeDefined()
    expect(wrapper.get('button.bg-primary-600').attributes('disabled')).toBeDefined()
    expect(wrapper.get('button.bg-emerald-600').attributes('disabled')).toBeDefined()
    failWith = null
    await wrapper.findAll('button').find(button => button.text() === '刷新')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('已发布 v3 · 草稿版本 4')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.get('textarea').attributes('disabled')).toBeUndefined()
  })
})
