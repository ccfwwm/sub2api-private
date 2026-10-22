import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import MediaTaskDetail from '../MediaTaskDetail.vue'
import type { MediaTaskRecord } from '@/api/media'
const api=vi.hoisted(()=>({review:vi.fn(),refund:vi.fn(),verifyCost:vi.fn(),reconcile:vi.fn(),detail:vi.fn(),exportTask:vi.fn(),artifactAccess:vi.fn(),userDetail:vi.fn()}))
vi.mock('@/api/media',()=>({mediaAPI:api}))
vi.mock('vue-i18n',()=>({useI18n:()=>({locale:ref('zh')})}))
const record={task:{id:'task-a',model:'MinimaxH3-2K',status:'unknown',created_at:123,billing:{version:7,quantities:{},state:'manual_review',accepted:'0',settled:'0',refunded:'0'},data:[]}} as unknown as MediaTaskRecord
const create=(admin=true)=>mount(MediaTaskDetail,{props:{record,admin},global:{stubs:{BaseDialog:{template:'<div><slot /></div>'}}}})
describe('Media administrative review',()=>{
 beforeEach(()=>{vi.clearAllMocks();api.detail.mockResolvedValue({record,request:{},response:{}});api.userDetail.mockResolvedValue({record});api.review.mockResolvedValue({})})
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

 it('shows incoming, normalized, prepared, upstream request and response separately with media payloads intact', async () => {
  api.detail.mockResolvedValueOnce({
   record, request: { images: ['legacy'] }, response: { status: 'in_progress' },
   incoming_request: { images: ['https://media.example.org/ref.png?variant=large'] },
   normalized_request: { images: [{ url: 'https://media.example.org/ref.png?variant=large', role: 'reference' }] },
   prepared_request: { method: 'POST', path: '/v1/videos', body: { images: ['https://media.example.org/ref.png?variant=large'] } },
   upstream_request: { method: 'POST', path: '/v1/videos', encoding: 'json', body: { images: ['https://media.example.org/ref.png?variant=large'] } },
   submission: { stage: 'attempted', response_status_code: 400, response_content_type: 'application/json', response: { error: 'invalid_request' }, references: { image: 1 }, request: {} }
  })
  const w=create();await flushPromises()
  expect(w.text()).toContain('1. 用户入站请求')
  expect(w.text()).toContain('2. 归一化与计费前参数')
  expect(w.text()).toContain('3. 模板准备请求')
  expect(w.text()).toContain('4. 实际上游请求')
  expect(w.text()).toContain('5. 上游提交与响应')
  expect(w.text()).toContain('HTTP 400')
  expect(w.text()).toContain('variant=large')
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { clipboard: { writeText } })
  await w.get('[data-testid="copy-outgoing"]').trigger('click'); await flushPromises()
  expect(JSON.parse(writeText.mock.calls[0]![0])).toEqual({ images: ['https://media.example.org/ref.png?variant=large'] })
  await w.get('[data-testid="copy-response"]').trigger('click'); await flushPromises()
  expect(JSON.parse(writeText.mock.calls[1]![0])).toEqual({ error: 'invalid_request' })
  vi.unstubAllGlobals()
  w.unmount()
 })
})


describe('Live task details and user artifacts', () => {
 afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks() })
 it('polls active evidence while preserving manual inputs and expanded sections, and stops at completion', async () => {
  vi.useFakeTimers(); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  const active = {...record, task:{...record.task,status:'in_progress'}} as MediaTaskRecord
  api.detail.mockResolvedValue({record:active,incoming_request:{prompt:'live'}})
  const w=create(); await flushPromises()
  await w.get('input[placeholder^="操作原因"]').setValue('核对草稿')
  const sections=w.findAll('details'); sections[2]!.element.open=true
  api.detail.mockResolvedValueOnce({record:{...active,task:{...active.task,status:'completed'}},incoming_request:{prompt:'updated'}})
  await vi.advanceTimersByTimeAsync(3000); await flushPromises()
  expect(w.get('input[placeholder^="操作原因"]').element.value).toBe('核对草稿')
  expect(w.findAll('details')[2]!.element.open).toBe(true)
  expect(w.text()).toContain('updated')
  const count=api.detail.mock.calls.length
  await vi.advanceTimersByTimeAsync(9000); expect(api.detail).toHaveBeenCalledTimes(count)
  w.unmount()
 })
 it('stops polling when closed and ignores late responses for the old task', async () => {
  vi.useFakeTimers()
  let resolve!: (v: unknown) => void
  api.detail.mockImplementationOnce(()=>new Promise(r=>{resolve=r}))
  const w=create(); await w.setProps({record:null})
  resolve({record,incoming_request:{prompt:'stale'}}); await flushPromises()
  await vi.advanceTimersByTimeAsync(9000)
  expect(api.detail).toHaveBeenCalledTimes(1); expect(w.text()).not.toContain('stale'); w.unmount()
 })
 it.each(['image','video','audio'])('lets owners preview and download %s without an API key or admin evidence', async type => {
  const completed={...record,task:{...record.task,status:'completed',data:[{key:'result',type,mime_type:type+'/fixture',available:true}]}} as MediaTaskRecord
  api.userDetail.mockResolvedValue({record:completed})
  api.artifactAccess.mockResolvedValue({url:'/api/v1/media/playback/tasks/task-a/artifacts/result/content?access=fixture',expires_at:123})
  const w=create(false); await flushPromises()
  await w.findAll('button').find(b=>b.text().includes('预览'))!.trigger('click'); await flushPromises()
  expect(api.artifactAccess).toHaveBeenLastCalledWith('task-a','result',false)
  expect(w.find(type==='image'?'img':type).attributes('src')).toContain('/media/playback/')
  expect(w.get('a').attributes('href')).toContain('&download=1')
  expect(w.find('[aria-label="管理员请求与响应证据"]').exists()).toBe(false); w.unmount()
 })
})
