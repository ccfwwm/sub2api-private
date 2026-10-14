import pythonClient from '../../public/examples/imagevideo.py?raw'
import javascriptClient from '../../public/examples/imagevideo.mjs?raw'

export const docsTitle = '词元互联图片、视频与音乐接口文档'
export const docsVersion = '2026-10-14'
export const apiOrigin = 'https://code.aicodeme.xyz'
export type DocBlock = { kind: 'text'; text: string } | { kind: 'code'; language: string; text: string; label: string } | { kind: 'table'; headers: string[]; rows: string[][] }
export interface DocSection { id: string; title: string; subtitle: string; blocks: DocBlock[] }
const text = (value: string): DocBlock => ({ kind: 'text', text: value })
const code = (language: string, value: string, label = language): DocBlock => ({ kind: 'code', language, text: value, label })
const table = (headers: string[], rows: string[][]): DocBlock => ({ kind: 'table', headers, rows })
const json = (value: unknown, label = '响应示意（字段按场景变化）') => code('json', JSON.stringify(value, null, 2), label)
const curl = (path: string, payload?: unknown) => code('bash', `curl '${apiOrigin}${path}' \\\n  -H "Authorization: Bearer $MEDIA_API_KEY"${payload === undefined ? '' : ` \\\n  -H 'Content-Type: application/json' \\\n  -d '${JSON.stringify(payload, null, 2)}'`}`, 'cURL')
const taskResponse = { id: 'task_example', object: 'video', model: 'grok-imagine-video', status: 'queued', progress: 0, created_at: 1791936000, billing: { currency: 'USD', state: 'held', reserved: '0.00', accepted: '0.00', settled: '0.00', released: '0.00', refunded: '0.00', maximum_charge: '0.00', version: 1, quantities: {} }, data: [] }
const taskRead = text('异步提交返回 HTTP 202 和任务 ID。保存 id，按“通用任务”章节查询 /v1/tasks/{id}；完成后读取 data 或制品列表，携带原 API Key 下载。响应中的金额仅为结构示意，实际以 billing 为准。')
const imageRead = text('通常同步返回 HTTP 200：读取 data[].url，拼接本站根地址并携带原 API Key 下载；b64_json 是 Base64 文件内容。同步等待最多约 175 秒，也可能返回 HTTP 202 的任务状态；此时保存 id 并继续查询，勿重复提交。')

export const imagevideoModels: { id: string; type: string; limits: string; evidence: string }[] = [
  { id: 'omni-flash', type: 'video', limits: '4 / 6 / 8 / 10 秒，默认 10；720p；16:9；纯文本', evidence: '有完成记录' },
  ...['veo-3.1-fast','veo-3.1-lite','veo-3.1-quality'].map(id => ({ id, type: 'video', limits: '固定 8 秒；720p；16:9；纯文本', evidence: id === 'veo-3.1-lite' ? '有历史完成记录，当前参数需验证' : '已配置，待渠道验证' })),
  { id: 'grok-imagine-video', type: 'video', limits: '1–15 秒，默认 8；480p / 720p；最多 1 张图', evidence: '有完成记录' },
  { id: 'grok-imagine-video-1.5', type: 'video', limits: '1–15 秒，默认 8；480p / 720p / 1080p；最多 1 张图', evidence: '已配置，待渠道验证' },
  ...['lyria-3-pro','lyria-3.5'].map(id => ({ id, type: 'audio', limits: '音乐；时长提示 5–300 秒；不接收参考素材', evidence: id === 'lyria-3-pro' ? '有历史完成记录，当前参数需验证' : '已配置，待渠道验证' })),
  ...['gpt-image-2-1k','gpt-image-2-4k','gpt-image-2.5-4k','gpt-image-2.5-flare-1k','gpt-image-2.5-sunburst-1k','gpt-image-2.5-sunburst-4k'].map(id => ({ id, type: 'image', limits: '1024x1024 / 1536x1024 / 1024x1536；n=1；编辑素材能力待配置', evidence: id === 'gpt-image-2-1k' ? '有完成记录' : '已配置，待渠道验证' })),
  ...['gemini-3-pro-image-default','gemini-3-pro-image-preview-default','gemini-3.1-flash-image-default','gemini-3.1-flash-image-preview-default','gemini-nano-banana-2.1-default'].map(id => ({ id, type: 'image', limits: 'Gemini 原生文本生图；未配置参考素材和比例范围', evidence: id === 'gemini-3.1-flash-image-preview-default' ? '有完成记录' : '已配置，待渠道验证' })),
  { id: 'MinimaxH3-2K', type: 'video', limits: '4–15 秒，默认 15；2K / 768p；图 9 / 视频 3 / 音频 3', evidence: '已配置，待渠道验证' },
  { id: 'JSadanco2.0', type: 'video', limits: '显式 seconds 5–15 秒；最多 9 张图', evidence: '已配置，待渠道验证' },
  { id: 'doolaa2.0', type: 'video', limits: '显式 seconds 5–15 秒；纯文本', evidence: '有待核对任务，尚无成功承诺' },
  { id: 'doolaa2.5', type: 'video', limits: '显式 seconds 5–30 秒；纯文本', evidence: '已配置，待渠道验证' },
  { id: 'XXsedanco2.5S', type: 'video', limits: '显式 seconds 5–30 秒；纯文本', evidence: '已配置，待渠道验证' },
  { id: 'TTseadanco2.5-720', type: 'video', limits: '显式 seconds 5–30 秒；纯文本', evidence: '已配置，待渠道验证' },
  { id: 'wan3-1080p1', type: 'video', limits: '显式 seconds 5–30 秒；纯文本；勿由名字推断分辨率', evidence: '已配置，待渠道验证' }
]

export const imagevideoSections: DocSection[] = [
  { id: 'start', title: '快速开始', subtitle: '一个 Key，三种创作方式。', blocks: [
    text('在平台创建绑定“图片音频视频分组”（imagevideo）的 API Key，设置 MEDIA_API_KEY 环境变量。所有调用使用 Authorization: Bearer <API Key>；控制台登录令牌不能代替生成接口 Key。代码中的素材 URL 和 task_example 需要替换为自己的值。'),
    table(['用途','完整地址'], [['OpenAI 兼容接口根地址',`${apiOrigin}/v1`],['Gemini 原生接口根地址',`${apiOrigin}/v1beta`],['模型发现',`${apiOrigin}/v1/models`]]),
    text('本页为发布快照，模型 ID 区分大小写并保留别名。先查询自己的 Key 当前可见模型；配置能力不等于每个场景均已成功验证。模型清单与文档版本一同更新。'),
    curl('/v1/models'), json({ object: 'list', data: [{ id: 'grok-imagine-video', object: 'model' }] }, '模型发现响应节选'),
    curl('/v1/images/generations', { model: 'gpt-image-2-1k', prompt: '白色背景上的极简绿色植物插画', n: 1, size: '1024x1024' }), imageRead,
    table(['模型 ID','类型','已配置范围','验证情况'], imagevideoModels.map(m => [m.id,m.type,m.limits,m.evidence]))
  ] },
  { id: 'images', title: 'GPT 图片', subtitle: '从文本生成画面。', blocks: [
    text('六个 GPT 图片别名见模型清单。别名中的 1k / 4k 不是 size 参数值；当前配置只接受表中的像素尺寸。默认使用最小文本生图请求，n 仅支持 1。'),
    table(['参数','类型','要求 / 范围'], [['model','string','必填，使用模型清单中的 GPT 图片 ID'],['prompt','string','必填，非空画面描述'],['size','string','可选，1024x1024 / 1536x1024 / 1024x1536'],['n','integer','可选，仅 1'],['response_format','string','url 或 b64_json；大文件优先 URL'],['async','boolean','可选，true 立即返回任务状态']]),
    curl('/v1/images/generations', { model: 'gpt-image-2-1k', prompt: '清晨山谷，柔和自然光，摄影风格', size: '1536x1024', n: 1, response_format: 'url' }),
    json({ id:'task_example', created:1791936000, data:[{url:'/v1/tasks/task_example/artifacts/output-0/content'}] }), imageRead,
    text('编辑路径为 /v1/images/edits，使用 multipart/form-data 的 image / image[] 与 prompt。当前六个公开别名虽登记 image_edit 操作，但未设置可用素材数量，因此本快照不提供可成功执行的编辑示例。待分组补齐素材能力并完成验证后再开放；不能用内部模型名 gpt-image-2 绕过模型清单。')
  ] },
  { id: 'gemini', title: 'Gemini 图片', subtitle: '保留原生请求与响应。', blocks: [
    table(['参数','类型','要求'], [['URL 中的 model','string','模型清单中五个 Gemini ID 之一'],['contents','array','必填，parts[].text 提供文本提示'],['generationConfig','object','可选；本快照仅使用 responseModalities'],['参考素材 / 比例','—','尚无明确能力范围，本快照仅说明纯文本请求']]),
    curl('/v1beta/models/gemini-3.1-flash-image-preview-default:generateContent', { contents:[{role:'user',parts:[{text:'生成一张水彩风格的湖畔日出图片'}]}], generationConfig:{responseModalities:['TEXT','IMAGE']} }),
    json({task_id:'task_example', candidates:[{content:{role:'model',parts:[{text:'生成结果'},{inline_data:{mime_type:'image/png',data:'<Base64 图片内容>'}}]},finishReason:'STOP'}]}),
    text('读取 candidates[].content.parts[]；响应可使用 inline_data.data / inlineData.data，均为 Base64，按 mime_type / mimeType 保存。若返回 file_data.file_uri / fileData.fileUri，按实际 URI 读取；本站制品路径仍需 Bearer 鉴权。成功响应附带 task_id，可统一查询制品；若 HTTP 202 则使用 id 轮询。不要假定一定存在 OpenAI 的 data 数组。')
  ] },
  { id: 'grok', title: 'Grok 视频', subtitle: '文本与单张参考图。', blocks: [
    table(['参数','类型','范围 / 说明'], [['model','string','grok-imagine-video / grok-imagine-video-1.5'],['prompt','string','文本生视频必填；图生视频建议填写'],['seconds','integer','1–15，默认 8；duration 为同义字段，勿同时提交冲突值'],['resolution','string','480p / 720p；1.5 另支持 1080p'],['image','object','最多 1 张图；JSON 使用 {"url":"公开图片 URL"}'],['input_reference','file','multipart 参考图；与 JSON image 二选一'],['ratio','string','图生视频必须省略，比例由输入图决定']]),
    curl('/v1/videos', {model:'grok-imagine-video',prompt:'镜头缓慢掠过金色麦田',seconds:8,resolution:'720p'}), json(taskResponse), taskRead,
    curl('/v1/videos', {model:'grok-imagine-video',prompt:'让画面中的云朵缓慢移动',seconds:8,image:{url:'https://code.aicodeme.xyz/your-public-image.png'}}),
    code('bash', `curl '${apiOrigin}/v1/videos' \\\n  -H "Authorization: Bearer $MEDIA_API_KEY" \\\n  -F 'model=grok-imagine-video' -F 'prompt=让画面缓慢移动' \\\n  -F 'seconds=8' -F 'input_reference=@reference.png'`, 'multipart 图生视频'),
    text('参考图最大 32 MiB，请求总大小也受 32 MiB 限制，上传需留出表单开销。图生与文本生均返回相同的任务结构；用 id 查询并鉴权取片。模型主版本有完成记录，1.5 及不同素材场景仍以实际渠道验证为准。')
  ] },
  { id: 'veo-omni', title: 'Omni 与 Veo', subtitle: '按模型选择固定参数。', blocks: [
    table(['模型','seconds','resolution','ratio','素材'], [['omni-flash','4 / 6 / 8 / 10，默认 10','720p','16:9','本快照纯文本'],['veo-3.1-fast / lite / quality','固定 8','720p','16:9','本快照纯文本']]),
    curl('/v1/videos/generations', {model:'omni-flash',prompt:'一只小猫在窗边观察飘落的雪花',seconds:10,resolution:'720p',ratio:'16:9'}),
    curl('/v1/videos', {model:'veo-3.1-fast',prompt:'航拍海岸线，平稳运镜',seconds:8,resolution:'720p',ratio:'16:9'}),
    json({...taskResponse, model:'veo-3.1-fast'}), taskRead,
    text('当前 Omni、Veo 的素材 max_count 未配置为有限值；虽然协议支持参考图字段，本站校验会拒绝未配置的输入能力。本快照不承诺参考图、首尾帧、视频续写。Omni 与 Veo Lite 有历史完成记录；当前配置的具体场景及 Veo Fast、Quality 仍需渠道验证。')
  ] },
  { id: 'other-video', title: '其他视频模型', subtitle: '多素材与显式时长。', blocks: [
    table(['参数','支持范围 / 要求'], [['model','MinimaxH3-2K、JSadanco2.0 及清单中的其他视频 ID'],['seconds','JSadanco2.0、doolaa、XX、TT、wan 必须显式提交；范围见模型清单'],['resolution','仅 MinimaxH3-2K 明确配置 2K / 768p；其余不要由模型名推断'],['images','JS / Minimax 最多 9 张，支持公开 URL 字符串或 {url}；JS 另支持 data URL'],['videos / audios','仅 Minimax 明确配置，各最多 3 项；每项最长 30 秒、32 MiB'],['素材格式','JSON URL 对象可带 duration_seconds；支持配置内格式，不要混用同义字段'],['请求大小','合计最多 32 MiB；素材 URL 需公开可访问']]),
    curl('/v1/videos', {model:'JSadanco2.0',prompt:'把参考图转换成平稳的电影镜头',seconds:10,images:[{url:'https://code.aicodeme.xyz/your-public-image.png'}]}),
    curl('/v1/videos', {model:'MinimaxH3-2K',prompt:'依据参考画面与节奏生成短片',seconds:10,resolution:'768p',images:['https://code.aicodeme.xyz/your-public-image.png'],videos:[{url:'https://code.aicodeme.xyz/your-public-video.mp4',duration_seconds:10}],audios:[{url:'https://code.aicodeme.xyz/your-public-audio.mp3',duration_seconds:10}]}),
    curl('/v1/videos', {model:'doolaa2.5',prompt:'城市夜景，缓慢推进镜头',seconds:15}),
    json({...taskResponse,model:'JSadanco2.0'}), taskRead,
    text('以上为已配置能力的调用模板，尚无逐场景成功承诺。doolaa2.0 存在待核对历史任务；unknown 时先核对受理结果。XX 的素材数量未设置，TT、doolaa、wan 未开放素材能力；这些模型只提交文本及明确配置参数。')
  ] },
  { id: 'music', title: 'Lyria 音乐', subtitle: 'Chat 与专用音乐接口。', blocks: [
    text('音乐模型仅 lyria-3-pro 与 lyria-3.5。不含语音识别、语音合成或实时音频。默认同步等待；显式 async:true 才立即返回任务状态。时长是生成提示，实际文件时长与收费数量以任务结果为准。'),
    table(['参数','类型','范围 / 说明'], [['model','string','lyria-3-pro / lyria-3.5'],['prompt','string','专用接口必填；1–10000 字符'],['messages','array','Chat 接口使用 user 文本消息'],['duration','integer','可选提示，5–300 秒；与 seconds 同义'],['title / lyrics','string','可选，分别最多 160 / 20000 字符'],['instrumental','boolean','纯音乐；true 时 lyrics 被忽略'],['bpm','integer','可选，30–300'],['seed','integer','可选，0–2147483647'],['audio_format','string','可选，mp3 / wav / m4a；实际文件按 MIME 处理'],['n','integer','仅 1'],['async / stream','boolean','async 为异步开关；Chat stream 为文字 SSE']]),
    curl('/v1/audio/generations', {model:'lyria-3.5',prompt:'温暖钢琴与弦乐，宁静舒缓，适合阅读',duration:60,instrumental:true,async:true}),
    json({...taskResponse,object:'audio.generation',model:'lyria-3.5'}), taskRead,
    text('兼容路径 /v1/music/generations 与 /v1/audio/generations 使用同样请求。GET /v1/audio/generations/{id} 查询状态，/{id}/content 获取首个产物；也可使用通用任务路径。'),
    curl('/v1/music/generations', {model:'lyria-3-pro',prompt:'轻快的电子音乐，清晰的鼓点',duration:30,instrumental:true}),
    json({id:'task_example',created:1791936000,data:[{url:'/v1/tasks/task_example/artifacts/output-0/content',duration:'30.12'}]}), imageRead,
    curl('/v1/chat/completions', {model:'lyria-3.5',messages:[{role:'user',content:'生成一段轻柔的爵士音乐，以钢琴为主'}],duration:60,stream:false}),
    json({id:'task_example',task_id:'task_example',object:'chat.completion',model:'lyria-3.5',choices:[{index:0,message:{role:'assistant',content:'音乐生成完成'},finish_reason:'stop'}],data:[{url:'/v1/tasks/task_example/artifacts/output-0/content'}]}),
    text('Chat 保留 choices 和实际 usage（如有）；优先用 task_id 查询制品。stream:true 在生成结束后发送文字事件，读取 choices[].delta.content，不能把 SSE 当作可播放音频。等待超时或人工核对时仍可能返回 application/json 的 202；先检查 HTTP 状态与 Content-Type。'),
    code('bash', `curl -N '${apiOrigin}/v1/chat/completions' \\\n  -H "Authorization: Bearer $MEDIA_API_KEY" -H 'Content-Type: application/json' \\\n  -d '{"model":"lyria-3.5","messages":[{"role":"user","content":"生成舒缓钢琴音乐"}],"stream":true}'`, '音乐文字 SSE'),
    code('javascript', `// 服务端示例：fetch 后先检查状态及 Content-Type。
if (!response.ok) throw new Error('HTTP ' + response.status);
if (response.status === 202 || response.headers.get('content-type')?.includes('application/json')) {
  const task = await response.json(); console.log(task.id || task.task_id);
} else {
  const reader = response.body.getReader(), decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    buffer = buffer.replace(/\\r\\n/g, '\\n');
    let end;
    while ((end = buffer.indexOf('\\n\\n')) >= 0) {
      const event = buffer.slice(0, end); buffer = buffer.slice(end + 2);
      const data = event.split('\\n').filter(l => l.startsWith('data:')).map(l => l.slice(5).trim()).join('\\n');
      if (data && data !== '[DONE]') console.log(JSON.parse(data).choices?.[0]?.delta?.content || '');
    }
    if (done) break;
  }
}`, '按文字事件读取 SSE')
  ] },
  { id: 'tasks', title: '通用任务与下载', subtitle: '提交一次，持续跟踪。', blocks: [
    table(['状态','处理方式'], [['queued','已排队，继续查询；内部 submitting 对外也映射为 queued'],['in_progress','生成中；建议间隔 10–20 秒查询'],['completed','读取 data 或制品列表并下载'],['failed','查看 diagnostic 与账务状态，确认失败后再决定新任务'],['unknown','受理结果待核对；保存任务 ID，禁止自动重新生成']]),
    curl('/v1/tasks/task_example'),
    json({...taskResponse,status:'completed',progress:100,completed_at:1791936060,data:[{key:'output-0',type:'video',mime_type:'video/mp4',size_bytes:123456,duration_seconds:'8',content_url:'/v1/tasks/task_example/artifacts/output-0/content',available:true}]}),
    curl('/v1/tasks/task_example/artifacts'), json({task_id:'task_example',artifacts:[{key:'output-0',type:'video',mime_type:'video/mp4',size_bytes:123456,duration_seconds:'8',content_url:'/v1/tasks/task_example/artifacts/output-0/content',available:true}]}),
    code('bash', `curl '${apiOrigin}/v1/tasks/task_example/artifacts/output-0/content' \\\n  -H "Authorization: Bearer $MEDIA_API_KEY" -D headers.txt -o artifact.bin`, '鉴权下载'),
    text('content_url 通常是相对路径，须与根地址拼接；仅向本站发送 Key，勿向其他域名转发认证头。下载不是公开直链，使用创建任务的原 Key；用户或 Key 不匹配不能访问。根据 Content-Type 决定 .png / .mp4 / .mp3 / .wav 等扩展名，勿固定保存成 MP3。available:false 或 HTTP 410 表示已失效。视频兼容 GET /v1/videos/{id} 与 /v1/videos/{id}/content。'),
    table(['账务字段','说明'], [['reserved / maximum_charge','预留额度 / 最大可能收费'],['accepted / settled','受理费 / 已结算费用'],['released / refunded','释放预留 / 已退款金额'],['state','held、pending_usage、settled、released、manual_review'],['quantities / calculation','计费数量与明细（结算后可用）'],['multiplier / time_multiplier / price_version','媒体倍率 / 分时倍率 / 锁定价格版本'],['version','管理员账务操作的乐观锁版本']]),
    text('预留不代表已消费；净费用通常为 accepted + settled − refunded。模型返回完成状态也需同时检查 billing.state；缺少真实用量时可能等待结算或进入人工核对，不要仅凭前端状态推断账务完成。')
  ] },
  { id: 'errors', title: '估价与错误处理', subtitle: '先确认参数，再创建任务。', blocks: [
    curl('/v1/sub2api/estimates', {operation:'video',request:{model:'grok-imagine-video',prompt:'海边日落',seconds:8}}),
    json({currency:'USD',calculation:{currency:'USD',total:'0.00',uncapped:'0.00',items:[],missing_usage:[]},maximum_charge:'0.00',price_version:'example',locks_price:false}),
    text('估价只做能力校验与费用计算，不发起生成。operation 使用 image、image_edit、gemini_image、video、audio 或 chat_music；request 为对应提交参数。真实输出时长、token 和账务明细可能使最终结算与预估不同。'),
    table(['HTTP / 错误','建议'], [['400','参数范围、冲突同义字段、未配置素材或必填 seconds；修正请求'],['401 / 403','核对 Key、分组、模型权限及任务归属'],['409','幂等参数冲突；管理员版本冲突时刷新后核对'],['413','请求或素材过大'],['422','能力或价格尚未配置；联系管理员补齐'],['429','并发或额度限制；GET 可等待后再查，POST 勿盲重试'],['502 / 503 / 504 / 网络超时','可能已经受理，先用已有任务 ID 查询'],['410','制品已失效']]),
    json({error:{type:'unsupported_capability',message:'Unsupported resolution'}}, '错误结构示意'),
    text('一般模型单能力并发为 2、观察窗口约 3600 秒，最终以分组配置为准。可使用 Idempotency-Key 固定同一提交；相同 Key 绑定的请求参数必须一致。客户端超时不取消任务。提交前保存本地状态文件，提交成功保存 id / task_id；中断后只恢复查询，绝不自动再 POST。')
  ] },
  { id: 'clients', title: '完整客户端示例', subtitle: 'Python 与 JavaScript。', blocks: [
    text('Python 3.10+ 仅用标准库；JavaScript 使用 Node.js 20+，在服务端运行。将上方任一 JSON 请求保存为 request.json。脚本分为 submit 与 resume：每个状态文件只提交一次；resume 完成轮询与制品下载。异步响应、同步成功及同步回退 202 均通过任务 ID 汇合到同一流程。脚本不消费 Chat SSE，使用 stream:false。'),
    code('bash', `# 设置 MEDIA_API_KEY 后执行；不把密钥写入脚本。
python imagevideo.py submit --endpoint /v1/videos --payload request.json --state task.json
python imagevideo.py resume --state task.json --output results

node imagevideo.mjs submit --endpoint /v1/audio/generations --payload request.json --state music.json
node imagevideo.mjs resume --state music.json --output music-results

# 图片用 /v1/images/generations；Gemini 用完整 /v1beta/models/{model}:generateContent。
# 恢复查询用原状态文件；新的生成必须使用新的状态文件。`, '运行方式'),
    code('python',pythonClient,'imagevideo.py'), code('javascript',javascriptClient,'imagevideo.mjs')
  ] }
]

export function sectionMarkdown(section: DocSection): string {
  return `## ${section.title}\n\n${section.subtitle}\n\n` + section.blocks.map(block => {
    if (block.kind === 'text') return block.text
    if (block.kind === 'code') return `### ${block.label}\n\n\`\`\`${block.language}\n${block.text}\n\`\`\``
    const row = (cells: string[]) => '| ' + cells.map(cell => cell.replace(/\|/g, '\\|').replace(/\n/g, '<br>')).join(' | ') + ' |'
    return [row(block.headers),row(block.headers.map(() => '---')),...block.rows.map(row)].join('\n')
  }).join('\n\n')
}
export const imagevideoMarkdown = () => `# ${docsTitle}\n\n版本：${docsVersion}\n\n根地址：${apiOrigin}\n\n` + imagevideoSections.map(sectionMarkdown).join('\n\n')
