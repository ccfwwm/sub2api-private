import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import PlazaMediaPricing from '../PlazaMediaPricing.vue'
const state = vi.hoisted(() => ({ isAdmin: true, save: vi.fn(), savePrice: vi.fn(), catalog: vi.fn() }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => state }))
vi.mock('@/api/modelPlaza', () => ({ saveMediaDescription: state.save, saveMediaReferencePrice: state.savePrice, getPublicMediaCatalog: state.catalog }))
const model = { model: 'sample', media_type: 'video', operations: ['video'], duration: { mode: 'fixed', values: [10] }, prices: [], resolutions: ['720p'], display_description: '原说明' }
function render() {
  return mount(PlazaMediaPricing, { props: { models: [model] as never, multiplier: 1, groupId: 19, descriptionVersion: 2 }, global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: {} } })] } })
}
describe('group model descriptions', () => {
  beforeEach(() => { vi.clearAllMocks(); state.isAdmin = true })
  it('saves the selected group and exact public model, then displays plain text', async () => {
    state.save.mockResolvedValue({ description_version: 3, display_description: '<b>9图</b> / 720P' })
    const w = render()
    await w.findAll('button').find(b => b.text() === '编辑说明')!.trigger('click')
    await w.get('textarea').setValue('<b>9图</b> / 720P')
    await w.findAll('button').find(b => b.text() === '保存')!.trigger('click'); await flushPromises()
    expect(state.save).toHaveBeenCalledWith(19, 'sample', '<b>9图</b> / 720P', 2)
    expect(w.text()).toContain('<b>9图</b>'); expect(w.find('b').exists()).toBe(false)
    w.unmount()
  })
  it('preserves the draft on conflict and uses the refreshed version only after review', async () => {
    state.save.mockRejectedValueOnce({ status: 409 })
    state.catalog.mockResolvedValue({ groups: [{ id: 19, description_version: 4, media_models: [{ ...model, display_description: '另一管理员的说明' }] }] })
    const w = render(); await w.findAll('button').find(b => b.text() === '编辑说明')!.trigger('click'); await w.get('textarea').setValue('我的草稿')
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

describe('official reference prices', () => {
  beforeEach(() => { vi.clearAllMocks(); state.isAdmin = true })
  const click = async (w: ReturnType<typeof render>, label: string) => { await w.findAll('button').find(b => b.text() === label)!.trigger('click'); await flushPromises() }
  it('saves USD per unit without applying sale multipliers and shares the metadata version with descriptions', async () => {
    const w = render()
    await w.setProps({ multiplier: 10 })
    state.savePrice.mockResolvedValue({ description_version: 3, official_reference_price: { amount: '0.10', unit: '秒' } })
    await click(w, '编辑参考价')
    await w.get('[aria-label="官方参考价金额"]').setValue('0.10')
    await w.get('[aria-label="官方参考价单位"]').setValue('秒')
    await click(w, '保存参考价')
    expect(state.savePrice).toHaveBeenCalledWith(19, 'sample', { amount: '0.10', unit: '秒' }, 2)
    expect(w.text()).toContain('$0.10 / 秒')
    state.save.mockResolvedValue({ description_version: 4, display_description: 'new' })
    await click(w, '编辑说明'); await click(w, '保存')
    expect(state.save).toHaveBeenLastCalledWith(19, 'sample', '原说明', 3)
    expect(w.text()).toContain('$0.10 / 秒')
    await click(w, '编辑参考价'); await click(w, '清空参考价')
    state.savePrice.mockResolvedValue({ description_version: 5, official_reference_price: null })
    await click(w, '保存参考价')
    expect(state.savePrice).toHaveBeenLastCalledWith(19, 'sample', null, 4)
    expect(w.text()).not.toContain('$0.10 / 秒'); w.unmount()
  })
  it('keeps the price draft on conflict and refreshes the current value before explicit save', async () => {
    const w = render()
    await click(w, '编辑参考价')
    await w.get('[aria-label="官方参考价金额"]').setValue('0')
    state.savePrice.mockRejectedValueOnce({ status: 409 })
    await click(w, '保存参考价')
    expect((w.get('[aria-label="官方参考价金额"]').element as HTMLInputElement).value).toBe('0')
    state.catalog.mockResolvedValue({ groups: [{ id: 19, description_version: 5, media_models: [{ ...model, official_reference_price: { amount: '2', unit: '张' } }] }] })
    await click(w, '刷新版本')
    expect(w.text()).toContain('当前参考价：$2 / 张')
    expect(state.savePrice).toHaveBeenCalledTimes(1)
    state.savePrice.mockResolvedValue({ description_version: 6, official_reference_price: { amount: '0', unit: '次' } })
    await click(w, '保存参考价')
    expect(state.savePrice).toHaveBeenLastCalledWith(19, 'sample', { amount: '0', unit: '次' }, 5)
    expect(w.text()).toContain('$0 / 次'); w.unmount()
  })
  it('rejects invalid amounts locally and hides editing from nonadmins while showing stored prices', async () => {
    const w = render(); await click(w, '编辑参考价')
    await w.get('[aria-label="官方参考价金额"]').setValue('-2'); await click(w, '保存参考价')
    expect(state.savePrice).not.toHaveBeenCalled(); expect(w.get('[role="alert"]').text()).toContain('非负')
    w.unmount(); state.isAdmin = false
    const viewer = render()
    await viewer.setProps({ models: [{ ...model, official_reference_price: { amount: '0.10', unit: '<b>秒</b>' } }] as never })
    expect(viewer.text()).toContain('$0.10 / <b>秒</b>'); expect(viewer.find('b').exists()).toBe(false)
    expect(viewer.text()).not.toContain('编辑参考价'); viewer.unmount()
  })
})
