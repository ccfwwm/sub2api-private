<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import AccountTestModal from '@/components/admin/account/AccountTestModal.vue'
import { accountMediaCapabilities, newMediaCapability, type MediaCapability } from '@/types/media'
import type { Account } from '@/types'
const props = defineProps<{ modelValue: MediaCapability[]; accountId?: number; account?: Account; baseUrl: string; apiKey?: string }>()
const emit = defineEmits<{ 'update:modelValue': [MediaCapability[]] }>()
const { locale } = useI18n()
const tr = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
const candidates = ref<string[]>([]), error = ref(''), loading = ref(false), selected = ref('')
const advanced = ref<number | null>(null), testingModel = ref<string | null>(null)
const adapters: Record<string, [string, string]> = { v6k_video_gateway:['兼容视频接口（V6K）','Compatible video (V6K)'], yunfei_gpt_images:['GPT 图片','GPT images'], yunfei_gemini_native:['Gemini 原生图片','Gemini native images'], yunfei_grok:['Grok 视频','Grok video'], yunfei_omni:['Omni / Veo 视频','Omni / Veo video'], yunfei_veo_preview:['Veo Preview 视频','Veo Preview video'], yunfei_lyria:['Lyria 音乐','Lyria music'], ark_native:['Ark 原生视频','Ark native video'] }
const val = (e: Event) => (e.target as HTMLInputElement).value
const csv = (s: string) => s.split(/[,，\n]/).map(v => v.trim()).filter(Boolean)
const saved = computed(() => props.account && accountMediaCapabilities(props.account.extra, props.account.credentials))
const canTest = computed(() => !!props.account && JSON.stringify(saved.value) === JSON.stringify(props.modelValue) && props.baseUrl === String(props.account.credentials?.base_url || '') && (!props.apiKey || props.apiKey === props.account.credentials?.api_key))
const mappingConflict = computed(() => props.account && props.modelValue.some(c => {
  const mapping = props.account?.credentials?.model_mapping as Record<string, string> | undefined
  return mapping?.[c.model] && mapping[c.model] !== c.upstream_model
}))
function edit(index: number, fn: (c: MediaCapability) => void) { const all: MediaCapability[] = JSON.parse(JSON.stringify(props.modelValue)); fn(all[index]); emit('update:modelValue', all) }
function add(model = '') { if (model && props.modelValue.some(c => c.model === model)) return; emit('update:modelValue', [...props.modelValue, newMediaCapability(model)]); advanced.value = props.modelValue.length }
function modelID(c: MediaCapability, value: string) {
  if (!c.adapter) { const preset = newMediaCapability(value); if (preset.adapter) { Object.assign(c, preset); return } }
  if (!c.upstream_model || c.upstream_model === c.model) c.upstream_model = value
  c.model = value
}
async function sync() {
  loading.value = true; error.value = ''
  try {
    const result = props.accountId && !props.apiKey && props.baseUrl === String(props.account?.credentials?.base_url || '') ? await adminAPI.accounts.syncUpstreamModels(props.accountId) : await adminAPI.accounts.syncUpstreamModelsPreview({platform:'imagevideo',type:'apikey',base_url:props.baseUrl,api_key:props.apiKey || String(props.account?.credentials?.api_key || '')})
    candidates.value = result.models
  } catch (e) { error.value = (e as {message?:string}).message || tr('同步失败','Sync failed') }
  finally { loading.value = false }
}
function adapter(c: MediaCapability, value: string) {
  c.adapter = value
  c.media_type = ['yunfei_gpt_images','yunfei_gemini_native'].includes(value) ? 'image' : value === 'yunfei_lyria' ? 'audio' : 'video'
  c.operations = value === 'yunfei_gpt_images' ? ['image','image_edit'] : value === 'yunfei_gemini_native' ? ['gemini_image'] : value === 'yunfei_lyria' ? ['audio','chat_music'] : ['video']
  c.formats = value === 'yunfei_gpt_images' ? ['json','multipart'] : ['json']
}
function testStatus(c: MediaCapability) {
  const results = props.account?.extra?.media_test_results as Record<string, {status:string;started_at:string;test_kind?:string}> | undefined
  const result = results?.[c.model]
  if (!result) return tr('未实测','Not tested')
  const state = ({completed:tr(result.test_kind === 'connection' ? '连接通过' : '生成通过',result.test_kind === 'connection' ? 'Connection passed' : 'Generation passed'),failed:tr('失败','Failed'),unknown:tr('结果待确认','Uncertain'),submitting:tr('提交中','Submitting'),in_progress:tr('进行中','Running')} as Record<string,string>)[result.status] || result.status
  return state + ' · ' + new Date(result.started_at).toLocaleString()
}
</script>
<template>
  <section class="space-y-3 rounded-xl border p-4 dark:border-dark-600">
    <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="font-semibold">{{ tr('模型配置','Model configuration') }}</h3><div class="flex gap-2"><button class="btn btn-secondary" type="button" :disabled="loading" @click="sync">{{ loading ? tr('同步中…','Syncing…') : tr('同步候选模型','Discover models') }}</button><button class="btn btn-secondary" type="button" @click="add()">{{ tr('手动添加','Add manually') }}</button></div></div>
    <p class="text-xs text-gray-500">{{ tr('已知模型自动使用接口预设。实测用于记录状态，可直接启用；业务调用仍需渠道或分组定价。同步不修改配置或价格。','Known models use protocol presets. Testing records status; models can be enabled directly. Business requests require channel or group pricing. Discovery does not change configuration or prices.') }}</p>
    <p v-if="mappingConflict" class="text-xs text-amber-600">{{ tr('旧映射与媒体能力不一致：当前按本表路由，保存后统一映射。','Legacy mappings differ from capabilities. This table determines routing; saving reconciles mappings.') }}</p>
    <p v-if="!canTest" class="text-xs text-gray-500">{{ tr('测试使用已保存的账号配置，请先保存新账号或配置修改。','Tests use saved settings. Save new accounts or changes first.') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <div v-if="candidates.length" class="flex flex-wrap gap-2"><select v-model="selected" class="input max-w-md"><option value="">{{ tr('选择候选模型','Select a model') }}</option><option v-for="m in candidates" :key="m">{{ m }}</option></select><button type="button" class="btn btn-secondary" :disabled="!selected" @click="add(selected)">{{ tr('添加到账号','Add to account') }}</button></div>
    <p v-if="!modelValue.length" class="text-sm text-gray-500">{{ tr('先同步候选模型或手动添加。','Discover models or add one manually.') }}</p>
    <div v-else class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr class="text-xs text-gray-500"><th>{{ tr('模型名称','Model') }}</th><th>{{ tr('上游映射','Upstream mapping') }}</th><th>{{ tr('类型','Type') }}</th><th>{{ tr('启用','Enabled') }}</th><th>{{ tr('测试结果','Test result') }}</th><th>{{ tr('操作','Actions') }}</th></tr></thead><tbody><template v-for="(c,i) in modelValue" :key="i">
      <tr class="border-t dark:border-dark-600"><td class="py-2 pr-2"><input class="input min-w-36" :aria-label="tr('模型名称','Model')" :value="c.model" @change="edit(i,c => modelID(c,val($event)))" /></td><td class="pr-2"><input class="input min-w-36" :aria-label="tr('上游映射','Upstream mapping')" :value="c.upstream_model" @change="edit(i,c => c.upstream_model = val($event))" /></td><td class="pr-2">{{ c.media_type || tr('待选接口','Choose interface') }}</td><td class="pr-2"><input type="checkbox" :aria-label="tr('启用模型','Enable model')" :checked="c.enabled" @change="edit(i,c => c.enabled = ($event.target as HTMLInputElement).checked)" /></td><td class="pr-2 text-xs">{{ testStatus(c) }}</td><td class="whitespace-nowrap"><button type="button" class="mr-2 text-primary-600 disabled:opacity-40" :disabled="!canTest" @click="testingModel = c.model">{{ tr('测试 / 查看','Test / view') }}</button><button type="button" class="mr-2 text-primary-600" @click="advanced = advanced === i ? null : i">{{ tr('高级设置','Advanced') }}</button><button type="button" class="text-red-600" @click="emit('update:modelValue',modelValue.filter((_,index) => index !== i))">{{ tr('移除','Remove') }}</button></td></tr>
      <tr v-if="advanced === i || !c.adapter"><td colspan="6" class="bg-gray-50 p-3 dark:bg-dark-800"><div class="grid gap-3 sm:grid-cols-2">
        <label class="text-xs">{{ tr('视频请求格式','Video request format') }}<select class="input mt-1" :value="c.request_format || 'legacy'" @change="edit(i,c => { c.request_format = val($event) as MediaCapability['request_format'] })"><option value="legacy">Legacy</option><option value="yunfei_multimodal">云飞 Base64 多媒体</option><option value="v6k_url">v6k 公网 URL</option></select></label>
        <label class="text-xs">{{ tr('接口类型','Interface type') }}<select class="input mt-1" :value="c.adapter" @change="edit(i,c => adapter(c,val($event)))"><option value="">{{ tr('请选择','Select') }}</option><option v-for="(name,a) in adapters" :key="a" :value="a">{{ tr(...name) }}</option></select></label>
        <template v-if="c.media_type === 'video'"><label class="text-xs">{{ tr('时长规则','Duration rule') }}<select class="input" :value="c.duration.mode" @change="edit(i,c => c.duration.mode = val($event))"><option value="fixed">{{ tr('固定','Fixed') }}</option><option value="enum">{{ tr('档位','Options') }}</option><option value="range">{{ tr('范围','Range') }}</option><option value="unknown">{{ tr('未知','Unknown') }}</option></select></label><label v-if="['fixed','enum'].includes(c.duration.mode)" class="text-xs">{{ tr('秒数','Seconds') }}<input class="input" :value="c.duration.values.join(', ')" @change="edit(i,c => c.duration.values = csv(val($event)).map(Number))" /></label><template v-if="c.duration.mode === 'range'"><label class="text-xs">{{ tr('最短秒数','Minimum seconds') }}<input type="number" min="1" class="input" :value="c.duration.min" @change="edit(i,c => c.duration.min = Number(val($event)))" /></label><label class="text-xs">{{ tr('最长秒数','Maximum seconds') }}<input type="number" min="1" class="input" :value="c.duration.max" @change="edit(i,c => c.duration.max = Number(val($event)))" /></label></template><label class="text-xs">{{ tr('默认秒数','Default seconds') }}<input type="number" min="1" class="input" :value="c.duration.default" @change="edit(i,c => c.duration.default = val($event) ? Number(val($event)) : null)" /></label><label class="flex items-center gap-2 text-xs"><input type="checkbox" :checked="c.duration.required" @change="edit(i,c => c.duration.required = ($event.target as HTMLInputElement).checked)" />{{ tr('请求必须指定时长','Explicit duration required') }}</label></template>
        <label v-if="c.media_type === 'image'" class="text-xs">{{ tr('图片像素尺寸','Image sizes') }}<input class="input" :value="c.sizes.join(', ')" @change="edit(i,c => c.sizes = csv(val($event)))" /></label>
        <template v-if="c.media_type && c.media_type !== 'audio'"><label v-for="field in ['resolutions','aspect_ratios','qualities'] as const" :key="field" class="text-xs">{{ ({resolutions:tr('分辨率档位','Resolution tiers'),aspect_ratios:tr('宽高比','Aspect ratios'),qualities:tr('质量','Quality')})[field] }}<input class="input" :value="c[field]?.join(', ')" @change="edit(i,c => c[field] = csv(val($event)))" /></label><label class="text-xs">{{ tr('分辨率规则','Resolution policy') }}<select class="input" :value="c.resolution_policy" @change="edit(i,c => c.resolution_policy = val($event))"><option value="optional">{{ tr('可选','Optional') }}</option><option value="required">{{ tr('必填','Required') }}</option><option value="omit">{{ tr('省略','Omit') }}</option></select></label></template>
      </div><details class="mt-3 text-xs"><summary class="cursor-pointer">{{ tr('协议与执行限制覆盖','Protocol and execution overrides') }}</summary><div class="mt-3 grid gap-3 sm:grid-cols-2"><label>{{ tr('请求格式','Request formats') }}<input class="input" :value="c.formats.join(', ')" @change="edit(i,c => c.formats = csv(val($event)))" /></label><label>{{ tr('接口用途','Operations') }}<input class="input" :value="c.operations.join(', ')" @change="edit(i,c => c.operations = csv(val($event)))" /></label><label>{{ tr('允许产物域名','Allowed artifact hosts') }}<input class="input" :value="c.artifact_hosts.join(', ')" @change="edit(i,c => c.artifact_hosts = csv(val($event)))" /></label><label v-for="field in ['max_request_bytes','max_concurrent_tasks','observation_seconds'] as const" :key="field">{{ ({max_request_bytes:tr('请求上限 bytes','Request limit bytes'),max_concurrent_tasks:tr('任务并发','Task concurrency'),observation_seconds:tr('观察期限秒','Observation seconds')})[field] }}<input class="input" type="number" min="1" :value="c.limits[field]" @change="edit(i,c => c.limits[field] = Number(val($event)))" /></label><label>{{ tr('查询间隔秒','Poll interval seconds') }}<input class="input" type="number" min="1" max="300" :value="c.delivery.poll_seconds" @change="edit(i,c => c.delivery.poll_seconds = Number(val($event)))" /></label><label v-for="(_,target) in c.usage_fields" :key="target">{{ target }}<input class="input" :value="c.usage_fields[target]" @change="edit(i,c => c.usage_fields[target] = val($event))" /></label><template v-for="(material,kind) in c.materials" :key="kind"><label>{{ kind }} · {{ tr('素材数量上限','Maximum reference count') }}<input class="input" type="number" min="0" :value="material.max_count" @change="edit(i,c => c.materials[kind].max_count = Number(val($event)))" /></label><label>{{ tr('素材文件上限 bytes','Reference file limit bytes') }}<input class="input" type="number" min="1" :value="material.max_file_bytes" @change="edit(i,c => c.materials[kind].max_file_bytes = Number(val($event)))" /></label></template></div></details></td></tr>
    </template></tbody></table></div>
    <AccountTestModal v-if="account && testingModel !== null" :show="true" :account="account" :initial-model-id="testingModel" @close="testingModel = null" />
  </section>
</template>
