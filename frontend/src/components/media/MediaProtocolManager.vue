<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import MediaParameterRulesEditor from './MediaParameterRulesEditor.vue'
import MediaMaterialsEditor from './MediaMaterialsEditor.vue'
import MediaResponseEditor from './MediaResponseEditor.vue'
import { mediaProtocolsAPI as api, mediaPreviewTimeoutMs, type ProtocolKind } from '@/api/media-protocols'
import { mediaAPI, type MediaConfigRecord, type MediaTask, type MediaTaskReview } from '@/api/media'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { useStepUp, isStepUpCancelled } from '@/composables/useStepUp'

const route = useRoute(), stepUp = useStepUp()
const kind = ref<ProtocolKind>(route.query.tab === 'bindings' ? 'bindings' : 'templates')
const templates = ref<MediaConfigRecord[]>([]), bindings = ref<MediaConfigRecord[]>([])
const search = ref(''), mediaFilter = ref(''), statusFilter = ref('')
const visibleRows = computed(() => rows.value.filter(row => {
  const query = search.value.trim().toLowerCase()
  return (!query || `${row.id} ${row.config.name || ''}`.toLowerCase().includes(query)) && (!mediaFilter.value || (row.config.media_types as string[] | undefined)?.includes(mediaFilter.value)) && (!statusFilter.value || row.status === statusFilter.value)
}))
const advancedFields = [
  { path: 'query', label: '任务查询接口', hint: '异步模式必填。配置只读轮询的 GET 方法和相对路径；{task_id} 会替换成上游返回的任务 ID。' },
  { path: 'discovery', label: '模型发现接口', hint: '只用于“发现候选模型”，不会启用模型。通常选择 GET /v1/models。' },
  { path: 'model_ids', label: '模型 ID 提取路径', hint: '填写模型列表响应里模型 ID 的 JSON 路径，例如 data[].id。' },
  { path: 'artifact_list', label: '单独的成品列表接口', hint: '仅当任务状态响应不包含成品、而上游另有只读 GET 列表接口时配置。' },
  { path: 'download_auth', label: '成品下载认证', hint: '只有下载成品需要与任务查询不同的认证时配置；密钥始终引用账号凭据。' },
  { path: 'recover', label: '超时后的只读任务恢复', hint: '按幂等 ID 查询是否已受理；这里不能配置 POST，也不会自动重提生成。' },
  { path: 'body', label: '请求体映射', hint: '将统一参数重组为上游请求体；普通字段直接保留，特殊字段用 $input 或 $variable 引用。' },
  { path: 'operation_submit', label: '操作专属提交接口', hint: '仅当同一模板中的不同操作使用不同提交路径或编码时填写。' }
]
function toggleOption(path: string, option: string, checked: boolean) {
  const current = (field(path) as string[] | undefined) || []
  set(path, checked ? [...new Set([...current, option])] : current.filter(item => item !== option))
}
function statusLabel(status: string) { return kind.value === 'templates' ? '可用' : ({ enabled: '已发布', draft: '草稿', disabled: '已停用' } as Record<string, string>)[status] || status }
const selected = ref<MediaConfigRecord | null>(null)
const json = ref(''), mode = ref('form'), busy = ref(false), error = ref(''), notice = ref(''), result = ref<unknown>()
const deleting = ref(false), bulkModels = ref('')
const previewInput = ref('{"model":"preview","prompt":"参考素材测试","seconds":5,"images":[]}')
const previewStage = ref<'idle' | 'preparing' | 'sending' | 'ready' | 'failed'>('idle'), previewBytes = ref(0)
const previewTimeoutMs = ref(30000)
const previewReady = ref(false)
const sample = ref('{}'), online = ref(false), operation = ref('video'), upstreamModel = ref('preview')
const previewKeyID = ref(''), previewAccountID = ref('')
const effectiveGroup=ref(String(route.query.group_id || '')), effectiveResult=ref<unknown>()
const keyID = ref(''), paid = ref(false), discoveryAccount = ref(''), discoveryGroup = ref('')
const paidTask = ref<MediaTask | null>(null), paidTaskReview = ref<MediaTaskReview | null>(null), paidTaskError = ref(''), paidTaskLoading = ref(false)
const paidArtifactAccess = ref<Record<string, { url: string; expires_at: number }>>({}), paidArtifactErrors = ref<Record<string, string>>({}), paidArtifactLoading = ref('')
const paidSubmitState = ref<'idle' | 'sending' | 'uncertain' | 'rejected'>('idle'), paidRequestBytes = ref(0)
const paidRecoveryBody = ref<Record<string, unknown> | null>(null)
let paidTaskTimer: ReturnType<typeof setTimeout> | undefined
let paidTaskGeneration = 0
// Retain the key after a timeout: clicking again cannot create another paid task.
const testKey = ref(crypto.randomUUID())
const draft = computed<Record<string, unknown> | null>(() => { try { const value = JSON.parse(json.value); return value && !Array.isArray(value) && typeof value === 'object' ? value : null } catch { return null } })
const accountFilter = ref(String(route.query.account_id || '')), groupFilter = ref(String(route.query.group_id || ''))
const rows = computed(() => kind.value === 'templates' ? templates.value : bindings.value.filter(row => (!accountFilter.value || String(row.config.account_id) === accountFilter.value || (groupFilter.value && !Number(row.config.account_id))) && (!groupFilter.value || String(row.config.group_id) === groupFilter.value)))
watch(() => route.query.account_id, value => { accountFilter.value = String(value || ''); selected.value = null; json.value = '' })
watch(() => route.query.group_id, value => { groupFilter.value = String(value || ''); effectiveGroup.value = String(value || '') })
const publishedTemplates = computed(() => templates.value.filter(row => row.published_version))
const paidTaskStage = computed(() => {
  if (!paidTask.value) return ''
  if (paidTask.value.status === 'completed') return '生成已完成'
  if (paidTask.value.status === 'failed') return '生成失败'
  if (paidTask.value.status === 'unknown') return '上游受理结果待核对'
  return ({ local_pending: '本站已受理，等待后台提交上游', submitting: '本站正在提交上游', upstream_accepted: '上游已受理，正在生成', reconciling: '正在核对上游是否已受理' } as Record<string, string>)[paidTask.value.submission_phase || ''] || (paidTask.value.status === 'queued' ? '本站任务排队中' : '任务处理中')
})
const paidTaskBilling = computed(() => {
  const bill = paidTask.value?.billing
  return bill ? `账单 ${bill.state} · 预留 ${bill.reserved} · 已接受 ${bill.accepted} · 已结算 ${bill.settled} · 已释放 ${bill.released}` : ''
})
// An unknown task is still financially unresolved; keep the operator on its
// original task instead of enabling a second paid generation.
const paidTaskPending = computed(() => !!paidTask.value && !['completed', 'failed'].includes(paidTask.value.status))
const previewSummary = computed(() => {
  if (!previewReady.value || !result.value || typeof result.value !== 'object') return ''
  const row = result.value as Record<string, unknown>
  const receipt = row.request && typeof row.request === 'object' ? row.request as Record<string, unknown> : {}
  const refs = receipt.references && typeof receipt.references === 'object' ? Object.entries(receipt.references as Record<string, unknown>).map(([kind, count]) => `${kind} ${count}`).join('、') : ''
  const route = typeof receipt.path === 'string' ? `映射接口 ${String(receipt.method || 'POST')} ${receipt.path}` : (row.mode === 'effective' ? '已解析实际绑定' : '已完成模板映射')
  return [`免费检查完成 · ${route}`, refs ? `素材：${refs}` : '', '未调用生成接口 · 未创建任务 · 未扣费'].filter(Boolean).join(' · ')
})
function field(path: string): unknown { return path.split('.').reduce<unknown>((value, key) => (value as Record<string, unknown> | undefined)?.[key], draft.value) }
function set(path: string, value: unknown) {
  if (!draft.value) return
  const object = JSON.parse(json.value) as Record<string, unknown>, parts = path.split('.'); let target = object
  for (const part of parts.slice(0, -1)) { target[part] ??= {}; target = target[part] as Record<string, unknown> }
  target[parts.at(-1)!] = value; json.value = JSON.stringify(object, null, 2)
}
function textField(path: string, event: Event) {
  const input = event.target as HTMLInputElement | HTMLTextAreaElement
  const start = input.selectionStart, end = input.selectionEnd
  set(path, input.value)
  void nextTick(() => input.setSelectionRange(start, end))
}
const value = (event: Event) => (event.target as HTMLInputElement).value
function jsonField(path: string, event: Event) { try { set(path, JSON.parse(value(event))); error.value = '' } catch { error.value = `${path} 需要有效 JSON` } }
async function action(run: () => Promise<void>, mutation = false) {
  busy.value = true; error.value = ''; notice.value = ''
  try { if (mutation) await stepUp.run(run); else await run() } catch (cause) { if (!isStepUpCancelled(cause)) error.value = (cause as Error).message || String(cause) } finally { busy.value = false }
}
async function refresh() {
  const results = await Promise.allSettled([api.list('templates'), api.list('bindings')])
  const failures: string[] = []
  if (results[0].status === 'fulfilled') templates.value = results[0].value.filter(row => row.status !== 'archived' && row.status !== 'disabled'); else failures.push('模板：' + String(results[0].reason?.message || results[0].reason))
  if (results[1].status === 'fulfilled') bindings.value = results[1].value; else failures.push('绑定：' + String(results[1].reason?.message || results[1].reason))
  error.value = failures.join('；')
}
async function select(row: MediaConfigRecord) { selected.value = row; const object = { ...row.config }; delete object.version; delete object.status; delete object.builtin_revision; json.value = JSON.stringify(object, null, 2); deleting.value = false }
function create() {
  selected.value = null; deleting.value = false; mode.value = 'form'; error.value = ''; notice.value = ''
  const object = kind.value === 'templates' ? { id: `custom-${crypto.randomUUID()}`, name: '自定义协议模板', description: '', schema_version: 1, media_types: ['video'], operations: ['video'], mode: 'async', auth: { mode: 'bearer' }, submit: { method: 'POST', path: '/v1/videos', encoding: 'json' }, query: { method: 'GET', path: '/v1/videos/{task_id}' }, response: { task_id: ['id'], status: ['status'], states: { queued: 'in_progress', completed: 'completed', failed: 'failed' }, urls: ['url'] }, poll_seconds: 10, materials: [], rules: [] } : { id: `binding-${crypto.randomUUID()}`, group_id: Number(groupFilter.value || 0), account_id: Number(accountFilter.value || 0), model: '', upstream_model: '', rules: [] }
  json.value = JSON.stringify(object, null, 2)
}
onBeforeUnmount(() => { paidTaskGeneration++; if (paidTaskTimer) clearTimeout(paidTaskTimer) })
function newTest() { if (paidTaskPending.value || paidSubmitState.value === 'sending') return; if (paidTaskTimer) clearTimeout(paidTaskTimer); paidTask.value=null; paidTaskReview.value=null; paidTaskError.value=''; paidArtifactAccess.value={}; paidArtifactErrors.value={}; paidRecoveryBody.value=null; paidSubmitState.value='idle'; testKey.value=crypto.randomUUID();paid.value=false }
function schedulePaidTaskRefresh(id: string) {
  if (paidTaskTimer) clearTimeout(paidTaskTimer)
  if (!paidTaskPending.value || paidTask.value?.id !== id) return
  paidTaskTimer = setTimeout(() => { void refreshPaidTask(id) }, 3000)
}
async function refreshPaidTask(id = paidTask.value?.id) {
  if (!id) return
  const version = paidTaskGeneration
  paidTaskLoading.value = true; paidTaskError.value = ''
  try {
    const review = await mediaAPI.detail(id)
    if (version !== paidTaskGeneration || paidTask.value?.id !== id) return
    paidTaskReview.value = review
    paidTask.value = review.record.task
  } catch (cause) { if (version === paidTaskGeneration) paidTaskError.value = (cause as Error).message || String(cause) }
  finally { if (version === paidTaskGeneration) { paidTaskLoading.value = false; schedulePaidTaskRefresh(id) } }
}
async function loadPaidArtifact(artifact: MediaTask['data'][number]) {
  const id = paidTask.value?.id
  if (!id || !artifact.available) return
  paidArtifactLoading.value = artifact.key
  paidArtifactErrors.value = { ...paidArtifactErrors.value, [artifact.key]: '' }
  try { paidArtifactAccess.value = { ...paidArtifactAccess.value, [artifact.key]: await mediaAPI.artifactAccess(id, artifact.key, true) } }
  catch (cause) { paidArtifactErrors.value = { ...paidArtifactErrors.value, [artifact.key]: (cause as Error).message || String(cause) } }
  finally { paidArtifactLoading.value = '' }
}
async function sendPaidTest(body: Record<string, unknown>) {
  const version = ++paidTaskGeneration
  paidSubmitState.value = 'sending'; paidTaskError.value = ''
  try {
    const task = await api.test(body, testKey.value) as MediaTask
    if (version !== paidTaskGeneration) return
    if (!task?.id) throw new Error('响应中没有任务 ID；保留当前幂等 ID，再点“恢复 / 查询原任务”，不要新建请求。')
    paidTask.value = task
    paidTaskReview.value = null
    paidSubmitState.value = 'idle'
    await refreshPaidTask(task.id)
  } catch (cause) {
    if (version === paidTaskGeneration) {
      const message = (cause as Error).message || String(cause)
      const failure = cause as { response?: { status?: number }; status?: number }
      const status = failure.response?.status || failure.status
      const definiteRejection = typeof status === 'number' && status >= 400 && status < 500 && status !== 408 && status !== 409
      paidSubmitState.value = definiteRejection ? 'rejected' : 'uncertain'
      paidTaskError.value = definiteRejection
        ? `本站拒绝了提交（HTTP ${status}）：${message}。没有返回任务 ID；修正问题后请明确开始新的测试。`
        : `本站受理结果尚未确认：${message}。已保留幂等 ID 和原请求内容；使用“恢复 / 查询原任务”安全重试同一请求，不会创建第二个任务。`
    }
    throw cause
  }
}
async function submitPaidTest() {
  if (!paid.value) throw new Error('请先确认本次真实生成可能收费。')
  if (!keyID.value.trim()) throw new Error('请输入中转 API Key。')
  if (paidTask.value) throw new Error('当前测试已经创建任务；请查询原任务或明确开始另一次测试。')
  const body = { api_key: keyID.value.trim(), operation: operation.value, request: JSON.parse(previewInput.value), confirm_paid_submission: true }
  paidRequestBytes.value = new TextEncoder().encode(JSON.stringify(body)).byteLength
  paidRecoveryBody.value = body
  paidArtifactAccess.value = {}; paidArtifactErrors.value = {}
  await sendPaidTest(body)
}
async function recoverPaidTest() {
  if (!paidRecoveryBody.value) throw new Error('没有可恢复的原请求，请先提交一次真实测试。')
  await sendPaidTest(paidRecoveryBody.value)
}
function copy() {
  if (!draft.value) return
  const object: Record<string, unknown> = { ...draft.value, id: `custom-${crypto.randomUUID()}`, name: `${draft.value.name || '协议模板'}（副本）` }
  delete object.version; delete object.status
  selected.value = null; deleting.value = false; json.value = JSON.stringify(object, null, 2)
  notice.value = '已复制为新模板，可修改名称和介绍后保存。'
}
async function save() {
  if (!draft.value) throw new Error('请输入有效的配置 JSON')
  const id = String(draft.value.id || '').trim()
  if (!id) throw new Error('请填写模板 ID')
  const existing = selected.value?.id === id ? selected.value : null
  const row = kind.value === 'templates'
    ? await api.updateTemplate({ id, expected_version: existing?.version || 0, expected_config: existing?.config, config: draft.value })
    : await api.save(kind.value, { id, expected_version: existing?.version || 0, config: draft.value, publish: true })
  await refresh(); await select(row); notice.value = '已保存，绑定模型使用当前模板。'
}
async function remove() {
  if (!selected.value) return
  if (kind.value === 'templates') await api.updateTemplate({id:selected.value.id,expected_version:selected.value.version,expected_config:selected.value.config,delete:true})
  else await api.archive(kind.value,selected.value.id,selected.value.version)
  selected.value = null; json.value = ''; deleting.value = false
  await refresh(); notice.value = '已删除。'
}
async function download() {
  if (!draft.value) return
  const object = { ...draft.value }; delete object.version; delete object.status; delete object.builtin_revision
  const url = URL.createObjectURL(new Blob([JSON.stringify(object, null, 2)], { type: 'application/json' }))
  const link = document.createElement('a'); link.href = url; link.download = `${object.id}.json`; link.click(); URL.revokeObjectURL(url)
}
async function upload(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]; if (!file) return
  const imported = JSON.parse(await file.text()), object = imported.versions?.[0]?.config ?? imported
  if (!object || typeof object !== 'object' || !object.id) throw new Error('无效的模板或绑定文件')
  json.value = JSON.stringify({ ...object, id: `${object.id}-import-${Date.now()}` }, null, 2); selected.value = null; deleting.value = false
}
async function preview(effective = false) {
  previewStage.value = 'preparing'; previewBytes.value = 0; previewReady.value = false
  try {
    const request = JSON.parse(previewInput.value)
    let payload: Record<string, unknown>
    if (effective) {
      if (!previewKeyID.value.trim()) throw new Error('请输入中转 API Key')
      payload = { mode: 'effective', api_key: previewKeyID.value.trim(), ...(previewAccountID.value ? { account_id: Number(previewAccountID.value) } : {}), operation: operation.value, request, sample_response: JSON.parse(sample.value), measure_media: online.value }
    } else {
      payload = { template: draft.value, operation: operation.value, upstream_model: upstreamModel.value, model: request.model || 'preview', request, rules: [], sample_response: JSON.parse(sample.value), measure_media: online.value }
    }
    previewBytes.value = new TextEncoder().encode(JSON.stringify(payload)).byteLength
    previewTimeoutMs.value = mediaPreviewTimeoutMs(payload)
    previewStage.value = 'sending'
    try {
      result.value = await api.preview(payload)
      previewStage.value = 'ready'
      previewReady.value = true
    } catch (cause) {
      const failure = cause as { message?: string; code?: string; response?: unknown }
      const message = failure.message || String(cause)
      const timeout = ['ECONNABORTED', 'ETIMEDOUT'].includes(failure.code || '') || /timeout|timed out/i.test(message)
      const disconnected = !failure.response && /network error|failed to fetch|network request failed/i.test(message)
      previewStage.value = 'failed'
      throw new Error(timeout || disconnected
        ? `免费预览${timeout ? '超时' : '连接失败'}。大素材上传或网络延迟可能导致请求未完成；本次没有创建生成任务、没有扣费，可以安全重试。`
        : `免费预览失败：${message}。本次没有创建生成任务、没有扣费；修正输入后可以重新预览。`)
    }
  } catch (cause) {
    previewStage.value = 'failed'
    throw cause
  }
}
function bindTemplate(id: string) { const row = templates.value.find(item => item.id === id); set('template', row ? { id, version: row.published_version } : null) }
async function bulkBind() {
  if (!draft.value) return
  const done: string[] = []
  for (const model of [...new Set(bulkModels.value.split(/[,，\n]/).map(v => v.trim()).filter(Boolean))]) {
    const config = { ...draft.value, id: `binding-${crypto.randomUUID()}`, model, upstream_model: model }
    try {await api.save('bindings', { id: config.id, expected_version: 0, config, publish: true }); done.push(model)} catch(e) {result.value={published_models:done,failed_model:model,error:(e as Error).message};await refresh();throw e}
  }
  result.value = { published_models: done }; await refresh()
}
onMounted(() => action(refresh))
</script>

<template>
  <section class="space-y-4 rounded-lg border bg-white p-5 shadow-sm dark:border-gray-700 dark:bg-dark-800" aria-label="管理员协议模板">
    <header class="flex flex-wrap items-start justify-between gap-3"><div><h2 class="text-xl font-semibold">{{ kind === 'bindings' ? '媒体能力 / 模型绑定' : '协议模板' }}</h2><p class="mt-1 text-sm text-gray-500">{{ kind === 'bindings' ? '查看分组和账号的有效绑定、模板与请求能力；账号模型页负责日常绑定和启用。' : '管理请求协议、素材映射与任务响应。模型绑定在“账号模型”中完成。' }}</p></div><div class="flex gap-2"><button class="btn btn-primary" @click="create"><Icon name="plus" size="sm" />新建</button><label class="btn btn-secondary cursor-pointer">导入<input type="file" accept="application/json,.json" class="hidden" @change="action(() => upload($event))" /></label><button class="btn btn-secondary" :title="kind === 'bindings' ? '刷新绑定' : '刷新模板'" :aria-label="kind === 'bindings' ? '刷新绑定' : '刷新模板'" :disabled="busy" @click="action(refresh)"><Icon name="refresh" size="sm" /></button></div></header>
    <div v-if="kind === 'bindings'" class="flex flex-wrap items-end gap-3"><label class="text-sm">账号筛选<input v-model="accountFilter" type="number" min="1" class="input block" aria-label="绑定账号筛选" /></label><label class="text-sm">分组筛选<input v-model="groupFilter" type="number" min="1" class="input block" /></label><RouterLink v-if="accountFilter" class="text-sm text-primary-600" :to="{path:'/admin/media',query:{tab:'accounts',account_id:accountFilter}}">编辑账号 #{{ accountFilter }} 的模型</RouterLink></div>
    <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950">{{ error }}</p><p v-if="notice" role="status" class="rounded-lg bg-primary-50 p-3 text-sm text-primary-700 dark:bg-primary-950">{{ notice }}</p>
    <details v-if="kind === 'bindings'"><summary>查看分组实际继承配置</summary><input v-model="effectiveGroup" type="number" class="input" placeholder="分组 ID" /><button class="btn btn-secondary" @click="action(async()=>{effectiveResult=await api.effective(Number(effectiveGroup))})">解析当前绑定</button><pre class="max-h-96 overflow-auto text-xs">{{JSON.stringify(effectiveResult,null,2)}}</pre></details>
    <div class="grid min-w-0 gap-6 lg:grid-cols-[260px_minmax(0,1fr)]">
      <aside class="min-w-0 border-b pb-4 dark:border-dark-600 lg:border-b-0 lg:border-r lg:pr-4">
        <label class="relative block"><span class="sr-only">搜索模板</span><input v-model="search" class="input w-full" aria-label="搜索模板" placeholder="搜索名称或 ID" /></label>
        <div class="my-3 grid gap-2" :class="kind === 'bindings' ? 'grid-cols-2' : 'grid-cols-1'"><select v-model="mediaFilter" class="input w-full" aria-label="模板媒体类型"><option value="">全部类型</option><option value="image">图片</option><option value="video">视频</option><option value="audio">音频</option></select><select v-if="kind === 'bindings'" v-model="statusFilter" class="input w-full" aria-label="绑定状态"><option value="">全部状态</option><option value="enabled">已发布</option><option value="draft">草稿</option><option value="disabled">已停用</option></select></div>
        <p class="mb-2 text-xs text-gray-500">{{ visibleRows.length }} {{ kind === 'bindings' ? '个绑定' : '个模板' }}</p>
        <div class="max-h-[650px] overflow-auto"><p v-if="!visibleRows.length" class="border border-dashed p-4 text-sm text-gray-500">{{ rows.length ? '没有符合筛选的模板。' : '暂无协议模板，点击新建或导入。' }}</p><button v-for="row in visibleRows" :key="row.id" class="block w-full border-b px-3 py-3 text-left text-sm transition-colors dark:border-dark-600" :class="selected?.id === row.id ? 'border-l-2 border-l-primary-500 bg-primary-50 dark:bg-primary-950' : 'hover:bg-gray-50 dark:hover:bg-dark-700'" :aria-pressed="selected?.id === row.id" @click="action(() => select(row))"><span class="flex items-start justify-between gap-2"><strong class="min-w-0 break-words">{{ row.config.name || row.config.model || row.id }}</strong><span class="shrink-0 rounded px-1.5 py-0.5 text-xs" :class="row.published_version ? 'bg-green-50 text-green-700 dark:bg-green-950 dark:text-green-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'">{{ statusLabel(row.status) }}</span></span><span class="mt-1 block truncate text-xs text-gray-400" :title="row.id">{{ row.id }}</span><span class="mt-2 block text-xs text-gray-500">{{ (row.config.media_types as string[] || []).join(' / ') }} · {{ row.config.description || '未填写介绍' }}</span></button></div>
      </aside>
      <div v-if="json" class="min-w-0 space-y-4 pb-20">
        <div class="flex flex-wrap items-center justify-between gap-3"><div><h3 class="font-semibold">{{ draft?.name || '新模板' }}</h3><p class="text-xs text-gray-500">{{ selected ? '编辑当前协议模板' : '新模板 · 尚未保存' }}</p></div><div class="flex gap-2"><div class="flex rounded-lg border p-1 dark:border-dark-600"><button class="rounded px-3 py-1 text-sm" :class="mode === 'form' ? 'bg-primary-50 text-primary-700 dark:bg-primary-950' : ''" @click="mode = 'form'">表单</button><button class="rounded px-3 py-1 text-sm" :class="mode === 'json' ? 'bg-primary-50 text-primary-700 dark:bg-primary-950' : ''" @click="mode = 'json'">JSON</button></div><button class="btn btn-secondary" @click="copy">复制模板</button><button v-if="selected" class="btn btn-secondary" @click="action(download)">导出模板</button></div></div>
        <div v-if="mode === 'form' && draft" class="grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <h4 class="border-t pt-4 font-semibold sm:col-span-2 xl:col-span-3 dark:border-dark-600">基础信息</h4>
          <p class="text-xs leading-relaxed text-gray-500 sm:col-span-2 xl:col-span-3">先确认模板用途、媒体类型和操作范围。保存后供账号模型和分组直接使用。可复制模板创建另一套独立配置。</p>
          <label class="text-sm" title="稳定的内部引用标识，使用小写字母、数字和连字符">配置 ID<input class="input w-full" :disabled="!!selected" placeholder="例如 provider-video" :value="field('id')" @change="set('id', value($event))" /></label>
          <template v-if="kind === 'templates'">
            <label class="text-sm" title="给管理员看的模板名称，不会发送给上游">名称<input class="input w-full" placeholder="例如 通用视频异步协议" :value="field('name')" aria-label="模板名称" @input="textField('name', $event)" /></label>
            <label class="text-sm sm:col-span-2 xl:col-span-3">模板介绍<textarea class="input mt-1 min-h-20 w-full" aria-label="模板介绍" placeholder="说明此协议的用途、适用渠道和注意事项" :value="String(field('description') || '')" @input="textField('description', $event)" /></label>
            <label class="text-sm" title="async 表示提交后轮询任务；sync_json 解析 JSON 结果；sync_binary 直接读取媒体响应">响应模式<select class="input w-full" :value="field('mode')" @change="set('mode', value($event))"><option value="async">异步轮询</option><option value="sync_json">同步 JSON</option><option value="sync_binary">同步媒体二进制</option></select></label>
            <fieldset class="text-sm"><legend class="mb-2">媒体类型</legend><label v-for="item in [{id:'image',name:'图片'},{id:'video',name:'视频'},{id:'audio',name:'音频'}]" :key="item.id" class="mr-4 inline-flex items-center gap-1.5"><input type="checkbox" :checked="(field('media_types') as string[] || []).includes(item.id)" @change="toggleOption('media_types',item.id,($event.target as HTMLInputElement).checked)" />{{ item.name }}</label></fieldset>
            <fieldset class="text-sm"><legend class="mb-2">支持的请求操作</legend><label v-for="op in ['image','image_edit','gemini_image','video','audio','speech','chat_music']" :key="op" class="mb-2 mr-3 inline-flex items-center gap-1.5"><input type="checkbox" :checked="(field('operations') as string[] || []).includes(op)" @change="toggleOption('operations',op,($event.target as HTMLInputElement).checked)" />{{ op }}</label></fieldset>
            <h4 class="border-t pt-4 font-semibold sm:col-span-2 xl:col-span-3 dark:border-dark-600">提交与认证</h4>
            <p class="text-xs leading-relaxed text-gray-500 sm:col-span-2 xl:col-span-3">相对路径会追加到账号的 Base URL。例：Base URL 为 https://api.example.com，路径填 /v1/videos。认证密钥只从账号读取；不要把真实 Key 写入模板。</p>
            <label class="text-sm" title="通常使用 POST 提交生成任务">提交方法<select class="input w-full" :value="field('submit.method')" @change="set('submit.method', value($event))"><option>POST</option><option>PUT</option><option>GET</option></select></label>
            <label class="text-sm" title="从上游文档复制接口路径，保留开头斜杠">提交相对路径<input class="input w-full" placeholder="/v1/videos" :value="field('submit.path')" @change="set('submit.path', value($event))" /></label>
            <label class="text-sm" title="json 发送 JSON；multipart 用于文件上传；form 发送表单">请求编码<select class="input w-full" :value="field('submit.encoding')" @change="set('submit.encoding', value($event))"><option>json</option><option>multipart</option><option>form</option></select></label>
            <label class="text-sm" title="bearer 通常对应 Authorization: Bearer；header/query 用自定义名称">账号认证方式<select class="input w-full" :value="field('auth.mode')" @change="set('auth.mode', value($event))"><option>bearer</option><option>header</option><option>query</option><option>none</option></select></label>
            <label class="text-sm">认证头 / 查询参数名称<input class="input w-full" placeholder="例如 X-API-Key" :value="field('auth.name')" @change="set('auth.name', value($event))" /></label>
            <label class="text-sm">认证前缀<input class="input w-full" placeholder="例如 Bearer " :value="field('auth.prefix')" @change="set('auth.prefix', value($event))" /></label>
            <label class="text-sm" title="异步模式查询任务的间隔秒数">轮询间隔（秒）<input type="number" min="1" max="300" class="input w-full" :value="field('poll_seconds')" @change="set('poll_seconds', Number(value($event)))" /></label>
            <h4 class="border-t pt-4 font-semibold sm:col-span-2 xl:col-span-3 dark:border-dark-600">参数规则</h4>
            <MediaParameterRulesEditor class="sm:col-span-2 xl:col-span-3" :model-value="(field('rules') as Record<string,unknown>[]) || []" @update:model-value="set('rules',$event)" />
            <label class="text-sm" title="上游模型字段路径；填 - 表示只把模型放进 URL 路径">模型写入目标<input class="input w-full" placeholder="model 或 model_id；仅路径填 -" :value="field('model_target')" @change="set('model_target', value($event))" /></label>
            <label class="text-sm" title="上游响应中媒体内容字段的路径，用于提取输出">固定内容字段路径<input class="input w-full" placeholder="data[].url" :value="field('content_path')" @change="set('content_path', value($event))" /></label>
            <details class="border-t pt-4 sm:col-span-2 xl:col-span-3 dark:border-dark-600"><summary class="cursor-pointer font-semibold">素材映射：统一输入如何转换成上游字段</summary><div class="mt-3 space-y-3"><p class="text-xs text-gray-500">例：用户统一输入 images，目标字段填 reference_images；根据接口要求选择 URL、Data URL 或 Base64，并选择字符串或对象结构。</p><MediaMaterialsEditor :model-value="(field('materials') as Record<string, unknown>[]) || []" @update:model-value="set('materials', $event)" /></div></details>
            <details class="border-t pt-4 sm:col-span-2 xl:col-span-3 dark:border-dark-600"><summary class="cursor-pointer font-semibold">任务响应：状态、任务 ID 和成品解析</summary><div class="mt-3 space-y-3"><p class="text-xs text-gray-500">异步接口至少要配置任务 ID、状态和终态映射。例：queued → in_progress，succeeded → completed；成品 URL 路径可写 data[].url。</p><MediaResponseEditor :model-value="(field('response') as Record<string, unknown>) || {}" @update:model-value="set('response', $event)" /></div></details>
            <details class="border-t pt-4 sm:col-span-2 xl:col-span-3 dark:border-dark-600"><summary class="cursor-pointer font-semibold">高级配置：请求体 JSON 和附加路径</summary><div class="mt-3 grid gap-4 sm:grid-cols-2"><label v-for="item in advancedFields" :key="item.path" class="min-w-0 text-sm">{{ item.label }}<span class="ml-2 text-xs text-gray-400">{{ item.path }}</span><p class="mt-1 text-xs text-gray-500">{{ item.hint }}</p><textarea class="input mt-1 min-h-28 w-full font-mono text-xs" :value="JSON.stringify(field(item.path) ?? null, null, 2)" @change="jsonField(item.path, $event)" /></label></div></details>
          </template>
          <template v-else>
            <label class="text-sm">分组 ID<input type="number" class="input w-full" :value="field('group_id')" @change="set('group_id', Number(value($event)))" /></label>
            <label class="text-sm">公开模型（留空为分组默认）<input class="input w-full" :value="field('model')" @change="set('model', value($event))" /></label>
            <label class="text-sm">账号 ID（0 为全部）<input type="number" class="input w-full" :value="field('account_id')" @change="set('account_id', Number(value($event)))" /></label>
            <label class="text-sm">上游模型<input class="input w-full" :value="field('upstream_model')" @change="set('upstream_model', value($event))" /></label>
            <label class="text-sm">协议模板<select class="input w-full" :value="field('template.id')" @change="bindTemplate(value($event))"><option value="">继承账号模板</option><option v-for="row in publishedTemplates" :key="row.id" :value="row.id">{{ row.config.name }}</option></select></label>

            <MediaParameterRulesEditor class="sm:col-span-2" :model-value="(field('rules') as Record<string,unknown>[]) || []" @update:model-value="set('rules',$event)" />
            <p class="text-xs text-gray-500 sm:col-span-2">继承顺序：模板 → 账号模型 → 分组默认 → 分组精确模型 → 分组精确模型与账号。相同选择器冲突时禁止发布。</p>
          </template>
          <p class="text-xs text-gray-500 sm:col-span-2">建议顺序：先填写提交/查询路径，再设素材目标字段，然后用真实响应样例预览解析；保存即可更新当前模板。模板只引用账号密钥，不能填写真实密钥。</p>
        </div>
        <textarea v-if="mode === 'json' || !draft" v-model="json" class="input min-h-[460px] w-full font-mono text-xs" aria-label="完整协议配置 JSON" />
        <div class="sticky bottom-0 z-10 flex flex-wrap items-center gap-3 border-t bg-white/95 py-4 dark:border-dark-600 dark:bg-dark-800">
          <button class="btn btn-primary" :disabled="busy || !draft" @click="action(save, true)">{{ busy ? '处理中…' : '保存模板' }}</button>
          <button v-if="selected" class="btn btn-secondary text-red-600" :disabled="busy" @click="deleting = true">删除模板</button>
          <span class="text-xs text-gray-500">保存后直接生效，已有任务继续使用提交时的配置。</span>
        </div>
        <div v-if="deleting" role="alertdialog" aria-label="确认删除模板" class="space-y-3 rounded-xl border border-red-200 bg-red-50 p-4 dark:bg-red-950">
          <p class="text-sm">确认删除“{{ selected?.config.name || selected?.id }}”？使用此模板的模型将解除绑定并停用，需重新配置后启用。已有任务不受影响。</p>
          <button class="btn btn-secondary" :disabled="busy" @click="deleting = false">取消</button>
          <button class="btn btn-primary ml-2" :disabled="busy" @click="action(remove, true)">确认删除</button>
        </div>
        <details v-if="kind === 'bindings'"><summary>批量发布相同模板</summary><p class="text-xs">每行一个公开模型，默认上游映射为相同 ID。每项仍独立检查账号及冲突；价格保持原设置。</p><textarea v-model="bulkModels" class="input min-h-24 w-full" /><button class="btn btn-primary" :disabled="busy || !draft" @click="action(bulkBind, true)">发布这些模型的绑定</button></details>
        <details>
          <summary>实际绑定、估价与发送摘要（只读）</summary>
          <div class="space-y-3 py-3">
            <p class="text-sm">按已有 API Key 的真实分组与价格解析已发布配置，展示覆盖差异、模板和绑定配置及准备发送的摘要。当前编辑中的草稿不参与；预览不预留额度、不创建任务、不调用生成接口。</p>
            <label class="block">中转 API Key<input v-model="previewKeyID" type="password" autocomplete="off" class="input" aria-label="预览 API Key" /></label>
            <label class="block">限定候选账号 ID（可选）<input v-model="previewAccountID" type="number" min="1" class="input" aria-label="预览候选账号 ID" /></label>
            <label class="block">操作<input v-model="operation" class="input" aria-label="实际绑定预览操作" /></label>
            <label class="block">统一输入<textarea v-model="previewInput" class="input min-h-28 w-full font-mono text-xs" aria-label="实际绑定预览输入" /></label>
            <label class="block text-sm"><input v-model="online" type="checkbox" /> 允许素材 GET 及可信测量；未勾选时需要测量的项目会明确列出，不推测价格。</label>
            <button class="btn btn-secondary" :disabled="busy || !previewKeyID" @click="action(() => preview(true))">预览实际绑定与估价</button>
          </div>
        </details>
        <p v-if="previewSummary" class="rounded border p-3 text-sm dark:border-dark-600" role="status" aria-live="polite">{{ previewSummary }}</p>
        <details v-if="kind === 'templates'"><summary>免费预览、模型发现与付费测试</summary><div class="space-y-3 py-3"><label class="block">操作<input v-model="operation" class="input" /></label><label class="block">上游模型<input v-model="upstreamModel" class="input" /></label><label class="block">统一输入<textarea v-model="previewInput" class="input min-h-28 w-full font-mono text-xs" /></label><label class="block">响应样例<textarea v-model="sample" class="input min-h-20 w-full font-mono text-xs" /></label><label class="block text-sm"><input v-model="online" type="checkbox" aria-label="允许素材 GET" /> 允许为编码转换执行素材 GET（不会调用生成接口）</label><button class="btn btn-secondary" :disabled="busy" @click="action(preview)">免费预览（不建任务、不扣费）</button><p v-if="previewStage !== 'idle'" class="rounded border p-3 text-sm dark:border-dark-600" role="status" aria-live="polite"><strong>{{ ({ preparing: '正在准备免费预览', sending: '正在上传素材并进行免费预览', ready: '免费预览完成', failed: '免费预览未完成' } as Record<string, string>)[previewStage] }}</strong><br>请求约 {{ (previewBytes / 1048576).toFixed(2) }} MiB · 超时上限 {{ Math.round(previewTimeoutMs / 1000) }} 秒。预览只做参数/映射/估价检查，不创建任务、不扣费；超时后可以安全重试。</p><div class="flex flex-wrap gap-2"><input v-model="discoveryAccount" class="input" placeholder="账号 ID" /><input v-model="discoveryGroup" class="input" placeholder="分组 ID" /><button class="btn btn-secondary" :disabled="busy" @click="action(async () => { result = await api.discover({ template: draft, account_id: Number(discoveryAccount), group_id: Number(discoveryGroup) }) })">发现候选模型（不启用）</button></div><p class="text-sm">真实测试使用 API Key 对应的已发布绑定及价格，通过标准媒体提交端点和任务查询链路。草稿不会直接发送；HTTP 202 只表示本站已持久化受理。</p><input v-model="keyID" class="input" type="password" autocomplete="off" placeholder="输入中转 API Key" /><label class="block text-sm"><input v-model="paid" type="checkbox" aria-label="确认本次生成可能收费" /> 确认本次生成可能收费</label><p class="break-all text-xs">请求约 {{ (paidRequestBytes / 1048576).toFixed(2) }} MiB · 幂等 ID：{{ testKey }} · 恢复时重用当前请求和此 ID</p><div class="flex flex-wrap gap-2"><button class="btn btn-primary" :disabled="busy || !paid || !keyID || !!paidTask || paidSubmitState === 'uncertain' || paidSubmitState === 'sending'" @click="action(submitPaidTest, true)">提交真实生成请求</button><button v-if="paidSubmitState === 'uncertain'" class="btn btn-secondary" :disabled="busy" @click="action(recoverPaidTest, true)">恢复 / 查询原任务</button><button v-if="paidTask" class="btn btn-secondary" :disabled="paidTaskLoading" @click="action(() => refreshPaidTask())">{{ paidTaskLoading ? '正在查询…' : '查询原任务' }}</button><button class="btn btn-secondary" :disabled="paidTaskPending || paidSubmitState === 'sending' || paidSubmitState === 'uncertain'" @click="newTest">明确开始另一次测试</button></div><p v-if="paidSubmitState === 'sending'" class="rounded border p-3 text-sm dark:border-dark-600" role="status" aria-live="polite">正在向本站标准媒体接口提交。素材较大时会按请求体大小延长传输超时；任务 ID 返回后再由后台提交上游。</p><p v-if="paidSubmitState === 'uncertain'" class="rounded border p-3 text-sm text-amber-700 dark:border-dark-600" role="status" aria-live="polite">本站受理结果不明确。不要新建请求；恢复操作会重用相同请求体和幂等 ID。</p><p v-if="paidSubmitState === 'rejected'" class="rounded border p-3 text-sm text-red-700 dark:border-dark-600" role="status" aria-live="polite">本站明确拒绝了请求；此状态没有任务 ID。修正输入或绑定后可明确开始新测试。</p><p v-if="paidTask" class="rounded border p-3 text-sm dark:border-dark-600" role="status" aria-live="polite"><strong>{{ paidTaskStage }}</strong><br>任务 ID：{{ paidTask.id }} · 状态：{{ paidTask.status }} · 阶段：{{ paidTask.submission_phase || '历史任务' }}<br>{{ paidTaskBilling }}<br>{{ paidTask.status === 'failed' ? paidTask.diagnostic || '查看失败原因与账单。' : paidTask.status === 'completed' ? '成品已就绪，请查看下方文件并核对账单。' : '本站后台会继续处理；刷新只查询原任务，不会重复提交。' }}</p><div v-if="paidTask?.data?.length" class="space-y-2 rounded border p-3 dark:border-dark-600"><h4 class="font-medium">成品与下载</h4><div v-for="artifact in paidTask.data" :key="artifact.key" class="flex flex-wrap items-center gap-2 text-sm"><span>{{ artifact.key }} · {{ artifact.mime_type }} · {{ artifact.size_bytes ?? '未知' }} B · {{ artifact.available ? '可用' : '处理中或不可用' }}</span><button v-if="artifact.available" class="btn btn-secondary" :disabled="paidArtifactLoading === artifact.key" @click="loadPaidArtifact(artifact)">{{ paidArtifactLoading === artifact.key ? '正在准备…' : paidArtifactAccess[artifact.key] ? '刷新下载链接' : '获取成品链接' }}</button><a v-if="paidArtifactAccess[artifact.key]?.url" class="btn btn-primary" :href="paidArtifactAccess[artifact.key].url" target="_blank" rel="noopener">打开 / 下载成品</a><span v-if="paidArtifactErrors[artifact.key]" class="text-red-600">{{ paidArtifactErrors[artifact.key] }}</span></div></div><p v-if="paidTaskError" role="alert" class="text-sm text-red-600">{{ paidTaskError }}</p><p v-if="paidTask?.diagnostic" class="text-sm text-amber-700">失败诊断：{{ paidTask.diagnostic }}</p></div></details>
        <details v-if="result || paidTaskReview" class="rounded-lg border p-3 dark:border-dark-600"><summary>预览和任务的完整 JSON / 上下游证据（默认折叠）</summary><div class="mt-3 grid gap-3"><pre v-if="result" class="max-h-96 overflow-auto rounded bg-gray-50 p-3 text-xs dark:bg-gray-900">{{ JSON.stringify(result, null, 2) }}</pre><pre v-if="paidTaskReview" class="max-h-96 overflow-auto rounded bg-gray-50 p-3 text-xs dark:bg-gray-900">{{ JSON.stringify({ incoming_request: paidTaskReview.incoming_request, normalized_request: paidTaskReview.normalized_request, prepared_request: paidTaskReview.prepared_request, upstream_request: paidTaskReview.upstream_request, submission: paidTaskReview.submission, upstream_response: paidTaskReview.upstream_response, task: paidTaskReview.record.task }, null, 2) }}</pre></div></details>
      </div>
      <div v-else class="flex min-h-72 items-center justify-center text-sm text-gray-500">从左侧选择协议模板，或新建模板。</div>
    </div>
    <TotpStepUpDialog :controller="stepUp" />
  </section>
</template>
