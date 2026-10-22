<script setup lang="ts">
import { computed, ref } from 'vue'

const props = defineProps<{ modelValue: Record<string, unknown> }>()
const emit = defineEmits<{ 'update:modelValue': [value: Record<string, unknown>] }>()
const error = ref('')
const fields = [
  { key: 'task_id', label: '上游任务 ID', example: 'id\ndata.id', hint: '异步提交响应里的唯一任务编号，用于后续查询。' },
  { key: 'status', label: '任务状态', example: 'status\ndata.status', hint: '读取上游状态后，按下面的状态表转换。' },
  { key: 'progress', label: '任务进度', example: 'progress\ndata.progress', hint: '可选，读取任务百分比或进度字段。' },
  { key: 'errors', label: '错误说明', example: 'error.message\nmessage', hint: '读取失败原因；保留具体错误便于排查。' },
  { key: 'urls', label: '成品 URL', example: 'data[].url\noutput.video_url', hint: '填写真正的图片、视频或音频直链字段。' },
  { key: 'base64', label: '成品 Base64', example: 'data[].b64_json', hint: '仅在上游直接返回编码媒体时使用。' },
]
const states = computed(() => Object.entries((props.modelValue.states || {}) as Record<string, string>))
function update(key: string, value: unknown) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}
function paths(key: string) { return ((props.modelValue[key] || []) as string[]).join('\n') }
function updatePaths(key: string, event: Event) {
  update(key, (event.target as HTMLTextAreaElement).value.split('\n').map(v => v.trim()).filter(Boolean))
}
function state(index: number, side: number, value: string) {
  const entries = states.value.map(row => [...row])
  entries[index][side] = value
  const keys = entries.map(row => row[0].trim())
  if (new Set(keys).size !== keys.length) { error.value = '上游状态不能重复'; return }
  error.value = ''
  update('states', Object.fromEntries(entries.map(([key, target]) => [key.trim(), target])))
}
function advanced(event: Event) {
  try {
    const value = JSON.parse((event.target as HTMLTextAreaElement).value)
    if (!value || Array.isArray(value) || typeof value !== 'object') throw new Error('请输入 JSON 对象')
    emit('update:modelValue', value); error.value = ''
  } catch { error.value = '高级响应配置必须是有效 JSON 对象，已保留之前的配置。' }
}
</script>

<template>
  <section class="space-y-4" aria-label="任务响应解析编辑器">
    <p class="rounded-lg bg-gray-50 p-3 text-sm text-gray-700 dark:bg-dark-700 dark:text-gray-200">这里填写上游响应中各字段的位置。每行一个候选路径，例如 <code>data.id</code>；<code>[]</code> 表示遍历数组。先粘贴真实响应样例，再用下方免费预览核对解析结果。</p>
    <div class="grid gap-4 sm:grid-cols-2">
      <label v-for="field in fields" :key="field.key" class="text-sm">{{ field.label }}<textarea class="input mt-1 min-h-20 w-full font-mono text-xs" :aria-label="field.label + '路径'" :value="paths(field.key)" :placeholder="field.example" @change="updatePaths(field.key, $event)" /><span class="mt-1 block text-xs text-gray-500">{{ field.hint }}</span></label>
      <label class="text-sm">默认成品 MIME<input class="input mt-1 w-full" aria-label="默认成品 MIME" :value="modelValue.mime || ''" placeholder="例如 video/mp4、image/png、audio/mpeg" @change="update('mime', ($event.target as HTMLInputElement).value)" /></label>
    </div>
    <fieldset class="space-y-2"><legend class="mb-2 text-sm font-semibold">上游状态转换</legend><p class="text-xs text-gray-500">例如上游 succeeded 转为已完成，queued 转为处理中。只转换明确配置的值。</p>
      <div v-for="([source, target], index) in states" :key="index" class="flex gap-2"><input class="input min-w-0 flex-1" :aria-label="`上游状态 ${index + 1}`" :value="source" placeholder="上游返回值，如 succeeded" @change="state(index, 0, ($event.target as HTMLInputElement).value)" /><select class="input min-w-0 flex-1" :aria-label="`统一状态 ${index + 1}`" :value="target" @change="state(index, 1, ($event.target as HTMLSelectElement).value)"><option value="in_progress">处理中</option><option value="completed">已完成</option><option value="failed">失败</option><option value="unknown">待核查</option></select><button type="button" class="btn btn-secondary" :aria-label="`删除状态 ${index + 1}`" @click="update('states', Object.fromEntries(states.filter((_, i) => i !== index)))">删除</button></div>
      <button type="button" class="btn btn-secondary" @click="update('states', { ...Object.fromEntries(states), ['state_' + (states.length + 1)]: 'in_progress' })">添加状态转换</button>
    </fieldset>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <details><summary class="cursor-pointer text-sm">完整响应配置（结果项、用量、PCM 等高级字段）</summary><p class="my-2 text-xs text-gray-500">常用表单仅修改对应字段，其他高级字段会保留。复杂列表成品用 results 配置；请参照模板示例并用真实响应预览验证。</p><textarea class="input min-h-44 w-full font-mono text-xs" aria-label="完整响应配置 JSON" :value="JSON.stringify(modelValue, null, 2)" @change="advanced" /></details>
  </section>
</template>
