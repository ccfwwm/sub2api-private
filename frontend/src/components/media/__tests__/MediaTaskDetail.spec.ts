import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import MediaTaskDetail from '../MediaTaskDetail.vue'
import type { MediaTaskRecord } from '@/api/media'
const api=vi.hoisted(()=>({review:vi.fn(),refund:vi.fn(),verifyCost:vi.fn(),reconcile:vi.fn(),detail:vi.fn(),exportTask:vi.fn(),artifactAccess:vi.fn()}))
vi.mock('@/api/media',()=>({mediaAPI:api}))
vi.mock('vue-i18n',()=>({useI18n:()=>({locale:ref('zh')})}))
const record={task:{id:'task-a',model:'MinimaxH3-2K',status:'unknown',created_at:123,billing:{version:7,quantities:{},state:'manual_review',accepted:'0',settled:'0',refunded:'0'},data:[]}} as unknown as MediaTaskRecord
const create=(admin=true)=>mount(MediaTaskDetail,{props:{record,admin},global:{stubs:{BaseDialog:{template:'<div><slot /></div>'}}}})
describe('Media administrative review',()=>{
 beforeEach(()=>{vi.clearAllMocks();api.detail.mockResolvedValue({record,request:{},response:{}});api.review.mockResolvedValue({})})
 it('hides administrative actions from ordinary users',()=>{const w=create(false);expect(w.find('details').exists()).toBe(false);expect(api.detail).not.toHaveBeenCalled();w.unmount()})
 it('defaults an unknown task to failed with full return without asking for a refund amount',async()=>{
  const w=create();await flushPromises()
  await w.get('[data-testid="accounting-submit"]').trigger('click');expect(api.review).not.toHaveBeenCalled()
  expect(w.find('input[inputmode="decimal"]').exists()).toBe(false)
  await w.get('input[placeholder^="操作原因"]').setValue('上游明确返回503')
  await w.get('[data-testid="accounting-submit"]').trigger('click');await flushPromises()
  expect(api.review).toHaveBeenCalledWith('task-a','failed','0','上游明确返回503',7,expect.any(String));expect(api.refund).not.toHaveBeenCalled();expect(w.emitted('updated')).toHaveLength(1);w.unmount()
 })
 it('refreshes a version conflict and preserves the chosen charge and reason for review',async()=>{
  const w=create();await flushPromises();api.review.mockRejectedValueOnce({status:409,error:{message:'version_conflict'}})
  api.detail.mockResolvedValue({record:{...record,task:{...record.task,billing:{...record.task.billing,version:8}}},request:{},response:{}})
  await w.get('[aria-label="任务状态"]').setValue('completed');await w.get('[aria-label="账务处理"]').setValue('charge')
  await w.get('input[inputmode="decimal"]').setValue('0.2');await w.get('input[placeholder^="操作原因"]').setValue('核对已完成')
  await w.get('[data-testid="accounting-submit"]').trigger('click');await flushPromises()
  expect(w.get('[role="alert"]').text()).toContain('已刷新最新数据');expect(w.get('input[inputmode="decimal"]').element.value).toBe('0.2')
  await w.get('[data-testid="accounting-submit"]').trigger('click');await flushPromises()
  expect(api.review).toHaveBeenLastCalledWith('task-a','completed','0.2','核对已完成',8,expect.any(String));w.unmount()
 })
})

describe('Submission evidence', () => {
 it.each([[null,'未记录'],['prepared','准备发送'],['attempted','已尝试发送'],['accepted','上游已受理']])('distinguishes %s from actual acceptance', async (stage,label) => {
  api.detail.mockResolvedValueOnce({record,request:{images:['draft-only']},response:{},submission:stage?{stage,references:{image:4,video:1,audio:0},request_hash:'fixture-hash',request:{future_option:true}}:null})
  const w=create();await flushPromises()
  expect(w.get('[data-testid="submission-stage"]').text()).toContain(label)
  if(stage)expect(w.get('[data-testid="submission-stage"]').text()).toContain('"video":1')
  else expect(w.get('[data-testid="submission-stage"]').text()).not.toContain('上游已受理')
  w.unmount()
 })
})
