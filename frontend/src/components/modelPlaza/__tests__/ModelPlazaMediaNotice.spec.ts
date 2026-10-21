import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createPinia, setActivePinia } from 'pinia'
import ModelPlazaContent from '../ModelPlazaContent.vue'

describe('model plaza partial media status', () => {
  it('displays a readable partial-load message alongside available groups', () => {
    setActivePinia(createPinia())
    const wrapper = mount(ModelPlazaContent, {
      props: { response: { description: '', groups: [{ id: 1, name: 'Available', platform: 'imagevideo', rate_multiplier: 1, models: [], media_models: [] }], notices: ['media_catalog_partial_unavailable'] }, loading: false, embedded: true },
      global: {
        plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { modelPlaza: { partialUnavailable: () => '部分媒体模型暂时无法加载。', anonymousHint: () => '' } } } })],
        stubs: { PlazaFilterBar: true, PlazaGroupSection: { props: ['group'], template: '<div>{{ group.name }}</div>' }, Icon: true }
      }
    })
    expect(wrapper.get('[role="status"]').text()).toContain('部分媒体模型暂时无法加载')
    expect(wrapper.text()).toContain('Available')
    wrapper.unmount()
  })
})
