import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import PlazaMediaPricing from '../PlazaMediaPricing.vue'
const state = vi.hoisted(() => ({ isAdmin: true, save: vi.fn(), catalog: vi.fn() }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => state }))
vi.mock('@/api/modelPlaza', () => ({ saveMediaDescription: state.save, getPublicMediaCatalog: state.catalog }))
const model = { model: 'sample', media_type: 'video', operations: ['video'], duration: { mode: 'fixed', values: [10] }, prices: [], resolutions: ['720p'], display_description: '原说明' }
function render() {
  return mount(PlazaMediaPricing, { props: { models: [model] as never, multiplier: 1, groupId: 19, descriptionVersion: 2 }, global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: {} } })] } })
}
describe('group model descriptions', () => {
  beforeEach(() => { vi.clearAllMocks(); state.isAdmin = true })
  it('saves the selected group and exact public model, then displays plain text', async () => {
    state.save.mockResolvedValue({ description_version: 3, display_description: '<b>9图</b> / 720P' })
    const w = render()
    await w.get('td button').trigger('click')
    await w.get('textarea').setValue('<b>9图</b> / 720P')
    await w.findAll('button').find(b => b.text() === '保存')!.trigger('click'); await flushPromises()
    expect(state.save).toHaveBeenCalledWith(19, 'sample', '<b>9图</b> / 720P', 2)
    expect(w.text()).toContain('<b>9图</b>'); expect(w.find('b').exists()).toBe(false)
    w.unmount()
  })
  it('preserves the draft on conflict and uses the refreshed version only after review', async () => {
    state.save.mockRejectedValueOnce({ status: 409 })
    state.catalog.mockResolvedValue({ groups: [{ id: 19, description_version: 4, media_models: [{ ...model, display_description: '另一管理员的说明' }] }] })
    const w = render(); await w.get('td button').trigger('click'); await w.get('textarea').setValue('我的草稿')
    await w.findAll('button').find(b => b.text() === '保存')!.trigger('click'); await flushPromises()
    expect((w.get('textarea').element as HTMLTextAreaElement).value).toBe('我的草稿')
    await w.findAll('button').find(b => b.text() === '刷新版本')!.trigger('click'); await flushPromises()
    expect((w.get('textarea').element as HTMLTextAreaElement).value).toBe('我的草稿')
    state.save.mockResolvedValue({ description_version: 5, display_description: '我的草稿' })
    await w.findAll('button').find(b => b.text() === '保存')!.trigger('click'); await flushPromises()
    expect(state.save).toHaveBeenLastCalledWith(19, 'sample', '我的草稿', 4); w.unmount()
  })
  it('shows descriptions without edit controls to ordinary and anonymous viewers', () => {
    state.isAdmin = false
    const w = render(); expect(w.text()).toContain('原说明'); expect(w.find('td button').exists()).toBe(false); w.unmount()
  })
})
