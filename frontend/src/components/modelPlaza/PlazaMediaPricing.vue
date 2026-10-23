<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaMediaModel, PlazaMediaCharge, PlazaMediaReferencePrice } from '@/api/modelPlaza'
import { getPublicMediaCatalog, saveMediaAvailability, saveMediaDescription, saveMediaReferencePrice } from '@/api/modelPlaza'
import { useAuthStore } from '@/stores/auth'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
const props = defineProps<{ models: PlazaMediaModel[]; multiplier: number; imageMultiplier?: number; videoMultiplier?: number; groupId?: number; descriptionVersion?: number }>()
const auth = useAuthStore()
const editing = ref(''), draft = ref(''), saving = ref(false), editError = ref(''), conflict = ref(false)
const editKind = ref<'description' | 'price'>('description')
const priceAmount = ref(''), priceUnit = ref('')
const version = ref(props.descriptionVersion ?? 0)
const descriptions = ref<Record<string, string>>({})
const referencePrices = ref<Record<string, PlazaMediaReferencePrice | null>>({})
const availabilities = ref<Record<string, 'available' | 'unavailable'>>({})
const availabilitySaving = ref('')
const availabilityError = ref('')
const availabilityErrorModel = ref('')
watch(() => props.descriptionVersion, value => { if (!editing.value) { version.value = value ?? 0; descriptions.value = {}; referencePrices.value = {} } })
watch(() => props.models.map(model => ({ model: model.model, availability: model.availability })), models => {
  const next = { ...availabilities.value }
  for (const model of models) {
    if (model.model !== availabilitySaving.value) next[model.model] = model.availability ?? 'available'
  }
  availabilities.value = next
})
const description = (model: PlazaMediaModel) => descriptions.value[model.model] ?? model.display_description ?? ''
const referencePrice = (model: PlazaMediaModel) => Object.prototype.hasOwnProperty.call(referencePrices.value, model.model) ? referencePrices.value[model.model] : model.official_reference_price
const availability = (model: PlazaMediaModel) => availabilities.value[model.model] ?? model.availability ?? 'available'
function edit(model: PlazaMediaModel, kind: 'description' | 'price' = 'description') {
  if (saving.value) return
  editing.value = model.model; editKind.value = kind; draft.value = description(model)
  priceAmount.value = referencePrice(model)?.amount ?? ''; priceUnit.value = referencePrice(model)?.unit ?? '次'
  editError.value = ''; conflict.value = false
}
async function refreshVersion() {
  saving.value = true
  try {
    const group = (await getPublicMediaCatalog()).groups.find(g => g.id === props.groupId)
    if (!group) throw new Error('分组已不可用')
    version.value = group.description_version ?? 0
    for (const model of group.media_models ?? []) {
      descriptions.value[model.model] = model.display_description ?? ''
      referencePrices.value[model.model] = model.official_reference_price ?? null
      availabilities.value[model.model] = model.availability ?? 'available'
    }
    conflict.value = false; editError.value = '已刷新版本并保留草稿，请核对当前内容后保存。'
  } catch (e) { editError.value = (e as Error).message }
  finally { saving.value = false }
}
async function toggleAvailability(model: PlazaMediaModel, value: boolean) {
  if (!props.groupId || !auth.isAdmin || availabilitySaving.value) return
  const next = value ? 'available' : 'unavailable'
  const previous = availability(model)
  availabilitySaving.value = model.model
  availabilityError.value = ''
  availabilityErrorModel.value = model.model
  availabilities.value[model.model] = next
  try {
    const result = await saveMediaAvailability(props.groupId, model.model, next, version.value)
    availabilities.value[model.model] = result.availability
    version.value = result.description_version
  } catch (e) {
    availabilities.value[model.model] = previous
    const failure = e as { status?: number; response?: { status?: number }; message?: string }
    availabilityError.value = failure.status === 409 || failure.response?.status === 409
      ? '本分组内容已被其他管理员修改，请刷新后重试。'
      : failure.message || '保存模型状态失败，请重试。'
  } finally {
    availabilitySaving.value = ''
  }
}
async function save() {
  if (!props.groupId || conflict.value) return
  saving.value = true; editError.value = ''
  try {
    if (editKind.value === 'price') {
      const amount = priceAmount.value.trim(), unit = priceUnit.value.trim()
      if (amount && (!/^(0|[1-9][0-9]{0,11})(\.[0-9]{1,8})?$/.test(amount) || !unit || [...unit].length > 40)) {
        editError.value = '请输入非负美元金额（最多 12 位整数、8 位小数）和不超过 40 字的计价单位。'
        return
      }
      const result = await saveMediaReferencePrice(props.groupId, editing.value, amount ? { amount, unit } : null, version.value)
      referencePrices.value[editing.value] = result.official_reference_price
      version.value = result.description_version
    } else {
      const result = await saveMediaDescription(props.groupId, editing.value, draft.value, version.value)
      descriptions.value[editing.value] = result.display_description
      version.value = result.description_version
    }
    editing.value = ''
  } catch (e) {
    const failure = e as { status?: number; response?: { status?: number }; message?: string }
    conflict.value = failure.status === 409 || failure.response?.status === 409
    editError.value = conflict.value ? '本分组内容已被其他管理员修改。请刷新版本，核对后保存；草稿已保留。' : failure.message || '保存失败，请重试。'
  } finally { saving.value = false }
}
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
function effectiveMultiplier(model: PlazaMediaModel): number {
  if (model.sale_multiplier !== undefined) return model.sale_multiplier
  if (model.media_type === 'image' && props.imageMultiplier !== undefined) return props.imageMultiplier
  if (model.media_type === 'video' && props.videoMultiplier !== undefined) return props.videoMultiplier
  if (model.media_type === 'audio' && model.music_multiplier !== undefined) return Number(model.music_multiplier)
  return props.multiplier
}
function displayedPrice(model: PlazaMediaModel, price: string | null, timeMultiplier?: string) {
  if (price === null) return '—'
  const value = Number(price) * effectiveMultiplier(model) * Number(timeMultiplier || '1')
  return Number.isFinite(value) ? `¥${Number(value.toPrecision(10))}` : '—'
}
</script>
<template>
  <div class="space-y-3"><div class="flex flex-wrap gap-2"><button v-for="kind in ['all','image','video','audio']" :key="kind" class="rounded-full px-3 py-1.5 text-sm" :class="filter === kind ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/40' : 'bg-gray-100 dark:bg-dark-800'" @click="filter = kind">{{ ({all:text('全部','All'),image:text('图片','Images'),video:text('视频','Video'),audio:text('音频','Audio')} as Record<string,string>)[kind] }}</button></div><div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-gray-700">
    <table class="w-full text-left text-sm"><thead class="bg-gray-50 dark:bg-gray-900"><tr><th class="p-3">{{ text('媒体模型 / 接口', 'Media model / operation') }}</th><th class="p-3">{{ text('实付价格 ¥ / 单位', 'Sale price ¥ / unit') }}</th><th class="p-3">{{ text('官方参考价 $', 'Official reference price $') }}</th><th class="p-3">{{ text('能力与收费规则', 'Capabilities and billing') }}</th></tr></thead>
      <tbody><tr v-for="model in visible" :key="model.model" class="border-t dark:border-gray-700"><td class="p-3 align-top"><div class="flex flex-wrap items-center gap-2"><div class="font-medium">{{ model.model }}</div><span :data-testid="`media-model-status-${model.model}`" class="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium" :class="availability(model) === 'available' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-400' : 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-400'"><Icon :name="availability(model) === 'available' ? 'checkCircle' : 'xCircle'" size="xs" class="h-3 w-3" />{{ text(availability(model) === 'available' ? '可用' : '不可用', availability(model) === 'available' ? 'Available' : 'Unavailable') }}</span><Toggle v-if="auth.isAdmin && groupId" :model-value="availability(model) === 'available'" :disabled="availabilitySaving === model.model" :aria-label="`${model.model} ${text('可用状态', 'availability')}`" :data-testid="`media-model-availability-toggle-${model.model}`" @update:model-value="toggleAvailability(model, $event)" /></div><p v-if="availabilityError && availabilityErrorModel === model.model && availabilitySaving === ''" role="alert" class="mt-1 text-xs text-red-600">{{ availabilityError }}</p><div class="mt-1 text-xs text-gray-500"><p v-for="op in model.operations" :key="op" class="break-all">{{ endpoints[op] || op }}</p></div></td><td class="p-3 align-top"><div v-for="price in prices(model)" :key="price.operation" class="mb-3"><div class="text-xs text-gray-500">{{ text('基础模型价格', 'Base model price') }}</div><div v-for="rule in price.rules" :key="rule.priority"><p v-if="Object.keys(rule.when).length" class="mt-1 text-xs text-gray-500">{{ conditions(rule.when) }}</p><div v-for="charge in rule.charges" :key="charge.name" class="mt-1">
                <template v-if="charge.method === 'progressive'">
                  <p class="text-xs">{{ text('累进分档', 'Progressive tiers') }}</p>
                  <p v-for="(tier, index) in charge.tiers" :key="index">{{ index === 0 ? '0' : charge.tiers[index - 1].up_to }}–{{ tier.up_to ?? '∞' }} {{ label(charge, model) }}: {{ displayedPrice(model, tier.unit_price, price.time_multiplier) }} / {{ label(charge, model) }}</p>
                </template>
                <p v-else-if="charge.method === 'package'">{{ charge.package_quantity }} {{ label(charge, model) }} {{ text('套餐总价', 'package total') }}: {{ displayedPrice(model, charge.unit_price, price.time_multiplier) }}</p>
                <p v-else>{{ displayedPrice(model, charge.unit_price, price.time_multiplier) }} / {{ label(charge, model) }}</p>
                <span v-if="charge.trigger === 'accepted'" class="text-xs text-amber-700 dark:text-amber-400">{{ text('受理后失败不退', 'Nonrefundable after acceptance') }}</span>
              </div></div><p v-if="price.time_multiplier && price.time_multiplier !== '1'" class="mt-1 text-xs">{{ text('当前模型分时倍率', 'Current model time multiplier') }} ×{{ price.time_multiplier }}</p><p v-if="price.maximum_charge !== null" class="mt-1 text-xs">{{ text('最高收费上限', 'Maximum charge cap') }} {{ displayedPrice(model, price.maximum_charge, price.time_multiplier) }}</p></div></td><td class="p-3 align-top min-w-56">
                <div v-if="editing === model.model && editKind === 'price'" class="space-y-2">
                  <label class="block text-xs font-medium">官方参考价（美元 / 单位）<input v-model="priceAmount" aria-label="官方参考价金额" inputmode="decimal" maxlength="21" class="input mt-2" placeholder="例如：0.10" /></label>
                  <label class="block text-xs font-medium">计价单位<input v-model="priceUnit" aria-label="官方参考价单位" maxlength="40" class="input mt-2" placeholder="例如：秒、张、次" /></label>
                  <p class="text-xs text-gray-500">按本分组保存；仅供参考，不影响实付价格。金额留空可清除。</p>
                  <p class="text-xs">当前参考价：{{ referencePrice(model) ? `$${referencePrice(model)!.amount} / ${referencePrice(model)!.unit}` : '未填写' }}</p>
                  <p v-if="editError" role="alert" class="text-xs text-amber-700">{{ editError }}</p>
                  <div class="flex flex-wrap gap-2"><button class="btn btn-primary" :disabled="saving || conflict" @click="save">保存参考价</button><button v-if="conflict" class="btn btn-secondary" :disabled="saving" @click="refreshVersion">刷新版本</button><button class="btn btn-secondary" :disabled="saving" @click="priceAmount = ''">清空参考价</button><button class="btn btn-secondary" :disabled="saving" @click="editing = ''">取消</button></div>
                </div>
                <template v-else><p>{{ referencePrice(model) ? `$${referencePrice(model)!.amount} / ${referencePrice(model)!.unit}` : '—' }}</p><button v-if="auth.isAdmin && groupId" class="mt-2 text-xs text-primary-600 hover:underline" :disabled="saving" @click="edit(model, 'price')">编辑参考价</button></template>
              </td><td class="p-3 align-top min-w-72"><div v-if="editing === model.model && editKind === 'description'" class="space-y-2"><label class="block text-xs font-medium">能力说明（本分组）<textarea v-model="draft" maxlength="2000" rows="4" class="input mt-2" placeholder="例如：最多 9 张参考图，每项含 url；720P；5 / 10 / 15 秒；支持 16:9、9:16" /></label><p class="text-xs text-gray-500">{{ draft.length }} / 2000 · 仅修改展示说明，请求规则在媒体管理中配置。</p><p class="whitespace-pre-wrap text-xs">当前说明：{{ description(model) || '未填写' }}</p><p v-if="editError" role="alert" class="text-xs text-amber-700">{{ editError }}</p><div class="flex flex-wrap gap-2"><button class="btn btn-primary" :disabled="saving || conflict" @click="save">保存</button><button v-if="conflict" class="btn btn-secondary" :disabled="saving" @click="refreshVersion">刷新版本</button><button class="btn btn-secondary" :disabled="saving" @click="draft = ''">清空说明</button><button class="btn btn-secondary" :disabled="saving" @click="editing = ''">取消</button></div></div><template v-else><p v-if="description(model)" class="mb-2 whitespace-pre-wrap break-words">{{ description(model) }}</p><button v-if="auth.isAdmin && groupId" class="mb-2 text-xs text-primary-600 hover:underline" @click="edit(model)">编辑说明</button></template><p class="text-xs text-gray-500">{{ model.resolutions?.join(' / ') }}</p><template v-for="(d, index) in model.duration_variants || [model.duration]" :key="index"><p v-if="d.mode === 'fixed'">{{ d.values[0] }}{{ text(' 秒固定时长', ' seconds, fixed') }}</p><p v-else-if="d.values?.length">{{ d.values.join(' / ') }} {{ text('秒', 'seconds') }}</p><p v-else-if="d.min">{{ d.min }}–{{ d.max }} {{ text('秒', 'seconds') }}</p></template></td></tr></tbody>
    </table>
  </div><p class="text-xs text-gray-500">{{ text('查询和下载不重复计费；分时倍率在创建任务时锁定。完整档位、取整和估价以对应价卡为准。', 'Queries and downloads are free; time multipliers lock at creation. Tier and rounding details follow the price card.') }}</p></div>
</template>
