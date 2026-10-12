<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaMediaModel, PlazaMediaCharge } from '@/api/modelPlaza'
const props = defineProps<{ models: PlazaMediaModel[]; multiplier: number; imageMultiplier?: number; videoMultiplier?: number }>()
const { locale } = useI18n()
const filter = ref('all')
const visible = computed(() => props.models.filter(m => filter.value === 'all' || m.media_type === filter.value))
const endpoints: Record<string,string> = {image:'POST /v1/images/generations',image_edit:'POST /v1/images/edits',gemini_image:'POST /v1beta/models/{model}:generateContent',video:'POST /v1/videos',audio:'POST /v1/audio/generations',chat_music:'POST /v1/chat/completions'}
function prices(model: PlazaMediaModel) { const seen = new Set<string>(); return model.prices.filter(p => { const signature = JSON.stringify(p.rules); if (seen.has(signature)) return false; seen.add(signature); return true }) }
const conditions = (when: Record<string,unknown>) => Object.entries(when).map(([k,v]) => `${k}: ${v}`).join(' · ')
const text = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
function label(charge: PlazaMediaCharge, model: PlazaMediaModel) {
  const units: Record<string, string> = {
    request: text('次', 'request'), output: model.media_type === 'image' ? text('张', 'image') : model.media_type === 'video' ? text('条', 'video') : text('首', 'track'),
    second: text('秒', 'second'), minute: text('分钟', 'minute'), token: text('token', 'token'), input_image: text('参考图', 'input image'),
    input_video_second: text('输入视频秒', 'input video second'), input_audio_second: text('输入音频秒', 'input audio second')
  }
  return units[charge.unit] ?? charge.unit
}
function effectiveMultiplier(model: PlazaMediaModel) {
  if (model.media_type === 'image' && props.imageMultiplier !== undefined) return props.imageMultiplier
  if (model.media_type === 'video' && props.videoMultiplier !== undefined) return props.videoMultiplier
  if (model.media_type === 'audio' && model.music_multiplier !== undefined) return model.music_multiplier
  return props.multiplier
}
</script>
<template>
  <div class="space-y-3"><div class="flex flex-wrap gap-2"><button v-for="kind in ['all','image','video','audio']" :key="kind" class="rounded-full px-3 py-1.5 text-sm" :class="filter === kind ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/40' : 'bg-gray-100 dark:bg-dark-800'" @click="filter = kind">{{ ({all:text('全部','All'),image:text('图片','Images'),video:text('视频','Video'),audio:text('音频','Audio')} as Record<string,string>)[kind] }}</button></div><div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-gray-700">
    <table class="w-full text-left text-sm"><thead class="bg-gray-50 dark:bg-gray-900"><tr><th class="p-3">{{ text('媒体模型 / 接口', 'Media model / operation') }}</th><th class="p-3">{{ text('模型价格 USD / 单位', 'Model price USD / unit') }}</th><th class="p-3">{{ text('能力与收费规则', 'Capabilities and billing') }}</th></tr></thead>
      <tbody><tr v-for="model in visible" :key="model.model" class="border-t dark:border-gray-700"><td class="p-3 align-top"><div class="font-medium">{{ model.model }}</div><div class="mt-1 text-xs text-gray-500"><p v-for="op in model.operations" :key="op" class="break-all">{{ endpoints[op] || op }}</p></div></td><td class="p-3 align-top"><div v-for="price in prices(model)" :key="price.operation" class="mb-3"><div class="text-xs text-gray-500">{{ text('基础模型价格', 'Base model price') }}</div><div v-for="rule in price.rules" :key="rule.priority"><p v-if="Object.keys(rule.when).length" class="mt-1 text-xs text-gray-500">{{ conditions(rule.when) }}</p><div v-for="charge in rule.charges" :key="charge.name" class="mt-1">
                <template v-if="charge.method === 'progressive'">
                  <p class="text-xs">{{ text('累进分档', 'Progressive tiers') }}</p>
                  <p v-for="(tier, index) in charge.tiers" :key="index">{{ index === 0 ? '0' : charge.tiers[index - 1].up_to }}–{{ tier.up_to ?? '∞' }} {{ label(charge, model) }}: {{ tier.unit_price === null ? text('待配置', 'Unconfigured') : `$${tier.unit_price}` }} / {{ label(charge, model) }}</p>
                </template>
                <p v-else-if="charge.method === 'package'">{{ charge.package_quantity }} {{ label(charge, model) }} {{ text('套餐总价', 'package total') }}: {{ charge.unit_price === null ? text('待配置', 'Unconfigured') : `$${charge.unit_price}` }}</p>
                <p v-else>{{ charge.unit_price === null ? text('待配置', 'Unconfigured') : `$${charge.unit_price}` }} / {{ label(charge, model) }}</p>
                <span v-if="charge.trigger === 'accepted'" class="text-xs text-amber-700 dark:text-amber-400">{{ text('受理后失败不退', 'Nonrefundable after acceptance') }}</span>
              </div></div><p v-if="price.time_multiplier && price.time_multiplier !== '1'" class="mt-1 text-xs">{{ text('当前模型分时倍率', 'Current model time multiplier') }} ×{{ price.time_multiplier }}</p><p v-if="price.maximum_charge !== null" class="mt-1 text-xs">{{ text('最高收费', 'Maximum charge') }} ${{ price.maximum_charge }}</p></div><p class="text-xs text-gray-500">{{ text('有效媒体倍率', 'Effective media multiplier') }} ×{{ effectiveMultiplier(model) }}</p></td><td class="p-3 align-top"><p>{{ model.resolutions?.join(' / ') }}</p><template v-for="(d, index) in model.duration_variants || [model.duration]" :key="index"><p v-if="d.mode === 'fixed'">{{ d.values[0] }}{{ text(' 秒固定时长', ' seconds, fixed') }}</p><p v-else-if="d.values?.length">{{ d.values.join(' / ') }} {{ text('秒', 'seconds') }}</p><p v-else-if="d.min">{{ d.min }}–{{ d.max }} {{ text('秒', 'seconds') }}</p></template><p class="mt-2 text-xs text-gray-500">{{ text('查询和下载不重复计费。分时倍率按创建任务时锁定。完整档位、取整和估价以对应价卡为准。', 'Queries and downloads are free. Time multipliers lock at creation. Tier and rounding details follow the price card.') }}</p></td></tr></tbody>
    </table>
  </div></div>
</template>
