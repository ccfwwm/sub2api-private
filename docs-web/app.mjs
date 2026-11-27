export const profilesForCategory = (profiles, category) => profiles.filter(p => Array.isArray(p.operations) && p.operations.some(op => category.ops.includes(op)))
export const optionValues = values => Array.isArray(values) ? values : []
export function parseErrorMessage(body) {
  try{const data=JSON.parse(body);return data?.error?.message||data?.message||body.slice(0,400)}catch{return String(body).slice(0,400)||'请求失败'}
}
export const API_ORIGIN = 'https://code.aicodeme.xyz'
export function docsVersionBadge(version) {
  const value=typeof version==='string'?version.trim():''
  if(!/^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(value))return 'MEDIA API'
  return `MEDIA API · ${value.startsWith('v')?value:`v${value}`}`
}
export async function loadDocsVersion(fetcher=globalThis.fetch,origin=globalThis.location?.origin) {
  if(typeof fetcher!=='function')return docsVersionBadge()
  const origins=[...new Set([origin,API_ORIGIN].filter(value=>typeof value==='string'&&/^https?:\/\//.test(value)))]
  for(const base of origins) {
    try {
      const response=await fetcher(`${base}/api/v1/settings/public`,{credentials:'omit',cache:'no-store',headers:{Accept:'application/json'},signal:AbortSignal.timeout(5000)})
      if(!response.ok)continue
      const payload=await response.json()
      const badge=docsVersionBadge(payload?.data?.version)
      if(badge!=='MEDIA API')return badge
    } catch {}
  }
  return docsVersionBadge()
}
export const categories = [
  { id:'guide', title:'快速接入', sections:['guide'], ops:[] },
  { id:'text', title:'文本与对话', sections:['text-api'], ops:['chat_completions','responses','messages','gemini_content','systemone'] },
  { id:'gpt-image', title:'OpenAI协议生图', sections:['gpt-text','gpt-edit','gpt-result'], ops:['image','image_edit'] },
  { id:'gemini-image', title:'Gemini协议生图', sections:['gemini','gemini-result'], ops:['gemini_image'] },
  { id:'video', title:'视频生成', sections:['grok-video-submit','grok-video-image','grok-video-result','omni-video-submit','omni-video-result','veo-text','veo-result','other-video','video-reference'], ops:['video'] },
  { id:'audio', title:'音乐与语音', sections:['audio-chat','audio-submit','audio-compat','audio-task-flow','audio-speech'], ops:['audio','chat_music','speech'] },
  { id:'tasks', title:'任务查询与错误处理', sections:['start','models','tasks','errors','clients'], ops:[] },
]
export const navigationGroups = [
  { title:'快速上手', sections:['guide','openai-compatible','start','models'] },
  { title:'客户端配置', sections:['cc-switch-config','nodejs-environment','codex-config','codex-cli','codex-desktop','codex-import','claude-code-config','claude-code-usage','gemini-api-cli','cursor-openai-compatible','cline-roo-config','trae-solo-config','workbuddy-config','openclaw-config','hermes-agent-config'] },
  { title:'文本与图片接口', sections:['text-api','image-api','gpt-text','gpt-edit','gpt-result','banana-image-api','gemini','gemini-result'] },
  { title:'视频与音频接口', sections:['grok-video-submit','grok-video-image','grok-video-result','omni-video-submit','omni-video-result','veo-text','veo-result','other-video','video-reference','audio-chat','audio-submit','audio-compat','audio-task-flow','audio-speech'] },
  { title:'任务、用量与示例', sections:['tasks','balance-query','clients'] },
  { title:'使用场景与选型', sections:['claude-api-cn','chatgpt-cn','api-relay-selection','why-aicodeme','about-aicodeme'] },
  { title:'常见报错与故障排查', sections:['api-errors','api-key-401','api-404-html','model-not-exist','chatgpt-model-not-found','errors'] },
]
const inlineTags = new Set(['strong','em','code'])
const listTags = new Set(['ul','ol','li','p'])

export function documentationLink(href) {
  if(typeof href!=='string')return ''
  if(/^#[a-z0-9-]+$/.test(href))return href
  try {
    const url=new URL(href)
    return url.protocol==='https:'&&!url.username&&!url.password?url.href:''
  } catch { return '' }
}

function inlineMarkup(nodes, markdown=false) {
  return nodes.map(node=>{
    if(typeof node==='string')return markdown?node:escape(node)
    const [tag,...children]=node
    if(tag==='br')return markdown?'  \n':'<br>'
    if(tag==='a') {
      const url=documentationLink(children[0]),label=inlineMarkup(children.slice(1),markdown)
      return !url?label:markdown?`[${label}](${url})`:`<a href="${escape(url)}"${url.startsWith('#')?'':' target="_blank" rel="noopener noreferrer"'}>${label}</a>`
    }
    const value=inlineMarkup(children,markdown)
    if(markdown&&tag==='code') {
      const fence='`'.repeat(1+Math.max(0,...[...value.matchAll(/`+/g)].map(match=>match[0].length)))
      return `${fence}${value.includes('`')?' ':''}${value}${value.includes('`')?' ':''}${fence}`
    }
    if(markdown&&tag==='strong')return `**${value}**`
    if(markdown&&tag==='em')return `*${value}*`
    return !markdown&&inlineTags.has(tag)?`<${tag}>${value}</${tag}>`:value
  }).join('')
}

function listMarkup(node, markdown=false, depth=0) {
  const [tag,...children]=node
  if(!listTags.has(tag))return ''
  if(tag==='p')return markdown?inlineMarkup(children,true):`<p>${inlineMarkup(children)}</p>`
  if(tag==='li') {
    const parts=children.map(child=>Array.isArray(child)&&listTags.has(child[0])?listMarkup(child,markdown,depth+1):inlineMarkup([child],markdown))
    return markdown?parts.join('').trim():`<li>${parts.join('')}</li>`
  }
  const items=children.filter(child=>Array.isArray(child)&&child[0]==='li')
  if(!markdown)return `<${tag}>${items.map(item=>listMarkup(item)).join('')}</${tag}>`
  const indent='  '.repeat(Math.max(0,depth-1))
  return (depth>1?'\n':'')+items.map((item,index)=>`${indent}${tag==='ol'?`${index+1}.`:'-'} ${listMarkup(item,true,depth)}`).join('\n')
}

const plainText = nodes => nodes.map(node=>typeof node==='string'?node:plainText(node.slice(node[0]==='a'?2:1))).join('')
const headingText = block => block.content?plainText(block.content):block.text
const cellMarkup = (cell,markdown=false) => Array.isArray(cell)?inlineMarkup(cell,markdown):markdown?String(cell):escape(cell)

export function sectionMarkdown(section) {
  const row = cells => '| '+cells.map(v=>cellMarkup(v,true).replaceAll('|','\\|').replaceAll('\n','<br>')).join(' | ')+' |'
  const intro=section.lead?inlineMarkup(section.lead,true):section.subtitle
  return `## ${section.title}\n\n${intro||''}\n\n`+section.blocks.map(b=>{
    if(b.kind==='heading') return `${b.level===3?'####':'###'} ${b.content?inlineMarkup(b.content,true):b.text}`
    if(b.kind==='paragraph') return inlineMarkup(b.content,true)
    if(b.kind==='callout') return `> ${inlineMarkup(b.content,true)}`
    if(b.kind==='list') return listMarkup(b.content,true)
    if(b.kind==='endpoint') return `**${b.method}** ${API_ORIGIN}${b.path}`
    if(b.kind==='code') {
      const fence='`'.repeat(1+Math.max(2,...[...b.text.matchAll(/`{3,}/g)].map(match=>match[0].length)))
      return `### ${b.label}\n\n${fence}${b.language}\n${b.text}\n${fence}`
    }
    if(b.kind==='table') return [row(b.headers),row(b.headers.map(()=>'---')),...b.rows.map(row)].join('\n')
    return b.text
  }).join('\n\n')
}
export const fullMarkdown = sections => '# 词元互联接口文档\n\n根地址：'+API_ORIGIN+'\n\n'+sections.map(sectionMarkdown).join('\n\n')
export function endpointFor(operation, model) {
  return {image:'/v1/images/generations',image_edit:'/v1/images/edits',video:'/v1/videos',audio:'/v1/audio/generations',speech:'/v1/audio/speech',chat_music:'/v1/chat/completions',gemini_image:`/v1beta/models/${encodeURIComponent(model)}:generateContent`,chat_completions:'/v1/chat/completions',responses:'/v1/responses',messages:'/v1/messages',gemini_content:`/v1beta/models/${encodeURIComponent(model)}:generateContent`,systemone:'/v1/systemone'}[operation]
}
export function endpointForStreaming(operation, model, streaming) {
  const endpoint=endpointFor(operation,model)
  return operation==='gemini_content'&&streaming?endpoint.replace(/:generateContent$/,'')+':streamGenerateContent?alt=sse':endpoint
}
export function parseGeneralModels(payload, geminiPayload) {
  const byID=new Map()
  const add=(id,operations,owner='')=>{
    const model=String(id||'').replace(/^models\//,'').trim()
    if(!model||owner==='sub2api')return
    const existing=byID.get(model)
    byID.set(model,{model,media_type:'text',operations:[...new Set([...(existing?.operations||[]),...operations])]})
  }
  for(const item of Array.isArray(payload?.data)?payload.data:[]) {
    const id=item?.id||item?.name
    if(String(id)==='jev-latest')add(id,['systemone'])
    else add(id,['chat_completions','responses','messages'],item?.owned_by)
  }
  for(const item of Array.isArray(geminiPayload?.models)?geminiPayload.models:[])add(item?.name,['gemini_content'])
  return [...byID.values()].sort((a,b)=>a.model.localeCompare(b.model))
}
export function previewRequestTimeoutMs(payload) {
  const bytes = new TextEncoder().encode(JSON.stringify(payload)).byteLength
  return Math.min(180000, 30000 + Math.ceil(bytes / (128 * 1024)) * 1000)
}
export function requestTimeoutMs(method, path, payload) {
  if (path === '/v1/sub2api/estimates' || method === 'POST' && payload?.async === true) return previewRequestTimeoutMs(payload || {})
  if (method === 'POST' && path === '/v1/chat/completions' && payload?.stream) return 185000
  return 30000
}
export function taskPhaseLabel(task) {
  if (task?.status === 'completed') return '生成已完成'
  if (task?.status === 'failed') return '生成失败'
  if (task?.status === 'unknown') return '上游受理结果待核对'
  return ({local_pending:'本站已受理，等待后台提交上游',submitting:'本站正在提交上游',upstream_accepted:'上游已受理，正在生成',reconciling:'正在核对上游是否已受理'})[task?.submission_phase] || (task?.status === 'queued' ? '本站任务排队中' : '任务处理中')
}
export function taskBillingSummary(billing) {
  if (!billing) return '账单信息暂不可用'
  return `账单 ${billing.state||'未知'} · 预留 ${billing.reserved??'0'} · 已接受 ${billing.accepted??'0'} · 已结算 ${billing.settled??'0'} · 已释放 ${billing.released??'0'}`
}
export function parsePendingRecovery(value) {
  try {
    const item=JSON.parse(value)
    if(item?.version!==1||typeof item.key!=='string'||typeof item.operation!=='string')return null
    return {key:item.key,operation:item.operation,id:typeof item.id==='string'?item.id:null,terminal:item.terminal===true,pollSeconds:Number(item.pollSeconds)||10}
  } catch { return null }
}
export function serializePendingRecovery(pending) {
  if(!pending?.key||!pending?.operation)return ''
  return JSON.stringify({version:1,key:pending.key,operation:pending.operation,id:pending.id||null,terminal:pending.terminal===true,pollSeconds:pending.pollSeconds||10})
}
export function dataURLToMaterial(value) {
  const match=String(value).match(/^data:([^;,]+);base64,([\s\S]*)$/)
  if(!match)throw Error('文件 Data URL 格式无效')
  return {b64_json:match[2],mime_type:match[1]}
}
export function parseMaterialText(value) {
  const text=String(value).trim()
  if(!text)return []
  if(text.startsWith('['))return JSON.parse(text)
  if(text.startsWith('{')){
    try{return [JSON.parse(text)]}catch(error){if(!text.includes('\n'))throw error}
  }
  return text.split(/\r?\n/).map(line=>line.trim()).filter(Boolean).map(line=>line.startsWith('{')?JSON.parse(line):line)
}
export function inputMaterialsFor(cap) {
  const operations=optionValues(cap.operations),materials=cap.materials&&typeof cap.materials==='object'&&!Array.isArray(cap.materials)?cap.materials:{}
  return { ...(operations.includes('video') ? {image:{},video:{},audio:{}} : {}), ...Object.fromEntries(Object.entries(materials).map(([kind,rule])=>[kind,rule&&typeof rule==='object'&&!Array.isArray(rule)?rule:{}])), ...(operations.includes('image_edit') && !materials.mask ? { mask: { max_file_bytes: 50 * 1048576 } } : {}) }
}
export function buildRequest(cap, operation, fields, materials={}) {
  if(['chat_completions','responses','messages','gemini_content','systemone'].includes(operation)) {
    const prompt=fields.prompt||''
    if(operation==='chat_completions')return {model:cap.model,messages:[{role:'user',content:prompt}],stream:fields.stream==='true'}
    if(operation==='responses')return {model:cap.model,input:prompt,stream:fields.stream==='true'}
    if(operation==='messages')return {model:cap.model,max_tokens:Number(fields.max_tokens)||1024,messages:[{role:'user',content:prompt}],stream:fields.stream==='true'}
    if(operation==='gemini_content')return {contents:[{role:'user',parts:[{text:prompt}]}]}
    return {model:'jev-latest',state:{},questions:{answer:{type:'noul',instructions:prompt||'回答问题'}}}
  }
  const p={model:cap.model,public_profile_id:cap.profile_id,prompt:fields.prompt,async:true,n:1}
  for(const key of ['size','quality','background','output_format','resolution','ratio','lyrics','bpm','audio_format','negative_prompt']) if(fields[key]!=='' && fields[key]!=null) p[key]=fields[key]
  if(fields.seconds!=='' && fields.seconds!=null) p.seconds=Number(fields.seconds)
  if(fields.generate_audio!=='' && fields.generate_audio!=null) p.generate_audio=fields.generate_audio==='true'
  if(fields.instrumental!=='' && fields.instrumental!=null) p.instrumental=fields.instrumental==='true'
  if(p.background==='transparent' && p.output_format==='jpeg') throw Error('透明背景请选择 PNG 或 WebP')
  if(operation==='gemini_image') {
    const parts=[{text:p.prompt},...(materials.image||[]).map(value=>{
      if(typeof value==='string'&&value.startsWith('data:')) {const material=dataURLToMaterial(value);return {inlineData:{mimeType:material.mime_type,data:material.b64_json}}}
      if(value&&typeof value==='object'&&value.b64_json) return {inlineData:{mimeType:value.mime_type||value.mimeType||'application/octet-stream',data:value.b64_json}}
      const url=typeof value==='string'?value:value?.url
      if(typeof url==='string')return {fileData:{fileUri:url}}
      throw Error('Gemini 参考图需提供 URL 或 b64_json')
    })]
    return {public_profile_id:cap.profile_id,async:true,contents:[{role:'user',parts}],generationConfig:{candidateCount:1,responseModalities:(!fields.modalities||fields.modalities==='both')?['IMAGE','TEXT']:[fields.modalities],imageConfig:{...(fields.ratio?{aspectRatio:fields.ratio}:{}),...(fields.size?{imageSize:fields.size}:{})}}}
  }
  if(operation==='image_edit') {p.images=materials.image||[];if(materials.mask?.[0])p.mask=materials.mask[0]}
  else if(operation==='chat_music') {p.messages=[{role:'user',content:p.prompt}];delete p.prompt}
  else if(operation==='speech') {p.input=p.prompt;delete p.prompt;delete p.n;for(const key of ['voice','response_format','speed'])if(fields[key]!==''&&fields[key]!=null)p[key]=key==='speed'?Number(fields[key]):fields[key]}
  for(const kind of ['image','video','audio']) {
    if(materials[kind]?.length)p[kind+'s']=[...materials[kind]]
  }
  for(const role of ['first_frame','last_frame']) {
    for(const reference of materials[role]||[]) {
      p.images||=[]
      p.images.push(typeof reference==='string'?{url:reference,role}:{...reference,role})
    }
  }
  // Managed media always uses the durable task protocol. Keep `stream` as a
  // compatibility input for clients, but do not advertise a direct SSE
  // response when the gateway returns HTTP 202 and a task ID.
  if(operation==='chat_music') p.stream=false
  return p
}
const escape = v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))
function codeLine(line) {
  return line.split(/("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|(?:^|\s)#.*$)/g).map((part,index)=>
    index%2?`<span class="${part.trimStart().startsWith('#')?'code-comment':'code-string'}">${escape(part)}</span>`:escape(part)
  ).join('')
}

export function renderDocumentationBlock(block, headingID='', rich=false) {
  if(block.kind==='heading') {
    const tag=block.level===2?'h2':'h3'
    return `<${tag} id="${escape(headingID)}" class="hash-anchor" tabindex="-1">${block.content?inlineMarkup(block.content):escape(block.text)}</${tag}>`
  }
  if(block.kind==='text')return `<p>${escape(block.text)}</p>`
  if(block.kind==='paragraph')return `<p>${inlineMarkup(block.content)}</p>`
  if(block.kind==='list')return listMarkup(block.content)
  if(block.kind==='callout')return `<div class="guide-callout"><div>${inlineMarkup(block.content)}</div></div>`
  if(block.kind==='endpoint')return `<div class="endpoint"><b>${escape(block.method)}</b><code>${escape(API_ORIGIN+block.path)}</code></div>`
  if(block.kind==='code') {
    const content=rich?block.text.split('\n').map(line=>`<span class="code-line">${codeLine(line)}</span>`).join(''):escape(block.text)
    return `<div class="code"><div class="code-title"><span>${escape(block.label)}</span><button type="button" data-copy>复制</button></div><pre><code>${content}</code></pre></div>`
  }
  if(block.kind==='table')return `<div class="table" tabindex="0" role="region" aria-label="文档表格"><table><thead><tr>${block.headers.map(cell=>`<th scope="col">${cellMarkup(cell)}</th>`).join('')}</tr></thead><tbody>${block.rows.map(row=>`<tr>${row.map(cell=>`<td>${cellMarkup(cell)}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`
  return ''
}
const pretty=v=>JSON.stringify(v,(key,value)=>typeof value==='string'&&(key==='b64_json'||(key==='data'&&/^[A-Za-z0-9+/\r\n]+={0,2}$/.test(value))||value.startsWith('data:')||value.length>4000)?`[编码内容已省略，${value.length} 字符]`:value,2)

const normalizeSearch = text => text.normalize('NFKC').toLowerCase().trim()

export function filterDocumentation(index, query) {
  const terms = normalizeSearch(query).split(/\s+/).filter(Boolean)
  if (!terms.length) return { ids: [], filtering: false }
  const ids = index.filter(({ text }) => terms.every(term => text.includes(term))).map(({ id }) => id)
  return { ids, filtering: ids.length > 0 }
}

async function start() {
  const [sections,generalSections,knowledgeSections]=await Promise.all([fetch(new URL('./content.json',import.meta.url)).then(response=>response.json()),fetch(new URL('./general-content.json',import.meta.url)).then(response=>response.json()),fetch(new URL('./knowledge-content.json',import.meta.url)).then(response=>response.json())])
  let category=categories[0],activeSectionID=category.sections[0],tab='文档',profiles=[],generalModels=[],key='',pending=parsePendingRecovery(localStorage.getItem('media-docs.pending-task.v1')||''),running=false,epoch=0,activeController=null,wakePoll=null,loadingModels=false,loadTimer=null,loadEpoch=0,docSearch=''
  const objectURLs=new Set()
  if(new URLSearchParams(location.search).get('embedded')==='1')document.body.classList.add('embedded')
  const root=document.querySelector('#app')
  root.innerHTML=`<button class="mobile-menu" id="menu" type="button" aria-controls="document-sidebar" aria-expanded="false">文档目录</button>
    <div class="portal-grid">
    <aside id="document-sidebar">
    <a class="brand" href="/" target="_top">词元互联</a>
    <form class="nav-search" role="search" aria-label="文档搜索" autocomplete="off">
      <div class="nav-search-field">
        <input id="doc-search" name="documentation-query" type="search" autocomplete="off" autocapitalize="none" spellcheck="false" data-1p-ignore data-lpignore="true" data-bwignore placeholder="搜索文档" aria-label="搜索文档" aria-controls="document-navigation" aria-describedby="doc-search-status">
        <button id="doc-search-clear" type="button" aria-label="清除搜索" hidden>清除</button>
      </div>
      <p id="doc-search-status" role="status" hidden></p>
    </form>
    <nav id="document-navigation" aria-label="文档目录" tabindex="0"></nav>
    <div class="aside-foot"><span id="api-version">接口文档</span><button id="theme" type="button">☾ 深色</button></div>
    </aside>
    <main>
      <header>
        <div><span id="page-group" class="eyebrow"></span><h1 tabindex="-1"></h1></div>
        <div id="header-controls" hidden><div class="tools">
          <input id="key" type="hidden" autocomplete="off" spellcheck="false" placeholder="调试时输入 API Key" aria-label="API Key">
          <button id="show-key" type="button">显示</button><button id="load" class="primary" type="button">加载可用模型</button>
        </div><div class="notice" role="status" id="notice">在线调试时填写 API Key。</div>
        <p class="key-note">API Key 仅保存在本页内存，关闭页面后清除。</p></div>
      </header>
      <div class="tabs" role="tablist"></div>
      <nav id="page-toc" aria-label="本页目录" hidden>
        <strong class="toc-label">本页目录</strong>
        <button id="page-toc-toggle" type="button" class="toc-toggle" aria-expanded="false" aria-controls="page-toc-links">本页目录</button>
        <div id="page-toc-links" role="group" aria-label="章节" tabindex="0"></div>
      </nav>
      <article></article>
      <footer>词元互联 · API 根地址 <a href="${API_ORIGIN}">${API_ORIGIN}</a> · <a href="./examples/imagevideo.py" download>Python 示例</a> · <a href="./examples/imagevideo.mjs" download>JavaScript 示例</a></footer>
    </main></div>`
  const $=s=>root.querySelector(s)
  const search = $('#doc-search')
  const clearSearchButton = $('#doc-search-clear')
  const searchStatus = $('#doc-search-status')
  const navigation = $('#document-navigation')
  const menu = $('#menu')
  const sidebar = $('#document-sidebar')
  const main = $('main')
  const pageToc = $('#page-toc')
  const tocLinks = $('#page-toc-links')
  const tocToggle = $('#page-toc-toggle')
  const closedGroups = new Set()
  let tocEntries = [], currentChapter = null, chapterFrame = 0
  const isAutofilled = input => input.matches(':autofill')
  let parentThemeObserver=null
  if(new URLSearchParams(location.search).get('embedded')==='1'){
    const syncTheme=()=>{
      let dark=document.documentElement.classList.contains('dark')
      try{dark=parent.document.documentElement.classList.contains('dark')}catch{}
      document.body.classList.toggle('dark',dark)
      const button=$('#theme');if(button)button.textContent=dark?'☀ 浅色':'☾ 深色'
    }
    syncTheme()
    try{parentThemeObserver=new parent.MutationObserver(syncTheme);parentThemeObserver.observe(parent.document.documentElement,{attributes:true,attributeFilter:['class']})}catch{}
  }
  loadDocsVersion().then(label=>{const badge=$('#api-version');if(badge)badge.textContent=label})
  const notice=(message,error=false)=>{$('#notice').textContent=message;$('#notice').classList.toggle('error',error)}
  const copy=async text=>{try{await navigator.clipboard.writeText(text);notice('已复制')}catch{notice('复制失败，请手动选择内容',true)}}
  const copyArticle=async (button,text)=>{
    if(button.getAttribute('aria-disabled')==='true')return
    const label=button.textContent
    button.setAttribute('aria-disabled','true')
    try{await navigator.clipboard.writeText(text);button.textContent='已复制'}
    catch{button.textContent='复制失败'}
    setTimeout(()=>{button.textContent=label;button.removeAttribute('aria-disabled')},1600)
  }
  const allSections=()=>[...knowledgeSections,...generalSections,...sections]
  const sectionsByID = new Map(allSections().map(section => [section.id, section]))
  const aliases = new Map(knowledgeSections.filter(section=>section.slug).map(section=>[section.slug,section]))
  const directory = navigationGroups.map(group => ({
    title: group.title,
    items: group.sections.map(id => sectionsByID.get(id)).filter(Boolean).map(section => {
      const [provider] = section.title.split(' · ')
      const label = section.navTitle || section.title
      return {section,label:provider === section.title || label.startsWith(provider) ? label : `${provider} · ${label}`}
    }),
  }))
  const groupBySection = new Map(directory.flatMap(group => group.items.map(({section}) => [section.id,group.title])))
  const searchIndex = directory.flatMap(group => group.items.map(({section,label}) => ({
    id: section.id,
    text: normalizeSearch([group.title, section.id, section.slug, section.title, label, section.lead?plainText(section.lead):section.subtitle, JSON.stringify(section.blocks)].join(' ')),
  })))
  const categoryForSection=id=>categories.find(item=>item.sections.includes(id))||categories[0]
  const code=b=>renderDocumentationBlock({...b,kind:'code'})
  function revealInDirectory(scroller,item) {
    if (!item?.getClientRects().length) return
    const bounds = scroller.getBoundingClientRect()
    const row = item.getBoundingClientRect()
    if(row.top < bounds.top + 6) scroller.scrollTop += row.top - bounds.top - 6
    else if(row.bottom > bounds.bottom - 6) scroller.scrollTop += row.bottom - bounds.bottom + 6
  }
  function renderNavigation() {
    const query = normalizeSearch(docSearch)
    const { ids, filtering } = filterDocumentation(searchIndex, query)
    const matches = new Set(ids)
    searchStatus.hidden = !query
    searchStatus.textContent = query
      ? filtering ? `找到 ${ids.length} 篇文档` : '没有匹配结果，已显示完整目录。'
      : ''
    clearSearchButton.hidden = !search.value
    navigation.innerHTML = directory.map(group => {
      const items = filtering ? group.items.filter(({section}) => matches.has(section.id)) : group.items
      if (!items.length) return ''
      const open = filtering || !closedGroups.has(group.title)
      return `<details class="nav-group"${open?' open':''}><summary>${escape(group.title)}</summary><div>${items.map(({section,label}) => {
        const selected = section.id === activeSectionID
        return `<a href="#${escape(section.id)}"${selected ? ' class="active" aria-current="page"' : ''}>${escape(label)}</a>`
      }).join('')}</div></details>`
    }).join('')
  }

  function selectDocumentInNavigation() {
    const selected = navigation.querySelector('.active')
    selected?.classList.remove('active')
    selected?.removeAttribute('aria-current')
    const current = navigation.querySelector(`a[href="#${CSS.escape(activeSectionID)}"]`)
    if(!current) return
    const group = current.closest('details')
    group.open = true
    closedGroups.delete(groupBySection.get(activeSectionID))
    current.classList.add('active')
    current.setAttribute('aria-current','page')
    revealInDirectory(navigation,current)
  }

  function syncCurrentChapter() {
    const offset = parseFloat(getComputedStyle(tocEntries[0].heading).scrollMarginTop) + 8
    let current = tocEntries[0].button
    for (const {heading,button} of tocEntries) {
      if (heading.getBoundingClientRect().top > offset) break
      current = button
    }
    if(window.scrollY > 0 && window.scrollY + window.innerHeight >= document.documentElement.scrollHeight - 2) {
      current = tocEntries.at(-1).button
    }
    if(current === currentChapter) return
    currentChapter?.removeAttribute('aria-current')
    currentChapter = current
    current.setAttribute('aria-current','location')
    revealInDirectory(tocLinks,current)
  }

  function scheduleChapterSync() {
    if (chapterFrame || !tocEntries.length) return
    chapterFrame = requestAnimationFrame(() => {
      chapterFrame = 0
      if (tocEntries.length) syncCurrentChapter()
    })
  }

  function renderPageToc(headings) {
    tocToggle.setAttribute('aria-expanded','false')
    currentChapter = null
    pageToc.hidden = !headings.length
    main.classList.toggle('has-page-toc',!pageToc.hidden)
    tocLinks.innerHTML = headings.map(({id,text,level}) =>
      `<button type="button" data-toc-target="${id}"${level===3?' class="toc-subheading"':''}>${escape(text)}</button>`
    ).join('')
    tocLinks.scrollTop = 0
    tocEntries = [...tocLinks.children].map(button => ({heading:document.getElementById(button.dataset.tocTarget),button}))
    if (tocEntries.length) syncCurrentChapter()
  }

  tocToggle.onclick = () => {
    const open = tocToggle.getAttribute('aria-expanded') !== 'true'
    tocToggle.setAttribute('aria-expanded',String(open))
    if (open) revealInDirectory(tocLinks,currentChapter)
  }

  tocLinks.onclick = event => {
    const button = event.target.closest('[data-toc-target]')
    if (!button) return
    const heading = document.getElementById(button.dataset.tocTarget)
    const inline = tocToggle.getClientRects().length > 0
    if (inline) tocToggle.setAttribute('aria-expanded','false')
    if (inline || event.detail === 0) heading.focus({preventScroll:true})
    heading.scrollIntoView({behavior:matchMedia('(prefers-reduced-motion: reduce)').matches?'instant':'smooth',block:'start'})
  }

  window.addEventListener('scroll',scheduleChapterSync,{passive:true})
  window.addEventListener('resize',scheduleChapterSync)

  function view() {
    const page=sectionsByID.get(activeSectionID)
    const guide=activeSectionID==='guide'||Boolean(page?.lead)
    main.classList.toggle('reading-guide',guide)
    $('#page-group').textContent=groupBySection.get(activeSectionID)||''
    $('h1').textContent=page?.title||category.title; document.title=(page?.title||category.title)+' · 词元互联'
    $('#header-controls').hidden=tab!=='在线调试'
    if (tab === '在线调试' && $('#key').type === 'hidden') $('#key').type = 'password'
    const tabs=guide?['文档']:category.ops.length?['接口文档','输入示例','输出示例','在线调试']:['文档','代码示例']
    if(!tabs.includes(tab))tab=tabs[0]
    $('.tabs').hidden=guide
    $('.tabs').innerHTML=tabs.map(t=>`<button role="tab" aria-selected="${t===tab}" class="${t===tab?'active':''}">${t}</button>`).join('')
    if(tab==='在线调试') {renderPageToc([]);debug();return}
    const blocks=(page?.blocks||[]).map((block,index)=>({block,id:`page-heading-${index}`})).filter(({block})=>{
      if(tab==='文档'||tab==='接口文档')return guide||block.kind!=='code'||block.language==='http'
      if(tab==='代码示例')return block.kind==='code'
      return block.kind==='code'&&(tab==='输出示例'?block.language==='json':block.language!=='json'&&block.language!=='http')
    })
    const metadata=guide?`<div class="guide-meta"><button id="share-guide" type="button">复制本页链接</button>${page.updated?`<time datetime="${escape(page.updated)}">更新于 ${escape(page.updated)}</time>`:''}</div>`:''
    const intro=page?.lead?inlineMarkup(page.lead):escape(page?.subtitle)
    $('article').innerHTML=page?`<section id="page-top" class="hash-anchor">${metadata}<p class="page-subtitle">${intro}</p>${blocks.map(({block,id})=>renderDocumentationBlock(block,id,guide)).join('')}<div class="buttons"><button id="copy-group" type="button">复制本页 Markdown</button></div></section>`:''
    const headings=blocks.flatMap(({block,id})=>block.kind==='heading'?[{id,text:headingText(block),level:block.level}]:[])
    renderPageToc(headings)
    if(!page)return
    if(guide)$('#share-guide').onclick=event=>copyArticle(event.currentTarget,`${API_ORIGIN}/media-docs/#${page.id}`)
    $('#copy-group').onclick=event=>copyArticle(event.currentTarget,sectionMarkdown(page))
    $('article').querySelectorAll('[data-copy]').forEach(button=>button.onclick=()=>{
      const code=button.closest('.code').querySelector('code')
      const lines=guide?[...code.querySelectorAll('.code-line')]:[]
      copyArticle(button,guide?lines.map(line=>line.textContent).join('\n'):code.textContent)
    })
  }
  const filtered=()=>category.id==='text'?generalModels:profilesForCategory(profiles,category)
  function debugText() {
    const protocols=[{id:'chat_completions',label:'OpenAI Chat Completions'},{id:'responses',label:'OpenAI Responses'},{id:'messages',label:'Anthropic Messages'},{id:'gemini_content',label:'Gemini generateContent'},{id:'systemone',label:'TypeSafe System One'}]
    const available=generalModels.flatMap(model=>model.operations.map(operation=>({model:model.model,operation})))
    if(!available.length){$('article').innerHTML='<p>当前 API Key 未发现通用文本模型。请检查 Key 的分组和模型权限，或选择图片、视频、音乐与语音目录查看多媒体模型。</p>';return}
    $('article').innerHTML=`<p>模型列表按当前 API Key 的分组和白名单加载。同步接口可返回 JSON，支持流式的接口可切换 SSE。发送可能产生费用，请先核对请求体和所选模型。</p><div class="fields"><label>接口协议<select id="text-operation" aria-label="选择接口协议">${protocols.filter(protocol=>available.some(item=>item.operation===protocol.id)).map(protocol=>`<option value="${protocol.id}">${protocol.label}</option>`).join('')}</select></label><label>模型<select id="text-model" aria-label="选择模型"></select></label><label id="text-stream-field">响应方式<select id="text-stream"><option value="false">同步 JSON</option><option value="true">流式响应</option></select></label><label id="text-token-field">最大输出 Token<input id="text-max-tokens" type="number" value="1024" min="1"></label></div><label class="wide">提示词<textarea id="text-prompt" placeholder="输入要发送给模型的问题"></textarea></label><label class="request-editor">可编辑请求 JSON<textarea id="text-request" spellcheck="false" aria-label="可编辑请求 JSON"></textarea></label><div class="buttons"><button id="send-text" class="primary" disabled>发送请求</button><button id="stop-text" disabled>停止等待</button></div><label class="confirm-paid"><input id="confirm-text-paid" type="checkbox"> 我确认请求可能产生费用</label><p class="status" id="text-status" role="status" aria-live="polite">输入 API Key 并加载模型后即可调试。</p><div id="text-error" class="error" role="alert"></div><section class="debug-panels"><div class="code"><div class="code-title"><span>请求路径</span></div><pre id="text-endpoint">尚未选择接口</pre></div><div class="code"><div class="code-title"><span>响应内容</span><button id="copy-text-response">复制</button></div><pre id="text-response">尚无响应</pre></div></section>`
    const operation=$('#text-operation'),model=$('#text-model'),request=$('#text-request'),response=$('#text-response'),send=$('#send-text'),stop=$('#stop-text'),status=$('#text-status'),error=$('#text-error'),confirmation=$('#confirm-text-paid')
    let controller=null,sequence=0
    const renderRequest=()=>{
      const op=operation.value,selected=model.value,profile=generalModels.find(item=>item.model===selected),cap={model:selected}
      if(!profile||!profile.operations.includes(op)){request.value='';$('#text-endpoint').textContent='请选择当前 Key 可用的模型和接口';return}
      const payload=buildRequest(cap,op,{prompt:$('#text-prompt').value,stream:$('#text-stream').value,max_tokens:$('#text-max-tokens').value})
      request.value=JSON.stringify(payload,null,2)
      const endpoint=endpointForStreaming(op,selected,$('#text-stream').value==='true')
      $('#text-endpoint').textContent=`POST ${API_ORIGIN}${endpoint}`
      $('#text-token-field').hidden=op!=='messages'
      $('#text-stream-field').hidden=op==='systemone'
      $('#text-stream').disabled=op==='systemone'
      send.disabled=!key||!confirmation.checked
    }
    const populateModels=()=>{
      const op=operation.value
      const rows=generalModels.filter(item=>item.operations.includes(op))
      model.innerHTML=rows.map(item=>`<option value="${escape(item.model)}">${escape(item.model)}</option>`).join('')
      model.disabled=rows.length===0
      if(!rows.length){status.textContent='当前 Key 未发现该接口可用的模型。请刷新模型目录或切换接口协议。';request.value='';$('#text-endpoint').textContent='没有匹配的模型';send.disabled=true;return}
      renderRequest()
    }
    operation.onchange=populateModels
    model.onchange=renderRequest
    $('#text-stream').onchange=renderRequest
    $('#text-max-tokens').oninput=renderRequest
    $('#text-prompt').oninput=renderRequest
    confirmation.onchange=()=>{send.disabled=!key||!model.value||!confirmation.checked}
    $('#copy-text-response').onclick=()=>copy(response.textContent||'')
    $('#text-request').oninput=()=>{try{JSON.parse(request.value);const op=operation.value,path=endpointForStreaming(op,model.value,op==='gemini_content'&&$('#text-stream').value==='true');$('#text-endpoint').textContent=`POST ${API_ORIGIN}${path}`}catch(error){$('#text-error').textContent=`JSON 格式错误：${error.message}`}}
    const readEventStream=async stream=>{
      const reader=stream.getReader(),decoder=new TextDecoder();let text=''
      while(true){const {done,value}=await reader.read();if(done)break;text+=decoder.decode(value,{stream:true});response.textContent=text}
      return text
    }
    send.onclick=async()=>{
      if(!key||!confirmation.checked||!model.value)return
      let payload
      try{payload=JSON.parse(request.value);if(!payload||Array.isArray(payload)||typeof payload!=='object')throw Error('请求 JSON 必须是对象')}catch(e){error.textContent=`JSON 格式错误：${e.message}`;return}
      const current=++sequence;controller=new AbortController();send.disabled=true;stop.disabled=false;error.textContent='';status.textContent='请求已发送，等待响应。';response.textContent='等待响应…'
      const op=operation.value,streaming=op==='gemini_content'?$('#text-stream').value==='true':payload.stream===true,path=endpointForStreaming(op,model.value,streaming)
      const headers={Authorization:`Bearer ${key}`,'Content-Type':'application/json',...(op==='messages'?{'anthropic-version':'2023-06-01'}:{})}
      $('#text-endpoint').textContent=`POST ${API_ORIGIN}${path}`
      const started=performance.now()
      try{
        const result=await fetch(path,{method:'POST',headers,body:JSON.stringify(payload),credentials:'omit',cache:'no-store',redirect:'error',signal:AbortSignal.any([AbortSignal.timeout(streaming?185000:120000),controller.signal])})
        const contentType=result.headers.get('content-type')||''
        const body=contentType.includes('text/event-stream')?await readEventStream(result.body):await result.text()
        response.textContent=body
        if(!result.ok)throw Error(`HTTP ${result.status}：${parseErrorMessage(body)}`)
        if(!contentType.includes('text/event-stream')){try{response.textContent=JSON.stringify(JSON.parse(body),null,2)}catch{}}
        status.textContent=`请求完成 · HTTP ${result.status} · ${Math.round(performance.now()-started)} ms`
      }catch(e){if(current===sequence){error.textContent=e.message||String(e);status.textContent=controller?.signal.aborted?'已停止等待':'请求未完成。请先确认服务端是否已处理，再决定是否重新发送。'}}
      finally{if(current===sequence){controller=null;stop.disabled=true;send.disabled=!confirmation.checked}}
    }
    stop.onclick=()=>{sequence++;controller?.abort();controller=null;stop.disabled=true;send.disabled=!confirmation.checked;status.textContent='已停止等待响应。若请求已送达服务端，停止等待不会撤回请求。'}
    if(available.length){operation.value=protocols.find(protocol=>available.some(item=>item.operation===protocol.id))?.id||'';populateModels()}
  }
  function debug() {
    if(category.id==='text'){debugText();return}
    const options=filtered(), uploads={}, uploadJobs=new Set()
    let requestDirty=false
    $('article').innerHTML=`<p>流程：标准操作请求 → 免费预览与估价 → 本站返回任务 ID → 查询提交阶段 → 查看成品与账单。免费预览不会创建生成任务或扣费；真实提交只有收到 HTTP 202 并保存任务 ID 后才表示本站已受理。连接中断时保留原幂等 ID，按同一操作的恢复端点查询，不自动再次 POST。</p><div class="fields"><label class="wide">模型 / 能力配置<select id="model" aria-label="选择调试模型"><option value="">${loadingModels?'正在加载模型…':options.length?'请选择模型':key?'当前分类暂无可用模型':'粘贴 API Key 后自动加载可用模型'}</option>${options.map((p,i)=>`<option value="${i}">${escape(p.model)} · ${escape(p.media_type)} · ${escape(p.group_name||p.profile_id?.slice(-8)||'')}</option>`).join('')}</select></label></div><p id="model-hint">${key ? (options.length ? `当前分类有 ${options.length} 个模型能力` : '请检查 Key 的分组、模型启用状态和渠道定价，或切换分类。') : '正在展示公开模型目录。输入中转 Key 后自动加载可调用能力。'}</p><div id="parameters"></div><label class="request-editor">可编辑请求 JSON<textarea id="media-request" spellcheck="false" aria-label="可编辑请求 JSON" placeholder="选择模型并填写参数后，请求 JSON 会显示在这里。"></textarea></label><div class="buttons"><button id="preview" disabled>免费预览映射与估价</button><button id="submit" class="primary" disabled>提交真实生成（可能收费）</button><button id="recover" ${pending?'':'disabled'}>恢复 / 查询原任务</button><button id="stop">停止轮询</button><button id="new">开始新任务</button></div><p class="status task-summary" id="task-status" role="status" aria-live="polite">${pending?.id?`已有任务 ${escape(pending.id)}，点击“恢复 / 查询原任务”。`:'尚未提交真实生成请求。'}</p><details class="debug-evidence"><summary>原始请求、响应与联调日志（默认折叠，Base64 已脱敏）</summary><div class="debug-panels">${code({label:'标准提交请求 JSON（Base64 已脱敏）',text:'尚未提交'})}${code({label:'任务查询响应',text:'尚无响应'})}</div><section class="debug-log"><h3>联调日志</h3><ol id="debug-log-list"></ol></section></details><div id="debug-error" class="error" role="alert"></div><div class="artifacts"></div>`
    const codes=$('article').querySelectorAll('.code code')
    const confirmation=document.createElement('label'), confirmPaid=document.createElement('input')
    confirmation.className='confirm-paid';confirmPaid.id='confirm-paid';confirmPaid.type='checkbox'
    confirmation.append(confirmPaid,document.createTextNode('我确认本次真实生成可能收费'))
    $('#submit').before(confirmation)
    const response=v=>codes[1].textContent=typeof v==='string'?v:pretty(v)
    const appendLog=(stage,detail)=>{
      const row=document.createElement('li'),meta=document.createElement('div'),content=document.createElement('pre')
      row.className='log-entry';meta.className='log-meta';meta.textContent=`${new Date().toLocaleTimeString()} · ${stage}`
      content.textContent=typeof detail==='string'?detail:pretty(detail);row.append(meta,content);$('#debug-log-list').prepend(row)
      while($('#debug-log-list').children.length>40)$('#debug-log-list').lastElementChild.remove()
    }
    const failure=e=>{$('#debug-error').textContent=e.message||String(e);appendLog('错误',e.message||String(e))}
    const model=$('#model')
    model.onchange=()=>{
      const cap=options[model.value]; if(!cap){$('#parameters').innerHTML='';$('#submit').disabled=true;$('#preview').disabled=true;return}
      for(const name of Object.keys(uploads))delete uploads[name]
      const pick=(name,label,values=[])=>{const choices=optionValues(values);return choices.length?`<label>${label}<select name="${name}"><option value="">自动 / 省略</option>${choices.map(v=>`<option>${escape(v)}</option>`).join('')}</select></label>`:''}
      const duration=cap.duration||{}
      const seconds=duration.default??duration.values?.[0]??duration.min??''
      const operations=optionValues(cap.operations)
      let html=`<label>调用场景<select name="operation">${operations.filter(op=>category.ops.includes(op)).map(op=>`<option value="${op}">${escape({image:'文字生图',image_edit:'参考图编辑',gemini_image:'Gemini 生图 / 编辑',video:'视频生成',audio:'专用音乐接口',chat_music:'Chat 音乐接口',speech:'语音合成'}[op])}</option>`).join('')}</select></label><label class="wide">提示词<textarea name="prompt" placeholder="输入生成描述或编辑要求" required></textarea></label>`
      html+=pick('size','尺寸 / 分辨率档位',cap.sizes)+pick('quality','画质',cap.qualities)+pick('background','背景',cap.backgrounds)+pick('output_format','图片输出格式',cap.output_formats)+pick('resolution','视频清晰度',cap.resolutions)+pick('ratio','画面比例',cap.aspect_ratios)
      if((cap.media_type==='video'||cap.media_type==='audio')&&!operations.every(op=>op==='speech')) html+=`<label>时长（秒）<input name="seconds" type="number" value="${escape(seconds)}" min="1"><small>目录建议：${escape(duration.values?.join(' / ')||'由所选配置决定')}；由上游确认支持范围</small></label>`
      if(cap.media_type==='video')html+=pick('generate_audio','生成声音',['true','false'])
      if(cap.media_type==='audio')html+=`<label class="wide">歌词<textarea name="lyrics"></textarea></label><label>BPM<input name="bpm" type="number" min="1"></label>`+pick('instrumental','纯音乐',['true','false'])+pick('audio_format','输出格式',['mp3','wav','m4a'])
      if(operations.includes('speech'))html+=`<label>声音<input name="voice" placeholder="使用管理员配置支持的声音 ID"></label><label>输出格式<input name="response_format" placeholder="如 mp3 / wav"></label><label>语速<input name="speed" type="number" min="0.01" step="0.01"></label>`
      if(operations.includes('chat_music'))html+='<p class="wide"><small>音乐按本站异步任务流程返回 HTTP 202；保存任务 ID 后查询状态与音频制品。此入口不提供 SSE 直返。</small></p>'
      if(cap.media_type==='video')for(const role of ['first_frame','last_frame'])html+=`<label class="wide">${role==='first_frame'?'首帧':'尾帧'}（计入参考图总数）<textarea data-material="${role}" placeholder="URL、Data URL 或素材 JSON 对象；字段按输入原样保留"></textarea></label>`
      if(operations.includes('gemini_image'))html+=pick('modalities','返回模态',['TEXT','IMAGE','both'])
      const inputMaterials=inputMaterialsFor(cap)
      for(const [kind,material] of Object.entries(inputMaterials)) {
        // Missing catalogue counts do not disable unified reference inputs.
        html+=`<label class="wide material-drop-zone" data-drop-zone="${kind}">${escape({image:'参考图片',video:'参考视频',audio:'参考音频',mask:'编辑遮罩（透明 PNG，与第一张原图同尺寸）'}[kind]||kind)} · URL / Data URL / b64_json${material.max_file_bytes?` · 每个最多 ${Math.floor(material.max_file_bytes/1048576)} MiB`:''}<textarea data-material="${kind}" placeholder="每行一个 URL，或粘贴 JSON 对象，如 {&quot;b64_json&quot;:&quot;...&quot;,&quot;mime_type&quot;:&quot;${kind==='image'?'image/png':kind==='video'?'video/mp4':'audio/mpeg'}&quot;}"></textarea><input data-files="${kind}" type="file" accept="${kind==='mask'?'image/png':kind+'/*'}" multiple><small>支持多选、分批添加或拖入文件；提交参数和日志会脱敏 Base64。</small><ul data-upload-list="${kind}" class="upload-list"></ul></label>`
      }
      $('#parameters').innerHTML='<div class="fields">'+html+'</div>'
      $('#submit').disabled=running||!!pending||!key||!$('#confirm-paid').checked||!!cap.catalog_only
      $('#preview').disabled=running||!key||!!cap.catalog_only
      if(cap.catalog_only)$('#model-hint').textContent='已选择公开目录模型。粘贴中转 Key 后自动匹配可调用配置。'
      installUploads()
      requestDirty=false
      $('#parameters').addEventListener('input',()=>{requestDirty=false;syncRequestDraft()})
      $('#parameters').addEventListener('change',()=>{requestDirty=false;syncRequestDraft()})
      syncRequestDraft()
    }
    function syncRequestDraft(){
      const cap=options[model.value];if(!cap)return
      try{
        const fields={};$('#parameters').querySelectorAll('[name]').forEach(el=>fields[el.name]=el.value)
        const materials={};$('#parameters').querySelectorAll('[data-material]').forEach(el=>materials[el.dataset.material]=parseMaterialText(el.value))
        for(const [kind,files] of Object.entries(uploads))materials[kind]=[...(materials[kind]||[]),...files.map(file=>file.material)]
        const payload=buildRequest(cap,fields.operation,fields,materials),text=JSON.stringify(payload,null,2)
        if(!requestDirty)$('#media-request').value=text
        codes[0].textContent=pretty(payload)
      }catch{}
    }
    $('#media-request').addEventListener('input',()=>{requestDirty=true;$('#debug-error').textContent=''})
    const api=async(path,method='GET',payload,headers={})=>{
      const url=new URL(path,location.origin);if(url.origin!==location.origin)throw Error('只允许请求本站 API')
      const started=performance.now();appendLog('请求',{method,path,request:payload})
      const timeoutMs=requestTimeoutMs(method,path,payload)
      const r=await fetch(url,{method,headers:{Authorization:'Bearer '+key,...(payload instanceof FormData?{}:{'Content-Type':'application/json'}),...headers},body:payload?(payload instanceof FormData?payload:JSON.stringify(payload)):undefined,signal:AbortSignal.any([AbortSignal.timeout(timeoutMs),activeController?.signal||new AbortController().signal]),credentials:'omit',cache:'no-store',redirect:'error'})
      let data
      if(r.headers.get('content-type')?.includes('text/event-stream')) {
        const reader=r.body.getReader(),decoder=new TextDecoder();let text=''
        while(true){const {done,value}=await reader.read();if(done)break;text+=decoder.decode(value,{stream:true});response(text)}
        const events=text.split(/\r?\n/).filter(line=>line.startsWith('data:')).map(line=>line.slice(5).trim()).filter(line=>line&&line!=='[DONE]');data=events.length?JSON.parse(events.at(-1)):{};data.sse=text
      }else{const text=await r.text();try{data=JSON.parse(text)}catch{throw Error(`HTTP ${r.status}：响应不是 JSON`)}}
      appendLog('响应',{method,path,status:r.status,elapsed_ms:Math.round(performance.now()-started),response:data})
      if(!r.ok){const e=Error(`HTTP ${r.status}：${data.error?.message||data.message||'请求失败'}`);e.status=r.status;throw e}
      return data
    }
    async function artifacts(task) {
      const result=await api(`/v1/tasks/${encodeURIComponent(task)}/artifacts`)
      $('.artifacts').innerHTML=''
      for(const artifact of result.artifacts||[]) {
        const wrap=document.createElement('div');wrap.className='artifact';$('.artifacts').append(wrap)
        const info=document.createElement('p');info.textContent=`${artifact.key} · ${artifact.mime_type||'类型待返回'} · ${artifact.size_bytes??'未知'} B · ${artifact.available?'可用':'处理中 / 已失效'}`;wrap.append(info)
        if(!artifact.available)continue
        const button=document.createElement('button');button.textContent='鉴权下载 / 预览原文件';wrap.append(button)
        button.onclick=async()=>{
          button.disabled=true
          try {
            const url=new URL(artifact.content_url,location.origin);if(url.origin!==location.origin)throw Error('文件地址不是本站同源地址')
            const r=await fetch(url,{headers:{Authorization:'Bearer '+key},credentials:'omit',redirect:'error',signal:AbortSignal.timeout(180000)})
            if(!r.ok)throw Error('文件下载失败：HTTP '+r.status)
            const mime=(r.headers.get('content-type')||'').split(';')[0];if(!/^(image|audio|video)\//.test(mime))throw Error('响应不是媒体文件：'+mime)
            const blob=await r.blob(),local=URL.createObjectURL(blob);objectURLs.add(local)
            const extension=({'image/png':'png','image/jpeg':'jpg','image/webp':'webp','video/mp4':'mp4','audio/mpeg':'mp3','audio/wav':'wav','audio/mp4':'m4a'})[mime]||'bin'
            const link=document.createElement('a');link.href=local;link.download=`${artifact.key}.${extension}`;link.textContent='保存原文件';wrap.append(link)
            const media=document.createElement(mime.startsWith('image/')?'img':mime.startsWith('video/')?'video':'audio');media.src=local;if(media.tagName==='IMG')media.alt=artifact.key;else media.controls=true;wrap.append(media)
          }catch(e){failure(e)}finally{button.disabled=false}
        }
      }
    }
    async function poll(token) {
      while(token===epoch && pending?.id) {
        const task=await api(`/v1/tasks/${encodeURIComponent(pending.id)}`);if(token!==epoch)return;response(task)
        const progress=task.progress===undefined?'':` · 进度 ${task.progress}%`
        const stage=taskPhaseLabel(task)
        const diagnostic=task.diagnostic?` · 原因：${task.diagnostic}`:''
        const next=task.status==='unknown'||['submitting','reconciling'].includes(task.submission_phase)?'保留任务 ID 并继续查询；不要重新提交。':task.status==='failed'?'查看失败原因与账单。':task.status==='completed'?'成品已就绪，查看下方文件并核对账单。':'后台会继续处理，可安全刷新当前任务。'
        $('#task-status').textContent=`${stage} · 任务 ID ${pending.id} · 状态 ${task.status}${progress} · ${taskBillingSummary(task.billing)}${diagnostic} · ${next}`
        if(['completed','failed'].includes(task.status)) {
          if(task.status==='completed')await artifacts(pending.id)
          if(['settled','released'].includes(task.billing?.state)||task.status==='failed') {pending.terminal=true;localStorage.setItem('media-docs.pending-task.v1',serializePendingRecovery(pending));return}
        }
        localStorage.setItem('media-docs.pending-task.v1',serializePendingRecovery(pending))
        if(task.status==='unknown')throw Error('上游受理结果仍待核对。请保留任务 ID，继续恢复查询或联系管理员；系统没有重新提交。')
        await new Promise(resolve=>{const timer=setTimeout(()=>{wakePoll=null;resolve()},(pending?.pollSeconds||10)*1000);wakePoll=()=>{clearTimeout(timer);wakePoll=null;resolve()}})
      }
    }
    async function resume(token) {
      if(!pending)return
      if(!pending.id){const result=await api(`/v1/media/tasks/recover?operation=${encodeURIComponent(pending.operation)}&idempotency_key=${encodeURIComponent(pending.key)}`);pending.id=result.id;response(result)}
      if(!pending.id)throw Error('尚未找到原任务，请稍后恢复查询；不会重新生成。')
      await poll(token)
    }
    const execute=async fn=>{if(running)return;running=true;model.disabled=true;$('#preview').disabled=true;activeController=new AbortController();const token=++epoch;$('#submit').disabled=true;$('#debug-error').textContent='';try{await fn(token)}catch(e){if(token===epoch)failure(e)}finally{if(token===epoch){running=false;model.disabled=false;activeController=null;$('#recover').disabled=!pending;$('#submit').disabled=!!pending||!model.value||!key||!$('#confirm-paid').checked||!!options[model.value]?.catalog_only;$('#preview').disabled=!model.value||!key||!!options[model.value]?.catalog_only}}}
    function installUploads(){
      const queue=(kind,files,input)=>{
        if(input)input.value=''
        if(!files.length)return
        const job=(async()=>{
          const cap=options[model.value],rule=inputMaterialsFor(cap)[kind],field=$('#parameters').querySelector(`[data-material="${kind}"]`)
          const typedCount=parseMaterialText(field?.value||'').length
          if(rule?.max_count>0&&typedCount+(uploads[kind]?.length||0)+files.length>rule.max_count)throw Error(`${kind} 最多支持 ${rule.max_count} 个素材`)
          for(const file of files){
            if(rule?.max_file_bytes>0&&file.size>rule.max_file_bytes)throw Error(`${file.name} 超过单文件大小限制`)
            if(file.type&&!file.type.startsWith((kind==='mask'?'image':kind)+'/'))throw Error(`${file.name} 文件类型不匹配`)
          }
          const additions=await Promise.all(files.map(async file=>{
            const dataURL=await new Promise((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(reader.result);reader.onerror=()=>reject(Error('文件读取失败'));reader.readAsDataURL(file)})
            return {name:file.name,size:file.size,material:dataURLToMaterial(dataURL)}
          }))
          uploads[kind]=[...(uploads[kind]||[]),...additions]
          for(const file of additions)appendLog('添加参考素材',{kind,file:file.name,bytes:file.size})
          renderUploads(kind)
        })();uploadJobs.add(job);job.catch(failure).finally(()=>uploadJobs.delete(job))
      }
      for(const input of $('#parameters').querySelectorAll('[data-files]'))input.onchange=()=>queue(input.dataset.files,[...input.files],input)
      for(const zone of $('#parameters').querySelectorAll('[data-drop-zone]')){
        zone.ondragenter=zone.ondragover=event=>{event.preventDefault();zone.classList.add('drag-over')}
        zone.ondragleave=event=>{if(!zone.contains(event.relatedTarget))zone.classList.remove('drag-over')}
        zone.ondrop=event=>{event.preventDefault();zone.classList.remove('drag-over');queue(zone.dataset.dropZone,[...(event.dataTransfer?.files||[])])}
      }
    }
    function renderUploads(kind){
      const list=$('#parameters').querySelector(`[data-upload-list="${kind}"]`);if(!list)return
      list.innerHTML=(uploads[kind]||[]).map((file,index)=>`<li><span>${index+1}. ${escape(file.name)} · ${(file.size/1048576).toFixed(2)} MiB</span><button type="button" data-remove="${index}">移除</button></li>`).join('')
      list.querySelectorAll('[data-remove]').forEach(button=>button.onclick=()=>{uploads[kind].splice(Number(button.dataset.remove),1);renderUploads(kind)})
      syncRequestDraft()
    }
    async function collectRequest(){
      const cap=options[model.value],fields={},materials={}
      if(!cap || cap.catalog_only)throw Error('请先填写中转 Key 并加载可调用模型')
      await Promise.all([...uploadJobs])
      $('#parameters').querySelectorAll('[name]').forEach(el=>fields[el.name]=el.value)
      if(!fields.prompt?.trim())throw Error('请填写提示词')
      $('#parameters').querySelectorAll('[data-material]').forEach(el=>materials[el.dataset.material]=parseMaterialText(el.value))
      for(const [kind,files] of Object.entries(uploads))materials[kind]=[...(materials[kind]||[]),...files.map(file=>file.material)]
      for(const [kind,refs] of Object.entries(materials)){const limit=inputMaterialsFor(cap)[kind]?.max_count;if(limit>0&&refs.length>limit)throw Error(`${kind} 最多支持 ${limit} 个素材`)}
      let payload=buildRequest(cap,fields.operation,fields,materials)
      if(fields.operation==='image_edit'&&!materials.image?.length)throw Error('图片编辑必须提供参考图')
      if(requestDirty){try{payload=JSON.parse($('#media-request').value);if(!payload||Array.isArray(payload)||typeof payload!=='object')throw Error('请求 JSON 必须是对象')}catch(error){throw Error(`请求 JSON 无效：${error.message}`)}}
      codes[0].textContent=pretty(payload)
      return {cap,fields,payload}

    }
    $('#preview').onclick=()=>execute(async()=>{
      const {fields,payload}=await collectRequest()
      appendLog('开始联调检查',{checks:['Key 鉴权与分组','模型与操作','素材参数','渠道计费规则'],generation:false})
      let estimate
      try{estimate=await api('/v1/sub2api/estimates','POST',{operation:fields.operation,request:payload})}catch(error){const e=error instanceof Error?error:Error(String(error));e.message=`免费预览失败：${e.message}。本次没有创建生成任务、没有扣费，可修正后安全重试。`;throw e}
      response(estimate);appendLog('参数校验与估价完成',estimate)
    })
    $('#confirm-paid').onchange=()=>{$('#submit').disabled=running||!!pending||!model.value||!key||!$('#confirm-paid').checked||!!options[model.value]?.catalog_only}
    $('#submit').onclick=()=>execute(async token=>{
      if(pending)throw Error('已有任务，请先恢复查询')
      if(!key)throw Error('请先拉取模型')
      if(!$('#confirm-paid').checked)throw Error('请先确认真实生成可能收费')
      const {cap,fields,payload}=await collectRequest()
      appendLog('提交请求（Base64 已脱敏）',{endpoint:endpointFor(fields.operation,cap.model),request:payload})
      pending={key:crypto.randomUUID(),operation:fields.operation,id:null,terminal:false,pollSeconds:cap.poll_seconds||10}
      localStorage.setItem('media-docs.pending-task.v1',serializePendingRecovery(pending))
      $('#task-status').textContent='本站正在持久化请求与计费预留 · 幂等 ID '+pending.key+' · 正在等待 HTTP 202；此时还不能认为上游已受理。'
      try{const result=await api(endpointFor(fields.operation,cap.model),'POST',payload,{'Idempotency-Key':pending.key});pending.id=result.task_id||result.id;localStorage.setItem('media-docs.pending-task.v1',serializePendingRecovery(pending));response(result);appendLog('接口响应',result)}catch(e){if(e.status&&e.status<500&&e.status!==408){pending.terminal=true;localStorage.setItem('media-docs.pending-task.v1',serializePendingRecovery(pending));throw e}const reason=e.name==='TimeoutError'?'请求传输超时':e.message||'网络连接中断';throw Error(`提交结果尚未确认（${reason}）。已保留原幂等 ID；请点“恢复 / 查询原任务”，不要新建请求。`)}
      if(!pending.id)throw Error('HTTP 请求已返回，但响应中没有任务 ID。保留原幂等 ID，点击“恢复 / 查询原任务”；不要再次提交。')
      $('#task-status').textContent=`本站已受理 · 任务 ID ${pending.id} · 正在读取提交阶段。HTTP 202 不代表上游已受理。`
      $('#recover').disabled=false
      await resume(token)
    })
    $('#recover').onclick=()=>execute(resume)
    $('#stop').onclick=()=>{epoch++;activeController?.abort();activeController=null;wakePoll?.();running=false;$('#recover').disabled=!pending;notice('已停止轮询等待；任务仍在服务器运行，可恢复原任务查询。')}
    $('#new').onclick=()=>{if(pending&&!pending.terminal){failure(Error('原任务尚未结束或受理不明确，请恢复查询。'));return}pending=null;localStorage.removeItem('media-docs.pending-task.v1');$('#confirm-paid').checked=false;$('#submit').disabled=true;$('#recover').disabled=true;$('#task-status').textContent='尚未提交真实生成请求。';operationCleanup()}
    function operationCleanup(){objectURLs.forEach(url=>URL.revokeObjectURL(url));objectURLs.clear();$('.artifacts').innerHTML=''}
    if(options.length){model.value='0';model.onchange()}
    $('article').querySelectorAll('[data-copy]').forEach(button=>button.onclick=()=>copy(button.closest('.code').querySelector('code').textContent))
  }
  async function loadModels(){
    const credential=key,version=++loadEpoch
    if(!credential){notice('请填写中转 API Key，即可加载可调用模型。',true);return}
    loadingModels=true;$('#load').disabled=true;notice('正在验证 Key 并加载模型…')
    try{
      const headers={Authorization:'Bearer '+credential,Accept:'application/json'}
      const requestOptions={headers,credentials:'omit',redirect:'error',cache:'no-store',signal:AbortSignal.timeout(30000)}
      const [mediaResult,modelResult,geminiResult]=await Promise.allSettled([fetch('/v1/media/capabilities',requestOptions),fetch('/v1/models',requestOptions),fetch('/v1beta/models',requestOptions)])
      if(version!==loadEpoch||credential!==key)return
      const read=async result=>result.status==='fulfilled'&&result.value.ok?result.value.json():null
      const [mediaCatalog,modelCatalog,geminiCatalog]=await Promise.all([read(mediaResult),read(modelResult),read(geminiResult)])
      profiles=Array.isArray(mediaCatalog?.models)?mediaCatalog.models:[]
      generalModels=parseGeneralModels(modelCatalog,geminiCatalog)
      const count=new Set([...profiles.map(item=>item.model),...generalModels.map(item=>item.model)]).size
      notice(count?`已加载 ${count} 个可用模型及其接口能力。选择模型后，可预览媒体计费或发送同步、流式请求。`:'此 Key 暂无可用模型。请检查 API Key、分组权限和模型开放状态。',!count)
    }catch(e){if(version===loadEpoch){profiles=[];notice(e.message,true)}}
    finally{if(version===loadEpoch){loadingModels=false;$('#load').disabled=false;if(tab==='在线调试'&&!running)debug()}}
  }
  $('#key').oninput=event=>{
    if (!event.isTrusted || document.activeElement !== $('#key') || isAutofilled($('#key'))) {
      $('#key').value = key
      return
    }
    if(pending&&key){$('#key').value=key;notice('已有任务，完成核对前请使用原 Key。',true);return}
    key=$('#key').value.trim().replace(/^Bearer\s+/i,'');profiles=[];generalModels=[];++loadEpoch;clearTimeout(loadTimer)
    if(key)loadTimer=setTimeout(loadModels,450)
    else{loadingModels=false;$('#load').disabled=false;notice('请填写中转 API Key 后加载模型。');if(tab==='在线调试')debug()}
  }
  $('#show-key').onclick=()=>{$('#key').type=$('#key').type==='password'?'text':'password';$('#show-key').textContent=$('#key').type==='password'?'显示':'隐藏'}
  $('#load').onclick=()=>{clearTimeout(loadTimer);loadModels()}
  $('#theme').onclick=()=>{const dark=!document.body.classList.contains('dark');document.body.classList.toggle('dark',dark);document.documentElement.classList.toggle('dark',dark);$('#theme').textContent=dark?'☀ 浅色':'☾ 深色'}
  function setMenu(open) {
    sidebar.classList.toggle('open', open)
    menu.setAttribute('aria-expanded', String(open))
    if(open && !docSearch) revealInDirectory(navigation,navigation.querySelector('[aria-current="page"]'))
  }
  menu.onclick = () => setMenu(!sidebar.classList.contains('open'))
  search.form.onsubmit = event => event.preventDefault()
  function resetSearch(focus = false) {
    docSearch = search.value = ''
    renderNavigation()
    revealInDirectory(navigation,navigation.querySelector('[aria-current="page"]'))
    if (focus) search.focus()
  }
  function restoreSearch() {
    if (search.value !== docSearch) search.value = docSearch
  }
  search.onfocus = restoreSearch
  search.onblur = restoreSearch
  search.oninput = event => {
    if (!event.isTrusted || document.activeElement !== search || isAutofilled(search)) {
      restoreSearch()
      return
    }
    docSearch = search.value
    renderNavigation()
    navigation.scrollTop = 0
  }
  clearSearchButton.onclick = () => resetSearch(true)
  sidebar.addEventListener('keydown', event => {
    if (event.key !== 'Escape') return
    if (search.value) {
      event.preventDefault()
      resetSearch(true)
    } else if (sidebar.classList.contains('open')) {
      setMenu(false)
      menu.focus()
    }
  })
  let focusArticle = false
  navigation.addEventListener('click', event => {
    const summary = event.target.closest('summary')
    if (summary && !docSearch) {
      if (summary.parentElement.open) closedGroups.add(summary.textContent)
      else closedGroups.delete(summary.textContent)
    }
    const link = event.target.closest('a[href]')
    if (!link || event.button || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
    if (running) {
      event.preventDefault()
      notice('任务正在查询，请先停止等待。')
      return
    }
    focusArticle = event.detail === 0 || sidebar.classList.contains('open')
    if (link.hash === location.hash) {
      event.preventDefault()
      navigate()
    }
  })
  $('.tabs').onclick=e=>{if(e.target.matches('button')){if(running){notice('请先停止等待或等待本轮查询完成。');return}tab=e.target.textContent;view()}}
  function navigate() {
    if (running) { notice('任务正在查询，请先停止等待。'); return }
    let hash = location.hash.slice(1)
    try { hash = decodeURIComponent(hash) } catch {}
    const previous = activeSectionID
    const page = sectionsByID.get(hash) || aliases.get(hash)
    category = page ? categoryForSection(page.id) : categories.find(item => item.id === hash) || categories[0]
    activeSectionID = page?.id || category.sections[0]
    tab = category.ops.length ? '接口文档' : '文档'
    const wasSearching = Boolean(docSearch)
    docSearch = search.value = ''
    if (wasSearching || !navigation.firstElementChild) renderNavigation()
    selectDocumentInNavigation()
    if (focusArticle || previous !== activeSectionID) window.scrollTo({top:0,behavior:'instant'})
    view()
    setMenu(false)
    if (focusArticle) $('h1').focus({ preventScroll: true })
    scheduleChapterSync()
    focusArticle = false
  }
  window.addEventListener('hashchange',navigate)
  window.addEventListener('pagehide',()=>{key='';$('#key').value='';epoch++;parentThemeObserver?.disconnect();objectURLs.forEach(url=>URL.revokeObjectURL(url))})
  navigate()
  fetch('/api/v1/model-plaza',{credentials:'omit',signal:AbortSignal.timeout(15000)}).then(r=>r.ok?r.json():null).then(catalog=>{
    if(key||!catalog?.data?.groups)return
    profiles=catalog.data.groups.flatMap(group=>(group.media_models||[]).map(model=>({...model,group_name:group.name,catalog_only:true})))
    if(tab==='在线调试')debug()
  }).catch(()=>{})
}

if(typeof document!=='undefined')start().catch(()=>{document.querySelector('#app').textContent='文档加载失败，请刷新页面。'})
