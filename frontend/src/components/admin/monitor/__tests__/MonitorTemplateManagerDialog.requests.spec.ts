import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

import MonitorTemplateManagerDialog from '../MonitorTemplateManagerDialog.vue'
const { list, create, showError, showSuccess } = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { channelMonitorTemplate: { list, create } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
const response = (name: string) => ({ items: [{ id: 1, name, provider: 'anthropic', body_override_mode: 'off' }] })
function mountDialog() {
  return shallowMount(MonitorTemplateManagerDialog, {
    props: { show: true },
    global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } },
  })
}
describe('monitor template list requests', () => {
  beforeEach(() => { vi.resetAllMocks() })
  it('blocks media template saves while a published protocol selection is invalid', async () => {
    list.mockResolvedValue({ items: [] })
    create.mockResolvedValue({})
    const wrapper = mountDialog()
    await flushPromises()
    const vm = wrapper.vm as any
    vm.openCreateForm()
    vm.form.provider = 'imagevideo'; vm.form.name = 'Explicit template'
    await flushPromises()
    const media = wrapper.findComponent({ name: 'MonitorMediaConfig' })
    media.vm.$emit('validity', false)
    await flushPromises()
    await vm.handleSubmit()
    expect(create).not.toHaveBeenCalled()
    media.vm.$emit('validity', true)
    await flushPromises()
    await vm.handleSubmit()
    expect(create).toHaveBeenCalledTimes(1)
  })
  it.each(['resolve', 'reject'] as const)('ignores an old dialog request that %ss', async outcome => {
    const old = deferred<ReturnType<typeof response>>()
    list.mockReturnValueOnce(old.promise).mockResolvedValueOnce(response('current-template'))
    const wrapper = mountDialog()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    if (outcome === 'resolve') old.resolve(response('old-template'))
    else old.reject(new Error('obsolete error'))
    await flushPromises()
    expect(wrapper.text()).toContain('current-template')
    expect(wrapper.text()).not.toContain('old-template')
    expect(showError).not.toHaveBeenCalled()
  })
  it('keeps the reopened dialog loading until its own request completes', async () => {
    const old = deferred<ReturnType<typeof response>>()
    const current = deferred<ReturnType<typeof response>>()
    list.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = mountDialog()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    old.resolve(response('old-template'))
    await flushPromises()
    expect(wrapper.text()).toContain('common.loading')
    current.resolve(response('current-template'))
    await flushPromises()
    expect(wrapper.text()).toContain('current-template')
  })
  it('does not report an error after closing the dialog', async () => {
    const request = deferred<ReturnType<typeof response>>()
    list.mockReturnValueOnce(request.promise)
    const wrapper = mountDialog()
    await wrapper.setProps({ show: false })
    request.reject(new Error('closed dialog'))
    await flushPromises()
    expect(showError).not.toHaveBeenCalled()
  })
})
