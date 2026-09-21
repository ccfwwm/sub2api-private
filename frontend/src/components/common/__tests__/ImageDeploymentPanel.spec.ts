import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ImageDeploymentPanel from '../ImageDeploymentPanel.vue'
const api = vi.hoisted(() => ({ getImageDeployment: vi.fn(), requestImageDeployment: vi.fn() }))
vi.mock('@/api/admin/system', () => api)

describe('ImageDeploymentPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getImageDeployment.mockResolvedValue({ enabled: true, job: null })
    api.requestImageDeployment.mockResolvedValue({ job: { id: 'a'.repeat(32), version: 'v1.0.0', status: 'queued_prepare' } })
  })
  it('requires confirmation before preparation and never auto deploys', async () => {
    const wrapper = mount(ImageDeploymentPanel, { props: { version: '1.0.0' } })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    await wrapper.find('input[placeholder="v1.0.0"]').setValue('v1.0.0')
    const prepare = wrapper.findAll('button').find(b => b.text().includes('准备版本：'))!
    expect(prepare.attributes('disabled')).toBeDefined()
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await prepare.trigger('click')
    await flushPromises()
    expect(api.requestImageDeployment).toHaveBeenCalledOnce()
    expect(api.requestImageDeployment.mock.calls[0][0].action).toBe('prepare')
    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)
    wrapper.unmount()
  })
  it('disables preparation when host controller is disabled', async () => {
    api.getImageDeployment.mockResolvedValue({ enabled: false, job: null })
    const wrapper = mount(ImageDeploymentPanel)
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('生产部署尚未启用')
    expect(api.requestImageDeployment).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
