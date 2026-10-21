import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { newMediaCapability } from '@/types/media'
import Manager from '../MediaAccountsManager.vue'

const api = vi.hoisted(() => ({ list: vi.fn(), getById: vi.fn(), update: vi.fn(), pricingStatus: vi.fn(), stepUp: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: api } }))
vi.mock('@/api/media', () => ({ mediaAPI: api }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: { account_id: '2139' } }), useRouter: () => ({ replace: vi.fn() }) }))
vi.mock('@/composables/useStepUp', () => ({ useStepUp: () => ({ run: api.stepUp }), isStepUpCancelled: () => false }))
const models = [newMediaCapability('public-video')]
const saved = () => ({ id: 2139, platform: 'imagevideo', name: 'Direct account', group_ids: [19], extra: { media_models: models, unrelated: 'keep' }, credentials: { base_url: 'https://example.invalid', api_key: '[redacted]' } })
const options = { global: { stubs: { MediaAccountModelsEditor: { name: 'MediaAccountModelsEditor', props: ['modelValue'], template: '<div>Model editor</div>' }, TotpStepUpDialog: true, RouterLink: true } } }
beforeEach(() => {
  vi.resetAllMocks()
  api.list.mockResolvedValue({ items: [], total: 0 })
  api.getById.mockImplementation(async () => saved())
  api.pricingStatus.mockResolvedValue([{ model: 'public-video', operation: 'video', matched: false, reason: 'price_not_configured' }])
  api.stepUp.mockImplementation(action => action())
  api.update.mockImplementation(async (_id, body) => ({ ...saved(), extra: body.extra }))
})
describe('account model administration', () => {
  it('loads the linked account outside the list page and explains missing pricing without step-up', async () => {
    const wrapper = mount(Manager, options); await flushPromises()
    expect(api.getById).toHaveBeenCalledWith(2139)
    expect(wrapper.text()).toContain('Direct account · #2139')
    expect(wrapper.text()).toContain('未匹配价格')
    expect(api.stepUp).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('keeps the editor usable when independent list and price requests fail', async () => {
    api.list.mockRejectedValue(new Error('list failed')); api.pricingStatus.mockRejectedValue(new Error('price failed'))
    const wrapper = mount(Manager, options); await flushPromises()
    expect(wrapper.text()).toContain('Model editor')
    expect(wrapper.text()).toContain('price failed')
    expect(wrapper.text()).toContain('list failed')
    wrapper.unmount()
  })
  it('saves a snapshot with fresh unrelated extras and excludes masked credentials', async () => {
    const wrapper = mount(Manager, options); await flushPromises()
    const changed = [newMediaCapability('new-model')]
    wrapper.findComponent({ name: 'MediaAccountModelsEditor' }).vm.$emit('update:modelValue', changed)
    await flushPromises()
    api.getById.mockResolvedValueOnce({ ...saved(), extra: { ...saved().extra, unrelated: 'newer value' } })
    await wrapper.findAll('button').find(button => button.text() === '保存账号模型')!.trigger('click'); await flushPromises()
    expect(api.update).toHaveBeenCalledWith(2139, { extra: { unrelated: 'newer value', media_models: changed } })
    expect(api.stepUp).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('账号模型已保存')
    wrapper.unmount()
  })
})
