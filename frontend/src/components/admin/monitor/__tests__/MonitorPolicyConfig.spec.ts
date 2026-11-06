import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it } from 'vitest'
import MonitorPolicyConfig from '../MonitorPolicyConfig.vue'

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {} }, missingWarn: false, fallbackWarn: false })
describe('monitor probe policy', () => {
  it('writes milliseconds and isolates a model override from defaults', async () => {
    const wrapper = mount(MonitorPolicyConfig, { props: { modelValue: { defaults: { normal_latency_ms: 90000 } }, models: ['image-a', 'image-b'], provider: 'openai', probeType: 'image' }, global: { plugins: [i18n] } })
    await wrapper.get('[data-testid="policy-model"]').setValue('image-b')
    await wrapper.get('[data-testid="policy-request_timeout_ms"]').setValue('240')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ defaults: { normal_latency_ms: 90000 }, models: { 'image-b': { request_timeout_ms: 240000 } } })
  })
  it('rejects a synchronous normal limit beyond its request timeout', () => {
    const wrapper = mount(MonitorPolicyConfig, { props: { modelValue: { defaults: { normal_latency_ms: 200000 } }, models: ['image-a'], provider: 'openai', probeType: 'image' }, global: { plugins: [i18n] } })
    expect(wrapper.find('[data-testid="policy-invalid"]').exists()).toBe(true)
    expect(wrapper.emitted('validity')?.at(-1)).toEqual([false])
  })
  it('keeps the full request body as the prompt source in replace mode', () => {
    const wrapper = mount(MonitorPolicyConfig, { props: { models: ['text-a'], provider: 'openai', probeType: 'text', bodyMode: 'replace' }, global: { plugins: [i18n] } })
    expect(wrapper.find('[data-testid="policy-prompt"]').exists()).toBe(false)
  })
})
