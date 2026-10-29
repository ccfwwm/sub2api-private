import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MediaMaterialsEditor from '../MediaMaterialsEditor.vue'

describe('media material template editor', () => {
  it('explains the request mapping and lets an administrator add an exact property filter', async () => {
    const wrapper = mount(MediaMaterialsEditor, { props: { modelValue: [] } })
    expect(wrapper.text()).toContain('素材默认按用户输入原样透传')
    expect(wrapper.text()).toContain('b64_json')
    await wrapper.get('button').trigger('click')
    await wrapper.setProps({ modelValue: wrapper.emitted('update:modelValue')?.at(-1)?.[0] as never })
    await wrapper.findAll('textarea')[0].setValue('{"role":"metadata.frame_role","b64_json":"image.data"}')
    await wrapper.setProps({ modelValue: wrapper.emitted('update:modelValue')?.at(-1)?.[0] as never })
    await wrapper.findAll('textarea')[1].setValue('{"metadata.internal":[true]}')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([
      { kind: 'image', target: 'images', encoding: 'preserve', shape: 'preserve', map_properties: { role: 'metadata.frame_role', b64_json: 'image.data' }, drop_properties: { 'metadata.internal': [true] } }
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

  it('keeps the previous valid mapping when new mapping JSON is invalid', async () => {
    const wrapper = mount(MediaMaterialsEditor, { props: { modelValue: [{ kind: 'image', target: 'images', map_properties: { role: 'metadata.role' } }] } })
    await wrapper.findAll('textarea')[0].setValue('{"role":2}')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('字符串映射')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })
  it('supports explicit nested property value and type transformations', async () => {
    const wrapper = mount(MediaMaterialsEditor, { props:{modelValue:[{kind:'image',target:'images',encoding:'preserve',shape:'preserve'}]} })
    const rules=[{op:'map',source:'metadata.role',values:{reference:'input'},on_unmapped:'preserve'},{op:'convert',source:'metadata.weight',type:'number'}]
    await wrapper.findAll('textarea')[2].setValue(JSON.stringify(rules))
    expect((wrapper.emitted('update:modelValue')?.at(-1)?.[0] as Record<string,unknown>[])[0].property_rules).toEqual(rules)
    wrapper.unmount()
  })
})
