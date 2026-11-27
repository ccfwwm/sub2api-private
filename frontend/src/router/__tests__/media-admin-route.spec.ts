import { describe, expect, it, vi } from 'vitest'

vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ checkAuth: vi.fn(), isAuthenticated: false, isAdmin: false, isSimpleMode: false }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteName: 'Sub2API', backendModeEnabled: false, cachedPublicSettings: null }) }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/stores/adminCompliance', () => ({ useAdminComplianceStore: () => ({}) }))
vi.mock('@/composables/useNavigationLoading', () => ({ useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn(), isLoading: { value: false } }) }))
vi.mock('@/composables/useRoutePrefetch', () => ({ useRoutePrefetch: () => ({ triggerPrefetch: vi.fn(), cancelPendingPrefetch: vi.fn(), resetPrefetchState: vi.fn() }) }))
vi.mock('@/api/setup', () => ({ getSetupStatus: vi.fn() }))

describe('admin media route', () => {
  it('exposes capabilities and in-app documentation without login', async () => {
    const { default: router } = await import('@/router')
    for (const path of ['/media-capabilities', '/docs']) {
      const route=router.getRoutes().find(r=>r.path===path)
      expect(route?.meta.requiresAuth).toBe(false)
      expect(route?.redirect).toBeUndefined()
    }
    expect(router.resolve('/docs').meta.titleKey).toBe('nav.interfaceDocs')
    for (const path of ['/media-docs', '/docs/imagevideo']) {
      const legacy = router.resolve(path + '?from=home#gpt-edit')
      const redirect = legacy.matched.at(-1)?.redirect
      expect(typeof redirect).toBe('function')
      if (typeof redirect !== 'function') throw new Error('Missing documentation redirect')
      expect(router.resolve(redirect(legacy, legacy)).fullPath).toBe('/docs?from=home#gpt-edit')
    }
  })
  it('makes personal media history available to authenticated ordinary users', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find(record => record.name === 'MediaTasks')
    expect(route?.path).toBe('/media-tasks')
    expect(route?.meta.requiresAuth).toBe(true)
    expect(route?.meta.requiresAdmin).toBe(false)
  })
  it('opens the media management view and remains administrator-only', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find((record) => record.name === 'AdminMedia')

    expect(route?.path).toBe('/admin/media')
    expect(route?.redirect).toBeUndefined()
    expect(route?.meta.requiresAuth).toBe(true)
    expect(route?.meta.requiresAdmin).toBe(true)
    expect(route?.components?.default).toBeTypeOf('function')
  })
})
