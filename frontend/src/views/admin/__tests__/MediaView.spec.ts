import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import MediaView from '../MediaView.vue'

const mocks = vi.hoisted(() => ({
  route: { query: {} as Record<string, string> },
  replace: vi.fn()
}))

vi.mock('vue-router', () => ({ useRoute: () => mocks.route, useRouter: () => ({ replace: mocks.replace }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ locale: { value: 'zh' } }) }
})

describe('admin media navigation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.route = reactive({ query: {} }) as typeof mocks.route
    mocks.replace.mockImplementation(({ query }) => { mocks.route.query = query })
  })

  const mountView = () => mount(MediaView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        RouterLink: { template: '<a><slot /></a>' },
        MediaAccountsManager: { template: '<div data-testid="account-models" />' },
        MediaProtocolManager: { template: '<div data-testid="protocol-templates" />' },
        MediaDiagnostics: { template: '<div data-testid="media-diagnostics" />' },
        MediaTaskHistory: { template: '<div data-testid="media-task-history" />' }
      }
    }
  })

  it('keeps the legacy bindings deep link in the single account model editor', async () => {
    mocks.route.query = { tab: 'bindings', group_id: '66', account_id: '2114' }
    const wrapper = mountView()
    expect(wrapper.find('[data-testid="account-models"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="protocol-templates"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('opens templates from the media section sidebar', async () => {
    const wrapper = mountView()
    await wrapper.findAll('button').find(button => button.text() === '协议模板')!.trigger('click')
    await flushPromises()
    expect(mocks.route.query.tab).toBe('templates')
    expect(wrapper.find('[data-testid="protocol-templates"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
