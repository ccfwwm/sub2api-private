import test from 'node:test'
import assert from 'node:assert/strict'
import {buildRequest, endpointFor, fullMarkdown} from './app.mjs'
import {readFile} from 'node:fs/promises'

test('URL-only material is never silently dropped or converted', () => {
  const cap={model:'video',profile_id:'profile',request_format:'v6k_url',materials:{image:{reference_field:'images'}}}
  assert.throws(()=>buildRequest(cap,'video',{prompt:'test'},{image:['data:image/png;base64,AA==']}),/HTTP/)
  assert.deepEqual(buildRequest(cap,'video',{prompt:'test',seconds:'5'},{image:['https://example.org/image.png']}).images,[{url:'https://example.org/image.png'}])
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
  assert.equal(buildRequest(cap,'video',{prompt:'test'},{first_frame:['https://example.org/a.png']}).first_frame,'https://example.org/a.png')
})
test('published document and export share one source without old branding',async()=>{
  const content=JSON.parse(await readFile(new URL('../docs/multimodal/imagevideo-content.json',import.meta.url),'utf8'))
  const markdown=fullMarkdown(content)
  assert.match(markdown,/\/v1\/images\/edits/)
  assert.match(markdown,/4096/)
  assert.doesNotMatch(markdown,/yunfei\.best|v6k\.top|待验证|坤肉AI|云飞/)
  for(const section of content)assert.match(markdown,new RegExp(section.title.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')))
})
