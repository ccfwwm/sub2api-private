import test from 'node:test'
import assert from 'node:assert/strict'
import {API_ORIGIN, buildRequest, categories, dataURLToMaterial, docsVersionBadge, endpointFor, fullMarkdown, inputMaterialsFor, loadDocsVersion, optionValues, parseMaterialText, parsePendingRecovery, previewRequestTimeoutMs, profilesForCategory, requestTimeoutMs, serializePendingRecovery, taskBillingSummary, taskPhaseLabel} from './app.mjs'
import {readFile} from 'node:fs/promises'

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

test('Grok video is part of the video API category; only four media API categories are exposed',()=>{
  const mediaCategories=categories.filter(category=>category.ops.length>0)
  assert.equal(mediaCategories.length,4)
  assert.equal(categories.some(category=>category.id==='grok-video'),false)
  assert.ok(categories.find(category=>category.id==='video').sections.includes('grok-video-image'))
})

test('category model selection tolerates incomplete catalogs and keeps video aliases',()=>{
 const category=categories.find(item=>item.id==='video')
 assert.deepEqual(profilesForCategory([{model:'missing'},{model:'video-alias',operations:['video']},{model:'image',operations:['image']}],category).map(item=>item.model),['video-alias'])
})
