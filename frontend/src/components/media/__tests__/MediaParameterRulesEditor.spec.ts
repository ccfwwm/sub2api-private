import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import Editor from '../MediaParameterRulesEditor.vue'

describe('parameter rule form', () => {
  it('edits reactive rules and preserves numeric zero and false', async () => {
    const wrapper = mount(Editor, { props: { modelValue: reactive([{ phase: 'semantic', op: 'fixed', target: 'optional', value: false }]) } })
    await wrapper.get('[aria-label="规则值 1"]').setValue('true')
    let rules = wrapper.emitted('update:modelValue')!.at(-1)![0] as Record<string, unknown>[]
    expect(rules[0]!.value).toBe(true)
    await wrapper.setProps({ modelValue: rules })
    await wrapper.get('[aria-label="值类型 1"]').setValue('number')
    rules = wrapper.emitted('update:modelValue')!.at(-1)![0] as Record<string, unknown>[]
    expect(rules[0]!.value).toBe(0)
    await wrapper.setProps({ modelValue: rules })
    await wrapper.get('[aria-label="值类型 1"]').setValue('boolean')
    rules = wrapper.emitted('update:modelValue')!.at(-1)![0] as Record<string, unknown>[]
    expect(rules[0]!.value).toBe(false)
  })
  it('edits typed mapping values and removes fields from the previous operation', async () => {
    const wrapper = mount(Editor, { props: { modelValue: [{ phase: 'semantic', op: 'map', source: 'quality', target: 'quality', values: { high: 0, low: false }, on_unmapped: 'preserve' }] } })
    await wrapper.get('[aria-label="映射类型 1-1"]').setValue('boolean')
    let rules = wrapper.emitted('update:modelValue')!.at(-1)![0] as Record<string, unknown>[]
    expect(rules[0]!.values).toEqual({ high: false, low: false })
    await wrapper.setProps({ modelValue: rules })
    await wrapper.get('[aria-label="映射原值 1-1"]').setValue('low')
    expect(wrapper.get('[role="alert"]').text()).toContain('不能重复')
    await wrapper.get('[aria-label="规则类型 1"]').setValue('convert')
    rules = wrapper.emitted('update:modelValue')!.at(-1)![0] as Record<string, unknown>[]
    expect(rules[0]).toEqual({ phase: 'semantic', op: 'convert', source: 'quality', target: 'quality', type: 'string' })
  })
})
