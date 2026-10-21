import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountTestModal from '../AccountTestModal.vue'
import { newMediaCapability } from '@/types/media'

const { getAvailableModels, copyToClipboard } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels
    }
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.accounts.imagePromptDefault': 'Generate a cute orange cat astronaut sticker on a clean pastel background.'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.accounts.imageReceived' && params?.count) {
          return `received-${params.count}`
        }
        if (key === 'admin.accounts.imagePreviewAlt' && params?.index) {
          return `test-image-${params.index}`
        }
        return messages[key] || key
      }
    })
  }
})

function createStreamResponse(lines: string[]) {
  const encoder = new TextEncoder()
  const chunks = lines.map((line) => encoder.encode(line))
  let index = 0

  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index < chunks.length) {
            return { done: false, value: chunks[index++] }
          }
          return { done: true, value: undefined }
        })
      })
    }
  } as Response
}

function mountModal(account: Record<string, unknown> = {
  id: 42,
  name: 'Gemini Image Test',
  platform: 'gemini',
  type: 'apikey',
  status: 'active'
}) {
  return mount(AccountTestModal, {
    props: {
      show: false,
      account
    } as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Select: { template: '<div class="select-stub"></div>' },
        TextArea: {
          props: ['modelValue'],
          emits: ['update:modelValue'],
          template: '<textarea class="textarea-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
        },
        Icon: true
      }
    }
  })
}

describe('AccountTestModal', () => {
  beforeEach(() => {
    getAvailableModels.mockResolvedValue([
      { id: 'gemini-2.0-flash', display_name: 'Gemini 2.0 Flash' },
      { id: 'gemini-2.5-flash-image', display_name: 'Gemini 2.5 Flash Image' },
      { id: 'gemini-3.1-flash-image', display_name: 'Gemini 3.1 Flash Image' }
    ])
    copyToClipboard.mockReset()
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: vi.fn((key: string) => (key === 'auth_token' ? 'test-token' : null)),
        setItem: vi.fn(),
        removeItem: vi.fn(),
        clear: vi.fn()
      },
      configurable: true
    })
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"gemini-2.5-flash-image"}\n',
        'data: {"type":"image","image_url":"data:image/png;base64,QUJD","mime_type":"image/png"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('gemini 图片模型测试会携带提示词并渲染图片预览', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const promptInput = wrapper.find('textarea.textarea-stub')
    expect(promptInput.exists()).toBe(true)
    await promptInput.setValue('draw a tiny orange cat astronaut')

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'gemini-3.1-flash-image',
      prompt: 'draw a tiny orange cat astronaut',
      test_kind: 'minimal'
    })

    const preview = wrapper.find('img[alt="test-image-1"]')
    expect(preview.exists()).toBe(true)
    expect(preview.attributes('src')).toBe('data:image/png;base64,QUJD')
  })

  it('grok 账号测试默认选择 Grok 模型', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'grok-4.3', display_name: 'Grok 4.3' },
      { id: 'grok-build-0.1', display_name: 'Grok Build 0.1' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"grok-4.3"}\n',
        'data: {"type":"content","text":"ok"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 13,
      name: 'Grok Account',
      platform: 'grok',
      type: 'oauth',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'grok-4.3',
      prompt: '请只回复 OK。',
      test_kind: 'minimal',
      mode: 'text'
    })
  })

  it('OpenAI Compact 探测会携带 compact 测试模式', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'gpt-5.4', display_name: 'GPT-5.4' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 42,
      name: 'OpenAI OAuth',
      platform: 'openai',
      type: 'oauth',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    ;(wrapper.vm as any).selectedModelId = 'gpt-5.4'
    ;(wrapper.vm as any).testMode = 'compact'
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toMatchObject({
      model_id: 'gpt-5.4',
      prompt: '请只回复 OK。',
      test_kind: 'minimal',
      mode: 'compact'
    })
  })
  it('文本账号可编辑提示词并恢复默认', async () => {
    getAvailableModels.mockResolvedValue([{id:'gpt-5.4',display_name:'GPT'}])
    const wrapper = mountModal({id:42,name:'Text',platform:'openai',type:'apikey',status:'active'})
    await wrapper.setProps({show:true}); await flushPromises()
    await wrapper.find('textarea').setValue('Reply CUSTOM')
    await (wrapper.vm as any).startTest(); await flushPromises()
    const body=JSON.parse((global.fetch as any).mock.calls[0][1].body)
    expect(body.prompt).toBe('Reply CUSTOM')
    ;(wrapper.vm as any).resetPrompt()
    expect((wrapper.vm as any).testPrompt).toBe('请只回复 OK。')
    wrapper.unmount()
  })
  it('媒体提交成功仍显示处理中，关闭重开只查询持久任务', async () => {
    const cap=newMediaCapability('omni-flash')
    getAvailableModels.mockResolvedValue([{id:cap.model,display_name:cap.model}])
    const result={test_id:'mt_one',model:cap.model,status:'in_progress',test_kind:'minimal',operation:'video',started_at:new Date().toISOString(),config_changed:false,artifacts:[],request:{seconds:4}}
    global.fetch=vi.fn().mockImplementation((url,opts) => opts?.method==='POST'
      ? Promise.resolve(createStreamResponse([`data: ${JSON.stringify({type:'status',text:'in_progress',data:result})}\n`]))
      : Promise.resolve({ok:true,status:200,json:async()=>({data:result})})) as any
    const wrapper=mountModal({id:42,name:'Media',platform:'imagevideo',type:'apikey',status:'active',credentials:{},extra:{media_models:[cap]}})
    await wrapper.setProps({show:true});await flushPromises()
    expect((wrapper.vm as any).status).toBe('connecting')
    await wrapper.setProps({show:false});await wrapper.setProps({show:true});await flushPromises()
    expect((global.fetch as any).mock.calls.every((call:any[])=>!call[1]?.method)).toBe(true)
    expect((wrapper.vm as any).savedTest.test_id).toBe('mt_one')
    expect((wrapper.vm as any).status).toBe('connecting')
    wrapper.unmount()
  })
  it('关闭且无售价的媒体模型支持自定义参数，并拒绝非对象 JSON', async () => {
    const cap=newMediaCapability('omni-flash')
    cap.media_type='video';cap.operations=['video'];cap.adapter='template';cap.template_ref={id:'configured-video',version:2}
    getAvailableModels.mockResolvedValue([{id:cap.model,display_name:cap.model}])
    global.fetch=vi.fn().mockImplementation((_url,opts)=>opts?.method==='POST'
      ? Promise.resolve(createStreamResponse(['data: {"type":"test_complete","success":false,"error":"failed"}\n']))
      : Promise.resolve({ok:false,status:404})) as any
    const wrapper=mountModal({id:42,name:'Media',platform:'imagevideo',type:'apikey',status:'active',extra:{media_models:[cap]}})
    await wrapper.setProps({show:true});await flushPromises()
    expect((wrapper.vm as any).testOperation).toBe('video')
    ;(wrapper.vm as any).testKind='custom';(wrapper.vm as any).customParameters='{"seconds":6}'
    await (wrapper.vm as any).startTest();await flushPromises()
    const post=(global.fetch as any).mock.calls.find((call:any[])=>call[1]?.method==='POST')
    expect(JSON.parse(post[1].body)).toMatchObject({test_kind:'custom',operation:'video',model_id:cap.model,parameters:{seconds:6}})
    ;(wrapper.vm as any).customParameters='[]'
    const calls=(global.fetch as any).mock.calls.length
    await (wrapper.vm as any).startTest();expect((global.fetch as any).mock.calls.length).toBe(calls)
    wrapper.unmount()
  })

  it.each([
    { type: 'image', model: 'gpt-image-2.5', mime: 'image/png' },
    { type: 'video', model: 'omni-flash', mime: 'video/mp4' },
    { type: 'audio', model: 'lyria-3-pro', mime: 'audio/mp4' }
  ])('$type 持久产物经过认证加载，播放错误明确提示且不会重新生成', async ({ type, model, mime }) => {
    const cap = newMediaCapability(model)
    const source = type === 'image'
      ? `${window.location.origin}/api/v1/admin/accounts/42/tests/mt_saved/artifacts/media1`
      : '/api/v1/admin/accounts/42/tests/mt_saved/artifacts/media1'
    getAvailableModels.mockResolvedValue([{ id: model, display_name: model }])
    const objectURL = `blob:saved-${type}`
    const createObjectURL = vi.fn(() => objectURL)
    const revokeObjectURL = vi.fn()
    const OriginalURL = globalThis.URL
    vi.stubGlobal('URL', Object.assign(class extends OriginalURL {}, { createObjectURL, revokeObjectURL }))
    const result = {
      test_id: 'mt_saved', model, status: 'completed', test_kind: 'minimal', operation: type,
      started_at: new Date().toISOString(), config_changed: false, request: {},
      artifacts: [{ type, content_url: source, mime_type: mime, available: true }]
    }
    global.fetch = vi.fn().mockImplementation((url) => Promise.resolve(url.includes('/artifacts/')
      ? { ok: true, blob: async () => new Blob(['saved media'], { type: mime }) }
      : { ok: true, status: 200, json: async () => ({ data: result }) })) as any
    const wrapper = mountModal({ id: 42, name: 'Media', platform: 'imagevideo', type: 'apikey', status: 'active', extra: { media_models: [cap] } })
    await wrapper.setProps({ show: true })
    await flushPromises()
    const player = type === 'image' ? wrapper.get('img[alt="test-image-1"]') : wrapper.find(type)
    expect(player.attributes('src')).toBe(objectURL)
    const artifactCall = (global.fetch as any).mock.calls.find((call: any[]) => call[0].includes('/artifacts/'))
    expect(artifactCall[1].headers.Authorization).toBe('Bearer test-token')
    await player.trigger('error')
    expect(wrapper.find('[role="alert"]').text()).toContain('浏览器无法播放')
    expect(wrapper.find('a[download]').attributes('href')).toBe(objectURL)
    expect((global.fetch as any).mock.calls.every((call: any[]) => !call[1]?.method)).toBe(true)
    await player.trigger(type === 'image' ? 'load' : 'loadedmetadata')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
    expect(revokeObjectURL).toHaveBeenCalledWith(objectURL)
  })

  it('does not send administrator credentials to external artifact hosts', async () => {
    const cap = newMediaCapability('gpt-image-2.5')
    getAvailableModels.mockResolvedValue([{ id: cap.model, display_name: cap.model }])
    const result = {
      test_id: 'mt_external', model: cap.model, status: 'completed', test_kind: 'minimal', operation: 'image',
      started_at: new Date().toISOString(), config_changed: false, request: {},
      artifacts: [{ type: 'image', content_url: 'https://media.example.test/result.png', mime_type: 'image/png', available: true }]
    }
    global.fetch = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ data: result }) }) as any
    const wrapper = mountModal({ id: 42, name: 'Media', platform: 'imagevideo', type: 'apikey', status: 'active', extra: { media_models: [cap] } })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.get('img[alt="test-image-1"]').attributes('src')).toBe('https://media.example.test/result.png')
    expect((global.fetch as any).mock.calls).toHaveLength(1)
    wrapper.unmount()
  })

  it('discards artifact reads that finish after the dialog closes', async () => {
    const cap = newMediaCapability('gpt-image-2.5')
    getAvailableModels.mockResolvedValue([{ id: cap.model, display_name: cap.model }])
    let finishBlob!: (blob: Blob) => void
    const pendingBlob = new Promise<Blob>(resolve => { finishBlob = resolve })
    const createObjectURL = vi.fn(() => 'blob:stale')
    const OriginalURL = globalThis.URL
    vi.stubGlobal('URL', Object.assign(class extends OriginalURL {}, { createObjectURL, revokeObjectURL: vi.fn() }))
    const result = {
      test_id: 'mt_closed', model: cap.model, status: 'completed', test_kind: 'minimal', operation: 'image',
      started_at: new Date().toISOString(), config_changed: false, request: {},
      artifacts: [{ type: 'image', content_url: '/api/v1/admin/accounts/42/tests/mt_closed/artifacts/media1', available: true }]
    }
    global.fetch = vi.fn().mockImplementation(url => Promise.resolve(url.includes('/artifacts/')
      ? { ok: true, blob: () => pendingBlob }
      : { ok: true, status: 200, json: async () => ({ data: result }) })) as any
    const wrapper = mountModal({ id: 42, name: 'Media', platform: 'imagevideo', type: 'apikey', status: 'active', extra: { media_models: [cap] } })
    void wrapper.setProps({ show: true })
    await flushPromises()
    await wrapper.setProps({ show: false })
    finishBlob(new Blob(['image'], { type: 'image/png' }))
    await flushPromises()
    expect(createObjectURL).not.toHaveBeenCalled()
    expect(wrapper.find('img[alt="test-image-1"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('retains the completed test when artifact retrieval fails and retries only the read', async () => {
    const cap = newMediaCapability('gpt-image-2.5')
    const source = '/api/v1/admin/accounts/42/tests/mt_retry/artifacts/media1'
    getAvailableModels.mockResolvedValue([{ id: cap.model, display_name: cap.model }])
    const result = {
      test_id: 'mt_retry', model: cap.model, status: 'completed', test_kind: 'minimal', operation: 'image',
      started_at: new Date().toISOString(), config_changed: false, request: {},
      artifacts: [{ type: 'image', content_url: source, mime_type: 'image/png', available: true }]
    }
    const objectURL = 'blob:retry-image'
    const OriginalURL = globalThis.URL
    vi.stubGlobal('URL', Object.assign(class extends OriginalURL {}, { createObjectURL: vi.fn(() => objectURL), revokeObjectURL: vi.fn() }))
    let reads = 0
    global.fetch = vi.fn().mockImplementation(url => {
      if (url.includes('/artifacts/')) {
        reads++
        return Promise.resolve(reads === 1 ? { ok: false } : { ok: true, blob: async () => new Blob(['image'], { type: 'image/png' }) })
      }
      return Promise.resolve({ ok: true, status: 200, json: async () => ({ data: result }) })
    }) as any
    const wrapper = mountModal({ id: 42, name: 'Media', platform: 'imagevideo', type: 'apikey', status: 'active', extra: { media_models: [cap] } })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.text()).toContain('mt_retry')
    expect(wrapper.find('[role="alert"]').text()).toContain('浏览器无法播放')
    await wrapper.findAll('button').find(button => button.text() === '重试预览')!.trigger('click')
    await flushPromises()
    expect(reads).toBe(2)
    expect(wrapper.get('img[alt="test-image-1"]').attributes('src')).toBe(objectURL)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect((global.fetch as any).mock.calls.every((call: any[]) => !call[1]?.method)).toBe(true)
    wrapper.unmount()
  })

})
