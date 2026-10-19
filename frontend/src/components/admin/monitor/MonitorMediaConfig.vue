<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { newMediaCapability, type MediaCapability } from '@/types/media'

interface ModelConfig { capability?: MediaCapability; operation?: string; parameters?: Record<string, unknown> }
interface MediaConfig { mode: 'connection' | 'generation'; revision?: string; models: Record<string, ModelConfig> }
const props = defineProps<{ models: string[]; modelValue: Record<string, unknown> | null }>()
const emit = defineEmits<{ 'update:modelValue': [Record<string, unknown>] }>()
const { locale } = useI18n()
const tr = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
const selected = ref(''), error = ref(''), advanced = ref('')
const config = computed<MediaConfig>(() => (props.modelValue?.media_monitor as MediaConfig) || { mode: 'connection', models: {} })
const models = computed(() => [...new Set(props.models.map(m => m.trim()).filter(Boolean))])
watch(models, all => { if (!all.includes(selected.value)) selected.value = all[0] || '' }, { immediate: true })
const entry = computed(() => config.value.models[selected.value] || { capability: newMediaCapability(selected.value), parameters: {} })
const capability = computed(() => {
  const base = newMediaCapability(selected.value)
  const cap = entry.value.capability || base
  return { ...base, ...cap, duration: { ...base.duration, ...cap.duration }, operations: cap.operations || [], sizes: cap.sizes || [], resolutions: cap.resolutions || [], aspect_ratios: cap.aspect_ratios || [], qualities: cap.qualities || [] }
})
const params = computed(() => entry.value.parameters || {})
const modelSuggestion = computed(() => capability.value.upstream_model.replace(/^gpt-image(?=\d)/, 'gpt-image-'))
const defaultPrompt = computed(() => ({ image: 'A simple blue circle on a plain white background.', video: 'A fixed camera watching a blue ball slowly roll across a plain white floor.', audio: 'A short quiet instrumental piano melody, no vocals.' })[capability.value.media_type] || '')
const durations = computed(() => capability.value.duration.values || [])
const minimumSeconds = computed(() => durations.value.length ? Math.min(...durations.value) : capability.value.duration.min || 5)
const adapters = { yunfei_gpt_images: 'GPT Images', yunfei_gemini_native: 'Gemini Image', yunfei_omni: 'Omni / Veo', yunfei_veo_preview: 'Veo Preview', yunfei_grok: 'Grok Video', yunfei_lyria: 'Lyria Audio', v6k_video_gateway: 'V6K Video', ark_native: 'Ark Video' }
function write(fn: (c: MediaConfig) => void) {
  const c: MediaConfig = JSON.parse(JSON.stringify(config.value)); fn(c)
  c.models = Object.fromEntries(Object.entries(c.models).filter(([model]) => models.value.includes(model)))
  emit('update:modelValue', { media_monitor: c })
}
function changeMode(mode: string) {
  write(c => {
    c.mode = mode as MediaConfig['mode']
    if (mode === 'generation') for (const model of models.value) {
      const cap = newMediaCapability(model)
      if (cap.adapter && !c.models[model]) c.models[model] = { capability: cap, operation: cap.operations[0], parameters: {} }
    }
  })
}
function edit(fn: (c: ModelConfig) => void) {
  if (!selected.value) return
  write(c => {
    c.models[selected.value] ||= JSON.parse(JSON.stringify(entry.value))
    fn(c.models[selected.value])
  })
}
function parameter(key: string, value: unknown) { edit(c => { c.parameters ||= {}; if (value === '') delete c.parameters[key]; else c.parameters[key] = value }) }
function chooseAdapter(value: string) {
  edit(c => {
    c.capability ||= newMediaCapability(selected.value)
    const cap = c.capability
    cap.adapter = value
    cap.media_type = ['yunfei_gpt_images', 'yunfei_gemini_native'].includes(value) ? 'image' : value === 'yunfei_lyria' ? 'audio' : 'video'
    cap.operations = [value === 'yunfei_gemini_native' ? 'gemini_image' : cap.media_type === 'image' ? 'image' : cap.media_type === 'audio' ? 'audio' : 'video']
    c.operation = cap.operations[0]
    c.parameters = {}
  })
}
function upstreamModel(value: string) {
  edit(c => {
    c.capability ||= newMediaCapability(selected.value)
    const preset = newMediaCapability(value.trim())
    if (!c.capability.adapter && preset.adapter) {
      c.capability = { ...preset, model: selected.value, upstream_model: value.trim() }
      c.operation = preset.operations[0]
    } else c.capability.upstream_model = value.trim()
  })
}
function applyAdvanced() {
  try {
    const cap: MediaCapability = JSON.parse(advanced.value)
    if (!cap || Array.isArray(cap) || cap.model !== selected.value || typeof cap.upstream_model !== 'string' || !cap.upstream_model.trim()
      || !(cap.adapter in adapters) || !['image', 'video', 'audio'].includes(cap.media_type)
      || !Array.isArray(cap.operations) || !cap.operations.length || cap.operations.some(op => typeof op !== 'string')
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
    <p class="text-xs text-gray-500">{{ config.mode === 'connection' ? tr('读取模型列表，检查鉴权和所选模型是否可见。连接通过不代表生成通过。', 'Checks authentication and model visibility using the model list. This does not verify generation.') : tr('按检测间隔生成一个产物；异步任务未结束时只查询原任务。异常或结果不确定时停止提交，需手动重置。', 'Generates one output per detection cycle. Pending tasks are only queried. Failed or uncertain submissions require an explicit reset.') }}</p>
    <label v-if="models.length" class="input-label">{{ tr('配置模型', 'Configure model') }}<select v-model="selected" class="input mt-1"><option v-for="model in models" :key="model">{{ model }}</option></select></label>
    <p class="text-xs text-gray-500">{{ tr('模型名称须与该 Key 的 /v1/models 返回值完全一致，包括连字符；例如 gpt-image-2.5。上游模型是实际请求中发送的模型 ID。', 'Use the exact model ID returned by /v1/models for this key, including hyphens, e.g. gpt-image-2.5. The upstream model is the ID sent in the request.') }}</p>
    <p v-if="modelSuggestion !== capability.upstream_model" role="alert" class="text-xs text-amber-600">{{ tr('请核对模型名称，是否应为：', 'Check whether the model ID should be: ') }}{{ modelSuggestion }}</p>
    <div v-if="selected" class="grid gap-3 sm:grid-cols-2">
      <label class="input-label">{{ tr('视频请求格式','Video request format') }}<select class="input mt-1" v-model="capability.request_format"><option value="legacy">Legacy</option><option value="yunfei_multimodal">云飞 Base64 多媒体</option><option value="v6k_url">v6k 公网 URL</option></select></label>
      <label class="input-label">{{ tr('接口类型', 'Interface') }}<select class="input mt-1" :value="capability.adapter" @change="chooseAdapter(val($event))"><option value="">{{ tr('未知模型请选择接口', 'Choose an interface for unknown models') }}</option><option v-for="(label, value) in adapters" :key="value" :value="value">{{ label }}</option></select></label>
      <label class="input-label">{{ tr('上游模型', 'Upstream model') }}<input data-testid="media-monitor-upstream" class="input mt-1" :value="capability.upstream_model" @change="upstreamModel(val($event))" /></label>
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
