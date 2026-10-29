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
  map_properties?: Record<string, string>
  property_rules?: Record<string, unknown>[]
}

const props = defineProps<{ modelValue: MaterialRule[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: MaterialRule[]] }>()
const filterErrors = ref<Record<number, string>>({})
const mappingErrors = ref<Record<number, string>>({})
const propertyErrors = ref<Record<number, string>>({})
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
function updateMapping(index: number, event: Event) {
  try {
    const value = JSON.parse((event.target as HTMLTextAreaElement).value || '{}')
    if (!value || Array.isArray(value) || typeof value !== 'object' || Object.values(value).some(target => typeof target !== 'string')) {
      throw new Error('请输入 JSON 字符串映射，例如 {"role":"metadata.frame_role"}')
    }
    update(index, 'map_properties', value)
    mappingErrors.value = { ...mappingErrors.value, [index]: '' }
  } catch (error) {
    mappingErrors.value = { ...mappingErrors.value, [index]: (error as Error).message }
  }
}
function add() {
  emit('update:modelValue', [...(props.modelValue || []), { kind: 'image', target: 'images', encoding: 'preserve', shape: 'preserve' }])
}
function updatePropertyRules(index: number, event: Event) {
  try {
    const value = JSON.parse((event.target as HTMLTextAreaElement).value || '[]')
    if (!Array.isArray(value) || value.some(rule => !rule || typeof rule !== 'object' || !['map', 'convert'].includes(rule.op) || typeof rule.source !== 'string')) {
      throw new Error('请输入 map / convert 规则数组，并指定 source 字段路径')
    }
    update(index, 'property_rules', value)
    propertyErrors.value = { ...propertyErrors.value, [index]: '' }
  } catch (error) {
    propertyErrors.value = { ...propertyErrors.value, [index]: (error as Error).message }
  }
}
function remove(index: number) {
  emit('update:modelValue', (props.modelValue || []).filter((_, i) => i !== index))
  const errors = { ...filterErrors.value }; delete errors[index]; filterErrors.value = errors
  const mappings = { ...mappingErrors.value }; delete mappings[index]; mappingErrors.value = mappings
}
function filterText(rule: MaterialRule) { return JSON.stringify(rule.drop_properties || {}, null, 2) }
function mappingText(rule: MaterialRule) { return JSON.stringify(rule.map_properties || {}, null, 2) }
</script>

<template>
  <section class="space-y-3" aria-label="媒体素材映射编辑器">
    <div class="rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-blue-950 dark:border-blue-900 dark:bg-blue-950/40 dark:text-blue-100">
      <p class="font-semibold">素材默认按用户输入原样透传</p>
      <p class="mt-1">统一输入字段是 <code>images</code>、<code>videos</code>、<code>audios</code>。默认保留每项的定位字段和值：<code>url</code> 仍是 <code>url</code>，<code>b64_json</code> 仍是 <code>b64_json</code>，素材顺序及其他属性也不变。只有明确配置编码、结构、映射或过滤规则时才改动。</p>
      <p class="mt-1">上游目标字段填写素材数组名称。属性映射和过滤使用对象路径；例如 <code>role</code> 映射到 <code>metadata.frame_role</code>，或只过滤精确匹配的 <code>metadata.internal: true</code>。默认空映射、空过滤，不会猜测或改写上游字段。</p>
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
      <div class="grid gap-3 md:grid-cols-2">
        <label class="block text-sm">对象属性映射（可选 JSON）<textarea class="input mt-1 min-h-24 w-full font-mono text-xs" :value="mappingText(rule)" placeholder='{"role":"metadata.frame_role","b64_json":"image.data"}' @change="updateMapping(index, $event)" /><span class="mt-1 block text-xs text-gray-500">键是用户字段路径，值是上游字段路径。对象结构下可显式映射 <code>url</code> 或 <code>b64_json</code> 定位字段；保留原字段类型和值。</span><span v-if="mappingErrors[index]" role="alert" class="text-xs text-red-600">{{ mappingErrors[index] }}</span></label>
        <label class="block text-sm">按精确值过滤属性（可选 JSON）<textarea class="input mt-1 min-h-24 w-full font-mono text-xs" :value="filterText(rule)" placeholder='{"metadata.internal":[true]}' @change="updateFilter(index, $event)" /><span class="mt-1 block text-xs text-gray-500">字段路径和值都匹配时才移除。不能过滤素材 URL、Base64 内容；不匹配的属性会原样保留。</span><span v-if="filterErrors[index]" role="alert" class="text-xs text-red-600">{{ filterErrors[index] }}</span></label>
      </div>
      <label class="block text-sm">属性值映射与类型转换（可选 JSON）<textarea class="input mt-1 min-h-24 w-full font-mono text-xs" :value="JSON.stringify(rule.property_rules || [], null, 2)" placeholder='[{"op":"map","source":"metadata.role","values":{"reference":"input"},"on_unmapped":"preserve"},{"op":"convert","source":"metadata.weight","type":"number"}]' @change="updatePropertyRules(index, $event)" /><span class="mt-1 block text-xs text-gray-500">按过滤 → 字段映射 → 属性值规则执行。map 明确指定值映射，convert 支持 string、number、integer、boolean；未配置的属性原样保留，不改变 url / b64_json 素材内容。</span><span v-if="propertyErrors[index]" role="alert" class="text-xs text-red-600">{{ propertyErrors[index] }}</span></label>
    </div>

    <button type="button" class="btn btn-secondary" @click="add">添加素材映射</button>
  </section>
</template>
