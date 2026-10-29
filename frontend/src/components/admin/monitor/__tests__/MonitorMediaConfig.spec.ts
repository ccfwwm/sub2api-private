import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MonitorMediaConfig from '../MonitorMediaConfig.vue'
import { newMediaCapability, type MediaCapability } from '@/types/media'

const templateAPI = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('@/api/media-protocols', () => ({ mediaProtocolsAPI: templateAPI }))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: ref('zh') }) }))

function dialog(models = ['omni-flash', 'veo-3.1-lite'], initial: Record<string, unknown> | null = null) {
  return mount(defineComponent({
    components: { MonitorMediaConfig },
    setup: () => ({ config: ref(initial), models: ref(models) }),
    template: '<MonitorMediaConfig v-model="config" :models="models" />',
  }))
}

describe('media monitor configuration', () => {
  beforeEach(() => {
    templateAPI.list.mockReset().mockResolvedValue([
      { id: 'video-template', version: 4, published_version: 2, status: 'draft', config: { name: 'Unsaved draft', status: 'draft', media_types: ['image'], operations: ['image'] }, published_config: { name: 'Published video', status: 'enabled', media_types: ['video'], operations: ['video'] } },
      { id: 'mixed-template', version: 1, published_version: 1, status: 'enabled', config: { name: 'Mixed', status: 'enabled', media_types: ['image', 'audio'], operations: ['image', 'audio'] } },
      { id: 'draft-only', version: 1, published_version: null, status: 'draft', config: { name: 'Draft only' } },
    ])
  })
  const read = (wrapper: ReturnType<typeof dialog>) => wrapper.vm.config?.media_monitor as { mode: string; models: Record<string, { capability: MediaCapability; operation: string; parameters: Record<string, unknown> }>; revision?: string }

  it('preserves exact model IDs without inferring or suggesting a protocol', async () => {
    const wrapper = dialog(['gpt-image2.5'])
    await flushPromises()
    await wrapper.get('[data-testid="media-monitor-mode"]').setValue('generation')
    expect(wrapper.get('[data-testid="media-monitor-upstream"]').element).toHaveProperty('value', 'gpt-image2.5')
    expect(wrapper.text()).not.toContain('gpt-image-2.5')
    await wrapper.get('[data-testid="media-monitor-upstream"]').setValue('omni-flash')
    expect(read(wrapper).models['gpt-image2.5'].capability).toMatchObject({ model: 'gpt-image2.5', upstream_model: 'omni-flash', adapter: '', operations: [] })
    expect(wrapper.find('[data-testid="media-monitor-generation-blocked"]').exists()).toBe(true)
  })
  it('defaults to a connection check and requires an explicit generation choice', async () => {
    const wrapper = dialog()
    await flushPromises()
    expect(wrapper.get('[data-testid="media-monitor-mode"]').element).toHaveProperty('value', 'connection')
    expect(wrapper.find('[data-testid="media-monitor-prompt"]').exists()).toBe(false)
    await wrapper.get('[data-testid="media-monitor-mode"]').setValue('generation')
    const config = read(wrapper)
    expect(config.mode).toBe('generation')
    expect(config.models).toEqual({})
    expect(wrapper.get('[data-testid="media-monitor-generation-blocked"]').text()).toContain('未配置时不会生成')
    expect(wrapper.findComponent(MonitorMediaConfig).emitted('validity')?.at(-1)).toEqual([false])
    expect(wrapper.text()).toContain('异步任务未结束时只查询原任务')
  })

  it('saves custom prompts, restores defaults and persists an explicit reset', async () => {
    const wrapper = dialog(['omni-flash'])
    await flushPromises()
    await wrapper.get('[data-testid="media-monitor-mode"]').setValue('generation')
    await wrapper.get('[data-testid="media-monitor-prompt"]').setValue('A red ball moves slowly.')
    expect(read(wrapper).models['omni-flash'].parameters.prompt).toBe('A red ball moves slowly.')
    await wrapper.findAll('button').find(b => b.text().includes('恢复默认'))!.trigger('click')
    expect(read(wrapper).models['omni-flash'].parameters.prompt).toBeUndefined()
    await wrapper.findAll('button').find(b => b.text().includes('重置检测'))!.trigger('click')
    expect(read(wrapper).revision).toBeTruthy()
  })

  it('selects the current published template without exposing versions and rejects incomplete advanced JSON', async () => {
    const wrapper = dialog(['custom-video'])
    await flushPromises()
    await wrapper.get('[data-testid="media-monitor-mode"]').setValue('generation')
    await wrapper.get('[data-testid="media-monitor-upstream"]').setValue('omni-flash')
    expect(read(wrapper).models['custom-video'].capability.adapter).toBe('')
    const choices = wrapper.get('[data-testid="media-monitor-template"]')
    expect(choices.text()).not.toContain('Draft only')
    expect(choices.text()).not.toContain('Unsaved draft')
    expect(choices.text()).toContain('Published video')
    expect(choices.text()).not.toMatch(/v\d+/)
    await choices.setValue('video-template')
    const entry = read(wrapper).models['custom-video']
    expect(entry).toMatchObject({ capability: { model: 'custom-video', upstream_model: 'omni-flash', adapter: 'template', template_ref: { id: 'video-template', version: 2 }, media_type: 'video', operations: ['video'] }, operation: 'video' })
    expect(wrapper.find('[data-testid="media-monitor-generation-blocked"]').exists()).toBe(false)
    expect(wrapper.findComponent(MonitorMediaConfig).emitted('validity')?.at(-1)).toEqual([true])
    await wrapper.get('details textarea').setValue('{"model":"custom-video"}')
    await wrapper.findAll('button').find(b => b.text().includes('应用能力'))!.trigger('click')
    expect(wrapper.get('[role="alert"]').text()).toContain('有效')
    expect(read(wrapper).models['custom-video']).toEqual(entry)
  })

  it('requires explicit output and operation selections for a mixed template', async () => {
    const wrapper = dialog(['looks-like-video'])
    await flushPromises()
    await wrapper.get('[data-testid="media-monitor-mode"]').setValue('generation')
    await wrapper.get('[data-testid="media-monitor-template"]').setValue('mixed-template')
    expect(read(wrapper).models['looks-like-video']).toMatchObject({ capability: { media_type: '', operations: [] }, operation: '' })
    expect(wrapper.find('[data-testid="media-monitor-generation-blocked"]').exists()).toBe(true)
    await wrapper.get('[data-testid="media-monitor-type"]').setValue('audio')
    await wrapper.get('[data-testid="media-monitor-operation"]').setValue('audio')
    expect(read(wrapper).models['looks-like-video']).toMatchObject({ capability: { media_type: 'audio', operations: ['audio'] }, operation: 'audio' })
    expect(wrapper.find('[data-testid="media-monitor-generation-blocked"]').exists()).toBe(false)
  })

  it('preserves missing saved templates, parameters and models outside the current list', async () => {
    const cap = newMediaCapability('saved')
    cap.template_ref = { id: 'missing-template', version: 7 }; cap.adapter = 'template'; cap.media_type = 'video'; cap.operations = ['video']
    const initial = { existing_option: { preserve: true }, media_monitor: { mode: 'generation', models: { saved: { capability: cap, operation: 'video', parameters: { seconds: 8, quality: 'max' } }, hidden: { capability: newMediaCapability('hidden'), parameters: { prompt: 'retain' } } } } }
    const wrapper = dialog(['saved'], initial)
    await flushPromises()
    expect(wrapper.findComponent(MonitorMediaConfig).emitted('update:modelValue')).toBeUndefined()
    expect(read(wrapper)).toEqual(initial.media_monitor)
    expect(wrapper.get('[data-testid="media-monitor-template"]').text()).toContain('missing-template · 模板不可用')
    expect(wrapper.get('[data-testid="media-monitor-template"]').text()).not.toContain('v7')
    expect(wrapper.find('[data-testid="media-monitor-generation-blocked"]').exists()).toBe(true)
    await wrapper.get('[data-testid="media-monitor-upstream"]').setValue('exact-upstream')
    expect(wrapper.vm.config).toHaveProperty('existing_option', { preserve: true })
    expect(read(wrapper).models.saved.parameters).toEqual({ seconds: 8, quality: 'max' })
    expect(read(wrapper).models.hidden).toEqual(initial.media_monitor.models.hidden)
    expect(read(wrapper).models.saved.capability.template_ref).toEqual({ id: 'missing-template', version: 7 })
  })

  it('uses the current template for legacy references without loading history', async () => {
    const cap = newMediaCapability('saved')
    cap.template_ref = { id: 'video-template', version: 1 }; cap.adapter = 'template'; cap.media_type = 'audio'; cap.operations = ['audio']
    const initial = { media_monitor: { mode: 'generation', models: { saved: { capability: cap, operation: 'audio', parameters: { seconds: 5 } } } } }
    const wrapper = dialog(['saved'], initial)
    await flushPromises()
    expect(templateAPI).not.toHaveProperty('history')
    expect(read(wrapper)).toEqual(initial.media_monitor)
    expect(wrapper.find('[data-testid="media-monitor-generation-blocked"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="media-monitor-type"]').text()).toContain('video')
  })

  it('does not overwrite saved configuration when the administrator catalogue fails', async () => {
    templateAPI.list.mockRejectedValue(new Error('unavailable'))
    const initial = { media_monitor: { mode: 'generation', models: { saved: { capability: newMediaCapability('saved'), operation: 'video', parameters: { quality: 'max' } } } } }
    const wrapper = dialog(['saved'], initial)
    await flushPromises()
    expect(read(wrapper)).toEqual(initial.media_monitor)
    expect(wrapper.findComponent(MonitorMediaConfig).emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.text()).toContain('模板目录加载失败')
    expect(wrapper.get('[data-testid="media-monitor-template"]').attributes('disabled')).toBeDefined()
    expect(wrapper.findComponent(MonitorMediaConfig).emitted('validity')?.at(-1)).toEqual([false])
  })
})
