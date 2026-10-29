<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { adminAPI } from '@/api/admin'
import { mediaProtocolsAPI } from '@/api/media-protocols'
import { accountMediaCapabilities } from '@/types/media'
import type { Account, AccountListItem, AdminGroup } from '@/types'

type MaterialKind = 'image' | 'video' | 'audio'
type UploadedMaterial = { name: string; size: number; mime_type: string; b64_json: string }
type PreviewLog = { time: string; stage: string; detail: string }

const route = useRoute()
const showKey = ref(false)
const group = ref(String(route.query.group_id || '')), key = ref(''), account = ref(String(route.query.account_id || ''))
const groups = ref<AdminGroup[]>([]), accounts = ref<AccountListItem[]>([]), detail = ref<Account | null>(null)
const prompt = ref('A quiet valley'), request = ref('{\n  "model": ""\n}'), operation = ref('video'), model = ref('')
const materialText = ref<Record<MaterialKind, string>>({ image: '', video: '', audio: '' })
const uploaded = ref<Record<MaterialKind, UploadedMaterial[]>>({ image: [], video: [], audio: [] })
const result = ref<unknown>(), previewInput = ref<unknown>(), logs = ref<PreviewLog[]>([])
const resultData = computed(() => result.value && typeof result.value === 'object' && !Array.isArray(result.value) ? result.value as Record<string, unknown> : {})
const busy = ref(false), loading = ref(false), error = ref(''), scopeError = ref('')
const uploadsPending = ref(0)
const page = ref(1), total = ref(0)
const models = computed(() => accountMediaCapabilities(detail.value?.extra, detail.value?.credentials))
const selectedCapability = computed(() => models.value.find(item => item.model === model.value))
const operations = computed(() => selectedCapability.value?.operations || [])
const materialKinds: MaterialKind[] = ['image', 'video', 'audio']
const materialLabels: Record<MaterialKind, string> = { image: '参考图片', video: '参考视频', audio: '参考音频' }
let generation = 0, listGeneration = 0
let defaultSeconds: number | undefined

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

function appendLog(stage: string, detail: unknown) {
  logs.value = [...logs.value, { time: new Date().toLocaleTimeString(), stage, detail: typeof detail === 'string' ? detail : displayJSON(detail) }].slice(-30)
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

async function uploadMaterials(kind: MaterialKind, event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
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
      const dataURL = await readAsDataURL(file)
      const match = dataURL.match(/^data:([^;]+);base64,([\s\S]*)$/)
      if (!match) throw new Error(`无法将 ${file.name} 转为 Base64。`)
      uploaded.value = {
        ...uploaded.value,
        [kind]: [...uploaded.value[kind], { name: file.name, size: file.size, mime_type: file.type || match[1], b64_json: match[2] }]
      }
      appendLog('文件已转为 Base64', { kind, file: file.name, mime_type: file.type || match[1], bytes: file.size })
    }
  } catch (cause) {
    error.value = (cause as Error).message
    appendLog('文件转换失败', error.value)
  } finally {
    input.value = ''
    uploadsPending.value--
  }
}

function removeUpload(kind: MaterialKind, index: number) {
  uploaded.value = { ...uploaded.value, [kind]: uploaded.value[kind].filter((_, itemIndex) => itemIndex !== index) }
}

async function loadAccount() {
  const version = ++generation
  detail.value = null; result.value = undefined; scopeError.value = ''; loading.value = true
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
  loading.value = true; scopeError.value = ''; result.value = undefined
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

async function run(effective: boolean) {
  if (uploadsPending.value) return
  busy.value = true; error.value = ''; result.value = undefined; logs.value = []
  try {
    if (!Number.isSafeInteger(Number(group.value)) || Number(group.value) <= 0) throw new Error('请选择分组。')
    if (effective) {
      appendLog('检查实际绑定', { group_id: Number(group.value) })
      result.value = await mediaProtocolsAPI.effective(Number(group.value))
    } else {
      if (!key.value.trim()) throw new Error('请输入当前分组的中转 API Key。')
      if (!detail.value || !detail.value.group_ids?.includes(Number(group.value))) throw new Error('请选择当前分组的账号。')
      const requestBody = buildPreviewRequest()
      previewInput.value = requestBody
      appendLog('输入请求（编码素材已脱敏）', requestBody)
      result.value = await mediaProtocolsAPI.preview({ mode: 'effective', api_key: key.value.trim(), group_id: Number(group.value), account_id: detail.value.id, operation: operation.value, request: requestBody, measure_media: false })
    }
    appendLog('检查完成', { identity: 'Key 与分组匹配', binding: '已解析当前模板及账号', request_mapping: '通过', pricing: resultData.value.pricing, upstream_generation: '预览未提交生成请求' })
    appendLog('预览输出', result.value)
  } catch (cause) {
    error.value = (cause as Error).message
    appendLog('预览错误', error.value)
  } finally { busy.value = false }
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
    <header><h2 class="text-lg font-semibold">联调检测</h2><p class="mt-1 text-sm text-gray-500">预览实际模板、请求映射和渠道估价；此页面不创建任务、不提交上游生成请求。</p></header>
    <div class="grid gap-4 sm:grid-cols-2">
      <label class="text-sm">分组<select v-model="group" class="input mt-1 block w-full" aria-label="联调分组" @change="changeGroup"><option value="">请选择分组</option><option v-for="item in groups" :key="item.id" :value="String(item.id)">{{ item.name }}</option></select></label>
      <label class="text-sm">账号<select v-model="account" class="input mt-1 block w-full" aria-label="联调账号" :disabled="loading" @change="loadAccount"><option value="">{{ loading ? '正在加载…' : '请选择账号' }}</option><option v-if="detail && !accounts.some(item => item.id === detail?.id)" :value="String(detail.id)">{{ detail.name }} · #{{ detail.id }}</option><option v-for="item in accounts" :key="item.id" :value="String(item.id)">{{ item.name }} · #{{ item.id }}</option></select></label>
    </div>
    <div class="flex flex-wrap items-center gap-2 text-sm"><span>{{ total }} 个账号</span><button type="button" class="btn btn-secondary" :disabled="loading || page <= 1" @click="page--; loadAccounts()">上一页</button><button type="button" class="btn btn-secondary" :disabled="loading || page * 50 >= total" @click="page++; loadAccounts()">下一页</button><button type="button" class="btn btn-secondary" :disabled="busy || !group" @click="run(true)">检查实际绑定</button></div>
    <p v-if="scopeError" role="alert" class="text-red-600">{{ scopeError }} <button type="button" class="underline" @click="loadAccounts()">重试</button></p>
    <p v-else-if="!loading && !accounts.length && !detail" class="rounded border border-dashed p-4 text-sm text-gray-500">该分组没有媒体账号。</p>
    <div class="grid gap-4 sm:grid-cols-3">
      <label class="text-sm">模型<select v-model="model" class="input mt-1 block w-full" aria-label="联调模型" @change="selectModel"><option value="">请选择模型</option><option v-for="item in models" :key="item.model" :value="item.model">{{ item.model }}{{ item.enabled ? '' : '（未启用）' }}</option></select></label>
      <label class="text-sm">请求操作<select v-model="operation" class="input mt-1 block w-full" aria-label="联调操作"><option v-for="op in operations" :key="op">{{ op }}</option></select></label>
      <label class="text-sm">中转 API Key<div class="mt-1 flex gap-2"><input v-model="key" :type="showKey ? 'text' : 'password'" autocomplete="off" spellcheck="false" placeholder="粘贴中转密钥，如 sk-…" class="input block min-w-0 flex-1" aria-label="联调 API Key" /><button type="button" class="btn btn-secondary" @click="showKey = !showKey">{{ showKey ? '隐藏' : '显示' }}</button></div><RouterLink to="/keys" class="mt-1 block text-xs text-primary-600">创建或查看 API 密钥</RouterLink></label>
    </div>
    <p class="text-xs text-gray-500">API Key 必须属于当前分组。预览不会创建任务、扣费或调用上游生成接口。素材 URL 不会被读取；上传文件在本页转为 Base64，发送到本站预览接口校验，输入输出和日志中的编码内容会脱敏。</p>
    <label class="block text-sm">提示词<textarea v-model="prompt" class="input mt-1 min-h-24 w-full" aria-label="联调提示词" placeholder="单独输入提示词、编辑要求或语音文本" /></label>
    <label class="block text-sm">请求参数 JSON（不含提示词和参考素材）<textarea v-model="request" class="input mt-1 min-h-28 w-full font-mono text-xs" aria-label="联调请求选项" /></label>
    <div class="grid gap-4 lg:grid-cols-3">
      <fieldset v-for="kind in materialKinds" :key="kind" class="min-w-0 space-y-3 rounded-xl border bg-gray-50/60 p-4 dark:border-dark-600 dark:bg-dark-900/40">
        <legend class="text-sm font-medium">{{ materialLabels[kind] }} <span class="text-xs font-normal text-gray-500">{{ uploaded[kind].length }} 个文件</span></legend>
        <label class="block text-xs">URL、Data URL 或 JSON 素材对象<textarea v-model="materialText[kind]" class="input mt-1 min-h-24 w-full font-mono text-xs" :aria-label="`${materialLabels[kind]}输入`" placeholder="每行一个 URL；或 JSON，如 {&quot;b64_json&quot;:&quot;...&quot;,&quot;mime_type&quot;:&quot;image/png&quot;}" /></label>
        <label class="block text-xs">添加文件（支持多选、可分批添加）<input class="input mt-1 w-full file:mr-3 file:rounded-md file:border-0 file:bg-primary-50 file:px-3 file:py-2 file:text-primary-700" type="file" multiple :accept="`${kind}/*`" :aria-label="`上传${materialLabels[kind]}`" @change="uploadMaterials(kind, $event)" /></label>
        <ul v-if="uploaded[kind].length" class="space-y-1 text-xs">
          <li v-for="(file, index) in uploaded[kind]" :key="`${file.name}-${index}`" class="flex items-center justify-between gap-2 rounded-lg border bg-white p-2 dark:border-dark-600 dark:bg-dark-800">
            <span class="min-w-0 truncate">{{ file.name }} · {{ file.mime_type }} · {{ file.size }} B</span>
            <button type="button" class="btn btn-secondary shrink-0" :aria-label="`移除${materialLabels[kind]} ${file.name}`" @click="removeUpload(kind, index)">移除</button>
          </li>
        </ul>
        <p v-if="selectedCapability?.materials?.[kind]?.max_count" class="text-xs text-gray-500">能力目录最多 {{ selectedCapability.materials[kind].max_count }} 项；未声明的类型仍可预览模板映射。</p>
      </fieldset>
    </div>
    <button type="button" class="btn btn-primary" :disabled="busy || loading || uploadsPending > 0 || !detail || !model || !operation" @click="run(false)">{{ uploadsPending ? '正在读取素材…' : busy ? '检查中…' : '预览映射与渠道估价' }}</button>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
    <div v-if="previewInput || result" class="grid gap-4 xl:grid-cols-2">
      <section v-if="previewInput" class="min-w-0 border-t pt-3 dark:border-dark-600"><h3 class="mb-2 text-sm font-semibold">输入请求 · 编码内容已脱敏</h3><pre class="max-h-[480px] overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-3 text-xs dark:bg-dark-900">{{ displayJSON(previewInput) }}</pre></section>
      <section v-if="result" class="min-w-0 border-t pt-3 dark:border-dark-600"><h3 class="mb-2 text-sm font-semibold">规范化参数与实际请求映射</h3><pre class="max-h-[480px] overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-3 text-xs dark:bg-dark-900">{{ displayJSON({ normalized: resultData.normalized, upstream_request: resultData.request, effective: resultData.effective, inputs: resultData.inputs }) }}</pre></section>
      <section v-if="result" class="min-w-0 border-t pt-3 dark:border-dark-600"><h3 class="mb-2 text-sm font-semibold">估价、匹配规则与预览状态</h3><pre class="max-h-[480px] overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-3 text-xs dark:bg-dark-900">{{ displayJSON({ pricing: resultData.pricing, rule_provenance: resultData.rule_provenance, override_diff: resultData.override_diff, request_complete: resultData.request_complete, measurement_required: resultData.measurement_required, paid_submission: resultData.paid_submission }) }}</pre></section>
      <section v-if="logs.length" class="min-w-0 border-t pt-3 dark:border-dark-600"><h3 class="mb-2 text-sm font-semibold">联调日志</h3><ol class="max-h-[480px] space-y-3 overflow-auto"> <li v-for="(entry, index) in logs" :key="index" class="border-l-2 border-primary-500 pl-3"><div class="flex gap-2 text-xs"><time class="text-gray-500">{{ entry.time }}</time><strong>{{ entry.stage }}</strong></div><pre class="mt-1 whitespace-pre-wrap break-all text-xs">{{ entry.detail }}</pre></li></ol></section>
    </div>
  </section>
</template>
