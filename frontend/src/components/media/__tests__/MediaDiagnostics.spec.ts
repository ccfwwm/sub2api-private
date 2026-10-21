import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MediaDiagnostics from '../MediaDiagnostics.vue'

const api = vi.hoisted(() => ({ list: vi.fn(), getById: vi.fn(), getAllIncludingInactive: vi.fn(), effective: vi.fn(), preview: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: api, groups: api } }))
vi.mock('@/api/media-protocols', () => ({ mediaProtocolsAPI: { effective: api.effective, preview: api.preview } }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: { group_id: '19', account_id: '2139' } }) }))

const capabilities = [
  { model: 'video-choice', media_type: 'video', enabled: true, operations: ['video'], duration: { default: 5, values: [], min: 5 } },
  { model: 'music-choice', media_type: 'audio', enabled: true, operations: ['chat_music'], duration: {} }
]
const account = () => ({ id: 2139, name: 'Linked account', platform: 'imagevideo', group_ids: [19], extra: { media_models: capabilities }, credentials: {} })
const options = { global: { stubs: { RouterLink: true } } }

beforeEach(() => {
  vi.resetAllMocks()
  api.getAllIncludingInactive.mockResolvedValue([{ id: 19, name: 'Video group', platform: 'imagevideo' }])
  api.getById.mockResolvedValue(account())
  api.list.mockResolvedValue({ items: [], total: 0 })
  api.effective.mockResolvedValue({})
  api.preview.mockResolvedValue({ paid_submission: false })
})

describe('media diagnostics selectors', () => {
  it('keeps a linked account outside the current account page and loads its model choices', async () => {
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    expect(api.list).toHaveBeenCalledWith(1, 50, expect.objectContaining({ platform: 'imagevideo', group: '19' }))
    expect(wrapper.get('[aria-label="联调账号"]').element).toHaveProperty('value', '2139')
    expect(wrapper.get('[aria-label="联调模型"]').findAll('option').map(option => option.text())).toEqual(expect.arrayContaining(['video-choice', 'music-choice']))
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('该分组没有媒体账号')
    wrapper.unmount()
  })

  it('previews with the selected account, group, model and API key scope', async () => {
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    await wrapper.get('[aria-label="联调 API Key ID"]').setValue('123')
    await wrapper.findAll('button').find(button => button.text() === '预览映射与渠道估价')!.trigger('click')
    await flushPromises()
    expect(api.preview).toHaveBeenCalledWith(expect.objectContaining({
      mode: 'effective', api_key_id: 123, group_id: 19, account_id: 2139, operation: 'video',
      request: expect.objectContaining({ model: 'video-choice', seconds: 5 }), measure_media: false
    }))
    wrapper.unmount()
  })

  it('removes only its generated duration default when changing from video to audio', async () => {
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    const requestField = wrapper.get('[aria-label="联调请求"]')
    expect(JSON.parse((requestField.element as HTMLTextAreaElement).value)).toMatchObject({ model: 'video-choice', prompt: 'A quiet valley', seconds: 5 })
    await wrapper.get('[aria-label="联调模型"]').setValue('music-choice')
    await flushPromises()
    expect(JSON.parse((requestField.element as HTMLTextAreaElement).value)).toMatchObject({ model: 'music-choice', prompt: 'A quiet valley' })
    expect(JSON.parse((requestField.element as HTMLTextAreaElement).value)).not.toHaveProperty('seconds')
    wrapper.unmount()
  })
})
