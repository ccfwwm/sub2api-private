export const API_ORIGIN = 'https://code.aicodeme.xyz'
export const categories = [
  { id:'gpt-image', title:'OpenAI协议生图', sections:['gpt-text','gpt-edit','gpt-result'], ops:['image','image_edit'] },
  { id:'gemini-image', title:'Gemini协议生图', sections:['gemini','gemini-result'], ops:['gemini_image'] },
  { id:'video', title:'视频生成', sections:['omni-video-submit','omni-video-result','veo-text','veo-result','other-video','video-reference'], ops:['video'] },
  { id:'audio', title:'音乐与语音', sections:['audio-chat','audio-submit','audio-compat','audio-stream','audio-speech'], ops:['audio','chat_music','speech'] },
  { id:'grok-video', title:'Grok 视频', sections:['grok-video-submit','grok-video-image','grok-video-result'], ops:['video'] },
  { id:'guide', title:'快速开始与通用任务', sections:['start','models','tasks','errors','clients'], ops:[] },
]
export function sectionMarkdown(section) {
  const row = cells => '| '+cells.map(v=>String(v).replaceAll('|','\\|').replaceAll('\n','<br>')).join(' | ')+' |'
  return `## ${section.title}\n\n${section.subtitle}\n\n`+section.blocks.map(b=>{
    if(b.kind==='heading') return '### '+b.text
    if(b.kind==='endpoint') return `**${b.method}** ${API_ORIGIN}${b.path}`
    if(b.kind==='code') return `### ${b.label}\n\n\`\`\`${b.language}\n${b.text}\n\`\`\``
    if(b.kind==='table') return [row(b.headers),row(b.headers.map(()=>'---')),...b.rows.map(row)].join('\n')
    return b.text
  }).join('\n\n')
}
export const fullMarkdown = sections => '# 词元互联图片、视频与音乐接口文档\n\n根地址：'+API_ORIGIN+'\n\n'+sections.map(sectionMarkdown).join('\n\n')
export function endpointFor(operation, model) {
  return {image:'/v1/images/generations',image_edit:'/v1/images/edits',video:'/v1/videos',audio:'/v1/audio/generations',speech:'/v1/audio/speech',chat_music:'/v1/chat/completions',gemini_image:`/v1beta/models/${encodeURIComponent(model)}:generateContent`}[operation]
}
export function inputMaterialsFor(cap) {
  return { ...cap.materials, ...(cap.operations.includes('image_edit') && !cap.materials?.mask ? { mask: { max_file_bytes: 50 * 1048576 } } : {}) }
}
export function buildRequest(cap, operation, fields, materials={}) {
  const p={model:cap.model,public_profile_id:cap.profile_id,prompt:fields.prompt,async:true,n:1}
  for(const key of ['size','quality','background','output_format','resolution','ratio','lyrics','bpm','audio_format','negative_prompt']) if(fields[key]!=='' && fields[key]!=null) p[key]=fields[key]
  if(fields.seconds!=='' && fields.seconds!=null) p.seconds=Number(fields.seconds)
  if(fields.generate_audio!=='' && fields.generate_audio!=null) p.generate_audio=fields.generate_audio==='true'
  if(fields.instrumental!=='' && fields.instrumental!=null) p.instrumental=fields.instrumental==='true'
  if(p.background==='transparent' && p.output_format==='jpeg') throw Error('透明背景请选择 PNG 或 WebP')
  if(operation==='gemini_image') {
    const parts=[{text:p.prompt},...(materials.image||[]).map(value=>{
      if(value.startsWith('data:')) {const m=value.match(/^data:([^;]+);base64,(.+)$/s);if(!m)throw Error('图片 data URL 格式错误');return {inlineData:{mimeType:m[1],data:m[2]}}}
      return {fileData:{fileUri:value}}
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
  if(operation==='chat_music' && fields.stream==='true') {p.stream=true;p.async=false}
  return p
}
const escape = v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))
const pretty=v=>JSON.stringify(v,(key,value)=>typeof value==='string'&&(value.startsWith('data:')||value.length>4000)?`[编码内容已省略，${value.length} 字符]`:value,2)

async function start() {
  const sections=await (await fetch(new URL('./content.json',import.meta.url))).json()
  let category=categories[0],tab='接口文档',profiles=[],key='',pending=null,running=false,epoch=0,activeController=null,wakePoll=null
  const objectURLs=new Set()
  if(new URLSearchParams(location.search).get('embedded')==='1')document.body.classList.add('embedded')
  const root=document.querySelector('#app')
  root.innerHTML=`<button class="mobile-menu" id="menu">☰ 词元互联 · 接口文档</button><aside><a class="brand" href="/" target="_top">词元互联</a><nav></nav><div class="aside-foot">MEDIA API · v1.3.0<a href="https://code.aicodeme.cn" target="_top">国内入口 ↗</a><a href="https://code.aicodeme.xyz" target="_top">直连入口 ↗</a><a href="https://hkcode.aicodeme.xyz" target="_top">香港入口 ↗</a><button id="theme">☾ 深色</button><p>Key 仅保存在本页内存。关闭或刷新后清除。</p></div></aside><main><header><h1></h1><div><div class="tools"><a id="create-key" class="primary" href="/keys?create=1&amp;platform=imagevideo" target="_top">创建图片音频视频 Key</a><input id="key" type="password" autocomplete="off" spellcheck="false" placeholder="输入密钥（API Key）" aria-label="API Key"><button id="show-key">显示</button><button id="load" class="primary">拉取模型</button><button id="copy-all">复制全部 Markdown</button><button id="export">导出文档</button></div><div class="notice" role="status" id="notice">请先填写密钥，加载可用模型后即可在线调试。</div></div></header><div class="tabs" role="tablist"></div><article></article><footer>词元互联 · 所有生成请求均经过本站 API。<br>接口地址 ${API_ORIGIN} · <a href="./examples/imagevideo.py" download>Python 客户端</a> · <a href="./examples/imagevideo.mjs" download>JavaScript 客户端</a></footer></main>`
  const $=s=>root.querySelector(s)
  const notice=(message,error=false)=>{$('#notice').textContent=message;$('#notice').classList.toggle('error',error)}
  const copy=async text=>{try{await navigator.clipboard.writeText(text);notice('已复制')}catch{notice('复制失败，请手动选择内容',true)}}
  const selectedSections=()=>sections.filter(s=>category.sections.includes(s.id))
  const code=b=>`<div class="code"><div class="code-title"><span>${escape(b.label)}</span><button data-copy>复制</button></div><pre><code>${escape(b.text)}</code></pre></div>`
  const block=b=>b.kind==='heading'?`<h3>${escape(b.text)}</h3>`:b.kind==='text'?`<p>${escape(b.text)}</p>`:b.kind==='endpoint'?`<div class="endpoint"><b>${escape(b.method)}</b><code>${escape(API_ORIGIN+b.path)}</code></div>`:b.kind==='code'?code(b):`<div class="table"><table><thead><tr>${b.headers.map(x=>`<th>${escape(x)}</th>`).join('')}</tr></thead><tbody>${b.rows.map(r=>`<tr>${r.map(c=>`<td>${escape(c)}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`
  function view() {
    $('h1').textContent=category.title; document.title=category.title+' · 词元互联'
    $('nav').innerHTML=categories.map(c=>`<a href="#${c.id}" class="${c===category?'active':''}">${c.title}</a>`).join('')
    $('.tabs').innerHTML=['接口文档','输入示例','输出示例','在线调试'].map(t=>`<button role="tab" aria-selected="${t===tab}" class="${t===tab?'active':''}">${t}</button>`).join('')
    if(tab==='在线调试') {debug();return}
    $('article').innerHTML=selectedSections().map(s=>{
      const blocks=s.blocks.filter(b=>tab==='接口文档'?b.kind!=='code'||b.language==='http':b.kind==='code'&&(tab==='输出示例'?b.language==='json':b.language!=='json'&&b.language!=='http'))
      return blocks.length?`<section id="${escape(s.id)}" class="hash-anchor"><h2>${escape(s.title)}</h2><p>${escape(s.subtitle)}</p>${blocks.map(block).join('')}</section>`:''
    }).join('')+`<div class="buttons"><button id="copy-group">复制本类 Markdown</button></div>`
    $('#copy-group').onclick=()=>copy(selectedSections().map(sectionMarkdown).join('\n\n'))
    $('article').querySelectorAll('[data-copy]').forEach(button=>button.onclick=()=>copy(button.closest('.code').querySelector('code').textContent))
  }
  const filtered=()=>profiles.filter(p=>p.operations.some(op=>category.ops.includes(op)))
  function debug() {
    const options=filtered()
    $('article').innerHTML=`<p>先选择模型与能力配置。点击“提交生成”会实际扣费；请求不明确时保留原幂等键，只恢复查询。</p><div class="fields"><label class="wide">模型 / 能力配置<select id="model"><option value="">请选择模型</option>${options.map((p,i)=>`<option value="${i}">${escape(p.model)} · ${escape(p.media_type)} · ${escape(p.profile_id.slice(-8))}</option>`).join('')}</select></label></div><div id="parameters"></div><div class="buttons"><button id="submit" class="primary" disabled>提交生成（实际计费）</button><button id="recover" ${pending?'':'disabled'}>恢复原任务查询</button><button id="stop">停止等待</button><button id="new">新任务</button></div><p class="status" id="task-status"></p><div class="debug-panels">${code({label:'请求 JSON（编码内容省略）',text:'尚未提交'})}${code({label:'响应 / SSE 文字事件',text:'尚无响应'})}</div><div id="debug-error" class="error" role="alert"></div><div class="artifacts"></div>`
    const codes=$('article').querySelectorAll('.code code')
    const response=v=>codes[1].textContent=typeof v==='string'?v:pretty(v)
    const failure=e=>{$('#debug-error').textContent=e.message||String(e)}
    const model=$('#model')
    model.onchange=()=>{
      const cap=options[model.value]; if(!cap){$('#parameters').innerHTML='';$('#submit').disabled=true;return}
      const pick=(name,label,values=[])=>values.length?`<label>${label}<select name="${name}"><option value="">自动 / 省略</option>${values.map(v=>`<option>${escape(v)}</option>`).join('')}</select></label>`:''
      const duration=cap.duration||{}
      const seconds=duration.default??duration.values?.[0]??duration.min??''
      let html=`<label>调用场景<select name="operation">${cap.operations.filter(op=>category.ops.includes(op)).map(op=>`<option value="${op}">${escape({image:'文字生图',image_edit:'参考图编辑',gemini_image:'Gemini 生图 / 编辑',video:'视频生成',audio:'专用音乐接口',chat_music:'Chat 音乐接口',speech:'语音合成'}[op])}</option>`).join('')}</select></label><label class="wide">提示词<textarea name="prompt" placeholder="输入生成描述或编辑要求" required></textarea></label>`
      html+=pick('size','尺寸 / 分辨率档位',cap.sizes)+pick('quality','画质',cap.qualities)+pick('background','背景',cap.backgrounds)+pick('output_format','图片输出格式',cap.output_formats)+pick('resolution','视频清晰度',cap.resolutions)+pick('ratio','画面比例',cap.aspect_ratios)
      if((cap.media_type==='video'||cap.media_type==='audio')&&!cap.operations.every(op=>op==='speech')) html+=`<label>时长（秒）<input name="seconds" type="number" value="${escape(seconds)}" min="1"><small>目录建议：${escape(duration.values?.join(' / ')||'由所选配置决定')}；由上游确认支持范围</small></label>`
      if(cap.media_type==='video')html+=pick('generate_audio','生成声音',['true','false'])
      if(cap.media_type==='audio')html+=`<label class="wide">歌词<textarea name="lyrics"></textarea></label><label>BPM<input name="bpm" type="number" min="1"></label>`+pick('instrumental','纯音乐',['true','false'])+pick('audio_format','输出格式',['mp3','wav','m4a'])
      if(cap.operations.includes('speech'))html+=`<label>声音<input name="voice" placeholder="使用管理员配置支持的声音 ID"></label><label>输出格式<input name="response_format" placeholder="如 mp3 / wav"></label><label>语速<input name="speed" type="number" min="0.01" step="0.01"></label>`
      if(cap.operations.includes('chat_music'))html+=pick('stream','Chat 返回方式',['false','true'])
      if(cap.media_type==='video')for(const role of ['first_frame','last_frame'])html+=`<label class="wide">${role==='first_frame'?'首帧':'尾帧'}（计入参考图总数）<textarea data-material="${role}" placeholder="一个 URL 或 data URL，按所选模型素材能力填写"></textarea></label>`
      if(cap.operations.includes('gemini_image'))html+=pick('modalities','返回模态',['TEXT','IMAGE','both'])
      const inputMaterials=inputMaterialsFor(cap)
      for(const [kind,material] of Object.entries(inputMaterials)) {
        // Missing catalogue counts do not disable unified reference inputs.
        const base64=true
        html+=`<label class="wide">${escape({image:'参考图片',video:'参考视频',audio:'参考音频',mask:'编辑遮罩（透明 PNG，与第一张原图同尺寸）'}[kind]||kind)} · URL / Data URL / Base64 对象${material.max_file_bytes?` · 每个最多 ${Math.floor(material.max_file_bytes/1048576)} MiB`:''}<textarea data-material="${kind}" placeholder="${base64?'每行一个 HTTP(S) URL 或 data URL':'仅公网 HTTP(S) 文件直链，每行一个'}"></textarea>${base64?`<input data-files="${kind}" type="file" accept="${kind==='mask'?'image/png':kind+'/*'}" multiple>`:'<small>此能力仅支持 URL，不支持 Base64 和本地上传。</small>'}</label>`
      }
      $('#parameters').innerHTML='<div class="fields">'+html+'</div>'
      $('#submit').disabled=running||!!pending
    }
    const api=async(path,method='GET',payload,headers={})=>{
      const url=new URL(path,location.origin);if(url.origin!==location.origin)throw Error('只允许请求本站 API')
      const r=await fetch(url,{method,headers:{Authorization:'Bearer '+key,...(payload instanceof FormData?{}:{'Content-Type':'application/json'}),...headers},body:payload?(payload instanceof FormData?payload:JSON.stringify(payload)):undefined,signal:AbortSignal.any([AbortSignal.timeout(method==='POST'?185000:30000),activeController?.signal||new AbortController().signal]),credentials:'omit',cache:'no-store',redirect:'error'})
      let data
      if(r.headers.get('content-type')?.includes('text/event-stream')) {
        const reader=r.body.getReader(),decoder=new TextDecoder();let text=''
        while(true){const {done,value}=await reader.read();if(done)break;text+=decoder.decode(value,{stream:true});response(text)}
        const events=text.split(/\r?\n/).filter(line=>line.startsWith('data:')).map(line=>line.slice(5).trim()).filter(line=>line&&line!=='[DONE]');data=events.length?JSON.parse(events.at(-1)):{};data.sse=text
      }else{const text=await r.text();try{data=JSON.parse(text)}catch{throw Error(`HTTP ${r.status}：响应不是 JSON`)}}
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
        $('#task-status').textContent=`任务 ${pending.id} · ${task.status} · ${task.billing?.state||''}`
        if(['completed','failed'].includes(task.status)) {
          if(task.status==='completed')await artifacts(pending.id)
          if(['settled','released'].includes(task.billing?.state)||task.status==='failed') {pending.terminal=true;return}
        }
        if(task.status==='unknown')throw Error('受理或费用待核对，请保留任务 ID，联系管理员；没有重新提交。')
        await new Promise(resolve=>{const timer=setTimeout(()=>{wakePoll=null;resolve()},20000);wakePoll=()=>{clearTimeout(timer);wakePoll=null;resolve()}})
      }
    }
    async function resume(token) {
      if(!pending)return
      if(!pending.id){const result=await api(`/v1/media/tasks/recover?operation=${encodeURIComponent(pending.operation)}&idempotency_key=${encodeURIComponent(pending.key)}`);pending.id=result.id;response(result)}
      if(!pending.id)throw Error('尚未找到原任务，请稍后恢复查询；不会重新生成。')
      await poll(token)
    }
    const execute=async fn=>{if(running)return;running=true;activeController=new AbortController();const token=++epoch;$('#submit').disabled=true;$('#debug-error').textContent='';try{await fn(token)}catch(e){if(token===epoch)failure(e)}finally{if(token===epoch){running=false;activeController=null;$('#recover').disabled=!pending;$('#submit').disabled=!!pending||!model.value}}}
    $('#submit').onclick=()=>execute(async token=>{
      if(pending)throw Error('已有任务，请先恢复查询')
      if(!key)throw Error('请先拉取模型')
      const cap=options[model.value],fields={},materials={}
      $('#parameters').querySelectorAll('[name]').forEach(el=>fields[el.name]=el.value)
      if(!fields.prompt?.trim())throw Error('请填写提示词')
      $('#parameters').querySelectorAll('[data-material]').forEach(el=>materials[el.dataset.material]=el.value.split('\n').map(s=>s.trim()).filter(Boolean).map(s=>s.startsWith('{')?JSON.parse(s):s))
      for(const el of $('#parameters').querySelectorAll('[data-files]')) {
        const kind=el.dataset.files,material=inputMaterialsFor(cap)[kind];materials[kind]||=[]
        for(const file of el.files) {
          if(file.size>material.max_file_bytes)throw Error('素材超过模型大小限制')
          materials[kind].push(await new Promise((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(reader.result);reader.onerror=()=>reject(Error('文件读取失败'));reader.readAsDataURL(file)}))
        }
      }
      const payload=buildRequest(cap,fields.operation,fields,materials)
      if(fields.operation==='image_edit'&&!materials.image?.length)throw Error('图片编辑必须提供参考图')
      codes[0].textContent=pretty(payload)
      pending={key:crypto.randomUUID(),operation:fields.operation,id:null,terminal:false}
      $('#task-status').textContent='幂等键 '+pending.key+' · 提交中，请保留本页'
      try{const result=await api(endpointFor(fields.operation,cap.model),'POST',payload,{'Idempotency-Key':pending.key});pending.id=result.task_id||result.id;response(result)}catch(e){if(e.status&&e.status<500&&e.status!==408){pending.terminal=true}throw e}
      $('#recover').disabled=false
      await resume(token)
    })
    $('#recover').onclick=()=>execute(resume)
    $('#stop').onclick=()=>{epoch++;activeController?.abort();activeController=null;wakePoll?.();running=false;$('#recover').disabled=!pending;notice('已停止轮询等待；任务仍在服务器运行，可恢复原任务查询。')}
    $('#new').onclick=()=>{if(pending&&!pending.terminal){failure(Error('原任务尚未结束或受理不明确，请恢复查询。'));return}pending=null;$('#submit').disabled=!model.value;$('#recover').disabled=true;$('#task-status').textContent='';operationCleanup()}
    function operationCleanup(){objectURLs.forEach(url=>URL.revokeObjectURL(url));objectURLs.clear();$('.artifacts').innerHTML=''}
    $('article').querySelectorAll('[data-copy]').forEach(button=>button.onclick=()=>copy(button.closest('.code').querySelector('code').textContent))
  }
  $('#key').oninput=()=>{if(pending){$('#key').value=key;notice('已有任务，完成核对前请使用原 Key。',true);return}key=$('#key').value.trim();profiles=[]}
  $('#show-key').onclick=()=>{$('#key').type=$('#key').type==='password'?'text':'password';$('#show-key').textContent=$('#key').type==='password'?'显示':'隐藏'}
  $('#load').onclick=async()=>{
    if(!key){notice('请填写 API Key',true);return}$('#load').disabled=true
    try{const r=await fetch('/v1/media/capabilities',{headers:{Authorization:'Bearer '+key},credentials:'omit',redirect:'error',signal:AbortSignal.timeout(30000)});const p=await r.json();if(!r.ok)throw Error(p.error?.message||'模型读取失败');profiles=p.models||[];notice(`已加载 ${profiles.length} 组模型能力，提交前请选择具体模型。`);if(tab==='在线调试'&&!running)debug()}catch(e){notice(e.message,true)}finally{$('#load').disabled=false}
  }
  $('#copy-all').onclick=()=>copy(fullMarkdown(sections))
  $('#export').onclick=()=>{const url=URL.createObjectURL(new Blob([fullMarkdown(sections)],{type:'text/markdown;charset=utf-8'}));const a=document.createElement('a');a.href=url;a.download='词元互联媒体接口文档.md';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}
  $('#theme').onclick=()=>{document.body.classList.toggle('dark');$('#theme').textContent=document.body.classList.contains('dark')?'☀ 浅色':'☾ 深色'}
  $('#menu').onclick=()=>$('aside').classList.toggle('open')
  $('.tabs').onclick=e=>{if(e.target.matches('button')){if(running){notice('请先停止等待或等待本轮查询完成。');return}tab=e.target.textContent;view()}}
  function navigate(){if(running){notice('任务正在查询，请先停止等待。');return}const hash=decodeURIComponent(location.hash.slice(1));category=categories.find(c=>c.id===hash||c.sections.includes(hash))||categories[0];view();$('aside').classList.remove('open');if(hash!==category.id)document.getElementById(hash)?.scrollIntoView()}
  window.addEventListener('hashchange',navigate)
  window.addEventListener('pagehide',()=>{key='';$('#key').value='';epoch++;objectURLs.forEach(url=>URL.revokeObjectURL(url))})
  navigate()
}
if(typeof document!=='undefined')start().catch(()=>{document.querySelector('#app').textContent='文档加载失败，请刷新页面。'})
