<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { adminAPI } from '@/api/admin'
import { mediaProtocolsAPI } from '@/api/media-protocols'
import { accountMediaCapabilities } from '@/types/media'
import type { Account, AccountListItem, AdminGroup } from '@/types'
const route = useRoute()
const group = ref(String(route.query.group_id || '')), key = ref(''), account = ref(String(route.query.account_id || ''))
const groups = ref<AdminGroup[]>([]), accounts = ref<AccountListItem[]>([]), detail = ref<Account | null>(null)
const request = ref('{"model":"","prompt":"A quiet valley"}'), operation = ref('video'), model = ref('')
const result = ref<unknown>(), busy = ref(false), loading = ref(false), error = ref(''), scopeError = ref('')
const page = ref(1), total = ref(0)
const models = computed(() => accountMediaCapabilities(detail.value?.extra, detail.value?.credentials))
const operations = computed(() => models.value.find(item => item.model === model.value)?.operations || [])
let generation = 0, listGeneration = 0
let defaultSeconds: number | undefined
async function loadAccount() {
  const version = ++generation
  detail.value = null; result.value = undefined; scopeError.value = ''; loading.value = true
  try {
    if (!account.value) return
    const loaded = await adminAPI.accounts.getById(Number(account.value))
    if (version !== generation) return
    if (loaded.platform !== 'imagevideo' || !loaded.group_ids?.includes(Number(group.value))) throw new Error('所选账号不属于当前媒体分组。')
    detail.value = loaded
    model.value = models.value.find(item => item.enabled)?.model || models.value[0]?.model || ''
    selectModel()
  } catch (cause) { if (version === generation) scopeError.value = (cause as Error).message }
  finally { if (version === generation) loading.value = false }
}
async function loadAccounts(linked?: Account) {
  const version = ++listGeneration
  ++generation; detail.value = null
  loading.value = true; scopeError.value = ''; result.value = undefined
  try {
    const response = await adminAPI.accounts.list(page.value, 50, { platform: 'imagevideo', group: group.value })
    if (version !== listGeneration) return
    accounts.value = response.items; total.value = response.total
    if (linked?.group_ids?.includes(Number(group.value))) account.value = String(linked.id)
    else if (!accounts.value.some(item => String(item.id) === account.value)) account.value = String(accounts.value[0]?.id || '')
    await loadAccount()
  } catch (cause) { if (version === listGeneration) scopeError.value = (cause as Error).message }
  finally { if (version === listGeneration) loading.value = false }
}
async function changeGroup() {
  account.value = ''; detail.value = null; page.value = 1
  await loadAccounts()
}
function selectModel() {
  const capability = models.value.find(item => item.model === model.value)
  if (!capability) return
  if (!capability.operations.includes(operation.value)) operation.value = capability.operations[0] || ''
  let current: Record<string, unknown>
  try { current = JSON.parse(request.value) } catch { return }
  if (defaultSeconds !== undefined && current.seconds === defaultSeconds) delete current.seconds
  defaultSeconds = undefined
  current.model = capability.model
  if (capability.media_type === 'video' && current.seconds === undefined && current.duration === undefined) {
    defaultSeconds = capability.duration.default ?? capability.duration.values?.[0] ?? capability.duration.min ?? 5
    current.seconds = defaultSeconds
  }
  request.value = JSON.stringify(current, null, 2)
}
async function run(effective: boolean) {
  busy.value = true; error.value = ''; result.value = undefined
  try {
    if (!Number.isSafeInteger(Number(group.value)) || Number(group.value) <= 0) throw new Error('请选择分组。')
    if (effective) result.value = await mediaProtocolsAPI.effective(Number(group.value))
    else {
      if (!Number.isSafeInteger(Number(key.value)) || Number(key.value) <= 0) throw new Error('请输入已创建的 API Key ID。')
      if (!detail.value || !detail.value.group_ids?.includes(Number(group.value))) throw new Error('请选择当前分组的账号。')
      result.value = await mediaProtocolsAPI.preview({ mode: 'effective', api_key_id: Number(key.value), group_id: Number(group.value), account_id: detail.value.id, operation: operation.value, request: JSON.parse(request.value), measure_media: false })
    }
  } catch (cause) { error.value = (cause as Error).message } finally { busy.value = false }
}
onMounted(async () => {
  loading.value = true
  try {
    groups.value = (await adminAPI.groups.getAllIncludingInactive()).filter(item => ['imagevideo', 'composite'].includes(item.platform))
    let linked: Account | undefined
    if (account.value) {
      linked = await adminAPI.accounts.getById(Number(account.value))
      if (linked.platform !== 'imagevideo') throw new Error('请选择图片音频视频账号。')
      if (!group.value) group.value = String(linked.group_ids?.[0] || '')
    }
    if (!group.value) group.value = String(groups.value[0]?.id || '')
    if (group.value) await loadAccounts(linked)
  } catch (cause) { scopeError.value = (cause as Error).message }
  finally { loading.value = false }
})
</script>
<template>
  <section class="space-y-5 rounded-lg border bg-white p-5 dark:border-dark-600 dark:bg-dark-800">
    <header><h2 class="text-lg font-semibold">联调检测</h2><p class="mt-1 text-sm text-gray-500">选择分组、账号与模型，检查实际模板、请求映射和渠道估价。</p></header>
    <div class="grid gap-4 sm:grid-cols-2">
      <label class="text-sm">分组<select v-model="group" class="input mt-1 block w-full" aria-label="联调分组" @change="changeGroup"><option value="">请选择分组</option><option v-for="item in groups" :key="item.id" :value="String(item.id)">{{ item.name }}</option></select></label>
      <label class="text-sm">账号<select v-model="account" class="input mt-1 block w-full" aria-label="联调账号" :disabled="loading" @change="loadAccount"><option value="">{{ loading ? '正在加载…' : '请选择账号' }}</option><option v-if="detail && !accounts.some(item => item.id === detail?.id)" :value="String(detail.id)">{{ detail.name }} · #{{ detail.id }}</option><option v-for="item in accounts" :key="item.id" :value="String(item.id)">{{ item.name }} · #{{ item.id }}</option></select></label>
    </div>
    <div class="flex items-center gap-2 text-sm"><span>{{ total }} 个账号</span><button class="btn btn-secondary" :disabled="loading || page <= 1" @click="page--; loadAccounts()">上一页</button><button class="btn btn-secondary" :disabled="loading || page * 50 >= total" @click="page++; loadAccounts()">下一页</button><button class="btn btn-secondary" :disabled="busy || !group" @click="run(true)">检查实际绑定</button></div>
    <p v-if="scopeError" role="alert" class="text-red-600">{{ scopeError }} <button class="underline" @click="loadAccounts()">重试</button></p>
    <p v-else-if="!loading && !accounts.length && !detail" class="rounded border border-dashed p-4 text-sm text-gray-500">该分组没有媒体账号。</p>
    <div class="grid gap-4 sm:grid-cols-3">
      <label class="text-sm">模型<select v-model="model" class="input mt-1 block w-full" aria-label="联调模型" @change="selectModel"><option value="">请选择模型</option><option v-for="item in models" :key="item.model" :value="item.model">{{ item.model }}{{ item.enabled ? '' : '（未启用）' }}</option></select></label>
      <label class="text-sm">请求操作<select v-model="operation" class="input mt-1 block w-full" aria-label="联调操作"><option v-for="op in operations" :key="op">{{ op }}</option></select></label>
      <label class="text-sm">API Key ID<input v-model="key" type="number" min="1" class="input mt-1 block w-full" aria-label="联调 API Key ID" /><RouterLink to="/keys" class="mt-1 block text-xs text-primary-600">创建或查看 API 密钥</RouterLink></label>
    </div>
    <p class="text-xs text-gray-500">API Key 必须属于当前分组。此预览不创建生成任务；真实生成测试可从账号模型的“测试 / 查看”进入。</p>
    <label class="block text-sm">请求样例<textarea v-model="request" class="input mt-1 min-h-40 w-full font-mono text-xs" aria-label="联调请求" /></label><button class="btn btn-primary" :disabled="busy || loading || !detail || !model || !operation" @click="run(false)">{{ busy ? '检查中…' : '预览映射与渠道估价' }}</button>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p><pre v-if="result" class="max-h-[600px] overflow-auto rounded-lg bg-gray-50 p-4 text-xs dark:bg-dark-800">{{ JSON.stringify(result,null,2) }}</pre>
  </section>
</template>
