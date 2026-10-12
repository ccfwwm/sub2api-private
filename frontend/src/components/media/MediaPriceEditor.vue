<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaMediaCharge } from '@/api/modelPlaza'
const props = defineProps<{ config: Record<string, unknown> }>()
const emit = defineEmits<{ update: [config: Record<string, unknown>] }>()
const { locale } = useI18n()
const text = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
type Rule = { priority: number; when: Record<string, unknown>; charges: PlazaMediaCharge[] }
const rules = computed(() => (props.config.rules ?? []) as Rule[])
const scope = computed(() => props.config.scope as { kind: string; reference: string })
const reservation = computed(() => props.config.reservation as { maximum_charge: string | null; max_quantities: Record<string, string> })
const units: Record<string, [string, string, string]> = {
  request: ['次', 'request', 'task_count'], output: ['产物', 'output', 'successful_outputs'], second: ['秒', 'second', 'request_duration'],
  minute: ['分钟', 'minute', 'artifact_duration'], token: ['token', 'token', 'provider_tokens'], input_image: ['参考图', 'input image', 'accepted_input_images'],
  input_video_second: ['输入视频秒', 'input video second', 'accepted_input_video_duration'], input_audio_second: ['输入音频秒', 'input audio second', 'accepted_input_audio_duration']
}
const sources = (unit: string) => unit === 'second' || unit === 'minute' ? ['request_duration', 'fixed_duration', 'artifact_duration', 'provider_duration'] : [units[unit]?.[2]]
function update(run: (copy: Record<string, any>) => void) { const copy = JSON.parse(JSON.stringify(props.config)); run(copy); emit('update', copy) }
function setCharge(index: number, charge: number, field: string, value: unknown) { update(copy => { copy.rules[index].charges[charge][field] = value }) }
function setUnit(index: number, charge: number, unit: string) { update(copy => { Object.assign(copy.rules[index].charges[charge], { unit, quantity_source: units[unit][2], usage_field: unit === 'token' ? 'total_tokens' : null, fallback_source: null }) }) }
function addCharge(index: number) { update(copy => { copy.rules[index].charges.push({ name: `charge-${copy.rules[index].charges.length + 1}`, unit: 'request', unit_price: null, quantity_source: 'task_count', trigger: 'success', refundable: true, method: 'linear', package_quantity: null, tiers: [], rounding: { mode: 'none', step: '1', minimum: '0', scope: 'per_output' } }) }) }
function setMethod(index: number, charge: number, method: string) { update(copy => { Object.assign(copy.rules[index].charges[charge], { method, package_quantity: method === 'package' ? '30' : null, tiers: method === 'progressive' ? [{ up_to: '10', unit_price: null }, { up_to: null, unit_price: null }] : [] }) }) }
const value = (event: Event) => (event.target as HTMLInputElement).value
</script>
<template>
  <div class="space-y-4 rounded-lg bg-gray-50 p-4 dark:bg-gray-900">
    <div class="grid gap-3 sm:grid-cols-3">
      <label class="text-sm">{{ text('模型 ID', 'Model ID') }}<input class="input mt-1 w-full" :value="(config.model_match as any)?.value" @change="update(c => c.model_match.value = value($event))" /></label>
      <label class="text-sm">{{ text('配置范围', 'Scope') }}<select class="input mt-1 w-full" :value="scope?.kind" @change="update(c => c.scope.kind = value($event))"><option v-for="item in config.purpose === 'cost' ? ['account','account_group','supplier_template'] : ['group','channel','linked_template']" :key="item">{{ item }}</option></select></label>
      <label class="text-sm">{{ text('范围 ID', 'Scope reference') }}<input class="input mt-1 w-full" :value="scope?.reference" @change="update(c => c.scope.reference = value($event))" /></label>
      <label class="text-sm">{{ text('最高收费 USD', 'Maximum charge USD') }}<input class="input mt-1 w-full" inputmode="decimal" :value="reservation?.maximum_charge ?? ''" @change="update(c => c.reservation.maximum_charge = value($event) || null)" /></label>
      <label class="text-sm">{{ text('成功策略', 'Success policy') }}<select class="input mt-1 w-full" :value="config.success_policy" @change="update(c => c.success_policy = value($event))"><option value="any_output">{{ text('有效产物收费', 'Charge valid outputs') }}</option><option value="all_outputs">{{ text('全部成功收费', 'Require all outputs') }}</option></select></label>
    </div>
    <div v-for="(rule, ri) in rules" :key="ri" class="space-y-3 rounded-lg border p-3 dark:border-gray-700">
      <div class="flex flex-wrap gap-3"><label class="text-sm">{{ text('优先级', 'Priority') }}<input class="input ml-2 w-24" type="number" :value="rule.priority" @change="update(c => c.rules[ri].priority = Number(value($event)))" /></label><label class="min-w-52 flex-1 text-sm">{{ text('条件 JSON', 'Conditions JSON') }}<input class="input ml-2" :value="JSON.stringify(rule.when)" @change="update(c => c.rules[ri].when = JSON.parse(value($event)))" /></label></div>
      <div v-for="(charge, ci) in rule.charges" :key="ci" class="grid gap-3 border-t pt-3 sm:grid-cols-3 dark:border-gray-700">
        <label class="text-sm">{{ text('收费项名称', 'Charge name') }}<input class="input mt-1 w-full" :value="charge.name" @change="setCharge(ri,ci,'name',value($event))" /></label>
        <label class="text-sm">{{ text('计费单位', 'Billing unit') }}<select class="input mt-1 w-full" :value="charge.unit" @change="setUnit(ri,ci,value($event))"><option v-for="(names, unit) in units" :key="unit" :value="unit">{{ text(names[0],names[1]) }}</option></select></label>
        <label class="text-sm">{{ text('单价 USD（0 为免费）', 'Unit price USD (0 is free)') }}<input class="input mt-1 w-full" inputmode="decimal" :value="charge.unit_price ?? ''" @change="setCharge(ri,ci,'unit_price',value($event) || null)" /></label>
        <label class="text-sm">{{ text('数量来源', 'Quantity source') }}<select class="input mt-1 w-full" :value="charge.quantity_source" @change="setCharge(ri,ci,'quantity_source',value($event))"><option v-for="source in sources(charge.unit)" :key="source">{{ source }}</option></select></label>
        <label class="text-sm">{{ text('收费时机', 'Charge trigger') }}<select class="input mt-1 w-full" :value="charge.trigger" @change="update(c => { c.rules[ri].charges[ci].trigger = value($event); c.rules[ri].charges[ci].refundable = value($event) === 'success' })"><option value="success">{{ text('成功后收费', 'On success') }}</option><option v-if="charge.unit === 'request'" value="accepted">{{ text('受理后失败不退', 'Nonrefundable acceptance fee') }}</option></select></label>
        <label class="text-sm">{{ text('计价方式', 'Pricing method') }}<select class="input mt-1 w-full" :value="charge.method" @change="setMethod(ri,ci,value($event))"><option value="linear">{{ text('单价 × 数量', 'Price × quantity') }}</option><option value="package">{{ text('固定套餐', 'Fixed package') }}</option><option value="progressive">{{ text('累进档', 'Progressive tiers') }}</option></select></label>
        <label class="text-sm">{{ text('取整', 'Rounding') }}<select class="input mt-1 w-full" :value="charge.rounding.mode" @change="update(c => c.rules[ri].charges[ci].rounding.mode = value($event))"><option v-for="mode in ['none','ceil','floor','half_up']" :key="mode">{{ mode }}</option></select></label>
        <label class="text-sm">{{ text('步长 / 秒', 'Step / seconds') }}<input class="input mt-1 w-full" :value="charge.rounding.step" @change="update(c => c.rules[ri].charges[ci].rounding.step = value($event))" /></label>
        <label class="text-sm">{{ text('最低数量', 'Minimum quantity') }}<input class="input mt-1 w-full" :value="charge.rounding.minimum" @change="update(c => c.rules[ri].charges[ci].rounding.minimum = value($event))" /></label>
        <label class="text-sm">{{ text('取整范围', 'Rounding scope') }}<select class="input mt-1 w-full" :value="charge.rounding.scope" @change="update(c => c.rules[ri].charges[ci].rounding.scope = value($event))"><option value="per_output">{{ text('逐产物', 'Per output') }}</option><option value="aggregate">{{ text('汇总', 'Aggregate') }}</option></select></label>
        <label v-if="charge.unit === 'token'" class="text-sm">{{ text('用量字段', 'Usage field') }}<select class="input mt-1 w-full" :value="(charge as any).usage_field" @change="setCharge(ri,ci,'usage_field',value($event))"><option v-for="field in ['total_tokens','input_tokens','output_tokens','image_input_tokens','image_output_tokens']" :key="field">{{ field }}</option></select></label>
        <label v-if="charge.method === 'package'" class="text-sm">{{ text('套餐数量', 'Package quantity') }}<input class="input mt-1 w-full" :value="charge.package_quantity" @change="setCharge(ri,ci,'package_quantity',value($event))" /></label>
        <label v-if="charge.method === 'progressive'" class="text-sm sm:col-span-3">{{ text('累进档 JSON（数量上界与单价）', 'Progressive tiers JSON (upper bound and price)') }}<textarea class="input mt-1 w-full font-mono text-xs" :value="JSON.stringify(charge.tiers)" @change="setCharge(ri,ci,'tiers',JSON.parse(value($event)))" /></label>
        <button v-if="rule.charges.length > 1" class="btn btn-secondary" @click="update(c => c.rules[ri].charges.splice(ci,1))">{{ text('删除收费项', 'Remove charge') }}</button>
      </div>
      <button class="btn btn-secondary" @click="addCharge(ri)">{{ text('增加组合收费项', 'Add composite charge') }}</button>
    </div>
    <button class="btn btn-secondary" @click="update(c => c.rules.push({ priority: Math.max(0,...rules.map(r => r.priority)) + 10, when: {}, charges: JSON.parse(JSON.stringify(rules[0]?.charges ?? [])) }))">{{ text('增加条件档位', 'Add conditional rule') }}</button>
  </div>
</template>
