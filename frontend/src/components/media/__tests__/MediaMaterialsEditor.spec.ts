import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MediaMaterialsEditor from '../MediaMaterialsEditor.vue'

describe('media material template editor', () => {
  it('explains the request mapping and lets an administrator add an exact property filter', async () => {
    const wrapper = mount(MediaMaterialsEditor, { props: { modelValue: [] } })
    expect(wrapper.text()).toContain('统一输入字段是')
    expect(wrapper.text()).toContain('{"role":["reference"]}')
    await wrapper.get('button').trigger('click')
    await wrapper.setProps({ modelValue: wrapper.emitted('update:modelValue')?.at(-1)?.[0] as never })
    await wrapper.get('textarea').setValue('{"role":["reference"]}')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([
      { kind: 'image', target: 'images', encoding: 'preserve', shape: 'preserve', drop_properties: { role: ['reference'] } }
    ])
    wrapper.unmount()
  })

  it('keeps the previous valid filter when the new JSON is invalid', async () => {
    const wrapper = mount(MediaMaterialsEditor, { props: { modelValue: [{ kind: 'image', target: 'images', drop_properties: { role: ['reference'] } }] } })
    await wrapper.get('textarea').setValue('not json')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('JSON')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })
})
