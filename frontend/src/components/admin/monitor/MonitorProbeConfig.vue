<template>
  <section class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700">
    <label class="input-label">
      {{ t('probeConfig.type') }}
      <select class="input" data-testid="probe-type" :value="modelValue.probe_type" @change="set('probe_type', ($event.target as HTMLSelectElement).value)">
        <option value="text">{{ t('probeConfig.text') }}</option>
        <option v-if="supportsImages" value="image">{{ t('probeConfig.image') }}</option>
        <option value="custom">{{ t('probeConfig.custom') }}</option>
      </select>
    </label>
    <template v-if="modelValue.probe_type !== 'text'">
      <p class="text-xs text-amber-600 dark:text-amber-400">{{ t('probeConfig.manualHint') }}</p>
      <label class="flex items-center gap-2 text-sm">
        <input type="checkbox" data-testid="probe-schedule" :checked="modelValue.image_probe_schedule_enabled" @change="set('image_probe_schedule_enabled', ($event.target as HTMLInputElement).checked)" />
        {{ t('probeConfig.schedule') }}
      </label>
    </template>
    <template v-if="modelValue.probe_type === 'custom'">
      <label class="input-label">{{ t('probeConfig.method') }}
        <select class="input" :value="modelValue.custom_method" @change="set('custom_method', ($event.target as HTMLSelectElement).value)">
          <option v-for="method in methods" :key="method">{{ method }}</option>
        </select>
      </label>
      <label class="input-label">{{ t('probeConfig.path') }}
        <input class="input font-mono" data-testid="probe-path" :value="modelValue.custom_path" placeholder="/v1/custom/check" @input="set('custom_path', ($event.target as HTMLInputElement).value)" />
      </label>
      <p class="text-xs text-gray-500">{{ t('probeConfig.pathHint') }}</p>
      <p v-if="!pathValid" class="text-xs text-red-500" data-testid="probe-path-error">{{ t('probeConfig.pathHint') }}</p>
      <label class="input-label">{{ t('probeConfig.body') }}
        <textarea v-model="bodyText" data-testid="probe-body" rows="5" class="input font-mono text-xs" placeholder='{"prompt": "hello"}' @input="commitBody" />
      </label>
      <p v-if="bodyError" class="text-xs text-red-500">{{ t('probeConfig.invalidBody') }}</p>
      <label class="input-label">{{ t('probeConfig.responsePath') }}
        <input class="input font-mono" data-testid="probe-response-path" :value="modelValue.custom_response_json_path" placeholder="data.0.text" @input="set('custom_response_json_path', ($event.target as HTMLInputElement).value)" />
      </label>
      <p class="text-xs text-gray-500">{{ t('probeConfig.responseHint') }}</p>
      <label class="flex items-center gap-2 text-sm">
        <input type="checkbox" :checked="modelValue.custom_response_require_non_empty" @change="set('custom_response_require_non_empty', ($event.target as HTMLInputElement).checked)" />
        {{ t('probeConfig.nonEmpty') }}
      </label>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorProbeSettings, Provider } from '@/api/admin/channelMonitor'

const props = defineProps<{ modelValue: MonitorProbeSettings; provider: Provider }>()
const emit = defineEmits<{
  'update:modelValue': [value: MonitorProbeSettings]
  validity: [valid: boolean]
}>()
const { t } = useI18n()
const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
const supportsImages = computed(() => ['openai', 'gemini', 'grok'].includes(props.provider))
const bodyText = ref('')
const bodyError = ref(false)

function set(key: keyof MonitorProbeSettings, value: unknown) {
  const next = { ...props.modelValue, [key]: value }
  if (key === 'probe_type' && value !== props.modelValue.probe_type) next.image_probe_schedule_enabled = false
  emit('update:modelValue', next)
}
function commitBody() {
  try {
    const value = bodyText.value.trim() ? JSON.parse(bodyText.value) : null
    if (value !== null && (typeof value !== 'object' || Array.isArray(value))) throw new Error('object required')
    bodyError.value = false
    set('custom_body', value)
  } catch {
    bodyError.value = true
  }
}
watch(() => props.modelValue.custom_body, value => {
  bodyText.value = value == null ? '' : JSON.stringify(value, null, 2)
  bodyError.value = false
}, { immediate: true })
const pathValid = computed(() => {
  const path = props.modelValue.custom_path.trim()
  if (!path || /[\\?#]/.test(path) || path.startsWith('//') || /^[a-z][a-z0-9+.-]*:/i.test(path)) return false
  try {
    return !decodeURIComponent(path).split('/').some(segment => segment === '.' || segment === '..')
  } catch { return false }
})
watch(() => [props.provider, props.modelValue.probe_type], () => {
  if (!supportsImages.value && props.modelValue.probe_type === 'image') set('probe_type', 'text')
}, { immediate: true })
watch(() => [bodyError.value, pathValid.value, props.modelValue.probe_type], () => {
  emit('validity', props.modelValue.probe_type !== 'custom' || (!bodyError.value && pathValid.value))
}, { immediate: true })
</script>
