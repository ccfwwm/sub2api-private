import {beforeEach,describe,it,expect,vi} from 'vitest'
import {flushPromises,mount} from '@vue/test-utils'
import {accountMediaCapabilities,mergeDiscoveredMediaModels,newMediaCapability} from '@/types/media'
import MediaAccountModelsEditor from '../MediaAccountModelsEditor.vue'

const templateAPI = vi.hoisted(() => ({list:vi.fn(),history:vi.fn()}))
vi.mock('@/api/media-protocols',()=>({mediaProtocolsAPI:templateAPI}))
const accountAPI = vi.hoisted(() => ({ syncUpstreamModels: vi.fn(), syncUpstreamModelsPreview: vi.fn() }))
vi.mock('@/api/admin',()=>({adminAPI:{accounts:accountAPI}}))
vi.mock('vue-i18n',async original=>({...await original<Record<string,unknown>>(),useI18n:()=>({locale:{value:'zh'}})}))

describe('动态媒体模板配置',()=>{
 it('merges every discovered model idempotently and preserves existing settings',()=>{
  const existing = newMediaCapability('public'); existing.template_ref={id:'pinned',version:3};existing.enabled=true;existing.operations=['video'];existing.template_rules=[{phase:'semantic',op:'fixed',target:'quality',value:0}]
  const merged=mergeDiscoveredMediaModels([existing],['public',' new ','new','','NEW'])
  expect(merged).toHaveLength(3)
  expect(merged[0]).toEqual(existing)
  expect(merged[1]).toMatchObject({model:'new',enabled:false,adapter:'',operations:[]})
  expect(merged[1].template_ref).toBeUndefined()
  expect(mergeDiscoveredMediaModels(merged,['new','public','NEW'])).toEqual(merged)
 })
 it('does not infer a protocol from model names or upstream domains',()=>{
  const named = newMediaCapability('veo-3.1-fast')
  const renamed = newMediaCapability('veo-3.1-fast-copy')
  expect(named.adapter).toBe(''); expect(renamed.adapter).toBe('')
  expect(named.enabled).toBe(false); expect(named.duration.values).toEqual([])
  expect(accountMediaCapabilities({media_models:[named]}, {base_url:'https://img.yunfei.best'})[0].adapter).toBe('')
 })
 it('keeps explicit template references and user overrides unchanged',()=>{
  const cap=newMediaCapability('public-video'); cap.adapter='admin_template'; cap.template_ref={id:'generic-video-array',version:3}; cap.enabled=true; cap.limits.max_concurrent_tasks=7
  const [updated]=accountMediaCapabilities({media_models:[cap]}, {base_url:'https://another-upstream.invalid'})
  expect(updated).toMatchObject({adapter:'admin_template',template_ref:{id:'generic-video-array',version:3},enabled:true})
  expect(updated.limits.max_concurrent_tasks).toBe(7)
 })
 it('preserves explicit model mappings without selecting a provider adapter',()=>{
  expect(accountMediaCapabilities({media_models:[]},{model_mapping:{alias:'upstream-model'}})).toEqual([])
  const [mapped]=accountMediaCapabilities({}, {model_mapping:{alias:'upstream-model'}})
  expect(mapped).toMatchObject({model:'alias',upstream_model:'upstream-model',adapter:'',enabled:false})
 })
})

describe('账号模板输出配置',()=>{
 beforeEach(()=>{
  templateAPI.history.mockResolvedValue([])
  templateAPI.list.mockResolvedValue([
   {id:'multi-parts',published_version:2,config:{name:'Mixed parts',media_types:['image','audio'],operations:['image','audio']},published_config:{name:'Mixed parts',media_types:['image','audio'],operations:['image','audio']}},
   {id:'gemini-audio-parts',published_version:1,config:{name:'Audio parts',media_types:['audio'],operations:['audio']}}
  ])
 })
 async function editor(cap=newMediaCapability('arbitrary-model')) {
  const wrapper=mount(MediaAccountModelsEditor,{props:{modelValue:[cap],baseUrl:'https://example.invalid'},global:{stubs:{AccountTestModal:true}}})
  await flushPromises()
  return wrapper
 }
 it('requires explicit output type and operations for a multi-type template',async()=>{
  const wrapper=await editor()
  await wrapper.get('[aria-label="协议模板"]').setValue('multi-parts')
  const selected=wrapper.emitted('update:modelValue')!.at(-1)![0] as ReturnType<typeof newMediaCapability>[]
  expect(selected[0]).toMatchObject({media_type:'',operations:[],template_ref:{id:'multi-parts',version:2},enabled:false})
  await wrapper.setProps({modelValue:selected})
  await wrapper.get('[aria-label="输出媒体类型"]').setValue('audio')
  const audio=wrapper.emitted('update:modelValue')!.at(-1)![0] as ReturnType<typeof newMediaCapability>[]
  expect(audio[0].media_type).toBe('audio')
  expect(audio[0].operations).toEqual([])
  await wrapper.setProps({modelValue:audio})
  await wrapper.get('[aria-label="启用操作 audio"]').setValue(true)
  const enabled=wrapper.emitted('update:modelValue')!.at(-1)![0] as ReturnType<typeof newMediaCapability>[]
  expect(enabled[0]).toMatchObject({media_type:'audio',operations:['audio'],model:'arbitrary-model',upstream_model:'arbitrary-model'})
  expect(wrapper.find('[aria-label="启用操作 speech"]').exists()).toBe(false)
  wrapper.unmount()
 })
 it('uses the explicit audio-only template and preserves administrator rules',async()=>{
  const cap=newMediaCapability('looks-like-an-image-model')
  cap.template_rules=[{phase:'wire',op:'default',target:'generationConfig.speechConfig',value:{voice:'chosen'}}]
  const wrapper=await editor(cap)
  await wrapper.get('[aria-label="协议模板"]').setValue('gemini-audio-parts')
  const selected=wrapper.emitted('update:modelValue')!.at(-1)![0] as ReturnType<typeof newMediaCapability>[]
  expect(selected[0]).toMatchObject({media_type:'audio',operations:['audio'],template_ref:{id:'gemini-audio-parts',version:1},template_rules:cap.template_rules})
  await wrapper.setProps({modelValue:selected})
  await wrapper.get('[aria-label="账号模板规则覆盖"]').setValue('[{"phase":"wire","op":"default","target":"optional","value":false}]')
  const changed=wrapper.emitted('update:modelValue')!.at(-1)![0] as ReturnType<typeof newMediaCapability>[]
  expect(changed[0].template_rules).toEqual([{phase:'wire',op:'default',target:'optional',value:false}])
  wrapper.unmount()
 })
 it('retains compatible explicit selections when choosing another template',async()=>{
  const cap=newMediaCapability('public')
  cap.media_type='audio';cap.operations=['audio']
  const wrapper=await editor(cap)
  await wrapper.get('[aria-label="协议模板"]').setValue('multi-parts')
  const selected=wrapper.emitted('update:modelValue')!.at(-1)![0] as ReturnType<typeof newMediaCapability>[]
  expect(selected[0]).toMatchObject({media_type:'audio',operations:['audio']})
  wrapper.unmount()
 })
 it('uses the pinned historical template choices instead of the latest published version',async()=>{
  templateAPI.history.mockResolvedValue([{id:'multi-parts',version:1,config:{media_types:['audio'],operations:['speech']}}])
  const cap=newMediaCapability('public');cap.adapter='template';cap.template_ref={id:'multi-parts',version:1};cap.media_type='audio';cap.operations=['speech']
  const wrapper=await editor(cap)
  await wrapper.findAll('button').find(button=>button.text()==='配置请求能力')!.trigger('click')
  expect(templateAPI.history).toHaveBeenCalledWith('templates','multi-parts')
  expect((wrapper.get('[aria-label="启用操作 speech"]').element as HTMLInputElement).checked).toBe(true)
  expect(wrapper.find('[aria-label="启用操作 image"]').exists()).toBe(false)
  expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  wrapper.unmount()
 })
 it('imports all discovered models in one action without replacing existing bindings',async()=>{
  accountAPI.syncUpstreamModelsPreview.mockResolvedValue({models:['arbitrary-model','new-video','new-image','new-video']})
  const cap=newMediaCapability('arbitrary-model');cap.template_ref={id:'multi-parts',version:2};cap.enabled=true
  const wrapper=await editor(cap)
  await wrapper.findAll('button').find(button=>button.text()==='一键拉取全部模型')!.trigger('click')
  await flushPromises()
  const merged=wrapper.emitted('update:modelValue')!.at(-1)![0] as ReturnType<typeof newMediaCapability>[]
  expect(merged.map(item=>item.model)).toEqual(['arbitrary-model','new-video','new-image'])
  expect(merged[0]).toEqual(cap)
  expect(merged.slice(1).every(item=>!item.enabled && !item.template_ref)).toBe(true)
  expect(wrapper.get('[role="status"]').text()).toContain('新增 2 个')
  wrapper.unmount()
 })
})
