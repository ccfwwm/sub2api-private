import test from 'node:test'
import assert from 'node:assert/strict'
import {buildRequest, endpointFor, fullMarkdown, inputMaterialsFor} from './app.mjs'
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
})
test('GPT edit, Gemini modalities, music SSE and frame roles use native fields', () => {
  const cap={model:'test',profile_id:'profile',request_format:'yunfei_multimodal'}
  assert.equal(endpointFor('image_edit','test'),'/v1/images/edits')
  const edit=buildRequest(cap,'image_edit',{prompt:'test'},{image:['https://example.org/a.png'],mask:['data:image/png;base64,AA==']})
  assert.equal(edit.mask,'data:image/png;base64,AA==')
  const gemini=buildRequest(cap,'gemini_image',{prompt:'test',modalities:'both',size:'4K'},{image:['data:image/png;base64,AA==']})
  assert.deepEqual(gemini.generationConfig.responseModalities,['IMAGE','TEXT'])
  assert.deepEqual(gemini.contents[0].parts[1],{inlineData:{mimeType:'image/png',data:'AA=='}})
  const music=buildRequest(cap,'chat_music',{prompt:'test',stream:'true'})
  assert.equal(music.async,false);assert.equal(music.stream,true)
  assert.deepEqual(buildRequest(cap,'video',{prompt:'test'},{first_frame:['https://example.org/a.png']}).images,[{url:'https://example.org/a.png',role:'first_frame'}])
})

test('missing model catalogue counts keep reference inputs and edit masks available', () => {
  const materials={image:{reference_field:'images'},video:{reference_field:'videos'},audio:{reference_field:'audios'}}
  assert.deepEqual(inputMaterialsFor({operations:['video'],materials}),materials)
  assert.deepEqual(inputMaterialsFor({operations:['image_edit'],materials}).mask,{max_file_bytes:50*1048576})
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
