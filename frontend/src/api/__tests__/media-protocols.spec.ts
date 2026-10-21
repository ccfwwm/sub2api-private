import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mediaProtocolsAPI } from '@/api/media-protocols'

const request = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: request }))

describe('media protocol admin API', () => {
  beforeEach(() => vi.clearAllMocks())

  it('uses dedicated import, publish and rollback endpoints', async () => {
    request.post.mockResolvedValue({ data: { id: 'tpl', version: 2 } })
    await mediaProtocolsAPI.import('templates', { id: 'tpl', config: { name: 'x' } })
    await mediaProtocolsAPI.publish('templates', { id: 'tpl', expected_version: 1, config: { name: 'x' } })
    await mediaProtocolsAPI.rollback('templates', { id: 'tpl', expected_version: 2, source_version: 1 })
    expect(request.post.mock.calls.map((call) => call[0])).toEqual([
      '/admin/media/templates/import',
      '/admin/media/templates/publish',
      '/admin/media/templates/rollback'
    ])
  })
})
