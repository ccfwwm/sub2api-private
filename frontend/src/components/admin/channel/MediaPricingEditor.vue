<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { newMediaCharge, newMediaPricing, mediaUnits, quantitySources, unitNames, type MediaPricing } from '@/types/media'
const props = defineProps<{ modelValue?: MediaPricing }>()
const emit = defineEmits<{ 'update:modelValue': [MediaPricing] }>()
const { locale } = useI18n()
const tr = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
function change(fn: (p: MediaPricing) => void) { const p: MediaPricing = structuredClone(props.modelValue ? JSON.parse(JSON.stringify(props.modelValue)) : newMediaPricing()); fn(p); emit('update:modelValue', p) }
const val = (e: Event) => (e.target as HTMLInputElement).value
const sourceNames: Record<string, string> = { task_count: '任务数', successful_outputs: '成功产物数', request_duration: '请求时长', fixed_duration: '模型固定时长', artifact_duration: '实际产物时长', provider_duration: '上游计费时长', provider_tokens: '上游 token', accepted_input_images: '参考图数量', accepted_input_video_duration: '参考视频时长', accepted_input_audio_duration: '参考音频时长' }
</script>
<template>
  <div class="mt-3 space-y-4">
    <div class="grid gap-3 sm:grid-cols-3">
      <label class="text-xs">{{ tr('媒体类型', 'Media type') }}<select class="input mt-1" :value="modelValue?.media_type || 'video'" @change="change(p => p.media_type = val($event))"><option value="image">{{ tr('图片', 'Image') }}</option><option value="video">{{ tr('视频', 'Video') }}</option><option value="audio">{{ tr('音频', 'Audio') }}</option></select></label>
      <label class="text-xs">{{ tr('最高收费 USD（未知用量必填）', 'Maximum charge USD (required for unknown usage)') }}<input class="input mt-1" inputmode="decimal" :value="modelValue?.reservation.maximum_charge" placeholder="0.00" @input="change(p => p.reservation.maximum_charge = val($event) || null)" /></label>
      <label class="text-xs">{{ tr('多产物策略', 'Output policy') }}<select class="input mt-1" :value="modelValue?.success_policy || 'any_output'" @change="change(p => p.success_policy = val($event))"><option value="any_output">{{ tr('有成功产物即结算', 'Settle successful outputs') }}</option><option value="all_outputs">{{ tr('全部成功才收费', 'Charge only if all succeed') }}</option></select></label>
    </div>
    <p class="text-xs text-gray-500">{{ tr('手动配置 USD 价格。空值为未配置，0 为免费；不导入供应商价格。', 'Configure USD prices manually. Empty means unconfigured; 0 means free.') }}</p>
    <div v-for="(rule, ri) in (modelValue || newMediaPricing()).rules" :key="ri" class="rounded-lg border p-3 dark:border-dark-600">
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <strong>{{ tr('价格档', 'Price tier') }} {{ ri + 1 }}</strong>
        <label>{{ tr('优先级', 'Priority') }} <input type="number" class="input inline-block w-20" :value="rule.priority" @input="change(p => p.rules[ri].priority = Number(val($event)))" /></label>
        <button v-if="ri > 0" type="button" class="text-red-600" @click="change(p => p.rules.splice(ri, 1))">{{ tr('删除档位', 'Remove tier') }}</button>
      </div>
      <div class="my-3 grid gap-2 sm:grid-cols-3">
        <label v-for="field in ['resolution', 'size', 'quality', 'scene', 'seconds', 'generate_audio']" :key="field" class="text-xs">{{ ({resolution:tr('分辨率','Resolution'),size:tr('尺寸','Size'),quality:tr('质量','Quality'),scene:tr('场景','Scenario'),seconds:tr('时长（秒）','Duration (s)'),generate_audio:tr('带音频 true/false','Audio true/false')} as Record<string,string>)[field] }}<input class="input mt-1" :value="rule.when[field]" :placeholder="tr('留空：不限', 'Empty: any')" @input="change(p => { const v = val($event); if (!v) delete p.rules[ri].when[field]; else p.rules[ri].when[field] = field === 'generate_audio' ? v === 'true' : v })" /></label>
      </div>
      <div v-for="(charge, ci) in rule.charges" :key="ci" class="mb-3 space-y-2 rounded-lg bg-white p-3 dark:bg-dark-900">
        <div class="grid gap-2 sm:grid-cols-3">
          <label class="text-xs">{{ tr('收费项名称', 'Charge name') }}<input class="input mt-1" :value="charge.name" @input="change(p => p.rules[ri].charges[ci].name = val($event))" /></label>
          <label class="text-xs">{{ tr('计费单位', 'Unit') }}<select class="input mt-1" :value="charge.unit" @change="change(p => { const c = newMediaCharge(val($event)); c.unit_price = charge.unit_price; p.rules[ri].charges[ci] = c })"><option v-for="unit in mediaUnits" :key="unit" :value="unit">{{ tr(...unitNames[unit]) }}</option></select></label>
          <label class="text-xs">{{ tr('单价 USD / 单位', 'Price USD / unit') }}<input class="input mt-1" inputmode="decimal" :value="charge.unit_price" :placeholder="tr('必须填写；0 = 免费', 'Required; 0 = free')" @input="change(p => p.rules[ri].charges[ci].unit_price = val($event) || null)" /></label>
          <label class="text-xs">{{ tr('数量来源', 'Quantity source') }}<select class="input mt-1" :value="charge.quantity_source" @change="change(p => p.rules[ri].charges[ci].quantity_source = val($event))"><option v-for="source in quantitySources" :key="source" :value="source">{{ tr(sourceNames[source] || source, source) }}</option></select></label>
          <label class="text-xs">{{ tr('收费时机', 'Charge event') }}<select class="input mt-1" :value="charge.trigger" @change="change(p => { p.rules[ri].charges[ci].trigger = val($event); p.rules[ri].charges[ci].refundable = val($event) !== 'accepted' })"><option value="success">{{ tr('成功后收费', 'On success') }}</option><option value="accepted">{{ tr('受理费（失败不退）', 'On acceptance (non-refundable)') }}</option></select></label>
          <label v-if="charge.unit === 'token'" class="text-xs">{{ tr('用量字段', 'Usage field') }}<select class="input mt-1" :value="charge.usage_field" @change="change(p => p.rules[ri].charges[ci].usage_field = val($event))"><option v-for="f in ['total_tokens','input_tokens','output_tokens','image_input_tokens','image_output_tokens']" :key="f">{{ f }}</option></select></label>
        </div>
        <details class="text-xs"><summary class="cursor-pointer py-2">{{ tr('取整、套餐与累进档位', 'Rounding, package and progressive tiers') }}</summary>
          <div class="grid gap-2 sm:grid-cols-4">
            <label>{{ tr('算法', 'Method') }}<select class="input mt-1" :value="charge.method" @change="change(p => p.rules[ri].charges[ci].method = val($event))"><option value="linear">{{ tr('单价 × 数量', 'Linear') }}</option><option value="package">{{ tr('固定套餐', 'Package') }}</option><option value="progressive">{{ tr('累进档位', 'Progressive') }}</option></select></label>
            <label>{{ tr('取整', 'Rounding') }}<select class="input mt-1" :value="charge.rounding.mode" @change="change(p => p.rules[ri].charges[ci].rounding.mode = val($event))"><option v-for="m in ['none','ceil','floor','half_up']" :key="m">{{ m }}</option></select></label>
            <label>{{ tr('步长', 'Step') }}<input class="input mt-1" :value="charge.rounding.step" @input="change(p => p.rules[ri].charges[ci].rounding.step = val($event))" /></label>
            <label>{{ tr('最低数量', 'Minimum') }}<input class="input mt-1" :value="charge.rounding.minimum" @input="change(p => p.rules[ri].charges[ci].rounding.minimum = val($event))" /></label>
            <label>{{ tr('取整范围', 'Rounding scope') }}<select class="input mt-1" :value="charge.rounding.scope" @change="change(p => p.rules[ri].charges[ci].rounding.scope = val($event))"><option value="per_output">{{ tr('逐产物', 'Per output') }}</option><option value="aggregate">{{ tr('汇总', 'Aggregate') }}</option></select></label>
            <label v-if="charge.method === 'package'">{{ tr('套餐数量', 'Package quantity') }}<input class="input mt-1" :value="charge.package_quantity" @input="change(p => p.rules[ri].charges[ci].package_quantity = val($event))" /></label>
          </div>
          <div v-if="charge.method === 'progressive'" class="mt-2 space-y-2"><div v-for="(tier, ti) in charge.tiers" :key="ti" class="flex gap-2"><input class="input" :value="tier.up_to" :placeholder="tr('数量上限（末档可空）', 'Up to (last may be empty)')" @input="change(p => p.rules[ri].charges[ci].tiers[ti].up_to = val($event) || null)" /><input class="input" :value="tier.unit_price" :placeholder="tr('本档单价 USD', 'Tier price USD')" @input="change(p => p.rules[ri].charges[ci].tiers[ti].unit_price = val($event) || null)" /><button type="button" @click="change(p => p.rules[ri].charges[ci].tiers.splice(ti, 1))">×</button></div><button type="button" class="btn btn-secondary" @click="change(p => p.rules[ri].charges[ci].tiers.push({up_to:null,unit_price:null}))">{{ tr('添加累进档', 'Add progressive tier') }}</button></div>
        </details>
        <button v-if="rule.charges.length > 1" type="button" class="text-xs text-red-600" @click="change(p => p.rules[ri].charges.splice(ci, 1))">{{ tr('删除收费项', 'Remove charge') }}</button>
      </div>
      <button type="button" class="btn btn-secondary text-xs" @click="change(p => p.rules[ri].charges.push({...newMediaCharge(), name:`charge_${p.rules[ri].charges.length + 1}`}))">{{ tr('添加组合收费项', 'Add charge') }}</button>
    </div>
    <button type="button" class="btn btn-secondary text-xs" @click="change(p => p.rules.push({priority:p.rules.length,when:{},charges:[newMediaCharge()]}))">{{ tr('添加条件价格档', 'Add conditional price tier') }}</button>
  </div>
</template>
