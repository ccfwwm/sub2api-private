<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { newMediaCapability, type MediaCapability } from '@/types/media'
const props = defineProps<{ modelValue: MediaCapability[]; accountId?: number; baseUrl: string; apiKey?: string }>()
const emit = defineEmits<{ 'update:modelValue': [MediaCapability[]] }>()
const { locale } = useI18n()
const tr = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
const candidates = ref<string[]>([]), error = ref(''), loading = ref(false), selected = ref('')
const adapters = ['v6k_video_gateway','yunfei_gpt_images','yunfei_gemini_native','yunfei_grok','yunfei_omni','yunfei_veo_preview','yunfei_lyria','ark_native']
const adapterNames: Record<string, [string, string]> = { v6k_video_gateway:['兼容视频接口（V6K）','Compatible video (V6K)'], yunfei_gpt_images:['GPT 图片生成与编辑','GPT image generation / edits'], yunfei_gemini_native:['Gemini 原生图片','Gemini native images'], yunfei_grok:['Grok 视频','Grok video'], yunfei_omni:['Omni / Veo 视频','Omni / Veo video'], yunfei_veo_preview:['Veo Preview 视频','Veo Preview video'], yunfei_lyria:['Lyria 音乐','Lyria music'], ark_native:['Ark 原生视频','Ark native video'] }
const val = (e: Event) => (e.target as HTMLInputElement).value
const csv = (s: string) => s.split(/[,，\n]/).map(v => v.trim()).filter(Boolean)
function edit(index: number, fn: (c: MediaCapability) => void) { const all: MediaCapability[] = JSON.parse(JSON.stringify(props.modelValue)); fn(all[index]); emit('update:modelValue', all) }
function add(model = '') { if (model && props.modelValue.some(c => c.model === model)) return; emit('update:modelValue', [...props.modelValue, newMediaCapability(model)]) }
function modelID(c: MediaCapability, value: string) { if (!c.upstream_model || c.upstream_model === c.model) c.upstream_model = value; c.model = value }
async function sync() {
  loading.value = true; error.value = ''
  try {
    const result = props.accountId ? await adminAPI.accounts.syncUpstreamModels(props.accountId) : await adminAPI.accounts.syncUpstreamModelsPreview({platform:'imagevideo', type:'apikey', base_url:props.baseUrl,api_key:props.apiKey || ''})
    candidates.value = result.models
  } catch (e) { error.value = (e as {message?:string}).message || tr('同步失败', 'Sync failed') }
  finally { loading.value = false }
}
function adapter(c: MediaCapability, value: string) {
  c.adapter = value
  c.media_type = value === 'yunfei_gpt_images' || value === 'yunfei_gemini_native' ? 'image' : value === 'yunfei_lyria' ? 'audio' : 'video'
  c.operations = value === 'yunfei_gpt_images' ? ['image','image_edit'] : value === 'yunfei_gemini_native' ? ['gemini_image'] : value === 'yunfei_lyria' ? ['audio','chat_music'] : ['video']
}
</script>
<template>
  <section class="space-y-3 rounded-xl border p-4 dark:border-dark-600">
    <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="font-semibold">{{ tr('媒体模型', 'Media models') }} <span class="text-xs font-normal text-gray-500">imagevideo</span></h3><div class="flex gap-2"><button class="btn btn-secondary" type="button" :disabled="loading" @click="sync">{{ loading ? tr('同步中…','Syncing…') : tr('同步模型列表','Sync model list') }}</button><button class="btn btn-secondary" type="button" @click="add()">{{ tr('手动添加','Add manually') }}</button></div></div>
    <p class="text-xs text-gray-500">{{ tr('同步仅获取候选模型，不自动启用或更改价格。在渠道或分组的模型定价中配置对应价格。','Sync only discovers models. It does not enable models or change prices. Configure prices in channels or groups.') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <div v-if="candidates.length" class="flex flex-wrap gap-2"><select v-model="selected" class="input max-w-md"><option value="">{{ tr('选择候选模型','Select a model') }}</option><option v-for="m in candidates" :key="m">{{ m }}</option></select><button type="button" class="btn btn-secondary" :disabled="!selected" @click="add(selected)">{{ tr('添加到账号','Add to account') }}</button></div>
    <p v-if="!modelValue.length" class="text-sm text-gray-500">{{ tr('尚未配置模型。先同步或手动添加。','No models configured. Sync or add a model.') }}</p>
    <details v-for="(c, i) in modelValue" :key="i" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800" :open="!c.model">
      <summary class="cursor-pointer text-sm font-medium">{{ c.model || tr('新模型','New model') }} <span class="ml-2 text-xs text-gray-500">{{ c.media_type }} · {{ c.enabled ? tr('启用','Enabled') : tr('关闭','Disabled') }}</span></summary>
      <div class="mt-3 grid gap-3 sm:grid-cols-2">
        <label class="text-xs">{{ tr('模型 ID（大小写精确匹配）','Model ID (case sensitive)') }}<input class="input mt-1" :value="c.model" @input="edit(i, c => modelID(c, val($event)))" /></label>
        <label class="text-xs">{{ tr('上游模型 ID','Upstream model ID') }}<input class="input mt-1" :value="c.upstream_model" @input="edit(i, c => c.upstream_model = val($event))" /></label>
        <label class="text-xs">{{ tr('接口适配器','Protocol adapter') }}<select class="input mt-1" :value="c.adapter" @change="edit(i, c => adapter(c, val($event)))"><option v-for="a in adapters" :key="a" :value="a">{{ tr(...adapterNames[a]) }}</option></select></label>
        <label class="text-xs">{{ tr('请求格式','Request formats') }}<input class="input mt-1" :value="c.formats.join(', ')" placeholder="json, multipart" @input="edit(i, c => c.formats = csv(val($event)))" /></label>
        <label class="text-xs">{{ tr('接口用途（逗号分隔）','Operations (comma separated)') }}<input class="input mt-1" :value="c.operations.join(', ')" @input="edit(i, c => c.operations = csv(val($event)))" /></label>
        <label class="text-xs">{{ tr('场景（逗号分隔，可空）','Scenarios (comma separated, optional)') }}<input class="input mt-1" :value="c.scenarios?.join(', ')" @input="edit(i, c => c.scenarios = csv(val($event)))" /></label>
        <label class="text-xs">{{ tr('时长规则','Duration rule') }}<select class="input mt-1" :value="c.duration.mode" @change="edit(i, c => c.duration.mode = val($event))"><option value="fixed">{{ tr('固定时长','Fixed') }}</option><option value="enum">{{ tr('时长档位','Options') }}</option><option value="range">{{ tr('时长范围','Range') }}</option><option value="unknown">{{ tr('尚未验证','Unverified') }}</option></select></label>
        <label v-if="c.duration.mode === 'fixed' || c.duration.mode === 'enum'" class="text-xs">{{ tr('秒数（逗号分隔）','Seconds (comma separated)') }}<input class="input mt-1" :value="c.duration.values.join(', ')" @input="edit(i, c => c.duration.values = csv(val($event)).map(Number))" /></label>
        <template v-if="c.duration.mode === 'range'"><label class="text-xs">{{ tr('最短秒数','Minimum seconds') }}<input type="number" min="1" class="input mt-1" :value="c.duration.min" @input="edit(i, c => c.duration.min = Number(val($event)))" /></label><label class="text-xs">{{ tr('最长秒数','Maximum seconds') }}<input type="number" min="1" class="input mt-1" :value="c.duration.max" @input="edit(i, c => c.duration.max = Number(val($event)))" /></label></template>
        <label class="text-xs">{{ tr('默认秒数（可空）','Default seconds (optional)') }}<input type="number" min="1" class="input mt-1" :value="c.duration.default" @input="edit(i, c => c.duration.default = val($event) ? Number(val($event)) : null)" /></label>
        <label class="flex items-center gap-2 text-xs"><input type="checkbox" :checked="c.duration.required" @change="edit(i, c => c.duration.required = ($event.target as HTMLInputElement).checked)" />{{ tr('请求必须指定时长','Explicit duration required') }}</label>
        <label v-for="field in ['resolutions','sizes','aspect_ratios','qualities'] as const" :key="field" class="text-xs">{{ ({resolutions:tr('分辨率','Resolutions'),sizes:tr('图片像素尺寸','Image sizes'),aspect_ratios:tr('宽高比','Aspect ratios'),qualities:tr('质量','Qualities')})[field] }}<input class="input mt-1" :value="c[field]?.join(', ')" @input="edit(i, c => c[field] = csv(val($event)))" /></label>
        <label class="text-xs">{{ tr('分辨率参数规则','Resolution policy') }}<select class="input mt-1" :value="c.resolution_policy" @change="edit(i, c => c.resolution_policy = val($event))"><option value="optional">{{ tr('可选','Optional') }}</option><option value="required">{{ tr('必填','Required') }}</option><option value="omit">{{ tr('省略（如 00sd2.5）','Omit (e.g. 00sd2.5)') }}</option></select></label>
        <label class="text-xs">{{ tr('允许制品域名（逗号分隔）','Allowed artifact hosts (comma separated)') }}<input class="input mt-1" :value="c.artifact_hosts?.join(', ')" @input="edit(i, c => c.artifact_hosts = csv(val($event)))" /></label>
      </div>
      <details class="mt-3 text-xs"><summary class="cursor-pointer">{{ tr('参考素材、用量与执行限制','Reference material, usage and execution limits') }}</summary>
        <div v-for="kind in ['image','video','audio']" :key="kind" class="mt-3 grid gap-2 sm:grid-cols-3"><label>{{ kind }} · {{ tr('最多素材数（0 禁用）','Max materials (0 disables)') }}<input class="input" type="number" min="0" :value="c.materials[kind]?.max_count || 0" @input="edit(i, c => { if (!c.materials[kind]) c.materials[kind] = {formats:['public_url_string'],max_count:0,max_file_bytes:33554432,max_duration_seconds:'30'}; c.materials[kind].max_count = Number(val($event)) })" /></label><template v-if="c.materials[kind]"><label>{{ tr('允许素材格式','Material formats') }}<input class="input" :value="c.materials[kind].formats.join(', ')" placeholder="public_url_string, public_url_object, data_url, base64, multipart" @input="edit(i, c => c.materials[kind].formats = csv(val($event)))" /></label><label>{{ tr('单文件上限 bytes','File limit bytes') }}<input class="input" type="number" :value="c.materials[kind].max_file_bytes" @input="edit(i, c => c.materials[kind].max_file_bytes = Number(val($event)))" /></label><label v-if="kind !== 'image'">{{ tr('单素材最长秒数','Material maximum seconds') }}<input class="input" :value="c.materials[kind].max_duration_seconds" @input="edit(i, c => c.materials[kind].max_duration_seconds = val($event))" /></label></template></div>
        <div class="mt-3 grid gap-2 sm:grid-cols-2"><label>{{ tr('请求总上限 bytes','Request limit bytes') }}<input class="input" type="number" :value="c.limits.max_request_bytes" @input="edit(i, c => c.limits.max_request_bytes = Number(val($event)))" /></label><label>{{ tr('任务并发','Concurrent tasks') }}<input class="input" type="number" min="1" :value="c.limits.max_concurrent_tasks" @input="edit(i, c => c.limits.max_concurrent_tasks = Number(val($event)))" /></label><label>{{ tr('查询间隔秒','Poll interval seconds') }}<input class="input" type="number" min="1" max="300" :value="c.delivery.poll_seconds" @input="edit(i, c => c.delivery.poll_seconds = Number(val($event)))" /></label><label>{{ tr('观察期限秒','Observation seconds') }}<input class="input" type="number" min="1" :value="c.limits.observation_seconds" @input="edit(i, c => c.limits.observation_seconds = Number(val($event)))" /></label>
        <label v-for="target in ['provider_duration','provider_tokens.total_tokens','provider_tokens.input_tokens','provider_tokens.output_tokens','provider_tokens.image_input_tokens','provider_tokens.image_output_tokens']" :key="target">{{ target }}<input class="input" :value="c.usage_fields?.[target]" placeholder="usage.total_tokens" @input="edit(i, c => { c.usage_fields ||= {}; const v = val($event); if (v) c.usage_fields[target] = v; else delete c.usage_fields[target] })" /></label></div>
      </details>
      <div class="mt-3 grid gap-3 sm:grid-cols-2"><label class="text-xs">{{ tr('验证状态','Verification status') }}<select class="input mt-1" :value="c.validation_status" @change="edit(i, c => c.validation_status = val($event))"><option value="待渠道验证">{{ tr('待渠道验证','Unverified') }}</option><option value="本项目实测通过">{{ tr('本项目实测通过','Verified in this project') }}</option></select></label><label class="text-xs">{{ tr('实测凭据（日期与结果，无密钥）','Verification evidence (date and result, no secrets)') }}<input class="input mt-1" :value="c.verification_evidence" @input="edit(i, c => c.verification_evidence = val($event))" /></label></div>
      <div class="mt-3 flex justify-between"><label class="flex items-center gap-2 text-sm"><input type="checkbox" :checked="c.enabled" @change="edit(i, c => c.enabled = ($event.target as HTMLInputElement).checked)" />{{ tr('启用模型（需要能力、实测和价格）','Enable model (capability, verification and price required)') }}</label><button class="text-sm text-red-600" type="button" @click="emit('update:modelValue', modelValue.filter((_, index) => index !== i))">{{ tr('移除','Remove') }}</button></div>
    </details>
  </section>
</template>
