<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { adminAPI } from '@/api/admin'
import { mediaAPI, type MediaPricingStatus } from '@/api/media'
import type { Account, AccountListItem, AdminGroup } from '@/types'
import { accountMediaCapabilities, type MediaCapability } from '@/types/media'
import MediaAccountModelsEditor from '@/components/account/MediaAccountModelsEditor.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { useStepUp, isStepUpCancelled } from '@/composables/useStepUp'

const route = useRoute(), router = useRouter(), stepUp = useStepUp()
const accounts = ref<AccountListItem[]>([]), account = ref<Account | null>(null)
const models = ref<MediaCapability[]>([]), search = ref(''), page = ref(1), total = ref(0)
const loading = ref(false), saving = ref(false), error = ref(''), notice = ref(''), listError = ref('')
const pricing = ref<MediaPricingStatus[]>([]), pricingError = ref(''), pricingLoading = ref(false)
const group = ref(''), scopeGroup = ref(String(route.query.group_id || ''))
const groups = ref<AdminGroup[]>([]), groupsError = ref(''), listLoading = ref(false)
const dirty = computed(() => account.value && JSON.stringify(models.value) !== JSON.stringify(accountMediaCapabilities(account.value.extra, account.value.credentials)))
let generation = 0, listGeneration = 0, priceGeneration = 0, loadingId = 0
async function list() {
  const version = ++listGeneration
  listError.value = ''; listLoading.value = true
  try {
    const result = await adminAPI.accounts.list(page.value, 20, { platform: 'imagevideo', search: search.value, ...(scopeGroup.value ? { group: scopeGroup.value } : {}) })
    if (version !== listGeneration) return
    accounts.value = result.items; total.value = result.total
    if (!account.value && !route.query.account_id && result.items.length) choose(result.items[0].id)
  } catch (cause) { if (version === listGeneration) listError.value = (cause as Error).message }
  finally { if (version === listGeneration) listLoading.value = false }
}
async function load(id: number) {
  if (loading.value && loadingId === id) return
  const version = ++generation
  loadingId = id
  loading.value = true; account.value = null; error.value = ''; notice.value = ''; models.value = []; pricing.value = []; ++priceGeneration
  if (!Number.isSafeInteger(id) || id <= 0) { loading.value = false; return }
  try {
    const loaded = await adminAPI.accounts.getById(id)
    if (version !== generation) return
    if (loaded.platform !== 'imagevideo') throw new Error('请选择图片音频视频账号。')
    account.value = loaded; models.value = accountMediaCapabilities(loaded.extra, loaded.credentials)
    group.value = loaded.group_ids?.includes(Number(scopeGroup.value)) ? scopeGroup.value : String(loaded.group_ids?.[0] || '')
    scopeGroup.value = group.value
    void loadPricing()
  } catch (cause) { if (version === generation) error.value = (cause as Error).message }
  finally { if (version === generation) loading.value = false }
}
function choose(id: number) {
  if (dirty.value && !window.confirm('当前模型配置尚未保存，切换账号会放弃这些修改。')) return
  void router.replace({ query: { ...route.query, group_id: scopeGroup.value || undefined, account_id: String(id) } })
  if (account.value?.id !== id) void load(id)
}
async function chooseGroup(value: string) {
  if (dirty.value && !window.confirm('当前模型配置尚未保存，切换分组会放弃这些修改。')) return
  ++generation; ++priceGeneration; ++listGeneration; loading.value = false; account.value = null; models.value = []; pricing.value = []
  scopeGroup.value = value; group.value = value; page.value = 1
  await router.replace({ query: { ...route.query, group_id: value || undefined, account_id: undefined } })
  await list()
}
async function initialize() {
  groupsError.value = ''
  try {
    groups.value = (await adminAPI.groups.getAllIncludingInactive()).filter(item => ['imagevideo', 'composite'].includes(item.platform))
  } catch (cause) { groupsError.value = (cause as Error).message }
  if (route.query.account_id) await load(Number(route.query.account_id))
  else if (!scopeGroup.value) scopeGroup.value = String(groups.value[0]?.id || '')
  await list()
}
async function save() {
  if (!account.value) return
  const id = account.value.id, version = generation
  const original = JSON.stringify(accountMediaCapabilities(account.value.extra, account.value.credentials))
  const submitted = JSON.parse(JSON.stringify(models.value)) as MediaCapability[]
  saving.value = true; error.value = ''; notice.value = ''
  try {
    await stepUp.run(async () => {
      const latest = await adminAPI.accounts.getById(id)
      if (JSON.stringify(accountMediaCapabilities(latest.extra, latest.credentials)) !== original) throw new Error('账号模型已被其他操作更新，请刷新后重新编辑。')
      const updated = await adminAPI.accounts.update(id, { extra: { ...latest.extra, media_models: submitted } })
      if (generation !== version) return
      account.value = updated; models.value = accountMediaCapabilities(updated.extra, updated.credentials)
      notice.value = '账号模型已保存。'; void loadPricing()
    })
  } catch (cause) { if (generation === version && !isStepUpCancelled(cause)) error.value = (cause as Error).message }
  finally { saving.value = false }
}
async function loadPricing() {
  const version = ++priceGeneration
  pricing.value = []; pricingError.value = ''; pricingLoading.value = false
  if (!account.value || !group.value) return
  pricingLoading.value = true
  try { const rows = await mediaAPI.pricingStatus(Number(group.value), account.value.id); if (version === priceGeneration) pricing.value = rows }
  catch (cause) { if (version === priceGeneration) pricingError.value = (cause as Error).message }
  finally { if (version === priceGeneration) pricingLoading.value = false }
}
function pricingLabel(model: string) {
  const rows = pricing.value.filter(row => row.model === model)
  if (!group.value) return '账号未关联分组'
  if (!rows.length) return '保存模型并配置请求操作后匹配'
  return rows.map(row => `${row.operation}：${row.matched ? '已匹配 · ' + row.price_id : row.reason === 'price_conflict' ? '定价冲突' : '未匹配价格'}`).join('；')
}
watch(() => route.query.account_id, id => { if (Number(id) > 0 && Number(id) !== account.value?.id) void load(Number(id)) })
onMounted(() => { void initialize() })
</script>
<template>
  <section class="space-y-4" aria-label="账号媒体模型">
    <div class="flex flex-wrap items-end gap-3 rounded-lg border bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
      <label class="min-w-56 flex-1 text-sm">分组<select class="input mt-1 block w-full" aria-label="账号模型分组" :value="scopeGroup" :disabled="saving" @change="chooseGroup(($event.target as HTMLSelectElement).value)"><option value="">全部媒体分组</option><option v-for="item in groups" :key="item.id" :value="String(item.id)">{{ item.name }}{{ item.status === 'active' ? '' : '（已停用）' }}</option></select></label>
      <label class="text-sm">搜索账号<input v-model="search" class="input block" @keyup.enter="page = 1; list()" /></label>
      <button class="btn btn-secondary" @click="page = 1; list()">搜索</button>
      <label class="min-w-60 flex-1 text-sm">账号<select class="input mt-1 block w-full" aria-label="账号模型账号" :disabled="saving || listLoading" :value="account?.id || route.query.account_id || ''" @change="choose(Number(($event.target as HTMLSelectElement).value))"><option value="">{{ listLoading ? '正在加载账号…' : '请选择账号' }}</option><option v-if="account && !accounts.some(item=>item.id===account?.id)" :value="account.id">{{ account.name }} · #{{ account.id }}</option><option v-for="item in accounts" :key="item.id" :value="item.id">{{ item.name }} · #{{ item.id }}</option></select></label>
      <button class="btn btn-secondary" :disabled="page <= 1" @click="page--; list()">上一页</button><button class="btn btn-secondary" :disabled="page * 20 >= total" @click="page++; list()">下一页</button>
    </div>
    <p v-if="groupsError" role="alert" class="text-red-600">分组列表：{{ groupsError }} <button class="underline" @click="initialize">重新加载</button></p>
    <p v-if="!listLoading && !listError && !accounts.length && !account" class="rounded-lg border border-dashed bg-white p-6 text-sm text-gray-500 dark:bg-dark-800">当前分组没有媒体账号。请选择其他分组，或在账号管理中添加图片音频视频账号。</p>
    <p v-if="listError" role="alert" class="text-red-600">账号列表：{{ listError }}</p>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
    <p v-if="notice" role="status" class="text-green-700">{{ notice }}</p>
    <p v-if="loading" class="text-sm text-gray-500">正在读取账号…</p>
    <template v-else-if="account">
      <div class="flex flex-wrap items-center justify-between gap-3"><h2 class="text-lg font-semibold">{{ account.name }} · #{{ account.id }}</h2><button class="btn btn-primary" :disabled="saving || !dirty" @click="save">{{ saving ? '保存中…' : '保存账号模型' }}</button></div>
      <fieldset :disabled="saving" class="min-w-0"><MediaAccountModelsEditor :key="account.id" v-model="models" :account="account" :account-id="account.id" :base-url="String(account.credentials?.base_url || '')" /></fieldset>
      <section class="space-y-3 rounded-xl border p-4 dark:border-dark-600" aria-label="渠道定价匹配">
        <div class="flex flex-wrap items-center justify-between gap-3"><h3 class="font-semibold">渠道定价匹配</h3><RouterLink class="text-primary-600" to="/admin/channels/pricing">前往渠道定价</RouterLink></div>
        <p class="text-sm text-gray-500">按公开模型名精确匹配，区分大小写；分组价格优先，0 为有效价格。此处展示已保存配置的匹配状态。</p>
        <label class="text-sm">查看分组<select v-model="group" class="input ml-2" @change="loadPricing"><option value="">请选择分组</option><option v-for="id in account.group_ids" :key="id" :value="String(id)">{{ account.groups?.find(item=>item.id===id)?.name || '#' + id }}</option></select></label>
        <p v-if="pricingLoading">正在匹配…</p><p v-else-if="pricingError" role="alert" class="text-red-600">定价状态暂不可用：{{ pricingError }} <button class="underline" @click="loadPricing">重试</button></p>
        <dl v-else class="divide-y dark:divide-dark-600"><div v-for="model in models" :key="model.model" class="grid gap-1 py-2 text-sm sm:grid-cols-2"><dt class="break-all font-medium">{{ model.model }}</dt><dd>{{ pricingLabel(model.model) }}</dd></div></dl>
      </section>
    </template>
    <p v-else-if="!error" class="rounded-xl border border-dashed p-8 text-center text-gray-500">选择一个图片音频视频账号，拉取模型并绑定模板。</p>
    <TotpStepUpDialog :controller="stepUp" />
  </section>
</template>
