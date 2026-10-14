import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import MediaTaskHistory from '../MediaTaskHistory.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import type { MediaTaskQuery } from '@/api/media'

const api = vi.hoisted(() => ({ page: vi.fn(), searchKey: vi.fn(), searchUser: vi.fn(), groups: vi.fn(), keys: vi.fn() }))
vi.mock('@/api/media', () => ({ mediaAPI: { taskPage: api.page } }))
vi.mock('@/api/admin', () => ({ adminAPI: { usage: { searchApiKeys: api.searchKey, searchUsers: api.searchUser }, groups: { list: api.groups } } }))
vi.mock('@/api', () => ({ keysAPI: { list: api.keys }, userGroupsAPI: { getAvailable: async () => [{ id: 66, name: '图片音频视频分组' }] } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: ref('zh'), t: (key: string) => key }) }))
const create = (admin = false) => mount(MediaTaskHistory, { props: { admin, startDate: '2026-10-14', endDate: '2026-10-14' }, global: { stubs: { MediaTaskDetail: true } } })

describe('Media task table', () => {
  beforeEach(() => {
    vi.clearAllMocks(); localStorage.clear()
    api.groups.mockResolvedValue({ items: [{ id: 66, name: '图片音频视频分组' }] })
    api.keys.mockResolvedValue({ items: [{ id: 3, name: '自己的 Key' }] })
    api.searchKey.mockResolvedValue([])
    api.page.mockImplementation(async (_admin: boolean, query: MediaTaskQuery) => {
      const page = query.page || 1, size = query.page_size || 20
      const count = query.status === 'failed' ? 0 : 45
      const items = Array.from({ length: Math.max(0, Math.min(size, count-(page-1)*size)) }, (_, i) => ({
        task: { id: `task-${(page-1)*size+i}`, model: 'grok-imagine-video', status: 'completed', created_at: 123, completed_at: 133, data: [], billing: { state: 'settled', accepted: '0', settled: '1', refunded: '0' } },
        media_type: 'video', api_key_name: '自己的 Key', group_name: '图片音频视频分组', user_email: 'owner@test', account_name: 'supplier'
      }))
      return { items, total: count, pages: Math.max(1, Math.ceil(count/size)), page, page_size: size }
    })
  })
  it('loads 3 pages, refreshes current page, resets on a filter, and hides privileged columns', async () => {
    const w = create(); await flushPromises()
    expect(w.findAll('tbody tr')).toHaveLength(20)
    expect(w.text()).not.toContain('owner@test'); expect(w.text()).not.toContain('supplier')
    w.getComponent(Pagination).vm.$emit('update:page', 3); await flushPromises()
    expect(w.findAll('tbody tr')).toHaveLength(5)
    expect(w.text()).toContain('task-40')
    await w.findAll('button').find(b => b.text() === '刷新')!.trigger('click'); await flushPromises()
    expect(api.page.mock.lastCall?.[1].page).toBe(3)
    const status = w.findAllComponents(Select).find(c => c.props('options').some((o: { value: string }) => o.value === 'failed'))!
    status.vm.$emit('update:modelValue', 'failed'); status.vm.$emit('change', 'failed'); await flushPromises()
    expect(api.page.mock.lastCall?.[1].page).toBe(1)
    expect(w.findComponent(Pagination).exists()).toBe(false)
    w.unmount()
  })
  it('keeps regular user Key search local and dates use an exclusive end', async () => {
    const w = create(); await flushPromises()
    await w.get('input[list="media-keys"]').setValue('自己的 Key #3'); await flushPromises()
    expect(api.page.mock.lastCall?.[1].api_key_id).toBe(3)
    await w.get('input[list="media-keys"]').setValue('other key'); await flushPromises()
    expect(api.searchKey).not.toHaveBeenCalled(); expect(api.searchUser).not.toHaveBeenCalled()
    await w.setProps({ endDate: '2026-10-15' }); await flushPromises()
    const q = api.page.mock.lastCall?.[1]
    expect(new Date(q.end_time).getTime()-new Date(q.start_time).getTime()).toBe(2*86400000)
    w.unmount()
  })
  it('shows administrator names and persists optional cost columns', async () => {
    const w = create(true); await flushPromises()
    expect(w.text()).toContain('owner@test'); expect(w.text()).toContain('supplier')
    await w.findAll('button').find(b => b.text() === '列设置')!.trigger('click')
    const label = w.findAll('label').find(l => l.text() === '账号成本')!
    await label.get('input').setValue(true)
    expect(JSON.parse(localStorage.getItem('admin-media-hidden-columns')!)).not.toContain('cost')
    w.unmount()
  })
})
