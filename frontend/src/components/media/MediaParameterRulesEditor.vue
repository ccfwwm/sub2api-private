<script setup lang="ts">
import { ref } from 'vue'
const props = defineProps<{ modelValue: Record<string, unknown>[] }>()
const emit = defineEmits<{ 'update:modelValue': [Record<string, unknown>[]] }>()
const error = ref('')
const value = (event: Event) => (event.target as HTMLInputElement).value
const cloneRules = () => JSON.parse(JSON.stringify(props.modelValue)) as Record<string, unknown>[]
function set(index: number, field: string, next: unknown) {
  const rules = cloneRules()
  rules[index][field] = next
  emit('update:modelValue', rules)
}
function setOperation(index: number, op: string) {
  const rules = cloneRules(), previous = rules[index]
  const rule: Record<string, unknown> = { phase: previous.phase, op }
  if (op === 'allow') rule.fields = []
  else {
    if (!['default', 'fixed'].includes(op)) rule.source = previous.source || ''
    if (op !== 'remove') rule.target = previous.target || ''
    if (['default', 'fixed'].includes(op)) rule.value = ''
    if (op === 'map') { rule.values = {}; rule.on_unmapped = 'error' }
    if (op === 'convert') rule.type = 'string'
  }
  rules[index] = rule
  emit('update:modelValue', rules)
}
function mappings(rule: Record<string, unknown>): [string, unknown][] {
  return Object.entries((rule.values || {}) as Record<string, unknown>)
}
function editMapping(index: number, row: number, field: 'key' | 'value', raw: string) {
  try {
    const entries = mappings(props.modelValue[index])
    if (field === 'key') {
      if (entries.some(([key], i) => i !== row && key === raw)) throw new Error('映射原值不能重复。')
      entries[row]![0] = raw
    } else {
      const type = valueType({ value: entries[row]![1] })
      const parsed = type === 'string' ? raw : JSON.parse(raw)
      if ((type === 'number' || type === 'boolean') && typeof parsed !== type) throw new Error('映射结果与所选类型不符。')
      if (type === 'json' && (parsed === null || typeof parsed !== 'object')) throw new Error('映射结果需要对象或数组。')
      entries[row]![1] = parsed
    }
    set(index, 'values', Object.fromEntries(entries)); error.value = ''
  } catch (cause) { error.value = cause instanceof SyntaxError ? '映射结果格式无效，请核对所选类型。' : (cause as Error).message }
}
function mappingType(index: number, row: number, type: string) {
  const entries = mappings(props.modelValue[index])
  entries[row]![1] = ({ string: '', number: 0, boolean: false, null: null, json: {} } as Record<string, unknown>)[type]
  set(index, 'values', Object.fromEntries(entries))
}
function addMapping(index: number) {
  const entries = mappings(props.modelValue[index]), keys = new Set(entries.map(([key]) => key))
  let key = '新值'; while (keys.has(key)) key += '_'
  set(index, 'values', Object.fromEntries([...entries, [key, '']]))
}
function removeMapping(index: number, row: number) {
  set(index, 'values', Object.fromEntries(mappings(props.modelValue[index]).filter((_, i) => i !== row)))
}
function valueType(rule: Record<string, unknown>) {
  return rule.value === null ? 'null' : typeof rule.value === 'object' ? 'json' : typeof rule.value === 'number' ? 'number' : typeof rule.value === 'boolean' ? 'boolean' : 'string'
}
function setValue(index: number, event: Event) {
  try {
    const raw = value(event), type = valueType(props.modelValue[index])
    const parsed = type === 'string' ? raw : JSON.parse(raw)
    if (type === 'number' && typeof parsed !== 'number') throw new Error()
    if (type === 'boolean' && typeof parsed !== 'boolean') throw new Error()
    if (type === 'json' && (parsed === null || typeof parsed !== 'object')) throw new Error()
    set(index, 'value', parsed); error.value = ''
  } catch { error.value = '值格式无效，请核对所选类型。' }
}
function ruleHint(op: string) {
  const hints: Record<string, string> = {
    default: '仅当目标字段缺失时补值。例：目标 quality，类型 number，值 1；不会覆盖调用方已有值。',
    fixed: '无论调用方传什么都写入固定值。例：目标 model，类型 string，值 provider-image-v2。',
    rename: '把源字段移动到目标字段并移除源字段。例：resolution → output_resolution；路径不要相互包含。',
    copy: '复制源字段到目标字段并保留原字段。例：prompt → text，两个字段会同时存在。',
    map: '按原值精确匹配后替换。例：quality 的 high → hd；未匹配值可报错或保留。',
    convert: '将源值转换成指定基础类型。例：字符串 "5" 转为 integer 5；无效数字会报错。',
    wrap: '把源值放进目标对象路径。例：prompt → input.text 会生成 {"input":{"text":"..."}}。',
    remove: '删除源字段。例：删除客户端内部字段 debug；不能填写目标字段。',
    allow: '只保留列出的字段，其余字段会移除。例：model,prompt,quality；确认素材字段也包含在列表中。'
  }
  return hints[op] || '先选择规则类型，再按示例填写字段路径和值。'
}
</script>
<template>
  <fieldset class="space-y-3 rounded-lg border p-3 dark:border-dark-600">
    <legend class="px-1 text-sm font-medium">请求参数规则</legend>
    <p class="text-xs text-gray-500">阶段决定规则运行时点：能力与计价前用于统一、校验用户输入；上游发送前用于改写协议字段。源/目标支持点号路径，例如 contents.parts；模型字段由“模型写入目标”设置。</p>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <div v-for="(rule,index) in modelValue" :key="index" class="grid gap-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-800 sm:grid-cols-3">
      <label class="text-xs">阶段<select class="input w-full" :value="rule.phase" @change="set(index,'phase',value($event))"><option value="semantic">请求能力与计价前</option><option value="wire">上游发送前</option></select></label>
      <label class="text-xs">规则<select class="input w-full" :aria-label="'规则类型 ' + (index+1)" :value="rule.op" @change="setOperation(index,value($event))"><option v-for="[op,label] in [['default','默认值'],['fixed','固定值'],['rename','重命名'],['copy','复制'],['map','值映射'],['convert','类型转换'],['wrap','包装'],['remove','移除'],['allow','保留指定字段']]" :key="op" :value="op">{{ label }}</option></select></label>
      <button type="button" class="text-right text-xs text-red-600" @click="emit('update:modelValue',modelValue.filter((_,i)=>i!==index))">删除规则 {{ index+1 }}</button>
      <p class="sm:col-span-3 rounded border border-blue-100 bg-blue-50 px-2 py-1.5 text-xs leading-relaxed text-blue-900 dark:border-blue-900 dark:bg-blue-950 dark:text-blue-100">{{ ruleHint(String(rule.op)) }}</p>
      <label v-if="!['default','fixed','allow'].includes(String(rule.op))" class="text-xs">源字段<input class="input w-full" :placeholder="rule.op === 'map' ? 'quality' : 'resolution / contents.parts'" :value="rule.source" @change="set(index,'source',value($event))" /></label>
      <label v-if="!['remove','allow'].includes(String(rule.op))" class="text-xs">目标字段<input class="input w-full" :placeholder="rule.op === 'wrap' ? 'input.text' : 'output_resolution'" :value="rule.target" @change="set(index,'target',value($event))" /></label>
      <template v-if="['default','fixed'].includes(String(rule.op))">
        <label class="text-xs">值类型<select class="input w-full" :aria-label="'值类型 ' + (index+1)" :value="valueType(rule)" @change="set(index,'value',({string:'',number:0,boolean:false,null:null,json:{}} as Record<string,unknown>)[value($event)])"><option value="string">文本</option><option value="number">数字</option><option value="boolean">布尔值</option><option value="null">空值</option><option value="json">对象 / 数组</option></select></label>
        <label class="text-xs">值<input class="input w-full" :aria-label="'规则值 ' + (index+1)" :disabled="rule.value === null" :value="typeof rule.value === 'string' ? rule.value : JSON.stringify(rule.value)" @change="setValue(index,$event)" /></label>
      </template>
      <label v-if="rule.op === 'allow'" class="text-xs sm:col-span-2">保留字段（逗号分隔）<input class="input w-full" :value="(rule.fields as string[] || []).join(', ')" @change="set(index,'fields',value($event).split(',').map(v=>v.trim()).filter(Boolean))" /></label>
      <label v-if="rule.op === 'convert'" class="text-xs">转换类型<select class="input w-full" :value="rule.type" @change="set(index,'type',value($event))"><option v-for="type in ['string','number','integer','boolean']" :key="type">{{ type }}</option></select></label>
      <label v-if="rule.op === 'map'" class="text-xs">未匹配时<select class="input w-full" :value="rule.on_unmapped || 'error'" @change="set(index,'on_unmapped',value($event))"><option value="error">报错</option><option value="preserve">保留原值</option></select></label>
      <fieldset v-if="rule.op === 'map'" class="space-y-2 sm:col-span-3">
        <legend class="text-xs">值映射表（结果支持文本、数字、布尔值、对象及数组）</legend>
        <div v-for="([key,mapped],row) in mappings(rule)" :key="row" class="flex flex-wrap items-end gap-2">
          <label class="text-xs">原值<input class="input block" :aria-label="'映射原值 ' + (index+1) + '-' + (row+1)" :value="key" @change="editMapping(index,row,'key',value($event))" /></label>
          <label class="text-xs">结果类型<select class="input block" :aria-label="'映射类型 ' + (index+1) + '-' + (row+1)" :value="valueType({value:mapped})" @change="mappingType(index,row,value($event))"><option value="string">文本</option><option value="number">数字</option><option value="boolean">布尔值</option><option value="null">空值</option><option value="json">对象 / 数组</option></select></label>
          <label class="text-xs">结果<input class="input block" :aria-label="'映射结果 ' + (index+1) + '-' + (row+1)" :disabled="mapped === null" :value="typeof mapped === 'string' ? mapped : JSON.stringify(mapped)" @change="editMapping(index,row,'value',value($event))" /></label>
          <button type="button" class="text-xs text-red-600" @click="removeMapping(index,row)">删除映射 {{ row+1 }}</button>
        </div>
        <button type="button" class="text-xs text-primary-600" @click="addMapping(index)">添加值映射</button>
      </fieldset>
    </div>
    <button type="button" class="btn btn-secondary" @click="emit('update:modelValue',[...modelValue,{phase:'semantic',op:'default',target:'',value:''}])">添加参数规则</button>
  </fieldset>
</template>
