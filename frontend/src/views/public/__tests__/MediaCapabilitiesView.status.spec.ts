import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MediaCapabilitiesView from '../MediaCapabilitiesView.vue'

const { getPublicMediaCatalog } = vi.hoisted(() => ({ getPublicMediaCatalog: vi.fn() }))

vi.mock('@/api/modelPlaza', () => ({ getPublicMediaCatalog }))
describe('MediaCapabilitiesView per-model status', () => {
  it('keeps group filtering neutral and passes each model status to its model list', async () => {
    getPublicMediaCatalog.mockResolvedValue({ groups: [
      { id: 1, name: 'Ready', platform: 'imagevideo', media_availability: 'available', rate_multiplier: 1, media_models: [{ model: 'image-a', availability: 'available' }] },
      { id: 2, name: 'Paused', platform: 'imagevideo', media_availability: 'unavailable', rate_multiplier: 1, media_models: [{ model: 'image-b', availability: 'unavailable' }] },
      { id: 3, name: 'Legacy', platform: 'imagevideo', rate_multiplier: 1, media_models: [{ model: 'image-c' }] }
    ] })
    const wrapper = mount(MediaCapabilitiesView, {
      global: {
        stubs: {
          MediaPublicLayout: { template: '<div><slot /></div>' },
          PlazaMediaPricing: {
            props: ['models'],
            template: '<div class="model-list">{{ models.map(model => `${model.model}:${model.availability || "available"}`).join(",") }}</div>'
          }
        }
      }
    })
    await flushPromises()

    expect(wrapper.findAll('option').map(option => option.text())).toEqual([
      '全部分组', 'Ready', 'Paused', 'Legacy'
    ])
    expect(wrapper.findAll('.model-list').map(node => node.text())).toEqual([
      'image-a:available', 'image-b:unavailable', 'image-c:available'
    ])
    wrapper.unmount()
  })
})
