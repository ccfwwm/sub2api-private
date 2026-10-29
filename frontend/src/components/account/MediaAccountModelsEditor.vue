<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { mediaProtocolsAPI } from '@/api/media-protocols'
import type { MediaConfigRecord } from '@/api/media'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import AccountTestModal from '@/components/admin/account/AccountTestModal.vue'
import MediaParameterRulesEditor from '@/components/media/MediaParameterRulesEditor.vue'
import { accountMediaCapabilities, mergeDiscoveredMediaModels, newMediaCapability, type MediaCapability } from '@/types/media'
import type { Account } from '@/types'
const props = defineProps<{ modelValue: MediaCapability[]; accountId?: number; account?: Account; baseUrl: string; apiKey?: string }>()
const emit = defineEmits<{ 'update:modelValue': [MediaCapability[]] }>()
const { locale } = useI18n()
const tr = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
const candidates = ref<string[]>([]), error = ref(''), loading = ref(false), selected = ref('')
const retryAfterRemaining = ref(0)
let retryTimer: ReturnType<typeof setInterval> | undefined
const advanced = ref<number | null>(props.modelValue[0]?.adapter ? null : 0), testingModel = ref<string | null>(null)
const templates = ref<MediaConfigRecord[]>([])
const importNotice = ref(''), bulkTemplate = ref(''), selectedModels = ref<string[]>([])
const pinnedTemplates = ref<Record<string, Record<string, unknown>>>({})
const effectiveTemplates = ref<Record<string, string>>({})
const effectiveTemplateState = ref<'idle' | 'loading' | 'loaded' | 'error'>('idle')
onMounted(async () => {
  try {
    templates.value = await mediaProtocolsAPI.list('templates')
    for (const capability of props.modelValue) {
      const ref = capability.template_ref, row = templates.value.find(item => item.id === ref?.id)
      if (!ref || row?.published_version === ref.version || pinnedTemplates.value[`${ref.id}:${ref.version}`]) continue
      const history = await mediaProtocolsAPI.history('templates', ref.id)
      const pinned = history.find(item => item.version === ref.version)
      if (pinned) pinnedTemplates.value[`${ref.id}:${ref.version}`] = pinned.config
    }
    await loadEffectiveTemplates()
  } catch (e) {
    effectiveTemplateState.value = 'error'
    error.value = (e as Error).message
  }
})
onUnmounted(() => { if (retryTimer) clearInterval(retryTimer) })
const val = (e: Event) => (e.target as HTMLInputElement).value
const csv = (s: string) => s.split(/[,，\n]/).map(v => v.trim()).filter(Boolean)
const saved = computed(() => props.account && accountMediaCapabilities(props.account.extra, props.account.credentials))
const canTest = computed(() => !!props.account && JSON.stringify(saved.value) === JSON.stringify(props.modelValue) && props.baseUrl === String(props.account.credentials?.base_url || '') && (!props.apiKey || props.apiKey === props.account.credentials?.api_key))
const mappingConflict = computed(() => props.account && props.modelValue.some(c => {
  const mapping = props.account?.credentials?.model_mapping as Record<string, string> | undefined
  return mapping?.[c.model] && mapping[c.model] !== c.upstream_model
}))
function edit(index: number, fn: (c: MediaCapability) => void) { const all: MediaCapability[] = JSON.parse(JSON.stringify(props.modelValue)); fn(all[index]); emit('update:modelValue', all) }
function add(model = '') { if (model && props.modelValue.some(c => c.model === model)) return; emit('update:modelValue', [...props.modelValue, newMediaCapability(model)]); advanced.value = props.modelValue.length }
function modelID(c: MediaCapability, value: string) {
  if (!c.upstream_model || c.upstream_model === c.model) c.upstream_model = value
  c.model = value
}
async function sync() {
  loading.value = true; error.value = ''
  try {
    const result = props.accountId && !props.apiKey && props.baseUrl === String(props.account?.credentials?.base_url || '') ? await adminAPI.accounts.syncUpstreamModels(props.accountId) : await adminAPI.accounts.syncUpstreamModelsPreview({platform:'imagevideo',type:'apikey',base_url:props.baseUrl,api_key:props.apiKey || String(props.account?.credentials?.api_key || '')})
    candidates.value = result.models
    const merged = mergeDiscoveredMediaModels(props.modelValue, result.models)
    importNotice.value = tr(`发现 ${result.models.length} 个模型，新增 ${merged.length - props.modelValue.length} 个待绑定模型。保存账号后生效。`, `Found ${result.models.length} models; added ${merged.length - props.modelValue.length} unbound models. Save the account to apply.`)
    emit('update:modelValue', merged)
  } catch (e) {
    const failure = e as { status?: number; code?: string | number; reason?: string; retryAfterSeconds?: number; message?: string }
    const retrySeconds = Math.max(0, Math.min(24 * 60 * 60, Number(failure.retryAfterSeconds) || 0))
    if (retrySeconds > 0) {
      retryAfterRemaining.value = retrySeconds
      if (retryTimer) clearInterval(retryTimer)
      retryTimer = setInterval(() => {
        retryAfterRemaining.value = Math.max(0, retryAfterRemaining.value - 1)
        if (retryAfterRemaining.value === 0 && retryTimer) {
          clearInterval(retryTimer)
          retryTimer = undefined
        }
      }, 1000)
    }
    if (failure.reason === 'UPSTREAM_RATE_LIMITED') {
      error.value = retrySeconds > 0
        ? tr(`上游模型列表接口触发 429 限流，请 ${retrySeconds} 秒后手动重试。未自动重试。`, `The upstream model-list endpoint returned 429. Retry manually in ${retrySeconds} seconds; no automatic retry was sent.`)
        : tr('上游模型列表接口触发 429 限流，请稍后手动重试。未自动重试。', 'The upstream model-list endpoint returned 429. Retry manually later; no automatic retry was sent.')
    } else if (failure.code === 'RATE_LIMITED') {
      error.value = retrySeconds > 0
        ? tr(`Sub2API 管理接口触发限流，请 ${retrySeconds} 秒后重试。`, `Sub2API admin API is rate-limited. Retry in ${retrySeconds} seconds.`)
        : tr('Sub2API 管理接口触发限流，请稍后重试。', 'Sub2API admin API is rate-limited. Retry later.')
    } else {
      error.value = failure.message || tr('同步失败','Sync failed')
    }
  }
  finally { loading.value = false }
}
function chooseTemplate(c: MediaCapability, id: string) {
  const row=templates.value.find(item=>item.id===id)
  if (!row?.published_version) return
  c.template_ref={id,version:row.published_version};c.adapter='template';c.request_format='unified'
  const published=row.published_config ?? row.config
  const mediaTypes = published.media_types as string[], operations = published.operations as string[]
  c.media_type = mediaTypes.includes(c.media_type) ? c.media_type : mediaTypes.length === 1 ? mediaTypes[0] : ''
  c.operations = c.operations.filter(operation => operations.includes(operation))
  if (!c.operations.length && operations.length === 1) c.operations = [...operations]
  c.formats=['json','multipart'];c.duration.required=c.media_type==='video'
}
function bindSelected() {
  const all: MediaCapability[] = JSON.parse(JSON.stringify(props.modelValue))
  for (const cap of all) if (selectedModels.value.includes(cap.model)) { chooseTemplate(cap, bulkTemplate.value); cap.enabled = false }
  emit('update:modelValue', all)
}
function selectedTemplate(c: MediaCapability): Record<string, unknown> | undefined {
  const row = templates.value.find(item => item.id === c.template_ref?.id)
  if (c.template_ref && row?.published_version !== c.template_ref.version) return pinnedTemplates.value[`${c.template_ref.id}:${c.template_ref.version}`]
  return row?.published_config ?? row?.config
}
function templateOptions(c: MediaCapability, field: 'media_types' | 'operations'): string[] {
  return (selectedTemplate(c)?.[field] as string[] | undefined) ?? []
}
async function loadEffectiveTemplates() {
  const accountID = props.accountId ?? props.account?.id
  const groupIDs = [...new Set(props.account?.group_ids ?? [])]
  if (!accountID || groupIDs.length === 0) {
    effectiveTemplateState.value = 'loaded'
    return
  }
  effectiveTemplateState.value = 'loading'
  const [bindings, groups] = await Promise.all([
    mediaProtocolsAPI.list('bindings'),
    Promise.all(groupIDs.map(async groupID => ({
      groupID,
      caps: await mediaProtocolsAPI.effective(groupID) as Array<Record<string, unknown>>
    })))
  ])
  const sourceFor = (cap: Record<string, unknown>, groupID: number) => {
    const versions = cap.binding_versions as Array<{ id: string; version: number }> | undefined
    const latest = versions?.at(-1)
    if (!latest) return tr('账号模型', 'Account model')
    const binding = bindings.find(row => row.id === latest.id)
    const config = (binding?.config || {}) as Record<string, unknown>
    if (config.account_id && config.model) return tr('账号+模型绑定', 'Account + model binding')
    if (config.account_id) return tr('账号绑定', 'Account binding')
    if (config.model) return tr(`分组 ${groupID} 模型绑定`, `Group ${groupID} model binding`)
    return tr(`分组 ${groupID} 默认绑定`, `Group ${groupID} default binding`)
  }
  for (const model of props.modelValue) {
    const matches = groups.flatMap(({ groupID, caps }) => caps
      .filter(cap => cap.model === model.model)
      .filter(cap => {
        const accountIDs = cap.account_ids as number[] | undefined
        return !accountIDs?.length || accountIDs.includes(accountID)
      })
      .map(cap => {
        const ref = cap.template_ref as { id: string; version: number } | undefined
        const execution = cap.execution_template as { name?: string; status?: string } | undefined
        const validationStatus = String(cap.validation_status || '')
        if (!ref) {
          return validationStatus
            ? `${validationStatus} · ${tr(`分组 ${groupID}`, `Group ${groupID}`)}`
            : tr(`分组 ${groupID}：未配置模板`, `Group ${groupID}: no template`)
        }
        if (!execution || execution.status && execution.status !== 'enabled') {
          return `${execution?.name || ref.id} · ${tr('模板不可用', 'Template unavailable')}`
        }
        return `${execution?.name || ref.id} · ${sourceFor(cap, groupID)}`
      }))
    effectiveTemplates.value[model.model] = matches.length > 0
      ? [...new Set(matches)].join(' / ')
      : tr('未解析到有效模板', 'No effective template resolved')
  }
  effectiveTemplateState.value = 'loaded'
}
function effectiveTemplateLabel(c: MediaCapability) {
  if (effectiveTemplateState.value === 'loading') return tr('正在解析当前生效模板', 'Resolving effective template')
  if (effectiveTemplateState.value === 'error') return tr('模板来源解析失败', 'Template source could not be resolved')
  const resolved = effectiveTemplates.value[c.model]
  if (resolved) return resolved
  if (c.template_ref) return `${(selectedTemplate(c)?.name as string) || c.template_ref.id} · ${tr('账号模型', 'Account model')}`
  return tr('未绑定模板', 'No template bound')
}
function selectMediaType(c: MediaCapability, mediaType: string) {
  if (!templateOptions(c, 'media_types').includes(mediaType)) return
  c.media_type = mediaType; c.duration.required = mediaType === 'video'
}
function selectOperation(c: MediaCapability, operation: string, enabled: boolean) {
  if (!templateOptions(c, 'operations').includes(operation)) return
  c.operations = enabled ? [...new Set([...c.operations, operation])] : c.operations.filter(value => value !== operation)
}
function setRules(index: number, event: Event) {
  try {
    const rules = JSON.parse(val(event))
    if (!Array.isArray(rules) || rules.some(rule => !rule || typeof rule !== 'object' || Array.isArray(rule))) throw new Error('invalid')
    edit(index, c => { c.template_rules = rules }); error.value = ''
  } catch { error.value = tr('模板规则需要 JSON 对象数组。','Template rules require a JSON array of objects.') }
}
function testStatus(c: MediaCapability) {
  const results = props.account?.extra?.media_test_results as Record<string, {status:string;started_at:string;test_kind?:string}> | undefined
  const result = results?.[c.model]
  if (!result) return tr('未实测','Not tested')
  const state = ({completed:tr(result.test_kind === 'connection' ? '连接通过' : '生成通过',result.test_kind === 'connection' ? 'Connection passed' : 'Generation passed'),failed:tr('失败','Failed'),unknown:tr('结果待确认','Uncertain'),submitting:tr('提交中','Submitting'),in_progress:tr('进行中','Running')} as Record<string,string>)[result.status] || result.status
  return state + ' · ' + new Date(result.started_at).toLocaleString()
}
</script>
<template>
  <section class="space-y-3 rounded-xl border p-4 dark:border-dark-600">
    <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="font-semibold">{{ tr('模型配置','Model configuration') }}</h3><div class="flex gap-2"><button class="btn btn-secondary" type="button" :disabled="loading || retryAfterRemaining > 0" @click="sync">{{ loading ? tr('同步中…','Syncing…') : retryAfterRemaining > 0 ? `${tr('等待','Wait')} ${retryAfterRemaining}s` : tr('一键拉取全部模型','Import all models') }}</button><button class="btn btn-secondary" type="button" @click="add()">{{ tr('手动添加','Add manually') }}</button></div></div>
    <p class="text-xs text-gray-500">{{ tr('拉取模型后，在账号内选择协议模板、配置请求能力并保存。测试通过后启用；业务调用需要匹配分组或渠道定价。','Import models, bind a template, configure request capabilities and save this account. Test before enabling; requests require matching group or channel pricing.') }}</p>
    <p v-if="mappingConflict" class="text-xs text-amber-600">{{ tr('旧映射与媒体能力不一致：当前按本表路由，保存后统一映射。','Legacy mappings differ from capabilities. This table determines routing; saving reconciles mappings.') }}</p>
    <p v-if="!canTest" class="text-xs text-gray-500">{{ tr('测试使用已保存的账号配置，请先保存新账号或配置修改。','Tests use saved settings. Save new accounts or changes first.') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <p v-if="importNotice" role="status" class="text-sm text-green-700">{{ importNotice }}</p>
    <div v-if="modelValue.length" class="flex flex-wrap items-center gap-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-800"><label class="text-sm"><input type="checkbox" :checked="selectedModels.length === modelValue.length" @change="selectedModels = ($event.target as HTMLInputElement).checked ? modelValue.map(c=>c.model) : []" /> 全选模型</label><select v-model="bulkTemplate" class="input min-w-0 sm:max-w-sm" aria-label="批量协议模板"><option value="">选择批量绑定模板</option><option v-for="row in templates.filter(row=>row.published_version)" :key="row.id" :value="row.id">{{ row.config.name }}</option></select><button type="button" class="btn btn-secondary" :disabled="!selectedModels.length || !bulkTemplate" @click="bindSelected">绑定所选（{{ selectedModels.length }}）</button><span class="text-xs text-gray-500">批量绑定后请检查请求能力，模型保持停用。</span></div>
    <div v-if="candidates.length" class="flex flex-wrap gap-2"><select v-model="selected" class="input max-w-md"><option value="">{{ tr('选择候选模型','Select a model') }}</option><option v-for="m in candidates" :key="m">{{ m }}</option></select><button type="button" class="btn btn-secondary" :disabled="!selected" @click="add(selected)">{{ tr('添加到账号','Add to account') }}</button></div>
    <p v-if="!modelValue.length" class="text-sm text-gray-500">{{ tr('先同步候选模型或手动添加。','Discover models or add one manually.') }}</p>
    <div v-else class="space-y-4"><template v-for="(c,i) in modelValue" :key="i">
      <article class="min-w-0 rounded-xl border p-4 dark:border-dark-600">
        <div class="grid min-w-0 gap-4 md:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_100px]">
          <label class="min-w-0 text-xs text-gray-500"><span class="mb-1.5 flex items-center gap-2"><input v-model="selectedModels" type="checkbox" :value="c.model" :aria-label="'选择模型 ' + c.model" />{{ tr('公开模型名称','Public model') }}</span><input class="input w-full min-w-0" :title="c.model" :aria-label="tr('模型名称','Model')" :value="c.model" @change="edit(i,c => modelID(c,val($event)))" /></label>
          <label class="min-w-0 text-xs text-gray-500">{{ tr('上游映射（实际发送）','Upstream model (sent to provider)') }}<input class="input mt-1.5 w-full min-w-0" :title="c.upstream_model" :aria-label="tr('上游映射','Upstream mapping')" :value="c.upstream_model" @change="edit(i,c => c.upstream_model = val($event).trim())" /></label>
          <div class="flex items-end gap-3 text-xs"><span>{{ c.media_type || tr('待选接口','Choose interface') }}</span><label class="inline-flex items-center gap-1.5"><input type="checkbox" :aria-label="tr('启用模型','Enable model')" :checked="c.enabled" :disabled="!c.template_ref || !c.media_type || !c.operations.length" @change="edit(i,c => c.enabled = ($event.target as HTMLInputElement).checked)" />{{ tr('启用','Enabled') }}</label></div>
        </div>
        <div class="mt-3 flex flex-wrap items-center justify-between gap-3 border-t pt-3 dark:border-dark-600">
          <div class="min-w-0 space-y-1 text-xs"><p class="break-words text-gray-600 dark:text-gray-300">{{ effectiveTemplateLabel(c) }}</p><p class="text-gray-400">{{ testStatus(c) }}</p></div>
          <div class="flex flex-wrap gap-3 text-sm"><button type="button" class="text-primary-600 disabled:opacity-40" :disabled="!canTest" @click="testingModel = c.model">{{ tr('测试 / 查看','Test / view') }}</button><button type="button" class="text-primary-600" @click="advanced = advanced === i ? null : i">{{ advanced === i ? tr('收起配置','Collapse') : tr('配置请求能力','Configure capabilities') }}</button><button type="button" class="text-red-600" @click="emit('update:modelValue',modelValue.filter((_,index) => index !== i))">{{ tr('移除','Remove') }}</button></div>
        </div>
      <div v-if="advanced === i" class="mt-4 min-w-0 rounded-lg bg-gray-50 p-4 dark:bg-dark-800"><div class="grid min-w-0 gap-4 sm:grid-cols-2 [&>label]:min-w-0 [&_input]:w-full [&_select]:w-full">
        <label class="text-xs">协议模板<select class="input mt-1" aria-label="协议模板" :value="c.template_ref?.id || ''" @change="edit(i,c => chooseTemplate(c,val($event))); advanced = i"><option value="">请选择模板</option><option v-for="row in templates.filter(row => row.published_version)" :key="row.id" :value="row.id">{{ row.config.name }}</option></select><a class="block text-primary-600" :href="'/admin/media?tab=templates&account_id=' + (accountId || '')">管理协议模板</a></label>
        <p v-if="selectedTemplate(c)?.description" class="text-xs text-gray-500">{{ selectedTemplate(c)?.description }}</p>
        <label v-if="c.template_ref" class="text-xs">{{ tr('输出媒体类型','Output media type') }}<select class="input mt-1" :aria-label="tr('输出媒体类型','Output media type')" :value="c.media_type" @change="edit(i,c => selectMediaType(c,val($event)))"><option value="">{{ tr('请选择输出媒体类型','Choose output media type') }}</option><option v-if="c.media_type && !templateOptions(c,'media_types').includes(c.media_type)" :value="c.media_type">{{ c.media_type }} · {{ tr('当前配置','Current configuration') }}</option><option v-for="mediaType in templateOptions(c,'media_types')" :key="mediaType" :value="mediaType">{{ mediaType }}</option></select></label>
        <fieldset v-if="c.template_ref" class="text-xs"><legend>{{ tr('启用操作','Enabled operations') }}</legend><label v-for="operation in templateOptions(c,'operations')" :key="operation" class="mr-3 inline-flex items-center gap-1"><input type="checkbox" :aria-label="tr('启用操作 ','Enable operation ') + operation" :checked="c.operations.includes(operation)" @change="edit(i,c => selectOperation(c,operation,($event.target as HTMLInputElement).checked))" />{{ operation }}</label><p v-if="!c.media_type || !c.operations.length" class="mt-1 text-amber-600">{{ tr('请明确选择输出媒体类型及至少一项操作。','Choose an output media type and at least one operation.') }}</p></fieldset>
        <MediaParameterRulesEditor v-if="c.template_ref" class="sm:col-span-2" :model-value="c.template_rules || []" @update:model-value="rules => edit(i,c=>c.template_rules=rules)" />
        <label v-if="c.template_ref" class="text-xs sm:col-span-2">{{ tr('高级规则 JSON','Advanced rules JSON') }}<textarea class="input mt-1 w-full font-mono text-xs" :aria-label="tr('账号模板规则覆盖','Account template rule overrides')" :value="JSON.stringify(c.template_rules || [],null,2)" @change="setRules(i,$event)" /></label>
        <template v-if="c.media_type === 'video'"><label class="text-xs">{{ tr('时长规则','Duration rule') }}<select class="input" :value="c.duration.mode" @change="edit(i,c => c.duration.mode = val($event))"><option value="fixed">{{ tr('固定','Fixed') }}</option><option value="enum">{{ tr('档位','Options') }}</option><option value="range">{{ tr('范围','Range') }}</option><option value="unknown">{{ tr('未知','Unknown') }}</option></select></label><label v-if="['fixed','enum'].includes(c.duration.mode)" class="text-xs">{{ tr('秒数','Seconds') }}<input class="input" :value="c.duration.values.join(', ')" @change="edit(i,c => c.duration.values = csv(val($event)).map(Number))" /></label><template v-if="c.duration.mode === 'range'"><label class="text-xs">{{ tr('最短秒数','Minimum seconds') }}<input type="number" min="1" class="input" :value="c.duration.min" @change="edit(i,c => c.duration.min = Number(val($event)))" /></label><label class="text-xs">{{ tr('最长秒数','Maximum seconds') }}<input type="number" min="1" class="input" :value="c.duration.max" @change="edit(i,c => c.duration.max = Number(val($event)))" /></label></template><label class="text-xs">{{ tr('默认秒数','Default seconds') }}<input type="number" min="1" class="input" :value="c.duration.default" @change="edit(i,c => c.duration.default = val($event) ? Number(val($event)) : null)" /></label><label class="flex items-center gap-2 text-xs"><input type="checkbox" :checked="c.duration.required" @change="edit(i,c => c.duration.required = ($event.target as HTMLInputElement).checked)" />{{ tr('请求必须指定时长','Explicit duration required') }}</label></template>
        <label v-if="c.media_type === 'image'" class="text-xs">{{ tr('图片像素尺寸','Image sizes') }}<input class="input" :value="c.sizes.join(', ')" @change="edit(i,c => c.sizes = csv(val($event)))" /></label>
        <template v-if="c.media_type && c.media_type !== 'audio'"><label v-for="field in ['resolutions','aspect_ratios','qualities'] as const" :key="field" class="text-xs">{{ ({resolutions:tr('分辨率档位','Resolution tiers'),aspect_ratios:tr('宽高比','Aspect ratios'),qualities:tr('质量','Quality')})[field] }}<input class="input" :value="c[field]?.join(', ')" @change="edit(i,c => c[field] = csv(val($event)))" /></label><label class="text-xs">{{ tr('分辨率规则','Resolution policy') }}<select class="input" :value="c.resolution_policy" @change="edit(i,c => c.resolution_policy = val($event))"><option value="optional">{{ tr('可选','Optional') }}</option><option value="required">{{ tr('必填','Required') }}</option><option value="omit">{{ tr('省略','Omit') }}</option></select></label></template>
      </div><details class="mt-3 text-xs"><summary class="cursor-pointer">{{ tr('协议与执行限制覆盖','Protocol and execution overrides') }}</summary><div class="mt-3 grid gap-3 sm:grid-cols-2"><label>{{ tr('请求格式','Request formats') }}<input class="input" :value="c.formats.join(', ')" @change="edit(i,c => c.formats = csv(val($event)))" /></label><label>{{ tr('接口用途','Operations') }}<input class="input" :value="c.operations.join(', ')" @change="edit(i,c => c.operations = csv(val($event)))" /></label><label>{{ tr('允许产物域名','Allowed artifact hosts') }}<input class="input" :value="c.artifact_hosts.join(', ')" @change="edit(i,c => c.artifact_hosts = csv(val($event)))" /></label><label v-for="field in ['max_request_bytes','max_concurrent_tasks','observation_seconds'] as const" :key="field">{{ ({max_request_bytes:tr('请求上限 bytes','Request limit bytes'),max_concurrent_tasks:tr('任务并发','Task concurrency'),observation_seconds:tr('观察期限秒','Observation seconds')})[field] }}<input class="input" type="number" min="1" :value="c.limits[field]" @change="edit(i,c => c.limits[field] = Number(val($event)))" /></label><label>{{ tr('查询间隔秒','Poll interval seconds') }}<input class="input" type="number" min="1" max="300" :value="c.delivery.poll_seconds" @change="edit(i,c => c.delivery.poll_seconds = Number(val($event)))" /></label><label v-for="(_,target) in c.usage_fields" :key="target">{{ target }}<input class="input" :value="c.usage_fields[target]" @change="edit(i,c => c.usage_fields[target] = val($event))" /></label><template v-for="(material,kind) in c.materials" :key="kind"><label>{{ kind }} · {{ tr('素材数量上限','Maximum reference count') }}<input class="input" type="number" min="0" :value="material.max_count" @change="edit(i,c => c.materials[kind].max_count = Number(val($event)))" /></label><label>{{ tr('素材文件上限 bytes','Reference file limit bytes') }}<input class="input" type="number" min="1" :value="material.max_file_bytes" @change="edit(i,c => c.materials[kind].max_file_bytes = Number(val($event)))" /></label></template></div></details></div>
    </article></template></div>
    <AccountTestModal v-if="account && testingModel !== null" :show="true" :account="account" :initial-model-id="testingModel" @close="testingModel = null" />
  </section>
</template>
