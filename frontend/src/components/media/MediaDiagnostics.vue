<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { adminAPI } from '@/api/admin'
import { mediaProtocolsAPI, mediaPreviewTimeoutMs } from '@/api/media-protocols'
import { mediaAPI, type MediaArtifact, type MediaTask, type MediaTaskReview } from '@/api/media'
import { accountMediaCapabilities } from '@/types/media'
import type { Account, AccountListItem, AdminGroup } from '@/types'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'

type MaterialKind = 'image' | 'video' | 'audio'
type UploadedMaterial = { name: string; size: number; mime_type: string; b64_json: string }
type PreviewLog = { time: string; stage: string; detail: string }

const route = useRoute()
const stepUp = useStepUp()
const showKey = ref(false)
const group = ref(String(route.query.group_id || '')), key = ref(''), account = ref(String(route.query.account_id || ''))
const groups = ref<AdminGroup[]>([]), accounts = ref<AccountListItem[]>([]), detail = ref<Account | null>(null)
const prompt = ref('A quiet valley'), request = ref('{\n  "model": ""\n}'), operation = ref('video'), model = ref('')
const materialText = ref<Record<MaterialKind, string>>({ image: '', video: '', audio: '' })
const uploaded = ref<Record<MaterialKind, UploadedMaterial[]>>({ image: [], video: [], audio: [] })
const previewResult = ref<unknown>(), previewInput = ref<unknown>(), previewLogs = ref<PreviewLog[]>([])
const effectiveResult = ref<unknown>(), effectiveError = ref(''), submissionLogs = ref<PreviewLog[]>([])
const previewRevision = ref(-1), inputRevision = ref(0)
const paid = ref(false), testKey = ref(crypto.randomUUID()), submissionAttempted = ref(false), submissionInputChanged = ref(false)
const submissionSnapshot = ref<{ api_key: string; operation: string; request: Record<string, unknown> } | null>(null)
const submissionUncertain = ref(false)
const task = ref<MediaTask | null>(null), taskReview = ref<MediaTaskReview | null>(null)
const taskError = ref(''), taskLoading = ref(false), artifactBusy = ref(''), stepUpPending = ref(false)
const artifactAccess = ref<Record<string, { url: string; expires_at: number }>>({})
const artifactErrors = ref<Record<string, string>>({})
const draggingKind = ref<MaterialKind | null>(null)
const previewResultData = computed(() => previewResult.value && typeof previewResult.value === 'object' && !Array.isArray(previewResult.value) ? previewResult.value as Record<string, unknown> : {})
const effectiveResultData = computed(() => effectiveResult.value && typeof effectiveResult.value === 'object' && !Array.isArray(effectiveResult.value) ? effectiveResult.value as Record<string, unknown> : {})
const previewBusy = ref(false), effectiveBusy = ref(false), submitBusy = ref(false)
const loading = ref(false), error = ref(''), previewError = ref(''), submitError = ref(''), scopeError = ref('')
const uploadsPending = ref(0)
const previewStage = ref<'idle' | 'preparing' | 'sending' | 'ready' | 'failed' | 'stale'>('idle')
const previewBytes = ref(0)
const page = ref(1), total = ref(0)
const models = computed(() => accountMediaCapabilities(detail.value?.extra, detail.value?.credentials))
const selectedCapability = computed(() => models.value.find(item => item.model === model.value))
const operations = computed(() => selectedCapability.value?.operations || [])
const materialKinds: MaterialKind[] = ['image', 'video', 'audio']
const materialLabels: Record<MaterialKind, string> = { image: '参考图片', video: '参考视频', audio: '参考音频' }
let generation = 0, listGeneration = 0
let defaultSeconds: number | undefined
let taskGeneration = 0
let taskTimer: ReturnType<typeof setTimeout> | undefined

const taskPending = computed(() => !!task.value && (['queued', 'in_progress', 'unknown'].includes(task.value.status) || ['submitting', 'reconciling'].includes(task.value.submission_phase || '')))
const taskStageLabel = computed(() => {
  if (!task.value) return ''
  if (task.value.status === 'completed') return '生成已完成'
  if (task.value.status === 'failed') return '生成失败'
  if (task.value.status === 'unknown') return '受理结果待核对'
  switch (task.value.submission_phase) {
    case 'local_pending': return '本站已受理，等待后台提交上游'
    case 'submitting': return '本站正在提交上游'
    case 'upstream_accepted': return '上游已受理，正在生成'
    case 'reconciling': return '正在核对上游是否已受理'
    default: return task.value.status === 'queued' ? '本站任务已排队' : '任务处理中'
  }
})
const taskNextStep = computed(() => {
  if (!task.value) return ''
  if (task.value.status === 'completed') return '查看下方成品并核对账单。'
  if (task.value.status === 'failed') return '查看失败原因和账单；预留额度会按任务结算状态处理。'
  if (task.value.status === 'unknown' || ['reconciling', 'submitting'].includes(task.value.submission_phase || '')) return '保留当前测试 ID，刷新查询原任务；不要重新提交生成。'
  return '本站后台会继续处理；可以刷新状态，重复使用当前测试 ID 只会查询原任务。'
})
const taskBillingSummary = computed(() => {
  const billing = task.value?.billing
  if (!billing) return '账单信息暂不可用'
  return `账单 ${billing.state} · 预留 ${billing.reserved} · 已接受 ${billing.accepted} · 已结算 ${billing.settled} · 已释放 ${billing.released}`
})

watch([
  group, account, model, operation, prompt, request, key,
  () => materialText.value.image, () => materialText.value.video, () => materialText.value.audio,
  uploaded
], () => {
  inputRevision.value++
  previewRevision.value = -1
  previewStage.value = 'idle'
  if (submissionAttempted.value) submissionInputChanged.value = true
  else paid.value = false
})

onBeforeUnmount(() => {
  taskGeneration++
  if (taskTimer) clearTimeout(taskTimer)
})

function displayJSON(value: unknown) {
  return JSON.stringify(value, (field, item) => {
    if (typeof item !== 'string') return item
    if (item.startsWith('data:')) {
      const match = item.match(/^data:([^;]+);base64,([\s\S]*)$/)
      return match ? `[${match[1]} Base64 已脱敏，约 ${Math.floor(match[2].length * 3 / 4)} 字节]` : '[Data URL 已脱敏]'
    }
    if (field === 'b64_json' || (field === 'data' && /^[A-Za-z0-9+/\r\n]+={0,2}$/.test(item))) {
      return `[Base64 已脱敏，${item.length} 字符]`
    }
    if (/^https?:\/\//i.test(item)) {
      try {
        const url = new URL(item)
        for (const name of url.searchParams.keys()) url.searchParams.set(name, '[已脱敏]')
        return url.toString()
      } catch { return item }
    }
    return item
  }, 2)
}

function appendPreviewLog(stage: string, detail: unknown) {
  previewLogs.value = [...previewLogs.value, { time: new Date().toLocaleTimeString(), stage, detail: typeof detail === 'string' ? detail : displayJSON(detail) }].slice(-30)
}

function appendSubmissionLog(stage: string, detail: unknown) {
  submissionLogs.value = [...submissionLogs.value, { time: new Date().toLocaleTimeString(), stage, detail: typeof detail === 'string' ? detail : displayJSON(detail) }].slice(-30)
}

function parseMaterialInput(kind: MaterialKind) {
  const text = materialText.value[kind].trim()
  if (!text) return []
  try {
    const parsed = JSON.parse(text)
    return Array.isArray(parsed) ? parsed : [parsed]
  } catch {
    return text.split(/\r?\n/).map(line => line.trim()).filter(Boolean).map(line => {
      if (line.startsWith('{') || line.startsWith('[')) return JSON.parse(line)
      return line
    })
  }
}

function readAsDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(new Error(`读取文件失败：${file.name}`))
    reader.readAsDataURL(file)
  })
}

async function addMaterials(kind: MaterialKind, files: File[], input?: HTMLInputElement) {
  if (!files.length) return
  error.value = ''
  uploadsPending.value++
  try {
    const materialLimit = selectedCapability.value?.materials?.[kind]?.max_file_bytes || 0
    const requestLimit = selectedCapability.value?.limits?.max_request_bytes || 0
    const maxBytes = materialLimit || requestLimit
    const countLimit = selectedCapability.value?.materials?.[kind]?.max_count || 0
    if (countLimit && uploaded.value[kind].length + parseMaterialInput(kind).length + files.length > countLimit) throw new Error(`当前模型最多支持 ${countLimit} 个${materialLabels[kind]}。`)
    for (const file of files) {
      if (file.type && !file.type.startsWith(`${kind}/`)) throw new Error(`${file.name} 不是${materialLabels[kind]}文件。`)
      if (maxBytes > 0 && file.size > maxBytes) throw new Error(`${file.name} 超过当前能力的单文件限制（${maxBytes} 字节）。`)
    }
    const additions: UploadedMaterial[] = []
    for (const file of files) {
      const dataURL = await readAsDataURL(file)
      const match = dataURL.match(/^data:([^;]+);base64,([\s\S]*)$/)
      if (!match) throw new Error(`无法将 ${file.name} 转为 Base64。`)
      additions.push({ name: file.name, size: file.size, mime_type: file.type || match[1], b64_json: match[2] })
      appendPreviewLog('文件已转为 Base64', { kind, file: file.name, mime_type: file.type || match[1], bytes: file.size })
    }
    uploaded.value = { ...uploaded.value, [kind]: [...uploaded.value[kind], ...additions] }
  } catch (cause) {
    error.value = (cause as Error).message
    appendPreviewLog('文件转换失败', error.value)
  } finally {
    if (input) input.value = ''
    uploadsPending.value--
  }
}

async function uploadMaterials(kind: MaterialKind, event: Event) {
  const input = event.target as HTMLInputElement
  await addMaterials(kind, Array.from(input.files || []), input)
}

async function dropMaterials(kind: MaterialKind, event: DragEvent) {
  draggingKind.value = null
  await addMaterials(kind, Array.from(event.dataTransfer?.files || []))
}

function removeUpload(kind: MaterialKind, index: number) {
  uploaded.value = { ...uploaded.value, [kind]: uploaded.value[kind].filter((_, itemIndex) => itemIndex !== index) }
}

async function loadAccount() {
  const version = ++generation
  detail.value = null; previewResult.value = undefined; scopeError.value = ''; loading.value = true
  try {
    if (!account.value) return
    const loaded = await adminAPI.accounts.getById(Number(account.value))
    if (version !== generation) return
    if (loaded.platform !== 'imagevideo' || !loaded.group_ids?.includes(Number(group.value))) throw new Error('所选账号不属于当前媒体分组。')
    detail.value = loaded
    model.value = models.value.find(item => item.enabled)?.model || models.value[0]?.model || ''
    selectModel()
  } catch (cause) { if (version === generation) scopeError.value = (cause as Error).message }
  finally { if (version === generation) loading.value = false }
}

async function loadAccounts(linked?: Account) {
  const version = ++listGeneration
  ++generation; detail.value = null
  loading.value = true; scopeError.value = ''; previewResult.value = undefined
  try {
    const response = await adminAPI.accounts.list(page.value, 50, { platform: 'imagevideo', group: group.value })
    if (version !== listGeneration) return
    accounts.value = response.items; total.value = response.total
    if (linked?.group_ids?.includes(Number(group.value))) account.value = String(linked.id)
    else if (!accounts.value.some(item => String(item.id) === account.value)) account.value = String(accounts.value[0]?.id || '')
    await loadAccount()
  } catch (cause) { if (version === listGeneration) scopeError.value = (cause as Error).message }
  finally { if (version === listGeneration) loading.value = false }
}

async function changeGroup() {
  account.value = ''; detail.value = null; page.value = 1
  await loadAccounts()
}

function selectModel() {
  const capability = models.value.find(item => item.model === model.value)
  if (!capability) return
  if (!capability.operations.includes(operation.value)) operation.value = capability.operations[0] || ''
  let current: Record<string, unknown>
  try { current = JSON.parse(request.value) } catch { return }
  if (defaultSeconds !== undefined && current.seconds === defaultSeconds) delete current.seconds
  defaultSeconds = undefined
  current.model = capability.model
  if (capability.media_type === 'video' && current.seconds === undefined && current.duration === undefined) {
    defaultSeconds = capability.duration.default ?? capability.duration.values?.[0] ?? capability.duration.min ?? 5
    current.seconds = defaultSeconds
  }
  request.value = JSON.stringify(current, null, 2)
}

function buildPreviewRequest() {
  let body: Record<string, unknown>
  try { body = JSON.parse(request.value) } catch { throw new Error('请求参数必须是有效 JSON。') }
  if (!body || Array.isArray(body) || typeof body !== 'object') throw new Error('请求参数必须是 JSON 对象。')
  body.model = model.value
  delete body.prompt
  if (operation.value === 'speech') body.input = prompt.value
  else if (operation.value === 'chat_music') body.messages = [{ role: 'user', content: prompt.value }]
  else body.prompt = prompt.value
  for (const kind of materialKinds) {
    const refs = [...parseMaterialInput(kind), ...uploaded.value[kind].map(({ name: _name, size: _size, ...material }) => material)]
    const limit = selectedCapability.value?.materials?.[kind]?.max_count || 0
    if (limit && refs.length > limit) throw new Error(`当前模型最多支持 ${limit} 个${materialLabels[kind]}。`)
    if (refs.length) body[`${kind}s`] = refs
    else delete body[`${kind}s`]
  }
  return body
}

async function checkBinding() {
  if (effectiveBusy.value || !Number.isSafeInteger(Number(group.value)) || Number(group.value) <= 0) return
  effectiveBusy.value = true
  effectiveError.value = ''
  effectiveResult.value = undefined
  try {
    effectiveResult.value = await mediaProtocolsAPI.effective(Number(group.value))
  } catch (cause) {
    effectiveError.value = (cause as Error).message || String(cause)
  } finally {
    effectiveBusy.value = false
  }
}

async function runPreview() {
  if (uploadsPending.value || previewBusy.value) return
  previewBusy.value = true
  previewError.value = ''
  previewResult.value = undefined
  previewInput.value = undefined
  previewLogs.value = []
  previewRevision.value = -1
  previewBytes.value = 0
  previewStage.value = 'preparing'
  try {
    if (!Number.isSafeInteger(Number(group.value)) || Number(group.value) <= 0) throw new Error('请选择分组。')
    if (!key.value.trim()) throw new Error('请输入当前分组的中转 API Key。')
    if (!detail.value || !detail.value.group_ids?.includes(Number(group.value))) throw new Error('请选择当前分组的账号。')
    const requestBody = buildPreviewRequest()
    const revision = inputRevision.value
    previewInput.value = requestBody
    const previewPayload = { mode: 'effective', api_key: key.value.trim(), group_id: Number(group.value), account_id: detail.value.id, operation: operation.value, request: requestBody, measure_media: false }
    previewBytes.value = new TextEncoder().encode(JSON.stringify(previewPayload)).byteLength
    const timeoutMs = mediaPreviewTimeoutMs(previewPayload)
    previewStage.value = 'sending'
    appendPreviewLog('发送免费预览', { request_bytes: previewBytes.value, timeout_ms: timeoutMs, generation: false, billing: false })
    appendPreviewLog('输入请求（编码素材已脱敏）', requestBody)
    previewResult.value = await mediaProtocolsAPI.preview(previewPayload)
    if (revision === inputRevision.value) {
      previewRevision.value = revision
      previewStage.value = 'ready'
    } else previewStage.value = 'stale'
    appendPreviewLog('检查完成', { identity: 'Key 与分组匹配', binding: '已解析当前模板及账号', request_mapping: '通过', pricing: previewResultData.value.pricing, generation: '没有提交生成请求' })
    appendPreviewLog('预览输出', previewResult.value)
  } catch (cause) {
    previewStage.value = 'failed'
    const failure = cause as { message?: string; code?: string; response?: unknown }
    const message = failure.message || String(cause)
    const timeout = ['ECONNABORTED', 'ETIMEDOUT'].includes(failure.code || '') || /timeout|timed out/i.test(message)
    const disconnected = !failure.response && /network error|failed to fetch|network request failed/i.test(message)
    previewError.value = timeout || disconnected
      ? `免费预览${timeout ? '超时' : '连接失败'}。大素材上传或网络延迟可能导致请求未完成；本次没有创建生成任务、没有扣费，可以安全重试。`
      : `免费预览失败：${message}。本次没有创建生成任务、没有扣费；修正输入后可以重新预览。`
    appendPreviewLog('预览错误', previewError.value)
  } finally { previewBusy.value = false }
}

function newTest() {
  if (taskTimer) clearTimeout(taskTimer)
  taskGeneration++
  testKey.value = crypto.randomUUID()
  paid.value = false
  submissionAttempted.value = false
  submissionInputChanged.value = false
  submissionSnapshot.value = null
  submissionUncertain.value = false
  submitError.value = ''
  submissionLogs.value = []
  task.value = null
  taskReview.value = null
  taskError.value = ''
  previewStage.value = 'idle'
  artifactAccess.value = {}
  artifactErrors.value = {}
}

function scheduleTaskRefresh(id: string) {
  if (taskTimer) clearTimeout(taskTimer)
  if (!taskPending.value || task.value?.id !== id) return
  taskTimer = setTimeout(() => { void refreshTask(id) }, 2500)
}

async function loadArtifactAccess(id: string, artifacts: MediaArtifact[]) {
  await Promise.all(artifacts.filter(item => item.available).map(async artifact => {
    try {
      const access = await mediaAPI.artifactAccess(id, artifact.key, true)
      if (task.value?.id === id) artifactAccess.value = { ...artifactAccess.value, [artifact.key]: access }
    } catch (cause) {
      if (task.value?.id === id) artifactErrors.value = { ...artifactErrors.value, [artifact.key]: (cause as Error).message || String(cause) }
    }
  }))
}

async function refreshTask(id = task.value?.id) {
  if (!id) return
  const version = taskGeneration
  taskLoading.value = true
  taskError.value = ''
  try {
    const review = await mediaAPI.detail(id)
    if (version !== taskGeneration || task.value?.id !== id) return
    taskReview.value = review
    const latest = review.record.task
    task.value = latest
    if (['completed', 'failed'].includes(latest.status)) await loadArtifactAccess(id, latest.data || [])
  } catch (cause) {
    if (version === taskGeneration) taskError.value = (cause as Error).message || String(cause)
  } finally {
    if (version === taskGeneration) {
      taskLoading.value = false
      scheduleTaskRefresh(id)
    }
  }
}

async function previewArtifact(artifact: MediaArtifact) {
  if (!task.value) return
  artifactBusy.value = artifact.key
  artifactErrors.value = { ...artifactErrors.value, [artifact.key]: '' }
  try {
    const access = await mediaAPI.artifactAccess(task.value.id, artifact.key, true)
    artifactAccess.value = { ...artifactAccess.value, [artifact.key]: access }
  } catch (cause) {
    artifactErrors.value = { ...artifactErrors.value, [artifact.key]: (cause as Error).message || String(cause) }
  } finally { artifactBusy.value = '' }
}

async function submitTest() {
  const recovering = submissionAttempted.value && !!submissionSnapshot.value
  if (!recovering) {
    if (uploadsPending.value) return
    if (!paid.value) throw new Error('请先确认这次真实生成可能收费。')
    if (!key.value.trim()) throw new Error('请输入当前分组的中转 API Key。')
    if (!detail.value || !detail.value.group_ids?.includes(Number(group.value))) throw new Error('请选择当前分组的账号。')
  }

  const revision = inputRevision.value
  if (!recovering) {
    const requestBody = buildPreviewRequest()
    submissionSnapshot.value = { api_key: key.value.trim(), operation: operation.value, request: requestBody }
    submissionAttempted.value = true
    submissionInputChanged.value = false
  }
  const payload = submissionSnapshot.value
  if (!payload) throw new Error('原测试请求不可恢复，请开始新测试。')
  const version = taskGeneration
  submitBusy.value = true
  submitError.value = ''
  taskError.value = ''
  submissionUncertain.value = true
  try {
    appendSubmissionLog(recovering ? '使用原请求和幂等 ID 恢复测试' : '提交实际生成请求', { operation: payload.operation, model: payload.request.model, idempotency_key: testKey.value })
    const submitted = await mediaProtocolsAPI.test({ ...payload, confirm_paid_submission: true }, testKey.value) as MediaTask
    if (!submitted?.id) throw new Error('提交已返回，但响应中没有任务 ID；保留当前幂等 ID 后可安全重试恢复。')
    if (version !== taskGeneration) return
    submissionUncertain.value = false
    task.value = submitted
    taskReview.value = null
    artifactAccess.value = {}
    artifactErrors.value = {}
    appendSubmissionLog('任务已受理', { id: submitted.id, status: submitted.status, submission_phase: submitted.submission_phase })
    if (!recovering && revision !== inputRevision.value) {
      submissionInputChanged.value = true
      submitError.value = '提交期间输入发生变化。任务按提交时的原请求执行；恢复操作仍使用原请求和幂等 ID。'
      appendSubmissionLog('提交期间输入发生变化', { task_id: submitted.id, recovery: '使用已保存的原请求和幂等 ID' })
    }
    await refreshTask(submitted.id)
  } catch (cause) {
    if (version !== taskGeneration) return
    const failure = cause as { message?: string; response?: { status?: number } }
    const status = failure.response?.status
    submissionUncertain.value = !status || status >= 500
    submitError.value = failure.message || String(cause)
    appendSubmissionLog('提交或恢复结果不明确', { error: submitError.value, recovery: '保留原请求和幂等 ID；使用同一按钮恢复，勿重新生成。', uncertain_acceptance: submissionUncertain.value })
  } finally { if (version === taskGeneration) submitBusy.value = false }
}

async function runPaidTest() {
  if (stepUpPending.value || submitBusy.value) return
  stepUpPending.value = true
  try {
    await stepUp.run(submitTest)
  } catch (cause) {
    if (!isStepUpCancelled(cause)) submitError.value = (cause as Error).message || String(cause)
  } finally { stepUpPending.value = false }
}

onMounted(async () => {
  loading.value = true
  try {
    groups.value = (await adminAPI.groups.getAllIncludingInactive()).filter(item => ['imagevideo', 'composite'].includes(item.platform))
    let linked: Account | undefined
    if (account.value) {
      linked = await adminAPI.accounts.getById(Number(account.value))
      if (linked.platform !== 'imagevideo') throw new Error('请选择图片音频视频账号。')
      if (!group.value) group.value = String(linked.group_ids?.[0] || '')
    }
    if (!group.value) group.value = String(groups.value[0]?.id || '')
    if (group.value) await loadAccounts(linked)
  } catch (cause) { scopeError.value = (cause as Error).message }
  finally { loading.value = false }
})
</script>

<template>
  <section class="space-y-5 rounded-lg border bg-white p-5 dark:border-dark-600 dark:bg-dark-800">
    <header><h2 class="text-lg font-semibold">联调检测</h2><p class="mt-1 text-sm text-gray-500">可独立运行免费预估或直接提交真实请求；预估不会创建任务或扣费。</p></header>
    <div class="grid gap-4 sm:grid-cols-2">
      <label class="text-sm">分组<select v-model="group" class="input mt-1 block w-full" aria-label="联调分组" @change="changeGroup"><option value="">请选择分组</option><option v-for="item in groups" :key="item.id" :value="String(item.id)">{{ item.name }}</option></select></label>
      <label class="text-sm">账号<select v-model="account" class="input mt-1 block w-full" aria-label="联调账号" :disabled="loading" @change="loadAccount"><option value="">{{ loading ? '正在加载…' : '请选择账号' }}</option><option v-if="detail && !accounts.some(item => item.id === detail?.id)" :value="String(detail.id)">{{ detail.name }} · #{{ detail.id }}</option><option v-for="item in accounts" :key="item.id" :value="String(item.id)">{{ item.name }} · #{{ item.id }}</option></select></label>
    </div>
    <div class="flex flex-wrap items-center gap-2 text-sm"><span>{{ total }} 个账号</span><button type="button" class="btn btn-secondary" :disabled="loading || page <= 1" @click="page--; loadAccounts()">上一页</button><button type="button" class="btn btn-secondary" :disabled="loading || page * 50 >= total" @click="page++; loadAccounts()">下一页</button><button type="button" class="btn btn-secondary" :disabled="effectiveBusy || !group" @click="checkBinding">{{ effectiveBusy ? '检查中…' : '检查实际绑定' }}</button></div>
    <p v-if="scopeError" role="alert" class="text-red-600">{{ scopeError }} <button type="button" class="underline" @click="loadAccounts()">重试</button></p>
    <p v-else-if="!loading && !accounts.length && !detail" class="rounded border border-dashed p-4 text-sm text-gray-500">该分组没有媒体账号。</p>
    <div class="grid gap-4 sm:grid-cols-3">
      <label class="text-sm">模型<select v-model="model" class="input mt-1 block w-full" aria-label="联调模型" @change="selectModel"><option value="">请选择模型</option><option v-for="item in models" :key="item.model" :value="item.model">{{ item.model }}{{ item.enabled ? '' : '（未启用）' }}</option></select></label>
      <label class="text-sm">请求操作<select v-model="operation" class="input mt-1 block w-full" aria-label="联调操作"><option v-for="op in operations" :key="op">{{ op }}</option></select></label>
      <label class="text-sm">中转 API Key<div class="mt-1 flex gap-2"><input v-model="key" :type="showKey ? 'text' : 'password'" autocomplete="off" spellcheck="false" placeholder="粘贴中转密钥，如 sk-…" class="input block min-w-0 flex-1" aria-label="联调 API Key" /><button type="button" class="btn btn-secondary" @click="showKey = !showKey">{{ showKey ? '隐藏' : '显示' }}</button></div><RouterLink to="/keys" class="mt-1 block text-xs text-primary-600">创建或查看 API 密钥</RouterLink></label>
    </div>
    <p class="text-xs text-gray-500">API Key 必须属于当前分组。免费预览不会创建任务；实际提交按该 Key 的已发布绑定和价格走正常任务、上游及计费链路，可能产生费用。素材 URL 不会被读取；本地上传文件会转为 Base64，页面参数和日志中的编码内容会脱敏。</p>
    <details v-if="effectiveResult || effectiveError" class="rounded-lg border p-3 dark:border-dark-600">
      <summary class="cursor-pointer text-sm font-medium">当前绑定检查（默认折叠）</summary>
      <p v-if="effectiveError" role="alert" class="mt-3 text-sm text-red-600">{{ effectiveError }}</p>
      <pre v-if="effectiveResult" class="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-3 text-xs dark:bg-dark-900">{{ displayJSON(effectiveResultData) }}</pre>
    </details>
    <label class="block text-sm">提示词<textarea v-model="prompt" class="input mt-1 min-h-24 w-full" aria-label="联调提示词" placeholder="单独输入提示词、编辑要求或语音文本" /></label>
    <label class="block text-sm">请求参数 JSON（不含提示词和参考素材）<textarea v-model="request" class="input mt-1 min-h-28 w-full font-mono text-xs" aria-label="联调请求选项" /></label>
    <div class="grid gap-4 lg:grid-cols-3">
      <fieldset v-for="kind in materialKinds" :key="kind" :aria-label="`${materialLabels[kind]}拖放区域`" class="min-w-0 space-y-3 rounded-lg border bg-gray-50/60 p-4 transition-colors dark:border-dark-600 dark:bg-dark-900/40" :class="draggingKind === kind ? 'border-primary-500 bg-primary-50/60 dark:bg-primary-950/20' : ''" @dragenter.prevent="draggingKind = kind" @dragover.prevent @drop.prevent="dropMaterials(kind, $event)">
        <legend class="text-sm font-medium">{{ materialLabels[kind] }} <span class="text-xs font-normal text-gray-500">{{ uploaded[kind].length }} 个文件</span></legend>
        <label class="block text-xs">URL、Data URL 或 JSON 素材对象<textarea v-model="materialText[kind]" class="input mt-1 min-h-24 w-full font-mono text-xs" :aria-label="`${materialLabels[kind]}输入`" placeholder="每行一个 URL；或 JSON，如 {&quot;b64_json&quot;:&quot;...&quot;,&quot;mime_type&quot;:&quot;image/png&quot;}" /></label>
        <label class="block text-xs">添加文件（支持多选、可分批添加）<input class="input mt-1 w-full file:mr-3 file:rounded-md file:border-0 file:bg-primary-50 file:px-3 file:py-2 file:text-primary-700" type="file" multiple :accept="`${kind}/*`" :aria-label="`上传${materialLabels[kind]}`" @change="uploadMaterials(kind, $event)" /></label>
        <p class="text-xs text-gray-500">也可将{{ materialLabels[kind] }}文件拖到此区域。</p>
        <ul v-if="uploaded[kind].length" class="space-y-1 text-xs">
          <li v-for="(file, index) in uploaded[kind]" :key="`${file.name}-${index}`" class="flex items-center justify-between gap-2 rounded-lg border bg-white p-2 dark:border-dark-600 dark:bg-dark-800">
            <span class="min-w-0 truncate">{{ file.name }} · {{ file.mime_type }} · {{ file.size }} B</span>
            <button type="button" class="btn btn-secondary shrink-0" :aria-label="`移除${materialLabels[kind]} ${file.name}`" @click="removeUpload(kind, index)">移除</button>
          </li>
        </ul>
        <p v-if="selectedCapability?.materials?.[kind]?.max_count" class="text-xs text-gray-500">能力目录最多 {{ selectedCapability.materials[kind].max_count }} 项。</p>
      </fieldset>
    </div>
    <div class="flex flex-wrap items-center gap-3 border-t pt-4 dark:border-dark-600">
      <button type="button" class="btn btn-secondary" :disabled="previewBusy || loading || uploadsPending > 0 || !detail || !model || !operation" @click="runPreview">{{ uploadsPending ? '正在读取素材…' : previewBusy ? '预估处理中…' : '免费预览映射与估价' }}</button>
      <label class="flex items-center gap-2 text-sm"><input v-model="paid" type="checkbox" aria-label="确认真实生成可能收费" class="accent-primary-600" />我确认真实生成可能收费</label>
      <button type="button" class="btn btn-primary" :disabled="submitBusy || stepUpPending || (!submissionAttempted && (loading || uploadsPending > 0 || !detail || !model || !operation || !key || !paid))" @click="runPaidTest">{{ stepUpPending ? '等待安全验证…' : submitBusy ? '提交中…' : submissionAttempted ? '恢复 / 查询原测试' : '提交真实生成请求' }}</button>
      <button v-if="submissionAttempted" type="button" class="btn btn-secondary" :disabled="submitBusy || stepUpPending || taskPending || (submissionUncertain && !task)" @click="newTest">开始新测试</button>
      <span v-if="submissionAttempted" class="text-xs text-gray-500">测试 ID：{{ testKey }} · 重试只恢复原任务</span>
    </div>
    <div v-if="previewStage !== 'idle'" class="rounded-lg border p-3 text-sm dark:border-dark-600" :class="previewStage === 'failed' ? 'border-red-200 bg-red-50 text-red-700 dark:bg-red-950/30 dark:text-red-300' : 'bg-gray-50 text-gray-700 dark:bg-dark-900 dark:text-gray-300'" role="status" aria-live="polite">
      <strong>{{ ({ preparing: '正在准备免费预览', sending: '已发送免费预览，正在校验映射与估价', ready: '免费预览完成', failed: '免费预览未完成', stale: '预览完成，但输入已变化' } as Record<string, string>)[previewStage] }}</strong>
      <span v-if="previewBytes"> · 请求约 {{ (previewBytes / 1048576).toFixed(2) }} MiB</span>
      <span v-if="previewStage === 'sending'"> · 此阶段不会创建生成任务或扣费</span>
      <span v-else-if="previewStage === 'failed'"> · 没有提交生成请求，可安全重试</span>
    </div>
    <p v-if="submissionInputChanged" role="status" class="text-sm text-amber-700 dark:text-amber-400">当前参数与原测试不同。恢复操作会继续使用原请求和幂等 ID；任务结束后再开始新测试提交新参数。</p>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
    <p v-if="previewError" role="alert" class="text-red-600">{{ previewError }}</p>
    <p v-if="submitError" role="alert" class="text-red-600">{{ submitError }}</p>
    <p v-if="submissionUncertain && !task" role="status" class="text-sm text-amber-700 dark:text-amber-400">提交结果尚未确认。保留当前测试 ID 并恢复原请求；确认前不能开始新测试。</p>
    <details v-if="previewInput || previewResult || previewLogs.length" class="rounded-lg border p-3 dark:border-dark-600">
      <summary class="cursor-pointer text-sm font-medium">免费预览证据 · 输入、映射、估价与日志（默认折叠）</summary>
      <div class="mt-3 grid gap-4 xl:grid-cols-2">
        <section v-if="previewInput" class="min-w-0 border-t pt-3 dark:border-dark-600"><h3 class="mb-2 text-sm font-semibold">输入请求 · 编码内容已脱敏</h3><pre class="max-h-[480px] overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-3 text-xs dark:bg-dark-900">{{ displayJSON(previewInput) }}</pre></section>
        <section v-if="previewResult" class="min-w-0 border-t pt-3 dark:border-dark-600"><h3 class="mb-2 text-sm font-semibold">规范化参数与实际请求映射</h3><pre class="max-h-[480px] overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-3 text-xs dark:bg-dark-900">{{ displayJSON({ normalized: previewResultData.normalized, upstream_request: previewResultData.request, effective: previewResultData.effective, inputs: previewResultData.inputs }) }}</pre></section>
        <section v-if="previewResult" class="min-w-0 border-t pt-3 dark:border-dark-600"><h3 class="mb-2 text-sm font-semibold">估价、匹配规则与预览状态</h3><pre class="max-h-[480px] overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-3 text-xs dark:bg-dark-900">{{ displayJSON({ pricing: previewResultData.pricing, rule_provenance: previewResultData.rule_provenance, override_diff: previewResultData.override_diff, request_complete: previewResultData.request_complete, measurement_required: previewResultData.measurement_required, paid_submission: previewResultData.paid_submission }) }}</pre></section>
        <section v-if="previewLogs.length" class="min-w-0 border-t pt-3 dark:border-dark-600"><h3 class="mb-2 text-sm font-semibold">预估流程</h3><ol class="max-h-[480px] space-y-3 overflow-auto"> <li v-for="(entry, index) in previewLogs" :key="index" class="border-l-2 border-primary-500 pl-3"><div class="flex gap-2 text-xs"><time class="text-gray-500">{{ entry.time }}</time><strong>{{ entry.stage }}</strong></div><pre class="mt-1 whitespace-pre-wrap break-all text-xs">{{ entry.detail }}</pre></li></ol></section>
      </div>
    </details>
    <section v-if="task" class="space-y-4 border-t pt-4 dark:border-dark-600" aria-label="真实请求测试结果">
      <header class="flex flex-wrap items-center justify-between gap-3">
        <div><h3 class="text-base font-semibold">真实请求结果</h3><p class="mt-1 break-all text-xs text-gray-500">任务 ID：{{ task.id }} · {{ task.model }}</p></div>
        <button type="button" class="btn btn-secondary" :disabled="taskLoading" @click="refreshTask()">{{ taskLoading ? '正在刷新…' : '刷新任务' }}</button>
      </header>
      <div class="rounded-lg border p-3 dark:border-dark-600" role="status" aria-live="polite">
        <p class="font-semibold text-gray-900 dark:text-gray-100">{{ taskStageLabel }} <span v-if="task.progress !== undefined">· {{ task.progress }}%</span></p>
        <p class="mt-1 text-xs">状态：{{ task.status }} · 阶段：{{ task.submission_phase || '历史任务' }}</p>
        <p class="mt-1 text-xs">{{ taskBillingSummary }}</p>
        <p class="mt-1 text-xs">{{ taskNextStep }}</p>
      </div>
      <p v-if="taskError" role="alert" class="text-sm text-red-600">读取任务详情失败：{{ taskError }}</p>
      <p v-if="task.diagnostic" class="text-sm text-amber-700 dark:text-amber-400">{{ task.diagnostic }}</p>
      <details class="rounded-lg border p-3 dark:border-dark-600">
        <summary class="cursor-pointer text-sm font-medium">上下游请求、响应与诊断证据（默认折叠）</summary>
        <div class="mt-3 grid gap-4 xl:grid-cols-2">
          <section class="min-w-0 space-y-2 border-t pt-3 dark:border-dark-600">
            <h4 class="text-sm font-semibold">请求参数与上游映射</h4>
            <pre class="max-h-[480px] overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-3 text-xs dark:bg-dark-900">{{ displayJSON({ incoming_request: taskReview?.incoming_request, normalized_request: taskReview?.normalized_request, prepared_request: taskReview?.prepared_request, upstream_request: taskReview?.upstream_request, submission_status: taskReview?.submission_status }) }}</pre>
          </section>
          <section class="min-w-0 space-y-2 border-t pt-3 dark:border-dark-600">
            <h4 class="text-sm font-semibold">任务响应与上游结果</h4>
            <pre class="max-h-[480px] overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-3 text-xs dark:bg-dark-900">{{ displayJSON({ task: taskReview?.record.task || task, submission_response: taskReview?.submission?.response, upstream_response: taskReview?.upstream_response || taskReview?.response }) }}</pre>
          </section>
          <section v-if="submissionLogs.length" class="min-w-0 border-t pt-3 dark:border-dark-600"><h4 class="mb-2 text-sm font-semibold">提交与恢复流程</h4><ol class="max-h-[480px] space-y-3 overflow-auto"><li v-for="(entry, index) in submissionLogs" :key="index" class="border-l-2 border-primary-500 pl-3"><div class="flex gap-2 text-xs"><time class="text-gray-500">{{ entry.time }}</time><strong>{{ entry.stage }}</strong></div><pre class="mt-1 whitespace-pre-wrap break-all text-xs">{{ entry.detail }}</pre></li></ol></section>
        </div>
      </details>
      <section v-if="(taskReview?.record.task.data || task.data || []).length" class="space-y-3 border-t pt-3 dark:border-dark-600">
        <h4 class="text-sm font-semibold">生成成品</h4>
        <article v-for="artifact in (taskReview?.record.task.data || task.data || [])" :key="artifact.key" class="min-w-0 space-y-2 border-b pb-3 last:border-0 dark:border-dark-600">
          <div class="flex flex-wrap items-center gap-3 text-xs"><span class="font-medium">{{ artifact.key }}</span><span>{{ artifact.type }} · {{ artifact.mime_type }} · {{ artifact.size_bytes ?? '大小未知' }} B</span><span v-if="artifact.duration_seconds">{{ artifact.duration_seconds }} 秒</span><button type="button" class="btn btn-secondary" :disabled="!artifact.available || artifactBusy === artifact.key" @click="previewArtifact(artifact)">{{ artifactBusy === artifact.key ? '获取中…' : artifactAccess[artifact.key] ? '更新链接' : '加载成品' }}</button></div>
          <p v-if="artifactErrors[artifact.key]" role="alert" class="text-xs text-red-600">{{ artifactErrors[artifact.key] }}</p>
          <template v-if="artifactAccess[artifact.key]">
            <img v-if="artifact.type === 'image'" :src="artifactAccess[artifact.key]!.url" :alt="artifact.key" referrerpolicy="no-referrer" class="max-h-96 max-w-full rounded border object-contain dark:border-dark-600" />
            <video v-else-if="artifact.type === 'video'" :src="artifactAccess[artifact.key]!.url" controls preload="metadata" class="max-h-96 w-full" />
            <audio v-else-if="artifact.type === 'audio'" :src="artifactAccess[artifact.key]!.url" controls preload="metadata" />
            <a :href="`${artifactAccess[artifact.key]!.url}${artifactAccess[artifact.key]!.url.includes('?') ? '&' : '?'}download=1`" class="btn btn-secondary inline-flex" referrerpolicy="no-referrer">下载原文件</a>
          </template>
        </article>
      </section>
    </section>
    <TotpStepUpDialog :controller="stepUp" />
  </section>
</template>
