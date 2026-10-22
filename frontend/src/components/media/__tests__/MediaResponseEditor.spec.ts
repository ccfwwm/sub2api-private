import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import MediaResponseEditor from '../MediaResponseEditor.vue'

describe('media response editor', () => {
  it('edits candidate paths while retaining advanced parsing fields', async () => {
    const modelValue = { task_id: ['id'], states: { succeeded: 'completed' }, results: [{ path: 'data[]', url: 'url' }], usage: { seconds: 'usage.seconds' } }
    const wrapper = mount(MediaResponseEditor, { props: { modelValue } })
    await wrapper.get('[aria-label="上游任务 ID路径"]').setValue('id\ndata.task_id\n')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ ...modelValue, task_id: ['id', 'data.task_id'] })
    await wrapper.get('[aria-label="统一状态 1"]').setValue('failed')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ ...modelValue, states: { succeeded: 'failed' } })
  })

  it('does not overwrite valid response parsing with malformed advanced JSON', async () => {
    const wrapper = mount(MediaResponseEditor, { props: { modelValue: { urls: ['data[].url'] } } })
    await wrapper.get('[aria-label="完整响应配置 JSON"]').setValue('[]')
    expect(wrapper.find('[role="alert"]').text()).toContain('已保留')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
