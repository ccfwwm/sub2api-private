<template>
  <section class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="image-param-rules">
    <label class="input-label">{{ t('probeConfig.accountRules') }}</label>
    <p class="text-xs text-gray-500">{{ t('probeConfig.accountRulesHint') }}</p>
    <label class="input-label">{{ t('probeConfig.drop') }}
      <input v-model="dropText" class="input font-mono" data-testid="image-param-drop" placeholder="response_format" @input="commit" />
    </label>
    <label class="input-label">{{ t('probeConfig.rename') }}
      <textarea v-model="renameText" class="input font-mono" data-testid="image-param-rename" rows="3" placeholder="size=resolution" @input="commit" />
    </label>
    <p v-if="invalid" class="text-xs text-red-500">{{ t('probeConfig.invalidRules') }}</p>
  </section>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
const props = defineProps<{ modelValue: unknown }>()
const emit = defineEmits<{ 'update:modelValue': [value: unknown]; validity: [valid: boolean] }>()
const { t } = useI18n()
const dropText = ref('')
const renameText = ref('')
const invalid = ref(false)
const namePattern = /^[A-Za-z_][A-Za-z0-9_.\[\]-]{0,127}$/
let committed: unknown
watch(() => props.modelValue, value => {
  if (value === committed) return
  const rules = value as { drop?: string[]; rename?: Record<string, string> } | null
  dropText.value = (rules?.drop || []).join(', ')
  renameText.value = Object.entries(rules?.rename || {}).map(([key, target]) => `${key}=${target}`).join('\n')
  invalid.value = false
  emit('validity', true)
}, { immediate: true })
function commit() {
  try {
    const drop = [...new Set(dropText.value.split(/[,\s]+/).filter(Boolean))]
    const rename: Record<string, string> = Object.create(null)
    const targets = new Set<string>()
    for (const line of renameText.value.split('\n').map(v => v.trim()).filter(Boolean)) {
      const [source, target, ...rest] = line.split('=').map(v => v.trim())
      if (!source || !target || rest.length || Object.prototype.hasOwnProperty.call(rename, source) || targets.has(target) || !namePattern.test(source) || !namePattern.test(target)) throw new Error('invalid mapping')
      rename[source] = target
      targets.add(target)
    }
    if (drop.some(key => !namePattern.test(key))) throw new Error('invalid field')
    committed = drop.length || Object.keys(rename).length ? { drop, rename } : null
    invalid.value = false
    emit('update:modelValue', committed)
  } catch {
    invalid.value = true
  }
  emit('validity', !invalid.value)
}
</script>
