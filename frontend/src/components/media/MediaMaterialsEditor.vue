<script setup lang="ts">
import { ref } from 'vue'

type MaterialRule = Record<string, unknown> & {
  kind?: string
  target?: string
  encoding?: string
  shape?: string
  role?: string
  single?: boolean
  max_count?: number
  drop_properties?: Record<string, unknown[]>
}

const props = defineProps<{ modelValue: MaterialRule[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: MaterialRule[]] }>()
const filterErrors = ref<Record<number, string>>({})
const mediaTypes = [{ value: 'image', label: '图片' }, { value: 'video', label: '视频' }, { value: 'audio', label: '音频' }, { value: 'mask', label: '遮罩' }]
const encodings = [{ value: 'preserve', label: '保持原样' }, { value: 'data_url', label: 'Data URL' }, { value: 'base64', label: '纯 Base64' }]
const shapes = [
  { value: 'preserve', label: '保留对象结构' }, { value: 'string', label: '字符串 URL / 数据' },
  { value: 'url', label: '包含 url 的对象' }, { value: 'b64_json', label: '包含 b64_json 的对象' },
  { value: 'data_mime', label: 'data + mime_type 对象' }, { value: 'content', label: '多模态 content 对象' },
  { value: 'parts', label: 'inlineData 对象' }
]
function update(index: number, key: string, value: unknown) {
  const rows = JSON.parse(JSON.stringify(props.modelValue || [])) as MaterialRule[]
  rows[index] = { ...rows[index], [key]: value }
  emit('update:modelValue', rows)
}
function updateFilter(index: number, event: Event) {
  try {
    const value = JSON.parse((event.target as HTMLTextAreaElement).value || '{}')
    if (!value || Array.isArray(value) || typeof value !== 'object') throw new Error('请输入 JSON 对象，例如 {"role":["reference"]}')
    update(index, 'drop_properties', value)
    filterErrors.value = { ...filterErrors.value, [index]: '' }
  } catch (error) {
    filterErrors.value = { ...filterErrors.value, [index]: (error as Error).message }
  }
}
function add() {
  emit('update:modelValue', [...(props.modelValue || []), { kind: 'image', target: 'images', encoding: 'preserve', shape: 'preserve' }])
}
function remove(index: number) {
  emit('update:modelValue', (props.modelValue || []).filter((_, i) => i !== index))
  const errors = { ...filterErrors.value }; delete errors[index]; filterErrors.value = errors
}
function filterText(rule: MaterialRule) { return JSON.stringify(rule.drop_properties || {}, null, 2) }
</script>

<template>
  <section class="space-y-3" aria-label="媒体素材映射编辑器">
    <div class="rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-blue-950 dark:border-blue-900 dark:bg-blue-950/40 dark:text-blue-100">
      <p class="font-semibold">素材映射设置说明</p>
      <p class="mt-1">统一输入字段是 <code>images</code>、<code>videos</code>、<code>audios</code>。目标字段填写上游接口实际接收的字段名；编码决定 URL 或文件内容怎么发送；结构决定每项是字符串还是对象。</p>
      <p class="mt-1">属性过滤只移除明确列出的字段和值。比如上游拒绝普通参考图里的 <code>role: reference</code>，可填 <code>{"role":["reference"]}</code>；首帧、尾帧和其他属性不会被这条规则删除。若对象还含不能丢弃的属性，选择字符串结构会报错并要求调整规则。</p>
    </div>

    <div v-for="(rule, index) in modelValue || []" :key="index" class="space-y-3 rounded-lg border p-3 dark:border-dark-600" :aria-label="`素材映射 ${index + 1}`">
      <div class="flex items-center justify-between"><strong>素材映射 {{ index + 1 }}</strong><button type="button" class="btn btn-secondary" @click="remove(index)">删除</button></div>
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <label class="text-sm">输入类型<select class="input mt-1 w-full" :value="rule.kind || 'image'" @change="update(index, 'kind', ($event.target as HTMLSelectElement).value)"><option v-for="item in mediaTypes" :key="item.value" :value="item.value">{{ item.label }}</option></select></label>
        <label class="text-sm">上游目标字段<input class="input mt-1 w-full" :value="rule.target || ''" placeholder="例如 images、reference_images" @change="update(index, 'target', ($event.target as HTMLInputElement).value)" /></label>
        <label class="text-sm">素材编码<select class="input mt-1 w-full" :value="rule.encoding || 'preserve'" @change="update(index, 'encoding', ($event.target as HTMLSelectElement).value)"><option v-for="item in encodings" :key="item.value" :value="item.value">{{ item.label }}</option></select></label>
        <label class="text-sm">每项数据结构<select class="input mt-1 w-full" :value="rule.shape || 'preserve'" @change="update(index, 'shape', ($event.target as HTMLSelectElement).value)"><option v-for="item in shapes" :key="item.value" :value="item.value">{{ item.label }}</option></select></label>
        <label class="text-sm">只选择此 role（可留空）<input class="input mt-1 w-full" :value="rule.role || ''" placeholder="例如 first_frame" @change="update(index, 'role', ($event.target as HTMLInputElement).value)" /><span class="mt-1 block text-xs text-gray-500">留空表示处理该类型的全部素材；填写后只处理完全匹配的 role。</span></label>
        <label class="text-sm">最多素材数（0 表示不额外限制）<input type="number" min="0" class="input mt-1 w-full" :value="rule.max_count ?? 0" @change="update(index, 'max_count', Number(($event.target as HTMLInputElement).value))" /></label>
        <label class="flex items-center gap-2 self-center text-sm"><input type="checkbox" :checked="rule.single === true" @change="update(index, 'single', ($event.target as HTMLInputElement).checked)" />只发送一个素材，不包装成数组</label>
      </div>
      <label class="block text-sm">按精确值过滤属性（可选 JSON）<textarea class="input mt-1 min-h-20 w-full font-mono text-xs" :value="filterText(rule)" placeholder='{"role":["reference"]}' @change="updateFilter(index, $event)" /><span class="mt-1 block text-xs text-gray-500">字段和值都匹配时才移除。不能过滤素材 URL、Base64 内容；不匹配的属性会原样保留。</span><span v-if="filterErrors[index]" role="alert" class="text-xs text-red-600">{{ filterErrors[index] }}</span></label>
    </div>

    <button type="button" class="btn btn-secondary" @click="add">添加素材映射</button>
  </section>
</template>
