<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { mediaProtocolsAPI } from '@/api/media-protocols'
import type { MediaConfigRecord } from '@/api/media'
import { useI18n } from 'vue-i18n'
import type { MonitorProbePolicy, MonitorProbeConfig } from '@/api/admin/channelMonitor'

const props = defineProps<{
  modelValue?: MonitorProbeConfig
  models: string[]
  provider: string
  probeType: string
  body?: Record<string, unknown> | null
  bodyMode?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [MonitorProbeConfig]; validity: [boolean] }>()
const { t } = useI18n()
const selected = ref('')
const models = computed(() => [...new Set(props.models.map(v => v.trim()).filter(Boolean))])
const config = computed(() => props.modelValue ?? { defaults: {}, models: {} })
const current = computed(() => selected.value ? config.value.models?.[selected.value] ?? {} : config.value.defaults ?? {})
const templates = ref<MediaConfigRecord[]>([])
watch(() => props.provider, async provider => {
  if (provider === 'imagevideo') {
    try { templates.value = await mediaProtocolsAPI.list('templates') } catch { templates.value = [] }
  }
}, { immediate: true })
const media = computed(() => props.body?.media_monitor as { models?: Record<string, { capability?: { media_type?: string; execution_template?: { mode?: string }; template_ref?: { id: string } }; parameters?: { prompt?: string } }> } | undefined)
function kind(model: string) { return props.provider === 'imagevideo' ? media.value?.models?.[model]?.capability?.media_type ?? 'image' : props.probeType }
function defaults(model: string): MonitorProbePolicy {
  const type = kind(model)
  return { normal_latency_ms: ({ image: 90000, video: 300000, audio: 60000 } as Record<string, number>)[type] ?? 6000, request_timeout_ms: ['image', 'video', 'audio'].includes(type) ? 180000 : 45000, observation_timeout_ms: 3600000 }
}
function effective(model: string) { return { ...defaults(model), ...config.value.defaults, ...config.value.models?.[model] } }
const inherited = computed(() => selected.value ? { ...defaults(selected.value), ...config.value.defaults } : defaults(models.value[0] ?? ''))
const promptExternal = computed(() => props.probeType === 'custom' || (props.provider !== 'imagevideo' && props.bodyMode === 'replace'))
const valid = computed(() => (models.value.length ? models.value : ['']).every(model => {
  const p = effective(model)
  const cap = media.value?.models?.[model]?.capability
  const row = templates.value.find(row => row.id === cap?.template_ref?.id)
  const mode = cap?.execution_template?.mode ?? (row?.published_config ?? row?.config)?.mode
  const async = props.provider === 'imagevideo' && (mode === 'async' || (!mode && !!cap?.template_ref))
  const limit = async ? p.observation_timeout_ms! : p.request_timeout_ms!
  return (p.normal_latency_ms ?? 0) > 0 && p.normal_latency_ms! < limit
    && p.request_timeout_ms! >= 1000 && p.request_timeout_ms! <= 600000
    && p.observation_timeout_ms! >= 15000 && p.observation_timeout_ms! <= 86400000
    && new TextEncoder().encode(p.prompt ?? '').length <= 8192
}))
watch(valid, value => emit('validity', value), { immediate: true })
watch(models, value => { if (selected.value && !value.includes(selected.value)) selected.value = '' })
function update(key: keyof MonitorProbePolicy, value: string) {
  const c: MonitorProbeConfig = JSON.parse(JSON.stringify(config.value))
  c.defaults ??= {}; c.models ??= {}
  const target = selected.value ? (c.models[selected.value] ??= {}) : c.defaults
  if (value === '') delete target[key]
  else if (key === 'prompt') target.prompt = value
  else target[key] = Math.round(Number(value) * 1000)
  emit('update:modelValue', c)
}
const fields = ['normal_latency_ms', 'request_timeout_ms', 'observation_timeout_ms'] as const
</script>

<template>
  <section class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700" data-testid="monitor-policy">
    <h3 class="text-sm font-semibold">{{ t('monitorPolicy.title') }}</h3>
    <label class="input-label">{{ t('monitorPolicy.target') }}
      <select v-model="selected" class="input" data-testid="policy-model">
        <option value="">{{ t('monitorPolicy.defaults') }}</option>
        <option v-for="model in models" :key="model" :value="model">{{ model }}</option>
      </select>
    </label>
    <p class="text-xs text-gray-500">{{ t('monitorPolicy.inheritance') }}</p>
    <p v-if="promptExternal" class="text-xs text-gray-500">{{ t('monitorPolicy.externalPrompt') }}</p>
    <label v-else class="input-label">{{ t('monitorPolicy.prompt') }}
      <textarea class="input" rows="3" data-testid="policy-prompt" :value="current.prompt ?? ''" :placeholder="inherited.prompt || media?.models?.[selected]?.parameters?.prompt || t('monitorPolicy.defaultPrompt')" @input="update('prompt', ($event.target as HTMLTextAreaElement).value)" />
    </label>
    <div class="grid gap-3 sm:grid-cols-2">
      <template v-for="field in fields" :key="field">
        <label v-if="field !== 'observation_timeout_ms' || provider === 'imagevideo'" class="input-label">
          {{ t(`monitorPolicy.${field}`) }}
          <input class="input" type="number" step="0.001" :min="field === 'request_timeout_ms' ? 1 : field === 'observation_timeout_ms' ? 15 : 0.001" :max="field === 'request_timeout_ms' ? 600 : 86400" :data-testid="`policy-${field}`" :value="current[field] == null ? '' : current[field]! / 1000" :placeholder="String(inherited[field]! / 1000)" @input="update(field, ($event.target as HTMLInputElement).value)" />
        </label>
      </template>
    </div>
    <p class="text-xs text-gray-500">{{ t('monitorPolicy.latencyHint') }}</p>
    <p v-if="!valid" class="text-xs text-red-500" data-testid="policy-invalid">{{ t('monitorPolicy.invalid') }}</p>
  </section>
</template>
