import { defineComponent, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MonitorMediaConfig from '../MonitorMediaConfig.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: ref('zh') }) }))

function dialog(models = ['omni-flash', 'veo-3.1-lite']) {
  return mount(defineComponent({
    components: { MonitorMediaConfig },
    setup: () => ({ config: ref<Record<string, unknown> | null>(null), models }),
    template: '<MonitorMediaConfig v-model="config" :models="models" />',
  }))
}

describe('media monitor configuration', () => {
  it('defaults to a connection check and requires an explicit generation choice', async () => {
    const wrapper = dialog()
    expect(wrapper.get('[data-testid="media-monitor-mode"]').element).toHaveProperty('value', 'connection')
    expect(wrapper.find('[data-testid="media-monitor-prompt"]').exists()).toBe(false)
    await wrapper.get('[data-testid="media-monitor-mode"]').setValue('generation')
    const config = wrapper.vm.config?.media_monitor as { mode: string; models: Record<string, { capability: { duration: { values: number[] } } }> }
    expect(config.mode).toBe('generation')
    expect(config.models['omni-flash'].capability.duration.values).toContain(4)
    expect(config.models['veo-3.1-lite'].capability.duration.values).toEqual([8])
    expect(wrapper.text()).toContain('异步任务未结束时只查询原任务')
  })

  it('saves custom prompts, restores defaults and persists an explicit reset', async () => {
    const wrapper = dialog(['omni-flash'])
    await wrapper.get('[data-testid="media-monitor-mode"]').setValue('generation')
    await wrapper.get('[data-testid="media-monitor-prompt"]').setValue('A red ball moves slowly.')
    const read = () => wrapper.vm.config?.media_monitor as { revision?: string; models: Record<string, { parameters: Record<string, unknown> }> }
    expect(read().models['omni-flash'].parameters.prompt).toBe('A red ball moves slowly.')
    await wrapper.findAll('button').find(b => b.text().includes('恢复默认'))!.trigger('click')
    expect(read().models['omni-flash'].parameters.prompt).toBeUndefined()
    await wrapper.findAll('button').find(b => b.text().includes('重置检测'))!.trigger('click')
    expect(read().revision).toBeTruthy()
  })

  it('loads a known upstream preset for an alias and rejects incomplete advanced JSON', async () => {
    const wrapper = dialog(['custom-video'])
    await wrapper.get('[data-testid="media-monitor-upstream"]').setValue('omni-flash')
    const config = wrapper.vm.config?.media_monitor as { models: Record<string, { capability: { model: string; adapter: string } }> }
    expect(config.models['custom-video'].capability.model).toBe('custom-video')
    expect(config.models['custom-video'].capability.adapter).toBe('yunfei_omni')
    await wrapper.get('details textarea').setValue('{"model":"custom-video"}')
    await wrapper.findAll('button').find(b => b.text().includes('应用能力'))!.trigger('click')
    expect(wrapper.get('[role="alert"]').text()).toContain('有效')
    expect(config.models['custom-video'].capability.adapter).toBe('yunfei_omni')
  })
})
