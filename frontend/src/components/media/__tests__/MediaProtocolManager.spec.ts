import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import MediaProtocolManager from '../MediaProtocolManager.vue'

const api = vi.hoisted(() => ({ list: vi.fn(), history: vi.fn(), save: vi.fn(), publish: vi.fn(), preview: vi.fn() }))
const route = vi.hoisted(() => ({ query: {} as Record<string, string> }))
vi.mock('@/api/media-protocols', () => ({ mediaProtocolsAPI: api }))
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: (action: () => Promise<void>) => action() }),
  isStepUpCancelled: () => false
}))

async function click(wrapper: VueWrapper, label: string) {
  const button = wrapper.findAll('button').find((candidate) => candidate.text() === label)
  expect(button, label).toBeDefined()
  await button!.trigger('click')
  await flushPromises()
}

describe('media protocol publication', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    route.query = {}
    api.list.mockResolvedValue([])
    api.history.mockResolvedValue([])
    api.save.mockImplementation(async (_kind, body) => ({
      id: body.id, version: 1, published_version: null, status: 'draft',
      config: { ...body.config, version: '1', status: 'draft' }
    }))
    api.publish.mockImplementation(async (_kind, body) => ({
      id: body.id, version: 2, published_version: 2, status: 'enabled',
      config: { ...body.config, version: '2', status: 'enabled' }
    }))
  })

  it('filters account deep links while preserving binding data when templates fail', async () => {
    route.query = { tab: 'bindings', account_id: '2139', group_id: '19' }
    api.list.mockImplementation(async kind => {
      if (kind === 'templates') throw new Error('Template list unavailable')
      return [
        { id: 'desired', config: { account_id: 2139, group_id: 19 } },
        { id: 'other-account', config: { account_id: 2140, group_id: 19 } },
        { id: 'inherited', config: { account_id: 0, group_id: 19 } },
        { id: 'other-group', config: { account_id: 2139, group_id: 20 } }
      ]
    })
    const wrapper = mount(MediaProtocolManager, { global: { stubs: { TotpStepUpDialog: true, RouterLink: true } } })
    await flushPromises()
    expect(wrapper.get('[aria-label="绑定账号筛选"]').element).toHaveProperty('value', '2139')
    expect(wrapper.text()).toContain('desired')
    expect(wrapper.text()).toContain('inherited')
    expect(wrapper.text()).not.toContain('other-account')
    expect(wrapper.text()).not.toContain('other-group')
    expect(wrapper.get('[role="alert"]').text()).toContain('Template list unavailable')
    wrapper.unmount()
  })

  async function newDraft() {
    const wrapper = mount(MediaProtocolManager, { global: { stubs: { TotpStepUpDialog: true } } })
    await flushPromises()
    await click(wrapper, '新建')
    await click(wrapper, '审阅并发布')
    return wrapper
  }

  it('saves a new draft before calling the existing-version publish endpoint', async () => {
    const wrapper = await newDraft()
    await click(wrapper, '发布不可变新版本')
    expect(api.save).toHaveBeenCalledOnce()
    expect(api.save.mock.calls[0][1]).toMatchObject({ expected_version: 0, publish: false })
    expect(api.publish).toHaveBeenCalledOnce()
    expect(api.publish.mock.calls[0][1]).toMatchObject({ expected_version: 1, publish: true })
    expect(api.save.mock.invocationCallOrder[0]).toBeLessThan(api.publish.mock.invocationCallOrder[0])
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('retries publication with the saved draft after a publish failure', async () => {
    api.publish.mockRejectedValueOnce(new Error('Publication unavailable'))
    const wrapper = await newDraft()
    await click(wrapper, '发布不可变新版本')
    expect(wrapper.get('[role="alert"]').text()).toContain('Publication unavailable')
    await click(wrapper, '发布不可变新版本')
    expect(api.save).toHaveBeenCalledOnce()
    expect(api.publish).toHaveBeenCalledTimes(2)
    expect(api.publish.mock.calls[1][1]).toMatchObject({ expected_version: 1, publish: true })
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('previews published bindings with the real key identity instead of the unsaved draft', async () => {
    route.query = { tab: 'bindings' }
    api.preview.mockResolvedValue({ paid_submission: false, pricing: { maximum_charge: '0.25' } })
    const wrapper = mount(MediaProtocolManager, { global: { stubs: { TotpStepUpDialog: true } } })
    await flushPromises()
    await click(wrapper, '新建')
    await wrapper.get('[aria-label="预览 API Key ID"]').setValue('23')
    await wrapper.get('[aria-label="预览候选账号 ID"]').setValue('7')
    await wrapper.get('[aria-label="实际绑定预览输入"]').setValue('{"model":"public-video","seconds":8,"images":["https://example.org/reference.png"]}')
    await click(wrapper, '预览实际绑定与估价')
    expect(api.preview).toHaveBeenCalledOnce()
    expect(api.preview.mock.calls[0][0]).toEqual({
      mode: 'effective', api_key_id: 23, account_id: 7, operation: 'video',
      request: { model: 'public-video', seconds: 8, images: ['https://example.org/reference.png'] },
      sample_response: {}, measure_media: false
    })
    expect(api.save).not.toHaveBeenCalled()
    expect(api.publish).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
