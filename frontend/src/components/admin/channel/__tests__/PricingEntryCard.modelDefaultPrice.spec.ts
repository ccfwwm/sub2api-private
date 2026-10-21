import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PricingEntryCard from '../PricingEntryCard.vue'
import type { PricingFormEntry } from '../types'
import channelsAPI from '@/api/admin/channels'

vi.mock('@/api/admin/channels', () => ({
  default: { getModelDefaultPricing: vi.fn() },
}))

vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

const getModelDefaultPricing = vi.mocked(channelsAPI.getModelDefaultPricing)

function createEntry(models: string[] = []): PricingFormEntry {
  return {
    models,
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    fast_multiplier: null,
    flex_multiplier: null,
    reasoning_effort_multipliers: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    time_pricing: {
      timezone: 'Asia/Shanghai',
      weekdays_only: false,
      periods: [],
    },
  }
}

function addModels(entry: PricingFormEntry, models: string[]) {
  const wrapper = shallowMount(PricingEntryCard, { props: { entry } })
  wrapper.findComponent({ name: 'ModelTagInput' }).vm.$emit('update:models', models)
  return wrapper
}

const emittedEntries = (wrapper: ReturnType<typeof shallowMount>) =>
  (wrapper.emitted('update') ?? []).map(e => e[0] as PricingFormEntry)

describe('PricingEntryCard default price auto-fill', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getModelDefaultPricing.mockResolvedValue({ found: true, input_price: 3e-6, output_price: 15e-6 })
  })

  it('auto-fills the default price when a model is added to an empty entry', async () => {
    const wrapper = addModels(createEntry(), ['claude-opus-4-8'])
    await flushPromises()

    expect(getModelDefaultPricing).toHaveBeenCalledTimes(1)
    expect(getModelDefaultPricing).toHaveBeenCalledWith('claude-opus-4-8')
    expect(emittedEntries(wrapper).at(-1)).toMatchObject({
      models: ['claude-opus-4-8'],
      input_price: 3,
      output_price: 15,
    })
  })

  it('uses the existing default pricing rule for imagevideo entries', async () => {
    const wrapper = shallowMount(PricingEntryCard, { props: { entry: createEntry(), platform: 'imagevideo' } })
    wrapper.findComponent({ name: 'ModelTagInput' }).vm.$emit('update:models', ['gpt-image-2'])
    await flushPromises()
    expect(getModelDefaultPricing).toHaveBeenCalledWith('gpt-image-2')
    expect(emittedEntries(wrapper).at(-1)).toMatchObject({
      models: ['gpt-image-2'],
      input_price: 3,
      output_price: 15,
    })
  })

  it('keeps the shared billing modes available for imagevideo entries', () => {
    const wrapper = shallowMount(PricingEntryCard, { props: { entry: createEntry(), platform: 'imagevideo' } })
    const select = wrapper.findComponent({ name: 'Select' })
    expect(select.props('options')).toEqual([
      { value: 'characters', label: '语音输入字符（Unicode 码点）' },
      { value: 'token', label: 'admin.channels.billingMode.token' },
      { value: 'per_request', label: 'admin.channels.billingMode.perRequest' },
      { value: 'video', label: 'admin.channels.billingMode.video' },
    ])
  })

  it('selects character billing and preserves the entered per-code-point price', async () => {
    const wrapper = shallowMount(PricingEntryCard, { props: { entry: createEntry(), platform: 'imagevideo' } })
    wrapper.findComponent({ name: 'Select' }).vm.$emit('update:modelValue', 'characters')
    const selected = emittedEntries(wrapper).at(-1)!
    expect(selected).toMatchObject({ billing_mode: 'characters', intervals: [], per_request_price: null })
    await wrapper.setProps({ entry: selected })
    const input = wrapper.findAll('label').find(label => label.text().includes('USD / Unicode 码点'))!.get('input')
    await input.setValue('0.00002')
    const priced = emittedEntries(wrapper).at(-1)!
    expect(priced).toMatchObject({ billing_mode: 'characters', per_request_price: '0.00002' })
    await wrapper.setProps({ entry: priced })
    wrapper.findComponent({ name: 'ModelTagInput' }).vm.$emit('update:models', ['configured-speech-model'])
    await flushPromises()
    expect(getModelDefaultPricing).not.toHaveBeenCalled()
    expect(emittedEntries(wrapper).at(-1)).toMatchObject({ billing_mode: 'characters', per_request_price: '0.00002' })
  })

  it('does not overwrite prices when appending a model to an entry that already has one', async () => {
    const entry = createEntry(['claude-opus-4-7'])
    const wrapper = addModels(entry, ['claude-opus-4-7', 'claude-opus-4-8'])
    await flushPromises()

    expect(getModelDefaultPricing).not.toHaveBeenCalled()
    expect(emittedEntries(wrapper).at(-1)).toMatchObject({
      models: ['claude-opus-4-7', 'claude-opus-4-8'],
    })
    for (const emitted of emittedEntries(wrapper)) {
      expect(emitted.input_price).toBeNull()
      expect(emitted.output_price).toBeNull()
      expect(emitted.cache_write_price).toBeNull()
      expect(emitted.cache_read_price).toBeNull()
    }
  })

  it('does not auto-fill a shared price when several models are added to an empty entry at once', async () => {
    const wrapper = addModels(createEntry(), ['claude-opus-4-7', 'claude-opus-4-8'])
    await flushPromises()

    expect(getModelDefaultPricing).not.toHaveBeenCalled()
    expect(emittedEntries(wrapper).at(-1)).toMatchObject({
      models: ['claude-opus-4-7', 'claude-opus-4-8'],
      input_price: null,
      output_price: null,
    })
  })

  it('keeps manual prices untouched when a model is appended', async () => {
    const entry = { ...createEntry(['claude-opus-4-7']), input_price: '1.5' }
    const wrapper = addModels(entry, ['claude-opus-4-7', 'claude-opus-4-8'])
    await flushPromises()

    expect(getModelDefaultPricing).not.toHaveBeenCalled()
    for (const emitted of emittedEntries(wrapper)) {
      expect(emitted.input_price).toBe('1.5')
    }
  })
})
