import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MediaDiagnostics from '../MediaDiagnostics.vue'
import { mediaPreviewTimeoutMs } from '@/utils/mediaPreview'

const api = vi.hoisted(() => ({ list: vi.fn(), getById: vi.fn(), getAllIncludingInactive: vi.fn(), effective: vi.fn(), preview: vi.fn(), test: vi.fn(), detail: vi.fn(), artifactAccess: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: api, groups: api } }))
vi.mock('@/api/media-protocols', () => ({ mediaProtocolsAPI: { effective: api.effective, preview: api.preview, test: api.test }, mediaPreviewTimeoutMs: () => 30000 }))
vi.mock('@/api/media', () => ({ mediaAPI: { detail: api.detail, artifactAccess: api.artifactAccess } }))
vi.mock('@/composables/useStepUp', () => ({ useStepUp: () => ({ run: (action: () => Promise<void>) => action() }), isStepUpCancelled: () => false }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: { group_id: '19', account_id: '2139' } }) }))

const capabilities = [
  { model: 'video-choice', media_type: 'video', enabled: true, operations: ['video'], duration: { default: 5, values: [], min: 5 } },
  { model: 'music-choice', media_type: 'audio', enabled: true, operations: ['chat_music'], duration: {} }
]
const account = () => ({ id: 2139, name: 'Linked account', platform: 'imagevideo', group_ids: [19], extra: { media_models: capabilities }, credentials: {} })
const options = { global: { stubs: { RouterLink: true, TotpStepUpDialog: true } } }
const submittedTask = () => ({ id: 'task-91', model: 'video-choice', status: 'completed', submission_phase: 'upstream_accepted', data: [{ key: 'result-1', type: 'video', mime_type: 'video/mp4', size_bytes: 12, available: true }] })
const taskReview = () => ({
  record: { task: submittedTask() },
  incoming_request: { model: 'video-choice', prompt: 'test prompt' },
  normalized_request: { model: 'video-choice', prompt: 'test prompt' },
  prepared_request: { model: 'upstream-video', prompt: 'test prompt' },
  upstream_request: { method: 'POST', path: '/generate', body: { model: 'upstream-video' } },
  submission: { stage: 'accepted', response: { accepted: true } },
  submission_status: 'accepted',
  upstream_response: { status: 'completed', result: 'actual-output' }
})

beforeEach(() => {
  vi.resetAllMocks()
  api.getAllIncludingInactive.mockResolvedValue([{ id: 19, name: 'Video group', platform: 'imagevideo' }])
  api.getById.mockResolvedValue(account())
  api.list.mockResolvedValue({ items: [], total: 0 })
  api.effective.mockResolvedValue({})
  api.preview.mockResolvedValue({ paid_submission: false, normalized: { model: 'video-choice' }, request: { model: 'upstream-video' }, pricing: { maximum_charge: '0.25' } })
  api.test.mockResolvedValue(submittedTask())
  api.detail.mockResolvedValue(taskReview())
  api.artifactAccess.mockResolvedValue({ url: '/api/v1/admin/media/playback/task-91/result-1?access=signed', expires_at: 9999999999 })
})

describe('media diagnostics selectors', () => {
  it('extends large free-preview timeouts while keeping a hard cap', () => {
    expect(mediaPreviewTimeoutMs({ request: { prompt: 'short' } })).toBe(31000)
    expect(mediaPreviewTimeoutMs({ request: { images: [{ b64_json: 'x'.repeat(5 * 1024 * 1024) }] } })).toBe(71000)
    expect(mediaPreviewTimeoutMs({ request: { images: [{ b64_json: 'x'.repeat(40 * 1024 * 1024) }] } })).toBe(180000)
  })

  it('keeps a linked account outside the current account page and loads its model choices', async () => {
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    expect(api.list).toHaveBeenCalledWith(1, 50, expect.objectContaining({ platform: 'imagevideo', group: '19' }))
    expect(wrapper.get('[aria-label="联调账号"]').element).toHaveProperty('value', '2139')
    expect(wrapper.get('[aria-label="联调模型"]').findAll('option').map(option => option.text())).toEqual(expect.arrayContaining(['video-choice', 'music-choice']))
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('该分组没有媒体账号')
    wrapper.unmount()
  })

  it('previews with the selected account, group, model and API key scope', async () => {
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    await wrapper.get('[aria-label="联调 API Key"]').setValue('sk-local-preview')
    await wrapper.get('[aria-label="联调提示词"]').setValue('two reference frames')
    await wrapper.findAll('button').find(button => button.text() === '免费预览映射与估价')!.trigger('click')
    await flushPromises()
    expect(api.preview).toHaveBeenCalledWith(expect.objectContaining({
      mode: 'effective', api_key: 'sk-local-preview', group_id: 19, account_id: 2139, operation: 'video',
      request: expect.objectContaining({ model: 'video-choice', seconds: 5, prompt: 'two reference frames' }), measure_media: false
    }))
    expect(wrapper.text()).toContain('输入请求')
    expect(wrapper.text()).toContain('联调日志')
    expect(wrapper.find('details[open]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('explains preview timeouts as safe to retry without creating or billing a task', async () => {
    api.preview.mockRejectedValueOnce(Object.assign(new Error('timeout of 30000ms exceeded'), { code: 'ECONNABORTED' }))
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    await wrapper.get('[aria-label="联调 API Key"]').setValue('sk-local-preview')
    await wrapper.findAll('button').find(button => button.text() === '免费预览映射与估价')!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('没有创建生成任务、没有扣费')
    expect(wrapper.text()).toContain('可以安全重试')
    expect(wrapper.text()).toContain('没有提交生成请求')
    wrapper.unmount()
  })

  it('removes only its generated duration default when changing from video to audio', async () => {
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    const requestField = wrapper.get('[aria-label="联调请求选项"]')
    expect(JSON.parse((requestField.element as HTMLTextAreaElement).value)).toMatchObject({ model: 'video-choice', seconds: 5 })
    await wrapper.get('[aria-label="联调模型"]').setValue('music-choice')
    await flushPromises()
    expect(JSON.parse((requestField.element as HTMLTextAreaElement).value)).toMatchObject({ model: 'music-choice' })
    expect(JSON.parse((requestField.element as HTMLTextAreaElement).value)).not.toHaveProperty('seconds')
    wrapper.unmount()
  })

  it('uploads image, video and audio references as typed b64_json objects', async () => {
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    const cases = [
      { kind: '图片', mime: 'image/png', filename: 'reference.png', content: 'image-bytes', field: 'images' },
      { kind: '视频', mime: 'video/mp4', filename: 'reference.mp4', content: 'video-bytes', field: 'videos' },
      { kind: '音频', mime: 'audio/mpeg', filename: 'reference.mp3', content: 'audio-bytes', field: 'audios' }
    ]
    for (const item of cases) {
      const selector = `[aria-label="上传参考${item.kind}"]`
      const input = wrapper.get(selector).element as HTMLInputElement
      Object.defineProperty(input, 'files', { configurable: true, value: [new File([item.content], item.filename, { type: item.mime })] })
      await wrapper.get(selector).trigger('change')
      await vi.waitFor(() => expect(wrapper.text()).toContain(item.filename))
      Object.defineProperty(input, 'files', { configurable: true, value: [new File(['second'], 'second-' + item.filename, { type: item.mime })] })
      await wrapper.get(selector).trigger('change')
      await vi.waitFor(() => expect(wrapper.text()).toContain('second-' + item.filename))
    }
    await wrapper.get('[aria-label="联调 API Key"]').setValue('sk-local-preview')
    await wrapper.findAll('button').find(button => button.text() === '免费预览映射与估价')!.trigger('click')
    await flushPromises()
    expect(api.preview).toHaveBeenCalledWith(expect.objectContaining({
      request: expect.objectContaining({
        images: [{ b64_json: 'aW1hZ2UtYnl0ZXM=', mime_type: 'image/png' },{b64_json:'c2Vjb25k',mime_type:'image/png'}],
        videos: [{ b64_json: 'dmlkZW8tYnl0ZXM=', mime_type: 'video/mp4' },{b64_json:'c2Vjb25k',mime_type:'video/mp4'}],
        audios: [{ b64_json: 'YXVkaW8tYnl0ZXM=', mime_type: 'audio/mpeg' },{b64_json:'c2Vjb25k',mime_type:'audio/mpeg'}]
      })
    }))
    expect(wrapper.text()).toContain('reference.png')
    expect(wrapper.text()).toContain('reference.mp4')
    expect(wrapper.text()).toContain('reference.mp3')
    wrapper.unmount()
  })

  it('accepts multiple dragged references and includes them in the request', async () => {
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    const drops = [
      { kind: '图片', field: 'images', mime: 'image/png', files: [['one', 'drop-one.png'], ['two', 'drop-two.png']] },
      { kind: '视频', field: 'videos', mime: 'video/mp4', files: [['three', 'drop-three.mp4'], ['four', 'drop-four.mp4']] },
      { kind: '音频', field: 'audios', mime: 'audio/mpeg', files: [['five', 'drop-five.mp3'], ['six', 'drop-six.mp3']] }
    ] as const
    for (const item of drops) {
      const files = item.files.map(([content, name]) => new File([content], name, { type: item.mime }))
      const event = new Event('drop', { bubbles: true, cancelable: true })
      Object.defineProperty(event, 'dataTransfer', { value: { files } })
      wrapper.get(`[aria-label="参考${item.kind}拖放区域"]`).element.dispatchEvent(event)
      await vi.waitFor(() => expect(wrapper.text()).toContain(item.files[1][1]))
    }
    await wrapper.get('[aria-label="联调 API Key"]').setValue('sk-local-preview')
    await wrapper.findAll('button').find(button => button.text() === '免费预览映射与估价')!.trigger('click')
    await flushPromises()
    const expected = Object.fromEntries(drops.map(item => [item.field, item.files.map(([content]) => ({
      b64_json: Buffer.from(content).toString('base64'), mime_type: item.mime
    }))]))
    expect(api.preview).toHaveBeenCalledWith(expect.objectContaining({ request: expect.objectContaining(expected) }))
    wrapper.unmount()
  })

  it('submits a paid test with an idempotency key, displays evidence, and recovers the same task', async () => {
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    await wrapper.get('[aria-label="联调 API Key"]').setValue('sk-local-preview')
    await wrapper.get('[aria-label="联调提示词"]').setValue('test prompt')
    await wrapper.findAll('button').find(button => button.text() === '免费预览映射与估价')!.trigger('click')
    await flushPromises()
    await wrapper.get('[aria-label="确认真实生成可能收费"]').setValue(true)
    await wrapper.findAll('button').find(button => button.text() === '提交真实生成请求')!.trigger('click')
    await flushPromises()

    expect(api.test).toHaveBeenCalledTimes(1)
    expect(api.test).toHaveBeenCalledWith(expect.objectContaining({
      api_key: 'sk-local-preview', operation: 'video', confirm_paid_submission: true,
      request: expect.objectContaining({ model: 'video-choice', prompt: 'test prompt' })
    }), expect.any(String))
    const idempotencyKey = api.test.mock.calls[0][1]
    expect(idempotencyKey).toMatch(/^[0-9a-f-]{36}$/i)
    expect(api.detail).toHaveBeenCalledWith('task-91')
    expect(api.artifactAccess).toHaveBeenCalledWith('task-91', 'result-1', true)
    expect(wrapper.text()).toContain('真实请求结果')
    expect(wrapper.text()).toContain('任务 ID：task-91')
    expect(wrapper.text()).toContain('生成已完成')
    expect(wrapper.text()).toContain('账单')
    expect(wrapper.text()).toContain('actual-output')
    expect(wrapper.findAll('details[open]')).toHaveLength(0)
    expect(wrapper.get('video').attributes('src')).toContain('access=signed')
    expect(wrapper.get('video').attributes('controls')).toBeDefined()

    await wrapper.findAll('button').find(button => button.text() === '恢复 / 查询原测试')!.trigger('click')
    await flushPromises()
    expect(api.test).toHaveBeenCalledTimes(2)
    expect(api.test.mock.calls[1][1]).toBe(idempotencyKey)
    wrapper.unmount()
  })

  it('does not enable a new test while the original submission result is unknown', async () => {
    const unresolved = { ...submittedTask(), status: 'unknown', submission_phase: 'reconciling', data: [], billing: { state: 'manual_review', reserved: '0.25', accepted: '0.00', settled: '0.00', released: '0.00' } }
    api.test.mockResolvedValueOnce(unresolved)
    api.detail.mockResolvedValueOnce({ record: { task: unresolved }, incoming_request: { model: 'video-choice' } })
    const wrapper = mount(MediaDiagnostics, options)
    await flushPromises()
    await wrapper.get('[aria-label="联调 API Key"]').setValue('sk-local-preview')
    await wrapper.findAll('button').find(button => button.text() === '免费预览映射与估价')!.trigger('click')
    await flushPromises()
    await wrapper.get('[aria-label="确认真实生成可能收费"]').setValue(true)
    await wrapper.findAll('button').find(button => button.text() === '提交真实生成请求')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('受理结果待核对')
    const newTest = wrapper.findAll('button').find(button => button.text() === '开始新测试')!
    expect((newTest.element as HTMLButtonElement).disabled).toBe(true)
    wrapper.unmount()
  })
})
