import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import MediaProtocolManager from '../MediaProtocolManager.vue'

const api = vi.hoisted(() => ({ list: vi.fn(), history: vi.fn(), save: vi.fn(), updateTemplate: vi.fn(), archive: vi.fn(), preview: vi.fn() }))
const route = vi.hoisted(() => ({ query: {} as Record<string, string> }))
vi.mock('@/api/media-protocols', () => ({ mediaProtocolsAPI: api }))
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('@/composables/useStepUp', () => ({ useStepUp: () => ({ run: (fn: () => Promise<void>) => fn() }), isStepUpCancelled: () => false }))
const options = { global: { stubs: { TotpStepUpDialog: true, RouterLink: true } } }
async function click(wrapper: VueWrapper, label: string) {
  const button = wrapper.findAll('button').find(candidate => candidate.text() === label)
  expect(button, label).toBeDefined(); await button!.trigger('click'); await flushPromises()
}
const row = () => ({ id: 'video-template', version: 3, published_version: 3, status: 'enabled', config: { id: 'video-template', name: '原协议', description: '原介绍', version: '3', schema_version: 1, media_types: ['video'], operations: ['video'], mode: 'async', poll_seconds: 10, auth: {mode:'bearer'}, submit:{method:'POST',path:'/v1/videos',encoding:'json'} } })
beforeEach(() => {
  vi.resetAllMocks(); route.query = {}; api.list.mockResolvedValue([])
  api.updateTemplate.mockImplementation(async body => ({ id: body.id, version: 1, published_version: 1, status: 'enabled', config: body.config }))
})
describe('simple protocol template management', () => {
  it('creates and edits title and introduction through the form, then saves directly', async () => {
    const wrapper = mount(MediaProtocolManager, options); await flushPromises(); await click(wrapper, '新建')
    await wrapper.get('[aria-label="模板名称"]').setValue('自建视频协议')
    await wrapper.get('[aria-label="模板介绍"]').setValue('支持多个图片、视频和音频')
    await click(wrapper, '保存模板')
    expect(api.updateTemplate).toHaveBeenCalledWith(expect.objectContaining({ expected_version: 0, config: expect.objectContaining({name:'自建视频协议',description:'支持多个图片、视频和音频',poll_seconds:10}) }))
    expect(api.history).not.toHaveBeenCalled(); expect(api.save).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('已保存'); expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('草稿 v'); expect(wrapper.text()).not.toContain('历史、')
    wrapper.unmount()
  })
  it('loads the current configuration, copies to a distinct editable template and preserves the source', async () => {
    const source = row(); api.list.mockImplementation(async kind => kind === 'templates' ? [source] : [])
    const wrapper = mount(MediaProtocolManager, options); await flushPromises(); await wrapper.get('aside button').trigger('click'); await flushPromises()
    await wrapper.get('[aria-label="模板名称"]').setValue('改过的标题')
    await click(wrapper, '保存模板')
    expect(api.updateTemplate.mock.calls[0][0]).toMatchObject({id:source.id,expected_version:3,expected_config:source.config,config:{name:'改过的标题'}})
    await click(wrapper, '复制模板'); await click(wrapper, '保存模板')
    const copied=api.updateTemplate.mock.calls[1][0]
    expect(copied.expected_version).toBe(0); expect(copied.id).not.toBe(source.id); expect(copied.config.name).toContain('副本')
    expect(source.config.name).toBe('原协议'); wrapper.unmount()
  })
  it('deletes only after inline confirmation and shows backend conflicts', async () => {
    const source=row(); api.list.mockImplementation(async kind=>kind==='templates'?[source]:[])
    const wrapper=mount(MediaProtocolManager,options); await flushPromises(); await wrapper.get('aside button').trigger('click'); await flushPromises()
    await click(wrapper,'删除模板'); expect(api.updateTemplate).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alertdialog"]').text()).toContain('停用')
    api.updateTemplate.mockRejectedValueOnce(new Error('模板已被修改，请刷新后重试'))
    await click(wrapper,'确认删除'); expect(wrapper.get('[role="alert"]').text()).toContain('模板已被修改')
    api.updateTemplate.mockResolvedValueOnce({}); api.list.mockResolvedValue([])
    await click(wrapper,'确认删除'); expect(api.updateTemplate).toHaveBeenLastCalledWith({id:source.id,expected_version:3,expected_config:source.config,delete:true})
    expect(wrapper.text()).toContain('已删除'); wrapper.unmount()
  })
  it('retains account-scoped bindings when the template list fails', async()=>{
    route.query={tab:'bindings',account_id:'2139',group_id:'19'}
    api.list.mockImplementation(async kind=>{if(kind==='templates')throw Error('Template list unavailable');return [{id:'desired',config:{account_id:2139,group_id:19}},{id:'other-account',config:{account_id:2140,group_id:19}}]})
    const wrapper=mount(MediaProtocolManager,options);await flushPromises()
    expect(wrapper.text()).toContain('desired');expect(wrapper.text()).not.toContain('other-account');expect(wrapper.get('[role="alert"]').text()).toContain('Template list unavailable');wrapper.unmount()
  })
  it('previews with the actual relay key without saving a template',async()=>{
    api.preview.mockResolvedValue({paid_submission:false})
    const wrapper=mount(MediaProtocolManager,options);await flushPromises();await click(wrapper,'新建')
    await wrapper.get('[aria-label="预览 API Key"]').setValue('sk-local-test')
    await wrapper.get('[aria-label="预览候选账号 ID"]').setValue('7')
    await click(wrapper,'预览实际绑定与估价')
    expect(api.preview).toHaveBeenCalledWith(expect.objectContaining({mode:'effective',api_key:'sk-local-test',account_id:7}))
    expect(api.updateTemplate).not.toHaveBeenCalled();wrapper.unmount()
  })
})
