import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { monitorProbeDefaults } from '@/api/admin/channelMonitor'
import ProbeConfig from '../MonitorProbeConfig.vue'

vi.mock('vue-i18n', async original => ({
  ...await original<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

describe('monitor probe config', () => {
  it('keeps old settings as text and requires new consent when switching probe types', async () => {
    expect(monitorProbeDefaults()).toMatchObject({ probe_type: 'text', image_probe_schedule_enabled: false, custom_response_require_non_empty: true })
    const wrapper = mount(ProbeConfig, { props: { provider: 'openai', modelValue: monitorProbeDefaults({ image_probe_schedule_enabled: true }) } })
    await wrapper.get('[data-testid="probe-type"]').setValue('image')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toMatchObject({ probe_type: 'image', image_probe_schedule_enabled: false })
  })

  it('validates custom JSON and permits clearing the body', async () => {
    const wrapper = mount(ProbeConfig, { props: { provider: 'openai', modelValue: monitorProbeDefaults({ probe_type: 'custom', custom_path: '/health' }) } })
    await wrapper.get('[data-testid="probe-body"]').setValue('[1]')
    expect(wrapper.emitted('validity')?.at(-1)).toEqual([false])
    await wrapper.get('[data-testid="probe-body"]').setValue('{"prompt":"hello"}')
    expect(wrapper.emitted('validity')?.at(-1)).toEqual([true])
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toMatchObject({ custom_body: { prompt: 'hello' } })
    await wrapper.get('[data-testid="probe-body"]').setValue('')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toMatchObject({ custom_body: null })
  })

  it.each(['', 'https://host/path', '//host/path', '/%2e%2e/health', '/health?target=x'])('rejects unsafe custom paths: %s', path => {
    const wrapper = mount(ProbeConfig, { props: { provider: 'gemini', modelValue: monitorProbeDefaults({ probe_type: 'custom', custom_path: path }) } })
    expect(wrapper.emitted('validity')?.at(-1)).toEqual([false])
  })
})
