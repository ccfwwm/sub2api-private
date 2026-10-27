<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { newMediaCapability, type MediaCapability } from '@/types/media'
import { mediaProtocolsAPI } from '@/api/media-protocols'
import type { MediaConfigRecord } from '@/api/media'

interface ModelConfig { capability?: MediaCapability; operation?: string; parameters?: Record<string, unknown> }
interface MediaConfig { mode: 'connection' | 'generation'; revision?: string; models: Record<string, ModelConfig> }
const props = defineProps<{ models: string[]; modelValue: Record<string, unknown> | null }>()
const emit = defineEmits<{ 'update:modelValue': [Record<string, unknown>]; validity: [boolean] }>()
const { locale } = useI18n()
const tr = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
const selected = ref(''), error = ref(''), advanced = ref('')
const config = computed<MediaConfig>(() => (props.modelValue?.media_monitor as MediaConfig) || { mode: 'connection', models: {} })
const models = computed(() => [...new Set(props.models.map(m => m.trim()).filter(Boolean))])
const templates = ref<MediaConfigRecord[]>([]), loadingTemplates = ref(true), templateError = ref('')
const pinnedTemplates = ref<Record<string, Record<string, unknown> | null>>({})
const publishedTemplates = computed(() => templates.value.filter(row => row.published_version && (row.published_config ?? row.config).status === 'enabled'))
const refKey = (ref: NonNullable<MediaCapability['template_ref']>) => `${ref.id}:${ref.version}`
function templateFor(cap: MediaCapability): Record<string, unknown> | undefined {
  if (!cap.template_ref) return
  const row = publishedTemplates.value.find(row => row.id === cap.template_ref?.id && row.published_version === cap.template_ref.version)
  return row ? row.published_config ?? row.config : pinnedTemplates.value[refKey(cap.template_ref)] ?? undefined
}
const options = (cap: MediaCapability, field: 'media_types' | 'operations'): string[] => {
  const values = templateFor(cap)?.[field]
  return Array.isArray(values) ? values.filter((value): value is string => typeof value === 'string') : []
}
async function loadPinnedTemplates() {
  for (const entry of Object.values(config.value.models)) {
    const cap = entry.capability, reference = cap?.template_ref
    if (!cap || !reference || templateFor(cap) || refKey(reference) in pinnedTemplates.value) continue
    pinnedTemplates.value[refKey(reference)] = null
    const history = await mediaProtocolsAPI.history('templates', reference.id)
    const pinned = history.find(row => row.version === reference.version && row.status === 'enabled')
    if (pinned) pinnedTemplates.value[refKey(reference)] = pinned.config
  }
}
async function loadTemplates() {
  loadingTemplates.value = true; templateError.value = ''; pinnedTemplates.value = {}
  try {
    templates.value = await mediaProtocolsAPI.list('templates')
    await loadPinnedTemplates()
  } catch { templateError.value = tr('模板目录加载失败；已保存配置保留，请重试。', 'Template catalogue failed to load. Saved settings are preserved; retry loading.') }
  finally { loadingTemplates.value = false }
}
onMounted(loadTemplates)
watch(() => JSON.stringify(Object.values(config.value.models).map(entry => entry.capability?.template_ref)), () => {
  if (!loadingTemplates.value) void loadPinnedTemplates().catch(() => { templateError.value = tr('固定模板版本加载失败；已保存配置保留。', 'Pinned template failed to load. Saved settings are preserved.') })
})
watch(models, all => { if (!all.includes(selected.value)) selected.value = all[0] || '' }, { immediate: true })
const entry = computed(() => config.value.models[selected.value] || { capability: newMediaCapability(selected.value), parameters: {} })
const capability = computed(() => {
  const base = newMediaCapability(selected.value)
  const cap = entry.value.capability || base
  return { ...base, ...cap, duration: { ...base.duration, ...cap.duration }, operations: cap.operations || [], sizes: cap.sizes || [], resolutions: cap.resolutions || [], aspect_ratios: cap.aspect_ratios || [], qualities: cap.qualities || [] }
})
const params = computed(() => entry.value.parameters || {})
const defaultPrompt = computed(() => ({ image: 'A simple blue circle on a plain white background.', video: 'A fixed camera watching a blue ball slowly roll across a plain white floor.', audio: 'A short quiet instrumental piano melody, no vocals.' })[capability.value.media_type] || '')
const durations = computed(() => capability.value.duration.values || [])
const minimumSeconds = computed(() => durations.value.length ? Math.min(...durations.value) : capability.value.duration.min || 5)
const generationIssue = computed(() => {
  if (config.value.mode !== 'generation') return ''
  if (loadingTemplates.value) return tr('正在核验固定模板版本；不会生成。', 'Verifying pinned templates; generation is blocked.')
  if (templateError.value) return templateError.value
  if (!models.value.length) return tr('请先配置模型；不会生成。', 'Configure a model before generation.')
  for (const model of models.value) {
    const entry = config.value.models[model], cap = entry?.capability
    if (!cap || cap.model !== model || typeof cap.upstream_model !== 'string' || !cap.upstream_model.trim() || !cap.template_ref || !templateFor(cap)
      || !options(cap, 'media_types').includes(cap.media_type) || !entry.operation
      || !Array.isArray(cap.operations) || !cap.operations.includes(entry.operation) || !options(cap, 'operations').includes(entry.operation)) {
      return `${model}: ` + tr('请明确配置已发布的固定模板版本、输出类型和操作；未配置时不会生成。', 'Choose a published pinned template, output type and operation before generation.')
    }
  }
  return ''
})
watch(generationIssue, issue => emit('validity', !issue), { immediate: true })
function write(fn: (c: MediaConfig) => void) {
  const c: MediaConfig = JSON.parse(JSON.stringify(config.value)); fn(c)
  emit('update:modelValue', { ...props.modelValue, media_monitor: c })
}
function changeMode(mode: string) {
  write(c => { c.mode = mode as MediaConfig['mode'] })
}
function edit(fn: (c: ModelConfig) => void) {
  if (!selected.value) return
  write(c => {
    c.models[selected.value] ||= JSON.parse(JSON.stringify(entry.value))
    fn(c.models[selected.value])
  })
}
function parameter(key: string, value: unknown) { edit(c => { c.parameters ||= {}; if (value === '') delete c.parameters[key]; else c.parameters[key] = value }) }
function chooseTemplate(value: string) {
  const row = publishedTemplates.value.find(row => `${row.id}:${row.published_version}` === value)
  if (!row?.published_version) return
  const version = row.published_version
  edit(c => {
    c.capability ||= newMediaCapability(selected.value)
    const cap = c.capability
    const template = row.published_config ?? row.config
    const mediaTypes = template.media_types as string[], operations = template.operations as string[]
    cap.template_ref = { id: row.id, version }; cap.adapter = 'template'; cap.request_format = 'unified'
    cap.media_type = mediaTypes.includes(cap.media_type) ? cap.media_type : mediaTypes.length === 1 ? mediaTypes[0] : ''
    cap.operations = cap.operations.filter(operation => operations.includes(operation))
    if (!cap.operations.length && operations.length === 1) cap.operations = [...operations]
    c.operation = cap.operations.includes(c.operation || '') ? c.operation : cap.operations.length === 1 ? cap.operations[0] : ''
    cap.duration.required = cap.media_type === 'video'
  })
}
function upstreamModel(value: string) {
  edit(c => {
    c.capability ||= newMediaCapability(selected.value)
    c.capability.upstream_model = value.trim()
  })
}
function chooseMediaType(value: string) { if (options(capability.value, 'media_types').includes(value)) edit(c => { c.capability!.media_type = value; c.capability!.duration.required = value === 'video' }) }
function chooseOperation(value: string) { if (options(capability.value, 'operations').includes(value)) edit(c => { c.capability!.operations = [value]; c.operation = value }) }
function applyAdvanced() {
  try {
    const cap: MediaCapability = JSON.parse(advanced.value)
    if (!cap || Array.isArray(cap) || cap.model !== selected.value || typeof cap.upstream_model !== 'string' || !cap.upstream_model.trim()
      || !cap.template_ref || !Number.isInteger(cap.template_ref.version) || cap.template_ref.version < 1 || !templateFor(cap)
      || !options(cap, 'media_types').includes(cap.media_type)
      || !Array.isArray(cap.operations) || !cap.operations.length || cap.operations.some(op => typeof op !== 'string')
      || cap.operations.some(op => !options(cap, 'operations').includes(op)) || 'template' in cap
      || !cap.duration || typeof cap.duration !== 'object' || Array.isArray(cap.duration)
      || (cap.duration.values != null && (!Array.isArray(cap.duration.values) || cap.duration.values.some(n => typeof n !== 'number')))
      || (['sizes', 'resolutions', 'aspect_ratios', 'qualities'] as const).some(field => cap[field] != null && (!Array.isArray(cap[field]) || cap[field].some(v => typeof v !== 'string')))) throw new Error()
    edit(c => { c.capability = cap; c.operation = cap.operations[0] }); error.value = ''
  } catch { error.value = tr('请输入有效的模型能力 JSON，模型名称必须一致。', 'Enter valid capability JSON with the same model name.') }
}
const val = (e: Event) => (e.target as HTMLInputElement).value
</script>

<template>
  <section class="space-y-3 rounded-lg border border-sky-200 p-4 dark:border-sky-800" data-testid="monitor-media-config">
    <h3 class="text-sm font-semibold">{{ tr('媒体检测配置', 'Media detection settings') }}</h3>
    <label class="input-label">{{ tr('检测内容', 'Detection type') }}
      <select class="input mt-1" data-testid="media-monitor-mode" :value="config.mode" @change="changeMode(val($event))">
        <option value="connection">{{ tr('连接检查（默认，不生成）', 'Connection check (default, no generation)') }}</option>
        <option value="generation">{{ tr('生成检测（上游可能收费）', 'Generation probe (upstream may charge)') }}</option>
      </select>
    </label>
    <p class="text-xs text-gray-500">{{ config.mode === 'connection' ? tr('读取模型列表，检查鉴权和所选模型是否可见。连接通过不代表生成通过。', 'Checks authentication and model visibility using the model list. This does not verify generation.') : tr('按检测间隔生成一个产物；异步任务未结束时只查询原任务。失败记为本轮失败；确认上游明确拒绝后，下一轮自动重新探测。已受理或状态不确定的请求只查询原任务，避免重复提交。', 'Generates one output per detection cycle. Pending tasks are only queried. Failures are recorded for the current cycle; a definite rejection is probed again on the next cycle. Accepted or uncertain requests only query the original task to avoid duplicate submissions.') }}</p>
    <label v-if="models.length" class="input-label">{{ tr('配置模型', 'Configure model') }}<select v-model="selected" class="input mt-1"><option v-for="model in models" :key="model">{{ model }}</option></select></label>
    <p class="text-xs text-gray-500">{{ tr('模型名称须与该 Key 的 /v1/models 返回值完全一致。上游模型是实际发送的模型 ID；模型名称与域名不决定协议。', 'Use the exact model ID advertised by this key. The upstream model is the ID sent in the request; names and domains do not select a protocol.') }}</p>
    <p v-if="templateError" role="alert" class="text-xs text-red-600">{{ templateError }} <button type="button" @click="loadTemplates">{{ tr('重试加载', 'Retry loading') }}</button></p>
    <p v-if="generationIssue" role="alert" data-testid="media-monitor-generation-blocked" class="text-xs text-amber-600">{{ generationIssue }}</p>
    <div v-if="selected" class="grid gap-3 sm:grid-cols-2">
      <label class="input-label">{{ tr('协议模板', 'Protocol template') }}<select class="input mt-1" data-testid="media-monitor-template" :disabled="loadingTemplates || !!templateError" :value="capability.template_ref ? refKey(capability.template_ref) : ''" @change="chooseTemplate(val($event))"><option value="">{{ tr('请选择已发布模板', 'Choose a published template') }}</option><option v-if="capability.template_ref && !publishedTemplates.some(row => row.id === capability.template_ref?.id && row.published_version === capability.template_ref.version)" :value="refKey(capability.template_ref)">{{ capability.template_ref.id }} · v{{ capability.template_ref.version }} · {{ tr('已存固定版本', 'Saved pinned version') }}</option><option v-for="row in publishedTemplates" :key="row.id" :value="`${row.id}:${row.published_version}`">{{ (row.published_config ?? row.config).name }} · v{{ row.published_version }}</option></select></label>
      <label class="input-label">{{ tr('上游模型', 'Upstream model') }}<input data-testid="media-monitor-upstream" class="input mt-1" :value="capability.upstream_model" @change="upstreamModel(val($event))" /></label>
      <template v-if="capability.template_ref">
        <label class="input-label">{{ tr('输出媒体类型', 'Output media type') }}<select class="input mt-1" data-testid="media-monitor-type" :value="capability.media_type" @change="chooseMediaType(val($event))"><option value="">{{ tr('请选择类型', 'Choose a type') }}</option><option v-if="capability.media_type && !options(capability, 'media_types').includes(capability.media_type)" :value="capability.media_type">{{ capability.media_type }} · {{ tr('已存配置', 'Saved setting') }}</option><option v-for="type in options(capability, 'media_types')" :key="type">{{ type }}</option></select></label>
        <label class="input-label">{{ tr('操作', 'Operation') }}<select class="input mt-1" data-testid="media-monitor-operation" :value="entry.operation || ''" @change="chooseOperation(val($event))"><option value="">{{ tr('请选择操作', 'Choose an operation') }}</option><option v-if="entry.operation && !options(capability, 'operations').includes(entry.operation)" :value="entry.operation">{{ entry.operation }} · {{ tr('已存配置', 'Saved setting') }}</option><option v-for="operation in options(capability, 'operations')" :key="operation">{{ operation }}</option></select></label>
      </template>
      <template v-if="config.mode === 'generation'">
        <label class="input-label sm:col-span-2">{{ tr('提示词', 'Prompt') }}<textarea data-testid="media-monitor-prompt" class="input mt-1" :value="String(params.prompt ?? defaultPrompt)" @input="parameter('prompt', val($event))" /><button class="mt-1 text-xs text-primary-600" type="button" @click="parameter('prompt', '')">{{ tr('恢复默认提示词', 'Restore default prompt') }}</button></label>
        <label v-if="capability.media_type === 'video'" class="input-label">{{ tr('时长（秒）', 'Duration (seconds)') }}<select v-if="durations.length" class="input mt-1" :value="params.seconds ?? minimumSeconds" @change="parameter('seconds', Number(val($event)))"><option v-for="seconds in durations" :key="seconds" :value="seconds">{{ seconds }}</option></select><input v-else class="input mt-1" type="number" :min="capability.duration.min || 1" :max="capability.duration.max || 600" :value="params.seconds ?? minimumSeconds" @change="parameter('seconds', Number(val($event)))" /></label>
        <label v-if="capability.media_type === 'audio'" class="input-label">{{ tr('时长（秒）', 'Duration (seconds)') }}<input type="number" min="1" max="600" class="input mt-1" :value="params.seconds ?? 5" @change="parameter('seconds', Number(val($event)))" /></label>
        <label v-if="capability.media_type === 'image' && capability.sizes.length" class="input-label">{{ tr('图片尺寸', 'Image size') }}<select class="input mt-1" :value="params.size ?? capability.sizes[0]" @change="parameter('size', val($event))"><option v-for="size in capability.sizes" :key="size">{{ size }}</option></select></label>
        <label v-for="field in (['resolutions', 'aspect_ratios', 'qualities'] as const)" v-show="capability[field].length" :key="field" class="input-label">{{ ({resolutions: tr('分辨率', 'Resolution'), aspect_ratios: tr('宽高比', 'Aspect ratio'), qualities: tr('质量', 'Quality')})[field] }}<select class="input mt-1" :value="params[({resolutions: 'resolution', aspect_ratios: 'ratio', qualities: 'quality'})[field]] ?? ''" @change="parameter(({resolutions: 'resolution', aspect_ratios: 'ratio', qualities: 'quality'})[field], val($event))"><option value="">{{ tr('最小默认值', 'Minimal default') }}</option><option v-for="choice in capability[field]" :key="choice">{{ choice }}</option></select></label>
      </template>
    </div>
    <details v-if="selected" class="text-xs"><summary class="cursor-pointer" @click="advanced = JSON.stringify(capability, null, 2)">{{ tr('高级能力覆盖（未知模型时长、允许产物域名等）', 'Advanced capability overrides (duration, artifact hosts, etc.)') }}</summary><textarea v-model="advanced" class="input mt-2 h-48 font-mono" /><button type="button" class="btn btn-secondary mt-2" @click="applyAdvanced">{{ tr('应用能力配置', 'Apply capability') }}</button></details>
    <button v-if="config.mode === 'generation'" type="button" class="text-xs text-amber-600" @click="write(c => { c.revision = String(Date.now()) })">{{ tr('重置检测任务（下一轮允许重新生成）', 'Reset task (allow a new generation next cycle)') }}</button>
    <p v-if="config.revision" class="text-xs text-gray-500">{{ tr('已设置任务重置；保存后生效。', 'Task reset takes effect after saving.') }}</p>
    <p v-if="error" class="text-xs text-red-600" role="alert">{{ error }}</p>
  </section>
</template>
