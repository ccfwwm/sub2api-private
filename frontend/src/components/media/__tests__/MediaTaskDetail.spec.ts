import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import MediaTaskDetail from '../MediaTaskDetail.vue'
import type { MediaTaskRecord } from '@/api/media'

const api = vi.hoisted(() => ({ refund: vi.fn(), verifyCost: vi.fn(), reconcile: vi.fn() }))
vi.mock('@/api/media', () => ({ mediaAPI: api }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: ref('zh') }) }))
const record = { task: { id: 'task-a', model: 'lyria-3.5', status: 'completed', created_at: 123, billing: { version: 7, quantities: {}, accepted: '0', settled: '1', refunded: '0' }, data: [] } } as unknown as MediaTaskRecord
const create = (admin = true) => mount(MediaTaskDetail, { props: { record, admin }, global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } } })

describe('Media accounting details', () => {
  beforeEach(() => { vi.clearAllMocks(); api.refund.mockResolvedValue({}); api.verifyCost.mockResolvedValue({}); api.reconcile.mockResolvedValue({}) })
  it('only displays accounting controls for administrators', () => {
    const w = create(false)
    expect(w.find('details').exists()).toBe(false)
    expect(w.text()).toContain('lyria-3.5'); w.unmount()
  })
  it('requires a reason and sends the selected task version with refund and cost actions', async () => {
    const w = create()
    await w.get('button').trigger('click')
    expect(api.refund).not.toHaveBeenCalled()
    await w.get('input[inputmode="decimal"]').setValue('0.2')
    await w.get('input[placeholder^="操作原因"]').setValue('verified receipt')
    await w.get('button').trigger('click'); await flushPromises()
    expect(api.refund).toHaveBeenCalledWith('task-a','0.2','verified receipt',7,expect.any(String))
    expect(w.emitted('updated')).toHaveLength(1)
    await w.setProps({ record: { ...record, task: { ...record.task, billing: { ...record.task.billing, version: 8 } } } })
    await w.get('select').setValue('cost')
    await w.get('input[inputmode="decimal"]').setValue('0.1')
    await w.get('input[placeholder^="操作原因"]').setValue('cost receipt')
    await w.get('button').trigger('click'); await flushPromises()
    expect(api.verifyCost).toHaveBeenCalledWith('task-a','0.1','cost receipt',8,expect.any(String)); w.unmount()
  })
  it('keeps version conflicts visible and preserves reconciliation values', async () => {
    const w = create()
    api.reconcile.mockRejectedValue({ message: 'version_conflict: refresh task' })
    await w.get('select').setValue('reconcile')
    await w.findAll('select')[1]!.setValue('query')
    await w.get('input[placeholder^="上游任务"]').setValue('provider-task')
    await w.get('input[placeholder^="操作原因"]').setValue('acceptance receipt')
    await w.get('button').trigger('click'); await flushPromises()
    expect(api.reconcile).toHaveBeenCalledWith('task-a','query','provider-task','acceptance receipt',7)
    expect(w.get('[role="alert"]').text()).toContain('version_conflict')
    expect(w.emitted('updated')).toBeUndefined(); w.unmount()
  })
})
