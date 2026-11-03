import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import Editor from '../OpenAIImageParamRulesEditor.vue'

vi.mock('vue-i18n', async (original) => ({
  ...await original<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

describe('OpenAI image parameter rules', () => {
  it('loads existing rules and emits deletion and multipart key mappings', async () => {
    const wrapper = mount(Editor, { props: { modelValue: { drop: ['response_format'], rename: { size: 'resolution' } } } })
    expect((wrapper.get('[data-testid="image-param-drop"]').element as HTMLInputElement).value).toBe('response_format')
    await wrapper.get('[data-testid="image-param-rename"]').setValue('size=resolution\nimage[]=files[]')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({
      drop: ['response_format'], rename: { size: 'resolution', 'image[]': 'files[]' },
    })
    expect(wrapper.emitted('validity')?.at(-1)).toEqual([true])
  })

  it.each(['a=x\nb=x', 'a=x\na=y', 'bad field=x', 'a=x=y'])('blocks invalid mappings: %s', async value => {
    const wrapper = mount(Editor, { props: { modelValue: null } })
    await wrapper.get('[data-testid="image-param-rename"]').setValue(value)
    expect(wrapper.emitted('validity')?.at(-1)).toEqual([false])
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('clears rules without changing unrelated account settings', async () => {
    const wrapper = mount(Editor, { props: { modelValue: { drop: ['response_format'] } } })
    await wrapper.get('[data-testid="image-param-drop"]').setValue('')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([null])
  })
})
