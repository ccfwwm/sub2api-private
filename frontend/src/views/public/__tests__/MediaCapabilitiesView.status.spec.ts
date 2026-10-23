import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MediaCapabilitiesView from '../MediaCapabilitiesView.vue'

const { getPublicMediaCatalog } = vi.hoisted(() => ({ getPublicMediaCatalog: vi.fn() }))

vi.mock('@/api/modelPlaza', () => ({ getPublicMediaCatalog }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => ({
      'modelPlaza.mediaStatus.available': 'Available',
      'modelPlaza.mediaStatus.unavailable': 'Unavailable'
    }[key] ?? key) })
  }
})

describe('MediaCapabilitiesView media group status', () => {
  it('shows availability in the group filter and each media catalog card', async () => {
    getPublicMediaCatalog.mockResolvedValue({ groups: [
      { id: 1, name: 'Ready', platform: 'imagevideo', media_availability: 'available', rate_multiplier: 1, media_models: [{ model: 'image-a' }] },
      { id: 2, name: 'Paused', platform: 'imagevideo', media_availability: 'unavailable', rate_multiplier: 1, media_models: [{ model: 'image-b' }] },
      { id: 3, name: 'Legacy', platform: 'imagevideo', rate_multiplier: 1, media_models: [{ model: 'image-c' }] }
    ] })
    const wrapper = mount(MediaCapabilitiesView, {
      global: {
        stubs: { MediaPublicLayout: { template: '<div><slot /></div>' }, PlazaMediaPricing: true, Icon: true }
      }
    })
    await flushPromises()

    expect(wrapper.findAll('option').map(option => option.text())).toEqual([
      '全部分组', 'Ready（Available）', 'Paused（Unavailable）', 'Legacy（Available）'
    ])
    expect(wrapper.findAll('[data-testid="media-group-status"]').map(node => node.text())).toEqual([
      'Available', 'Unavailable', 'Available'
    ])
    wrapper.unmount()
  })
})
