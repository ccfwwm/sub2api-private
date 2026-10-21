<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { adminAPI } from '@/api/admin'
import { mediaAPI, type MediaPricingStatus } from '@/api/media'
import type { Account, AccountListItem } from '@/types'
import { accountMediaCapabilities, type MediaCapability } from '@/types/media'
import MediaAccountModelsEditor from '@/components/account/MediaAccountModelsEditor.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { useStepUp, isStepUpCancelled } from '@/composables/useStepUp'

const route = useRoute(), router = useRouter(), stepUp = useStepUp()
const accounts = ref<AccountListItem[]>([]), account = ref<Account | null>(null)
const models = ref<MediaCapability[]>([]), search = ref(''), page = ref(1), total = ref(0)
const loading = ref(false), saving = ref(false), error = ref(''), notice = ref(''), listError = ref('')
const pricing = ref<MediaPricingStatus[]>([]), pricingError = ref(''), pricingLoading = ref(false)
const group = ref('')
const dirty = computed(() => account.value && JSON.stringify(models.value) !== JSON.stringify(accountMediaCapabilities(account.value.extra, account.value.credentials)))
let generation = 0, listGeneration = 0, priceGeneration = 0
async function list() {
  const version = ++listGeneration
  listError.value = ''
  try {
    const result = await adminAPI.accounts.list(page.value, 20, { platform: 'imagevideo', search: search.value })
    if (version !== listGeneration) return
    accounts.value = result.items; total.value = result.total
  } catch (cause) { if (version === listGeneration) listError.value = (cause as Error).message }
}
async function load(id: number) {
  const version = ++generation
  loading.value = true; account.value = null; error.value = ''; notice.value = ''; models.value = []; pricing.value = []; ++priceGeneration
  if (!Number.isSafeInteger(id) || id <= 0) { loading.value = false; return }
  try {
    const loaded = await adminAPI.accounts.getById(id)
    if (version !== generation) return
    if (loaded.platform !== 'imagevideo') throw new Error('请选择图片音频视频账号。')
    account.value = loaded; models.value = accountMediaCapabilities(loaded.extra, loaded.credentials)
    group.value = String(loaded.group_ids?.[0] || '')
    void loadPricing()
  } catch (cause) { if (version === generation) error.value = (cause as Error).message }
  finally { if (version === generation) loading.value = false }
}
function choose(id: number) {
  if (dirty.value && !window.confirm('当前模型配置尚未保存，切换账号会放弃这些修改。')) return
  void router.replace({ query: { ...route.query, account_id: String(id) } })
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
watch(() => route.query.account_id, id => { void load(Number(id)) }, { immediate: true })
onMounted(() => { void list() })
</script>
<template>
  <section class="space-y-4" aria-label="账号媒体模型">
    <div class="flex flex-wrap items-end gap-2 rounded-xl border p-4 dark:border-dark-600">
      <label class="text-sm">搜索账号<input v-model="search" class="input block" @keyup.enter="page = 1; list()" /></label>
      <button class="btn btn-secondary" @click="page = 1; list()">搜索</button>
      <label class="min-w-60 text-sm">选择账号<select class="input block w-full" :disabled="saving" :value="account?.id || route.query.account_id || ''" @change="choose(Number(($event.target as HTMLSelectElement).value))"><option value="">请选择账号</option><option v-if="account && !accounts.some(item=>item.id===account?.id)" :value="account.id">{{ account.name }} · #{{ account.id }}</option><option v-for="item in accounts" :key="item.id" :value="item.id">{{ item.name }} · #{{ item.id }}</option></select></label>
      <button class="btn btn-secondary" :disabled="page <= 1" @click="page--; list()">上一页</button><button class="btn btn-secondary" :disabled="page * 20 >= total" @click="page++; list()">下一页</button>
    </div>
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
