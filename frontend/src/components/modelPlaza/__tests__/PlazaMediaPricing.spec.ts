import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import PlazaMediaPricing from '../PlazaMediaPricing.vue'
import type { PlazaMediaModel } from '@/api/modelPlaza'

function model(method: string): PlazaMediaModel {
  return { model: '00sd2.5', media_type: 'video', operations: ['video'], resolutions: [],
    duration: { mode: 'fixed', values: [30], min: null, max: null, default: 30 },
    prices: [{ operation: 'video', version: '1', maximum_charge: '1', rules: [{ priority: 1, when: {}, charges: [{
      name: 'video', unit: 'second', unit_price: method === 'progressive' ? null : '1',
      quantity_source: 'fixed_duration', trigger: 'success', method, package_quantity: '30',
      rounding: { mode: 'none', step: '1', minimum: '0', scope: 'per_output' },
      tiers: [{ up_to: '10', unit_price: '0.04' }, { up_to: null, unit_price: '0.02' }]
    }] }] }] }
}

function render(method: string) {
  return mount(PlazaMediaPricing, { props: { models: [model(method)], multiplier: 2 },
    global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: {} } })] } })
}

describe('media price disclosure', () => {
  it('discloses a 30 second package as a total instead of a per second price', () => {
    const text = render('package').text()
    expect(text).toContain('30 second package total: $1')
    expect(text).not.toContain('$1 / second')
  })
  it('shows configured progressive prices when the linear price is intentionally null', () => {
    const text = render('progressive').text()
    expect(text).toContain('0–10 second: $0.04 / second')
    expect(text).toContain('10–∞ second: $0.02 / second')
    expect(text).not.toContain('Unconfigured')
  })
})
