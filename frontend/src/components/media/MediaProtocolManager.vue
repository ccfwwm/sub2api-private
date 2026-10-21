<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import MediaParameterRulesEditor from './MediaParameterRulesEditor.vue'
import { mediaProtocolsAPI as api, type ProtocolKind } from '@/api/media-protocols'
import type { MediaConfigRecord } from '@/api/media'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { useStepUp, isStepUpCancelled } from '@/composables/useStepUp'

const route = useRoute(), stepUp = useStepUp()
const kind = ref<ProtocolKind>(route.query.tab === 'bindings' ? 'bindings' : 'templates')
const templates = ref<MediaConfigRecord[]>([]), bindings = ref<MediaConfigRecord[]>([])
const selected = ref<MediaConfigRecord | null>(null), history = ref<MediaConfigRecord[]>([])
const json = ref(''), mode = ref('form'), busy = ref(false), error = ref(''), result = ref<unknown>()
const review = ref(false), upgrades = ref<string[]>([]), bulkModels = ref(''), builtinFields=ref<string[]>([])
const builtinChanges=computed(()=>Object.keys(selected.value?.builtin_current ?? {}).filter(key=>!['id','version','status'].includes(key)&&JSON.stringify(selected.value?.builtin_current?.[key])!==JSON.stringify(draft.value?.[key])))
function loadBuiltin(all=false) {if(!draft.value||!selected.value?.builtin_current)return;const next=JSON.parse(JSON.stringify(draft.value));for(const key of all?Object.keys(selected.value.builtin_current):builtinFields.value) {if(!['id','version','status'].includes(key))next[key]=JSON.parse(JSON.stringify(selected.value.builtin_current[key]))}json.value=JSON.stringify(next,null,2)}
const previewInput = ref('{"model":"preview","prompt":"参考素材测试","seconds":5,"images":[]}')
const sample = ref('{}'), online = ref(false), operation = ref('video'), upstreamModel = ref('preview')
const previewKeyID = ref(''), previewAccountID = ref('')
const effectiveGroup=ref(String(route.query.group_id || '')), effectiveResult=ref<unknown>()
const keyID = ref(''), paid = ref(false), discoveryAccount = ref(''), discoveryGroup = ref('')
// Retain the key after a timeout: clicking again cannot create another paid task.
const testKey = ref(crypto.randomUUID())
const draft = computed<Record<string, unknown> | null>(() => { try { const value = JSON.parse(json.value); return value && !Array.isArray(value) && typeof value === 'object' ? value : null } catch { return null } })
const accountFilter = ref(String(route.query.account_id || '')), groupFilter = ref(String(route.query.group_id || ''))
const rows = computed(() => kind.value === 'templates' ? templates.value : bindings.value.filter(row => (!accountFilter.value || String(row.config.account_id) === accountFilter.value || (groupFilter.value && !Number(row.config.account_id))) && (!groupFilter.value || String(row.config.group_id) === groupFilter.value)))
watch(() => route.query.account_id, value => { accountFilter.value = String(value || ''); selected.value = null; json.value = '' })
watch(() => route.query.group_id, value => { groupFilter.value = String(value || ''); effectiveGroup.value = String(value || '') })
const affected = computed(() => bindings.value.filter(row => (row.config.template as { id?: string } | undefined)?.id === selected.value?.id && row.published_version))
const changes = computed(() => [...new Set([...Object.keys(selected.value?.config ?? {}), ...Object.keys(draft.value ?? {})])].filter(key => JSON.stringify(selected.value?.config[key]) !== JSON.stringify(draft.value?.[key])))
const publishedTemplates = computed(() => templates.value.filter(row => row.published_version))
function field(path: string): unknown { return path.split('.').reduce<unknown>((value, key) => (value as Record<string, unknown> | undefined)?.[key], draft.value) }
function set(path: string, value: unknown) {
  if (!draft.value) return
  const object = structuredClone(draft.value), parts = path.split('.'); let target = object
  for (const part of parts.slice(0, -1)) { target[part] ??= {}; target = target[part] as Record<string, unknown> }
  target[parts.at(-1)!] = value; json.value = JSON.stringify(object, null, 2)
}
const value = (event: Event) => (event.target as HTMLInputElement).value
function jsonField(path: string, event: Event) { try { set(path, JSON.parse(value(event))); error.value = '' } catch { error.value = `${path} 需要有效 JSON` } }
async function action(run: () => Promise<void>, mutation = false) {
  busy.value = true; error.value = ''
  try { if (mutation) await stepUp.run(run); else await run() } catch (cause) { if (!isStepUpCancelled(cause)) error.value = (cause as Error).message || String(cause) } finally { busy.value = false }
}
async function refresh() {
  const results = await Promise.allSettled([api.list('templates'), api.list('bindings')])
  const failures: string[] = []
  if (results[0].status === 'fulfilled') templates.value = results[0].value; else failures.push('模板：' + String(results[0].reason?.message || results[0].reason))
  if (results[1].status === 'fulfilled') bindings.value = results[1].value; else failures.push('绑定：' + String(results[1].reason?.message || results[1].reason))
  error.value = failures.join('；')
}
async function select(row: MediaConfigRecord) { selected.value = row; json.value = JSON.stringify(row.config, null, 2); history.value = await api.history(kind.value, row.id); review.value = false; upgrades.value = [] }
function switchKind(next: ProtocolKind) { kind.value = next; selected.value = null; json.value = ''; history.value = []; review.value = false }
function create() {
  selected.value = null; history.value = []; review.value = false
  const object = kind.value === 'templates' ? { id: `custom-${Date.now()}`, name: '自定义模板', version: '1', status: 'draft', schema_version: 1, media_types: ['video'], operations: ['video'], mode: 'async', auth: { mode: 'bearer' }, submit: { method: 'POST', path: '/v1/videos', encoding: 'json' }, query: { method: 'GET', path: '/v1/videos/{task_id}' }, response: { task_id: ['id'], status: ['status'], states: { queued: 'in_progress', completed: 'completed', failed: 'failed' }, urls: ['url'] }, poll_seconds: 10, materials: [], rules: [] } : { id: `binding-${Date.now()}`, version: '1', status: 'draft', group_id: Number(groupFilter.value || 0), account_id: Number(accountFilter.value || 0), model: '', upstream_model: '', rules: [] }
  json.value = JSON.stringify(object, null, 2)
}
function newTest() { testKey.value=crypto.randomUUID();paid.value=false }
function copy() { if (!draft.value) return; const object = { ...draft.value, id: `${draft.value.id}-copy-${Date.now()}` }; selected.value = null; history.value = []; json.value = JSON.stringify(object, null, 2) }
async function save(publish: boolean, sourceVersion?: number) {
  if (!draft.value) throw new Error('需要有效 JSON')
  const id = String(draft.value.id || '')
  const row = await api.save(kind.value, { id, expected_version: selected.value?.id === id ? selected.value.version : 0, config: draft.value, publish, source_version: sourceVersion, upgrade_bindings: publish ? upgrades.value : [] })
  await refresh(); await select(row)
}
async function saveImported() {
  if (!draft.value) throw new Error('需要有效 JSON')
  const object = { ...draft.value, id: String(draft.value.id || '') }
  if (!object.id) throw new Error('导入配置缺少 ID')
  const row = await api.import(kind.value, { id: object.id, expected_version: 0, config: object, publish: false })
  await refresh(); await select(row)
}
async function publishDraft() {
  if (!draft.value) throw new Error('需要有效 JSON')
  const id = String(draft.value.id || '')
  if (!id) throw new Error('配置缺少 ID')
  let current = selected.value?.id === id ? selected.value : null
  if (!current) {
    current = await api.save(kind.value, { id, expected_version: 0, config: draft.value, publish: false })
    // Keep the saved draft if publication fails so retrying uses its version.
    selected.value = current
    json.value = JSON.stringify(current.config, null, 2)
  }
  const row = await api.publish(kind.value, { id, expected_version: current.version, config: draft.value, publish: true, upgrade_bindings: upgrades.value })
  await refresh(); await select(row)
}
async function rollbackVersion(sourceVersion: number) {
  if (!selected.value) return
  const row = await api.rollback(kind.value, { id: selected.value.id, expected_version: selected.value.version, source_version: sourceVersion })
  await refresh(); await select(row)
}
async function download() {
  if (!selected.value) return
  const data = await api.export(kind.value, selected.value.id), url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }))
  const link = document.createElement('a'); link.href = url; link.download = `${selected.value.id}.json`; link.click(); URL.revokeObjectURL(url)
}
async function upload(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]; if (!file) return
  const imported = JSON.parse(await file.text()), object = imported.versions?.[0]?.config ?? imported
  if (!object || typeof object !== 'object' || !object.id) throw new Error('无效的模板或绑定文件')
  json.value = JSON.stringify({ ...object, id: `${object.id}-import-${Date.now()}` }, null, 2); selected.value = null; history.value = []
}
async function preview(effective = false) {
  const request = JSON.parse(previewInput.value)
  if (effective) {
    if (!Number.isSafeInteger(Number(previewKeyID.value)) || Number(previewKeyID.value) <= 0) throw new Error('实际绑定预览需要有效的 API Key ID')
    result.value = await api.preview({ mode: 'effective', api_key_id: Number(previewKeyID.value), ...(previewAccountID.value ? { account_id: Number(previewAccountID.value) } : {}), operation: operation.value, request, sample_response: JSON.parse(sample.value), measure_media: online.value })
  } else {
    result.value = await api.preview({ template: draft.value, operation: operation.value, upstream_model: upstreamModel.value, model: request.model || 'preview', request, rules: [], sample_response: JSON.parse(sample.value), measure_media: online.value })
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
  <section class="space-y-4 rounded-xl border p-4 dark:border-gray-700" aria-label="管理员协议模板与模型绑定">
    <h2 class="text-xl font-semibold">协议模板与模型绑定</h2>
    <p class="text-sm text-gray-500">选择格式模板并固定版本。账号保存启用模型与基础映射，分组绑定覆盖请求参数。普通用户只使用 images、videos、audios；模板不会自动调整价格。</p>
    <div class="flex flex-wrap gap-2"><button class="btn btn-secondary" @click="switchKind('templates')">协议模板</button><button class="btn btn-secondary" @click="switchKind('bindings')">模型绑定</button><button class="btn btn-secondary" @click="create">新建</button><label class="btn btn-secondary cursor-pointer">导入<input type="file" accept="application/json,.json" class="hidden" @change="action(() => upload($event))" /></label><button class="btn btn-secondary" @click="action(refresh)">刷新</button></div>
    <div v-if="kind === 'bindings'" class="flex flex-wrap items-end gap-3"><label class="text-sm">账号筛选<input v-model="accountFilter" type="number" min="1" class="input block" aria-label="绑定账号筛选" /></label><label class="text-sm">分组筛选<input v-model="groupFilter" type="number" min="1" class="input block" /></label><RouterLink v-if="accountFilter" class="text-sm text-primary-600" :to="{path:'/admin/media',query:{tab:'accounts',account_id:accountFilter}}">编辑账号 #{{ accountFilter }} 的模型</RouterLink></div>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
    <details v-if="kind === 'bindings'"><summary>查看分组实际继承配置</summary><input v-model="effectiveGroup" type="number" class="input" placeholder="分组 ID" /><button class="btn btn-secondary" @click="action(async()=>{effectiveResult=await api.effective(Number(effectiveGroup))})">解析当前绑定</button><pre class="max-h-96 overflow-auto text-xs">{{JSON.stringify(effectiveResult,null,2)}}</pre></details>
    <div class="grid gap-4 lg:grid-cols-[260px_1fr]">
      <aside class="max-h-[650px] space-y-2 overflow-auto"><p v-if="!rows.length" class="rounded-lg border border-dashed p-3 text-sm text-gray-500">{{ kind === 'bindings' ? '当前筛选没有独立绑定。账号内可直接绑定模板；如需分组覆盖，请点击新建。' : '暂无协议模板。' }}</p><button v-for="row in rows" :key="row.id" class="block w-full rounded-lg border p-3 text-left text-sm" @click="action(() => select(row))"><strong>{{ row.config.name || row.config.model || row.id }}</strong><p>{{ row.id }}</p><p>{{ row.status }} · 草稿 v{{ row.version }} · 发布 v{{ row.published_version || '—' }}</p><p v-if="kind === 'bindings'">分组 {{ row.config.group_id }} · 账号 {{ row.config.account_id || '全部' }} · {{ row.config.template }}</p></button></aside>
      <div v-if="json" class="space-y-3">
        <div class="flex gap-2"><button class="btn btn-secondary" @click="mode = 'form'">表单</button><button class="btn btn-secondary" @click="mode = 'json'">JSON</button><button class="btn btn-secondary" @click="copy">复制</button><button v-if="selected" class="btn btn-secondary" @click="action(download)">导出全部历史</button></div>
        <div v-if="mode === 'form' && draft" class="grid gap-3 sm:grid-cols-2">
          <label class="text-sm">配置 ID<input class="input w-full" :value="field('id')" @change="set('id', value($event))" /></label>
          <template v-if="kind === 'templates'">
            <label class="text-sm">名称<input class="input w-full" :value="field('name')" @change="set('name', value($event))" /></label>
            <label class="text-sm">模式<select class="input w-full" :value="field('mode')" @change="set('mode', value($event))"><option value="async">异步轮询</option><option value="sync_json">同步 JSON</option><option value="sync_binary">同步媒体二进制</option></select></label>
            <label class="text-sm">媒体类型<input class="input w-full" :value="(field('media_types') as string[])?.join(',')" @change="set('media_types', value($event).split(','))" /></label>
            <label class="text-sm">操作<input class="input w-full" :value="(field('operations') as string[])?.join(',')" @change="set('operations', value($event).split(','))" /></label>
            <label class="text-sm">提交方法<select class="input w-full" :value="field('submit.method')" @change="set('submit.method', value($event))"><option>POST</option><option>PUT</option><option>GET</option></select></label>
            <label class="text-sm">相对路径<input class="input w-full" :value="field('submit.path')" @change="set('submit.path', value($event))" /></label>
            <label class="text-sm">编码<select class="input w-full" :value="field('submit.encoding')" @change="set('submit.encoding', value($event))"><option>json</option><option>multipart</option><option>form</option></select></label>
            <label class="text-sm">账号认证<select class="input w-full" :value="field('auth.mode')" @change="set('auth.mode', value($event))"><option>bearer</option><option>header</option><option>query</option><option>none</option></select></label>
            <label class="text-sm">认证头 / 查询参数名称<input class="input w-full" :value="field('auth.name')" @change="set('auth.name', value($event))" /></label>
            <label class="text-sm">认证前缀<input class="input w-full" :value="field('auth.prefix')" @change="set('auth.prefix', value($event))" /></label>
            <label class="text-sm">轮询秒数<input type="number" min="1" max="300" class="input w-full" :value="field('poll_seconds')" @change="set('poll_seconds', Number(value($event)))" /></label>
            <label v-for="path in ['materials','response','query','discovery','model_ids','artifact_list','download_auth','recover','body','operation_submit']" :key="path" class="text-sm">{{ path }}<textarea class="input min-h-24 w-full font-mono text-xs" :value="JSON.stringify(field(path) ?? null, null, 2)" @change="jsonField(path, $event)" /></label>
            <MediaParameterRulesEditor class="sm:col-span-2" :model-value="(field('rules') as Record<string,unknown>[]) || []" @update:model-value="set('rules',$event)" />
            <label class="text-sm">模型写入目标（- 为仅路径）<input class="input w-full" :value="field('model_target')" @change="set('model_target', value($event))" /></label>
            <label class="text-sm">固定内容接口<input class="input w-full" :value="field('content_path')" @change="set('content_path', value($event))" /></label>
          </template>
          <template v-else>
            <label class="text-sm">分组 ID<input type="number" class="input w-full" :value="field('group_id')" @change="set('group_id', Number(value($event)))" /></label>
            <label class="text-sm">公开模型（留空为分组默认）<input class="input w-full" :value="field('model')" @change="set('model', value($event))" /></label>
            <label class="text-sm">账号 ID（0 为全部）<input type="number" class="input w-full" :value="field('account_id')" @change="set('account_id', Number(value($event)))" /></label>
            <label class="text-sm">上游模型<input class="input w-full" :value="field('upstream_model')" @change="set('upstream_model', value($event))" /></label>
            <label class="text-sm">协议模板<select class="input w-full" :value="field('template.id')" @change="bindTemplate(value($event))"><option value="">继承账号模板</option><option v-for="row in publishedTemplates" :key="row.id" :value="row.id">{{ row.config.name }} · v{{ row.published_version }}</option></select></label>
            <label class="text-sm">固定模板版本<input type="number" min="1" class="input w-full" :value="field('template.version')" @change="set('template.version', Number(value($event)))" /></label>
            <MediaParameterRulesEditor class="sm:col-span-2" :model-value="(field('rules') as Record<string,unknown>[]) || []" @update:model-value="set('rules',$event)" />
            <p class="text-xs text-gray-500 sm:col-span-2">继承顺序：模板 → 账号模型 → 分组默认 → 分组精确模型 → 分组精确模型与账号。相同选择器冲突时禁止发布。</p>
          </template>
          <p class="text-xs text-gray-500 sm:col-span-2">表单直接修改完整 JSON 对象；切换模式保留高级字段。服务端以同一版本 schema 校验。模板只引用账号密钥，不能填写真实密钥。</p>
        </div>
        <textarea v-if="mode === 'json' || !draft" v-model="json" class="input min-h-[460px] w-full font-mono text-xs" aria-label="完整协议配置 JSON" />
        <div class="flex flex-wrap gap-2"><button class="btn btn-secondary" :disabled="busy || !draft" @click="action(() => save(false), true)">保存草稿</button><button class="btn btn-secondary" :disabled="busy || !draft" @click="action(saveImported, true)">作为新配置导入</button><button class="btn btn-primary" :disabled="busy || !draft" @click="review = true">审阅并发布</button><button v-if="selected" class="btn btn-secondary" :disabled="busy" @click="action(async () => { await api.archive(kind, selected!.id, selected!.version); await refresh() }, true)">归档（保留固定版本）</button></div>
        <div v-if="review" class="space-y-2 rounded border border-blue-300 p-3"><p>变更字段：{{ changes.join(', ') || '无' }}</p><div v-for="key in changes" :key="key" class="grid gap-2 sm:grid-cols-2"><pre class="overflow-auto text-xs">{{ key }} 之前：{{ JSON.stringify(selected?.config[key], null, 2) }}</pre><pre class="overflow-auto text-xs">之后：{{ JSON.stringify(draft?.[key], null, 2) }}</pre></div><p>在途任务与未选绑定继续使用原版本。</p><label v-for="row in affected" :key="row.id" class="block text-sm"><input v-model="upgrades" type="checkbox" :value="row.id" /> 同时升级 {{ row.id }}（{{ row.config.model || '分组默认' }}）</label><button class="btn btn-primary" :disabled="busy" @click="action(publishDraft, true)">发布不可变新版本</button></div>
        <details v-if="selected?.builtin_current"><summary>随版本提供的内置模板差异</summary><p class="text-xs">管理员配置保持不变。选择字段载入编辑器，审阅发布后才产生新版本；未选择的绑定继续固定原版本。</p><label v-for="key in builtinChanges" :key="key" class="block text-sm"><input v-model="builtinFields" type="checkbox" :value="key" /> {{key}}<pre class="overflow-auto text-xs">{{JSON.stringify(selected.builtin_current[key],null,2)}}</pre></label><button class="btn btn-secondary" @click="loadBuiltin()">合并所选内置字段</button><button class="btn btn-secondary" @click="loadBuiltin(true)">载入内置原版并审阅</button></details>
        <details v-if="history.length"><summary>历史、内置原版对比与回滚</summary><div v-for="row in history" :key="row.version" class="my-2 rounded border p-2"><p>v{{ row.version }} · {{ row.status }} · {{ row.config.builtin_revision ? '内置来源' : '管理员配置' }}</p><button class="btn btn-secondary" @click="json = JSON.stringify(row.config, null, 2)">载入此版本并比较</button><button class="btn btn-secondary" @click="action(() => rollbackVersion(row.version), true)">以此版本回滚并发布</button><details><summary>查看配置</summary><pre class="max-h-64 overflow-auto text-xs">{{ JSON.stringify(row.config, null, 2) }}</pre></details></div></details>
        <details v-if="kind === 'bindings'"><summary>批量发布相同模板</summary><p class="text-xs">每行一个公开模型，默认上游映射为相同 ID。每项仍独立检查账号、版本及冲突；价格保持原设置。</p><textarea v-model="bulkModels" class="input min-h-24 w-full" /><button class="btn btn-primary" :disabled="busy || !draft" @click="action(bulkBind, true)">发布这些模型的绑定</button></details>
        <details>
          <summary>实际绑定、估价与发送摘要（只读）</summary>
          <div class="space-y-3 py-3">
            <p class="text-sm">按已有 API Key 的真实分组与价格解析已发布配置，展示覆盖差异、模板和绑定版本及准备发送的摘要。当前编辑中的草稿不参与；预览不预留额度、不创建任务、不调用生成接口。</p>
            <label class="block">API Key ID<input v-model="previewKeyID" type="number" min="1" class="input" aria-label="预览 API Key ID" /></label>
            <label class="block">限定候选账号 ID（可选）<input v-model="previewAccountID" type="number" min="1" class="input" aria-label="预览候选账号 ID" /></label>
            <label class="block">操作<input v-model="operation" class="input" aria-label="实际绑定预览操作" /></label>
            <label class="block">统一输入<textarea v-model="previewInput" class="input min-h-28 w-full font-mono text-xs" aria-label="实际绑定预览输入" /></label>
            <label class="block text-sm"><input v-model="online" type="checkbox" /> 允许素材 GET 及可信测量；未勾选时需要测量的项目会明确列出，不推测价格。</label>
            <button class="btn btn-secondary" :disabled="busy || !previewKeyID" @click="action(() => preview(true))">预览实际绑定与估价</button>
          </div>
        </details>
        <details v-if="kind === 'templates'"><summary>免费预览、模型发现与付费测试</summary><div class="space-y-3 py-3"><label class="block">操作<input v-model="operation" class="input" /></label><label class="block">上游模型<input v-model="upstreamModel" class="input" /></label><label class="block">统一输入<textarea v-model="previewInput" class="input min-h-28 w-full font-mono text-xs" /></label><label class="block">响应样例<textarea v-model="sample" class="input min-h-20 w-full font-mono text-xs" /></label><label class="block text-sm"><input v-model="online" type="checkbox" /> 允许为编码转换执行素材 GET（不会调用生成接口）</label><button class="btn btn-secondary" :disabled="busy" @click="action(preview)">免费预览</button><div class="flex flex-wrap gap-2"><input v-model="discoveryAccount" class="input" placeholder="账号 ID" /><input v-model="discoveryGroup" class="input" placeholder="分组 ID" /><button class="btn btn-secondary" :disabled="busy" @click="action(async () => { result = await api.discover({ template: draft, account_id: Number(discoveryAccount), group_id: Number(discoveryGroup) }) })">发现候选模型（不启用）</button></div><p class="text-sm">真实测试使用 API Key 对应的已发布绑定及价格，通过正常任务和计费链路。草稿不会直接发送。</p><input v-model="keyID" class="input" placeholder="用于记账的 API Key ID" /><label class="block text-sm"><input v-model="paid" type="checkbox" /> 确认本次生成可能收费</label><p class="break-all text-xs">幂等 ID：{{ testKey }}</p><button class="btn btn-primary" :disabled="busy || !paid || !keyID" @click="action(async () => { result = await api.test({ api_key_id: Number(keyID), operation, request: JSON.parse(previewInput), confirm_paid_submission: paid }, testKey) }, true)">提交 / 查询原测试</button><button class="btn btn-secondary" @click="newTest">明确开始另一次测试</button></div></details>
        <pre v-if="result" class="max-h-96 overflow-auto rounded bg-gray-50 p-3 text-xs dark:bg-gray-900">{{ JSON.stringify(result, null, 2) }}</pre>
      </div>
    </div>
    <TotpStepUpDialog :controller="stepUp" />
  </section>
</template>
