import test from 'node:test'
import assert from 'node:assert/strict'
import {API_ORIGIN, buildRequest, categories, dataURLToMaterial, docsVersionBadge, endpointFor, endpointForStreaming, fullMarkdown, inputMaterialsFor, loadDocsVersion, navigationGroups, optionValues, parseErrorMessage, parseGeneralModels, parseMaterialText, parsePendingRecovery, previewRequestTimeoutMs, profilesForCategory, requestTimeoutMs, serializePendingRecovery, taskBillingSummary, taskPhaseLabel} from './app.mjs'
import {readFile} from 'node:fs/promises'
import {documentationLink,filterDocumentation,renderDocumentationBlock,sectionMarkdown} from './app.mjs'

test('documentation search accepts multiple terms and preserves the directory for unmatched input', () => {
  const index = [
    { id: 'codex', text: 'codex cli 配置教程 openai responses' },
    { id: 'claude', text: 'claude code 配置教程 401' },
    { id: 'errors', text: '常见报错 404 网页内容' },
  ]
  assert.deepEqual(filterDocumentation(index, ' 配置教程　ＣＯＤＥＸ '), { ids: ['codex'], filtering: true })
  assert.deepEqual(filterDocumentation(index, '网页 ４０４'), { ids: ['errors'], filtering: true })
  assert.deepEqual(filterDocumentation(index, 'saved-login@example.invalid'), { ids: [], filtering: false })
  assert.deepEqual(filterDocumentation(index, '　'), { ids: [], filtering: false })
})

test('public requests preserve all references without selecting a supplier protocol', () => {
  const cap={model:'video',profile_id:'profile',request_format:'v6k_url',materials:{image:{reference_field:'images'}}}
  const images=['https://example.org/image.png','data:image/png;base64,AA==',{b64_json:'AA==',role:'first_frame',weight:0}]
  const videos=['data:video/mp4;base64,AAAA'],audios=[{url:'https://example.org/audio.wav'}]
  const request=buildRequest(cap,'video',{prompt:'test',seconds:'5'},{image:images,video:videos,audio:audios})
  assert.deepEqual(request.images,images)
  assert.deepEqual(request.videos,videos)
  assert.deepEqual(request.audios,audios)
  assert.equal(request.reference_images,undefined)
  assert.deepEqual(buildRequest({...cap,model:'renamed',request_format:'anything'},'video',{prompt:'test'},{image:images}).images,images)
  assert.deepEqual(buildRequest(cap,'video',{prompt:'test'},{image:[{b64_json:'AA==',mime_type:'image/png',role:'first_frame'}]}).images,[{b64_json:'AA==',mime_type:'image/png',role:'first_frame'}])
})

test('general model catalogs preserve protocol metadata and build sync and streaming requests',()=>{
  const models=parseGeneralModels({data:[{id:'gpt-test',owned_by:'openai'},{id:'claude-test',type:'model'},{id:'jev-latest',type:'model'},{id:'image-only',owned_by:'sub2api'}]},{models:[{name:'models/gemini-test'}]})
  assert.deepEqual(models.find(model=>model.model==='gpt-test').operations,['chat_completions','responses','messages'])
  assert.deepEqual(models.find(model=>model.model==='gemini-test').operations,['gemini_content'])
  assert.deepEqual(models.find(model=>model.model==='jev-latest').operations,['systemone'])
  assert.equal(models.some(model=>model.model==='image-only'),false)
  assert.equal(endpointFor('gemini_content','gemini-test'),'/v1beta/models/gemini-test:generateContent')
  assert.equal(endpointForStreaming('gemini_content','gemini-test',true),'/v1beta/models/gemini-test:streamGenerateContent?alt=sse')
  assert.equal(endpointForStreaming('gemini_content','gemini-test',false),'/v1beta/models/gemini-test:generateContent')
  assert.deepEqual(buildRequest({model:'gpt-test'},'chat_completions',{prompt:'hello',stream:'true'}),{model:'gpt-test',messages:[{role:'user',content:'hello'}],stream:true})
  assert.deepEqual(buildRequest({model:'gpt-test'},'responses',{prompt:'hello',stream:'false'}),{model:'gpt-test',input:'hello',stream:false})
  assert.match(parseErrorMessage('{"error":{"message":"denied"}}'),/denied/)
})

test('configuration and troubleshooting navigation resolves every page from its content collection',async()=>{
  const knowledge=JSON.parse(await readFile(new URL('./knowledge-content.json',import.meta.url),'utf8'))
  const general=JSON.parse(await readFile(new URL('./general-content.json',import.meta.url),'utf8'))
  const media=JSON.parse(await readFile(new URL('../docs/multimodal/imagevideo-content.json',import.meta.url),'utf8'))
  const ids=new Set([...knowledge,...general,...media].map(section=>section.id))
  assert.ok(navigationGroups.some(group=>group.title==='快速上手'))
  assert.ok(navigationGroups.some(group=>group.title==='国内使用与选型'))
  assert.ok(navigationGroups.find(group=>group.title==='常见报错与接入').sections.includes('api-404-html'))
  for(const group of navigationGroups)for(const id of group.sections)assert.ok(ids.has(id),`${id} should have a rendered document`)
  assert.match(knowledge.find(section=>section.id==='api-key-401').title,/401/)
  assert.match(JSON.stringify(knowledge.find(section=>section.id==='api-key-401')),/x-goog-api-key/)
  assert.ok(knowledge.find(section=>section.id==='chatgpt-model-not-found').blocks.some(block=>block.text?.includes('wire_api = "responses"')))
  assert.ok(knowledge.find(section=>section.id==='cc-switch-config').blocks.some(block=>block.kind==='heading'&&block.text==='常见问题'))
  assert.match(knowledge.find(section=>section.id==='api-404-html').title,/404/)
  assert.match(general.find(section=>section.id==='guide').blocks.find(block=>block.label==='cURL').text,/https:\/\/code\.aicodeme\.xyz\/v1\/chat\/completions/)
  assert.doesNotMatch(general.find(section=>section.id==='guide').blocks.find(block=>block.label==='cURL').text,/\n\+/)
  assert.doesNotMatch(fullMarkdown([...knowledge,...general,...media]),/apikl\.ai|code\.aicodeme\.cn/)
})

test('all 28 guides have full article bodies and their links resolve to published sections',async()=>{
  const catalog=JSON.parse(await readFile(new URL('./knowledge-content.json',import.meta.url),'utf8'))
  const guides=catalog.filter(page=>page.slug)
  const ids=new Set(navigationGroups.flatMap(group=>group.sections))
  assert.equal(guides.length,28)
  assert.equal(new Set(guides.map(page=>page.slug)).size,guides.length)
  for(const page of guides){
    assert.ok(page.lead.length&&page.blocks.length>=6,page.id)
    for(const [,id] of JSON.stringify(page).matchAll(/"a","#([a-z0-9-]+)"/g))assert.ok(ids.has(id),`${page.id}: ${id}`)
  }
  const desktop=guides.find(page=>page.id==='codex-desktop')
  const article=sectionMarkdown(desktop)
  assert.match(article,/Microsoft|微软商店/)
  assert.match(article,/9PLM9XGG6VKS/)
  assert.match(article,/https:\/\/code\.aicodeme\.xyz\/v1/)
  assert.match(article,/### 常见问题与报错/)
  const config=guides.find(page=>page.id==='codex-config')
  assert.ok(config.blocks.some(block=>block.kind==='code'&&block.text.includes('setx OPENAI_API_KEY')))
  assert.ok(config.blocks.filter(block=>block.kind==='code').length>=10)
})

test('rich content renders escaped markup and rejects unsafe links',()=>{
  const callout=renderDocumentationBlock({kind:'callout',content:[
    '<script>', ['strong','重点'], ['code','/v1'], ['a','javascript:alert(1)','bad'], ['a','#codex-config','教程'],
  ]})
  assert.match(callout,/&lt;script&gt;<strong>重点<\/strong><code>\/v1<\/code>bad<a href="#codex-config">教程<\/a>/)
  assert.doesNotMatch(callout,/javascript:/)
  assert.equal(documentationLink('https://user:password@example.org'),'')
  assert.equal(documentationLink('//example.org'),'')
  const sample=renderDocumentationBlock({kind:'code',label:'Bash',text:'curl "https://example.org"\n  # note'},'',true)
  assert.equal((sample.match(/class="code-line"/g)||[]).length,2)
  assert.match(sample,/class="code-string"/)
  assert.match(sample,/class="code-comment"/)
})

test('documentation sidebar version follows public settings and has a safe fallback', async()=>{
  assert.equal(docsVersionBadge('v1.3.18'),'MEDIA API · v1.3.18')
  assert.equal(docsVersionBadge('1.3.18'),'MEDIA API · v1.3.18')
  assert.equal(docsVersionBadge('<script>'),'MEDIA API')
  assert.equal(await loadDocsVersion(async(url,options)=>{
    assert.match(url,/\/api\/v1\/settings\/public$/)
    assert.equal(options.credentials,'omit')
    return {ok:true,json:async()=>({data:{version:'v1.3.18'}})}
  }),'MEDIA API · v1.3.18')
  const attempted=[]
  assert.equal(await loadDocsVersion(async url=>{
    attempted.push(url)
    return url.startsWith('https://local.example')
      ? {ok:false,json:async()=>({})}
      : {ok:true,json:async()=>({data:{version:'1.3.18'}})}
  },'https://local.example'),'MEDIA API · v1.3.18')
  assert.equal(attempted[0],'https://local.example/api/v1/settings/public')
  assert.equal(attempted[1],`${API_ORIGIN}/api/v1/settings/public`)
  assert.equal(await loadDocsVersion(async()=>{throw new Error('offline')}),'MEDIA API')
})

test('managed music uses the asynchronous task path and does not request SSE',async()=>{
  const request=buildRequest({model:'lyria-3.5',profile_id:'profile'},'chat_music',{prompt:'test',stream:'true'})
  assert.equal(request.async,true)
  assert.equal(request.stream,false)
  const content=JSON.parse(await readFile(new URL('../docs/multimodal/imagevideo-content.json',import.meta.url),'utf8'))
  assert.match(fullMarkdown(content),/托管音乐请求统一先持久化并返回 HTTP 202/)
  assert.doesNotMatch(fullMarkdown(content),/流式响应仍按其协议返回/)
})
test('GPT edit, Gemini modalities, music SSE and frame roles use native fields', () => {
  const cap={model:'test',profile_id:'profile',request_format:'yunfei_multimodal'}
  assert.equal(endpointFor('image_edit','test'),'/v1/images/edits')
  const edit=buildRequest(cap,'image_edit',{prompt:'test'},{image:['https://example.org/a.png'],mask:['data:image/png;base64,AA==']})
  assert.equal(edit.mask,'data:image/png;base64,AA==')
  const gemini=buildRequest(cap,'gemini_image',{prompt:'test',modalities:'both',size:'4K'},{image:['data:image/png;base64,AA==']})
  assert.deepEqual(gemini.generationConfig.responseModalities,['IMAGE','TEXT'])
  assert.deepEqual(gemini.contents[0].parts[1],{inlineData:{mimeType:'image/png',data:'AA=='}})
  const uploaded=dataURLToMaterial('data:image/jpeg;base64,QUJD')
  assert.deepEqual(uploaded,{b64_json:'QUJD',mime_type:'image/jpeg'})
  const geminiUpload=buildRequest(cap,'gemini_image',{prompt:'test'},{image:[uploaded]})
  assert.deepEqual(geminiUpload.contents[0].parts[1],{inlineData:{mimeType:'image/jpeg',data:'QUJD'}})
  const music=buildRequest(cap,'chat_music',{prompt:'test',stream:'true'})
  assert.equal(music.async,true);assert.equal(music.stream,false)
  assert.deepEqual(buildRequest(cap,'video',{prompt:'test'},{first_frame:['https://example.org/a.png']}).images,[{url:'https://example.org/a.png',role:'first_frame'}])
})

test('missing model catalogue counts keep reference inputs and edit masks available', () => {
  const materials={image:{reference_field:'images'},video:{reference_field:'videos'},audio:{reference_field:'audios'}}
  assert.deepEqual(inputMaterialsFor({operations:['video'],materials}),materials)
  assert.deepEqual(inputMaterialsFor({operations:['image_edit'],materials}).mask,{max_file_bytes:50*1048576})
  assert.deepEqual(Object.keys(inputMaterialsFor({operations:['video']})),['image','video','audio'])
  const refs=[{b64_json:'AA==',mime_type:'image/png',metadata:{weight:0.7}},{url:'https://example.org/second.png'}]
  assert.deepEqual(parseMaterialText(JSON.stringify(refs,null,2)),refs)
  assert.deepEqual(parseMaterialText(refs.map(ref=>JSON.stringify(ref)).join('\n')),refs)
  assert.throws(()=>parseMaterialText('{"url":'))
})

test('optional capability lists accept null and malformed values',()=>{
 assert.deepEqual(optionValues(null),[])
 assert.deepEqual(optionValues({values:['720p']}),[])
 assert.deepEqual(inputMaterialsFor({operations:null,materials:null}),{})
 assert.deepEqual(inputMaterialsFor({operations:['video'],materials:null}),{image:{},video:{},audio:{}})
 assert.deepEqual(inputMaterialsFor({operations:['video'],materials:{image:null,video:['invalid']}}),{image:{},video:{},audio:{}})
 assert.deepEqual(inputMaterialsFor({operations:['image_edit'],materials:null}).mask,{max_file_bytes:50*1048576})
})

test('speech uses the public speech input and configured voice', () => {
  assert.equal(endpointFor('speech','example'),'/v1/audio/speech')
  const request=buildRequest({model:'speech',profile_id:'p'},'speech',{prompt:'汉😀é',voice:'configured-voice',response_format:'wav',speed:'1.1'})
  assert.equal(request.input,'汉😀é')
  assert.equal(request.prompt,undefined)
  assert.equal(request.voice,'configured-voice')
  assert.equal(request.response_format,'wav')
  assert.equal(request.speed,1.1)
})
test('published document and export share one source without old branding',async()=>{
  const content=JSON.parse(await readFile(new URL('../docs/multimodal/imagevideo-content.json',import.meta.url),'utf8'))
  const markdown=fullMarkdown(content)
  assert.match(markdown,/\/v1\/images\/edits/)
  assert.match(markdown,/4096/)
  assert.doesNotMatch(markdown,/yunfei\.best|v6k\.top|待验证|坤肉AI|云飞/)
  for(const section of content)assert.match(markdown,new RegExp(section.title.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')))
})

test('async task labels distinguish local acceptance from upstream acceptance and show billing',()=>{
  assert.equal(taskPhaseLabel({status:'queued',submission_phase:'local_pending'}),'本站已受理，等待后台提交上游')
  assert.equal(taskPhaseLabel({status:'in_progress',submission_phase:'upstream_accepted'}),'上游已受理，正在生成')
  assert.match(taskBillingSummary({state:'held',reserved:'0.04',accepted:'0.00',settled:'0.00',released:'0.00'}),/预留 0\.04.*已结算 0\.00/)
})

test('large free previews get a bounded timeout and recovery metadata excludes credentials and prompts',()=>{
  assert.equal(previewRequestTimeoutMs({prompt:'small'}),31000)
  assert.equal(previewRequestTimeoutMs({images:[{b64_json:'x'.repeat(5*1024*1024)}]}),71000)
  assert.equal(previewRequestTimeoutMs({images:[{b64_json:'x'.repeat(40*1024*1024)}]}),180000)
  const pending={key:'idem-1',operation:'image_edit',id:'task-1',pollSeconds:7}
  const serialized=serializePendingRecovery(pending)
  assert.deepEqual(parsePendingRecovery(serialized),{key:'idem-1',operation:'image_edit',id:'task-1',terminal:false,pollSeconds:7})
  assert.doesNotMatch(serialized,/api_key|prompt|secret/i)
  assert.equal(parsePendingRecovery(serializePendingRecovery({...pending,terminal:true})).terminal,true)
  assert.equal(parsePendingRecovery('{broken'),null)
})

test('large async submissions get a bounded upload timeout and retain a recovery path',()=>{
  assert.equal(requestTimeoutMs('POST','/v1/videos',{async:true,prompt:'small'}),31000)
  assert.equal(requestTimeoutMs('POST','/v1/images/edits',{async:true,images:[{b64_json:'x'.repeat(5*1024*1024)}]}),71000)
  assert.equal(requestTimeoutMs('POST','/v1/videos',{async:true,images:[{b64_json:'x'.repeat(40*1024*1024)}]}),180000)
  assert.equal(requestTimeoutMs('GET','/v1/tasks/task-1'),30000)
})

test('Grok video is part of the video API category alongside the general interface categories',()=>{
  const mediaOperations=['image','image_edit','gemini_image','video','audio','chat_music','speech']
  const mediaCategories=categories.filter(category=>category.ops.some(operation=>mediaOperations.includes(operation)))
  assert.equal(mediaCategories.length,4)
  assert.equal(categories.find(category=>category.id==='text').title,'文本与对话')
  assert.equal(categories.some(category=>category.id==='grok-video'),false)
  assert.ok(categories.find(category=>category.id==='video').sections.includes('grok-video-image'))
})

test('category model selection tolerates incomplete catalogs and keeps video aliases',()=>{
 const category=categories.find(item=>item.id==='video')
 assert.deepEqual(profilesForCategory([{model:'missing'},{model:'video-alias',operations:['video']},{model:'image',operations:['image']}],category).map(item=>item.model),['video-alias'])
})
