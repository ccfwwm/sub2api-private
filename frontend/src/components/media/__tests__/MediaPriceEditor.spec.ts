import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import MediaPriceEditor from '../MediaPriceEditor.vue'

function config() {
  return { purpose: 'sale', scope: { kind: 'group', reference: '1' }, model_match: { kind: 'exact', value: '00sd2.5' },
    success_policy: 'any_output', reservation: { maximum_charge: null, max_quantities: {} }, rules: [{ priority: 100, when: {},
      charges: [{ name: 'generation', unit: 'request', unit_price: null, quantity_source: 'task_count', trigger: 'success',
        refundable: true, method: 'linear', package_quantity: null, tiers: [], rounding: { mode: 'none', step: '1', minimum: '0', scope: 'per_output' } }] }] }
}
const i18n = () => createI18n({ legacy: false, locale: 'en', messages: { en: {} } })
describe('MediaPriceEditor', () => {
  it('keeps free and unconfigured distinct and preserves decimal strings', async () => {
    const wrapper = mount(MediaPriceEditor, { props: { config: config() }, global: { plugins: [i18n()] } })
    const price = wrapper.findAll('label').find(label => label.text().includes('Unit price'))!.find('input')
    await price.setValue('0'); await price.trigger('change')
    expect((wrapper.emitted('update')!.at(-1)![0] as any).rules[0].charges[0].unit_price).toBe('0')
    await price.setValue('0.123456789012'); await price.trigger('change')
    expect((wrapper.emitted('update')!.at(-1)![0] as any).rules[0].charges[0].unit_price).toBe('0.123456789012')
    await price.setValue(''); await price.trigger('change')
    expect((wrapper.emitted('update')!.at(-1)![0] as any).rules[0].charges[0].unit_price).toBeNull()
  })
  it('selects a matching duration source and adds a distinct combination item', async () => {
    const initial = config()
    const wrapper = mount(MediaPriceEditor, { props: { config: initial }, global: { plugins: [i18n()] } })
    await wrapper.findAll('label').find(label => label.text().includes('Billing unit'))!.find('select').setValue('second')
    const changed = wrapper.emitted('update')!.at(-1)![0] as any
    expect(changed.rules[0].charges[0]).toMatchObject({ unit: 'second', quantity_source: 'request_duration' })
    expect(initial.rules[0].charges[0].unit).toBe('request')
    await wrapper.setProps({ config: changed })
    await wrapper.findAll('button').find(button => button.text().includes('Add composite'))!.trigger('click')
    expect((wrapper.emitted('update')!.at(-1)![0] as any).rules[0].charges).toHaveLength(2)
  })
  it('allows speech pricing by measured Unicode input characters', async () => {
    const wrapper = mount(MediaPriceEditor, { props: { config: config() }, global: { plugins: [i18n()] } })
    await wrapper.findAll('label').find(label => label.text().includes('Billing unit'))!.find('select').setValue('input_character')
    const changed = wrapper.emitted('update')!.at(-1)![0] as any
    expect(changed.rules[0].charges[0]).toMatchObject({ unit: 'input_character', quantity_source: 'input_characters' })
    await wrapper.setProps({ config: changed })
    const source = wrapper.findAll('label').find(label => label.text().includes('Quantity source'))!.find('select')
    expect(source.findAll('option').map(option => option.element.value)).toEqual(['input_characters'])
    wrapper.unmount()
  })
})
