<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { mediaAPI, type MediaTaskRecord, type MediaTaskQuery } from '@/api/media'
import { adminAPI } from '@/api/admin'
import { keysAPI, userGroupsAPI } from '@/api'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import MediaTaskDetail from './MediaTaskDetail.vue'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { imagevideoModels } from '@/data/imagevideoDocs'

const props = withDefaults(defineProps<{ admin?: boolean; startDate?: string; endDate?: string }>(), { admin: false })
const { locale } = useI18n()
const tr = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const tasks = ref<MediaTaskRecord[]>([]), total = ref(0), loading = ref(false), error = ref(''), copied = ref('')
const selected = ref<MediaTaskRecord | null>(null), page = ref(1), pageSize = ref(getPersistedPageSize()), sortOrder = ref<'asc' | 'desc'>('desc')
const filters = reactive({ task_id: '', model: '', media_type: '', status: '', settlement_state: '', api_key_id: '', group_id: '', user_id: '', account_id: '' })
const groups = ref<{ value: string; label: string }[]>([])
const names = reactive({ user: '', key: '', account: '' })
const suggestions = reactive<{ user: { id: number; label: string }[]; key: { id: number; label: string }[]; account: { id: number; label: string }[] }>({ user: [], key: [], account: [] })
const storageKey = computed(() => props.admin ? 'admin-media-hidden-columns' : 'user-media-hidden-columns')
const hidden = ref<string[]>(['cost', 'margin'])
try { const saved = JSON.parse(localStorage.getItem(storageKey.value) || 'null'); if (Array.isArray(saved)) hidden.value = saved } catch { /* use defaults */ }
const showColumns = ref(false), showFilters = ref(false)
const allColumns = computed(() => [
  ...(props.admin ? [{ key: 'user', label: tr('用户','User') }, { key: 'account', label: tr('账号','Account') }] : []),
  { key: 'key', label: tr('API 密钥','API key') }, { key: 'model', label: tr('模型 / 任务 ID','Model / task ID') },
  { key: 'group', label: tr('分组','Group') }, { key: 'type', label: tr('媒体类型','Media type') },
  { key: 'status', label: tr('任务状态','Task status') }, { key: 'costs', label: tr('费用','Charge') },
  ...(props.admin ? [{ key: 'cost', label: tr('账号成本','Account cost') }, { key: 'margin', label: tr('毛利','Margin') }] : []),
  { key: 'duration', label: tr('生成耗时','Elapsed') }, { key: 'created_at', label: tr('创建时间','Created'), sortable: true },
  { key: 'actions', label: tr('操作','Actions') }
])
const columns = computed(() => allColumns.value.filter(c => !hidden.value.includes(c.key)))
const optionalColumns = computed(() => allColumns.value.filter(c => !['model','actions'].includes(c.key)))
const modelOptions = computed(() => [...new Set([...imagevideoModels.map(m => m.id), ...tasks.value.map(r => r.task.model)])])
const typeOptions = computed(() => [{ value: '', label: tr('全部类型','All types') }, ...['image','video','audio'].map(value => ({ value, label: ({image:tr('图片','Image'),video:tr('视频','Video'),audio:tr('音乐','Music')} as Record<string,string>)[value] }))])
const statusLabel = (status: string) => ({ queued:tr('排队中','Queued'), in_progress:tr('生成中','In progress'), completed:tr('已完成','Completed'), failed:tr('失败','Failed'), unknown:tr('待核对','Unknown') }[status] || status)
const settlementLabel = (status: string) => ({ held:tr('预留中','Held'), pending_usage:tr('待结算','Pending usage'), settled:tr('已结算','Settled'), released:tr('已释放','Released'), manual_review:tr('人工核对','Manual review') }[status] || status)
const statusOptions = computed(() => [{ value: '', label: tr('全部状态','All statuses') }, ...['queued','in_progress','completed','failed','unknown'].map(value => ({value,label:statusLabel(value)}))])
const settlementOptions = computed(() => [{value:'',label:tr('全部账务状态','All billing states')}, ...['held','pending_usage','settled','released','manual_review'].map(value => ({value,label:settlementLabel(value)}))])
const money = (value: string | null | undefined) => value == null ? '—' : '$' + Number(value).toFixed(8).replace(/0+$/, '').replace(/\.$/, '')
const elapsed = (r: MediaTaskRecord) => r.task.completed_at ? `${Math.max(0,r.task.completed_at-r.task.created_at)} s` : ['failed','unknown'].includes(r.task.status) ? '—' : tr('进行中','In progress')
let requestVersion = 0, searchTimer: ReturnType<typeof setTimeout> | undefined
const searchVersions = { user: 0, key: 0, account: 0 }
async function refresh() {
  const version = ++requestVersion; loading.value = true; error.value = ''
  const query: MediaTaskQuery = { page: page.value, page_size: pageSize.value, sort_by: 'created_at', sort_order: sortOrder.value }
  for (const [key,value] of Object.entries(filters)) if (value) (query as Record<string,unknown>)[key] = key.endsWith('_id') && key !== 'task_id' ? Number(value) : value
  // The administrator usage page owns the date filter. User media history is
  // intentionally independent so the default 24-hour usage range cannot hide
  // older media tasks that still belong to the user.
  if (props.admin && props.startDate) query.start_time = new Date(props.startDate+'T00:00:00').toISOString()
  if (props.admin && props.endDate) { const end = new Date(props.endDate+'T00:00:00'); end.setDate(end.getDate()+1); query.end_time = end.toISOString() }
  try {
    const result = await mediaAPI.taskPage(props.admin,query)
    if (version !== requestVersion) return
    tasks.value = result.items; total.value = result.total
    if (result.total > 0 && page.value > result.pages) { page.value = result.pages; await refresh(); return }
    if (selected.value) selected.value = result.items.find(r => r.task.id === selected.value?.task.id) || null
  } catch (e) { if (version === requestVersion) error.value = (e as {message?:string}).message || String(e) }
  finally { if (version === requestVersion) loading.value = false }
}
function apply() { page.value = 1; void refresh() }
function reset() { Object.keys(filters).forEach(key => { (filters as Record<string,string>)[key] = '' }); names.user = names.key = names.account = ''; apply() }
function toggleColumn(key: string) { hidden.value = hidden.value.includes(key) ? hidden.value.filter(k => k !== key) : [...hidden.value,key]; try { localStorage.setItem(storageKey.value,JSON.stringify(hidden.value)) } catch { /* storage may be disabled */ } }
async function copyID(id: string) { try { await navigator.clipboard.writeText(id); copied.value = id } catch { error.value = tr('复制失败，请手动选择任务 ID','Copy failed; select the task ID manually') } }
async function search(kind: 'user' | 'key' | 'account') {
  if (!props.admin) return
  const version = ++searchVersions[kind], query = names[kind]
  try {
    const rows = kind === 'user' ? await adminAPI.usage.searchUsers(query) : kind === 'key' ? await adminAPI.usage.searchApiKeys(filters.user_id ? Number(filters.user_id) : undefined, query) : (await adminAPI.accounts.list(1,20,{search:query})).items
    if (version === searchVersions[kind]) suggestions[kind] = rows.map((r: { id: number; name?: string; email?: string }) => ({ id:r.id,label:r.email || r.name || `#${r.id}` }))
  } catch { if (version === searchVersions[kind]) suggestions[kind] = [] }
}
function onSearch(kind: 'user' | 'key' | 'account') {
  const key = kind === 'user' ? 'user_id' : kind === 'key' ? 'api_key_id' : 'account_id'
  const choice = suggestions[kind].find(s => names[kind] === `${s.label} #${s.id}`)
  filters[key] = choice ? String(choice.id) : ''
  if (choice || !names[kind]) { apply(); return }
  if (props.admin) { clearTimeout(searchTimer); searchTimer = setTimeout(() => void search(kind),250) }
}
function handleSort(key: string, order: 'asc' | 'desc') { if (key === 'created_at') { sortOrder.value = order; apply() } }
watch(() => props.admin ? [props.startDate,props.endDate] : [], apply)
onMounted(async () => {
  void refresh()
  try {
    groups.value = (props.admin ? (await adminAPI.groups.list(1,1000)).items : await userGroupsAPI.getAvailable()).map(g => ({value:String(g.id),label:g.name}))
    if (props.admin) void search('key')
    else suggestions.key = (await keysAPI.list(1,1000)).items.map(k => ({id:k.id,label:k.name || `#${k.id}`}))
  } catch { /* historical identifiers remain searchable */ }
})
onUnmounted(() => { ++requestVersion; clearTimeout(searchTimer) })
</script>

<template>
  <section data-testid="media-task-table" class="min-w-0">
    <div class="flex items-center justify-between gap-3 border-b border-gray-100 p-4 dark:border-dark-700/50 sm:hidden">
      <button class="btn btn-secondary" :aria-expanded="showFilters" @click="showFilters = !showFilters">{{ showFilters ? tr('收起筛选','Hide filters') : tr('筛选与列设置','Filters and columns') }}</button>
      <button class="btn btn-secondary" :disabled="loading" @click="refresh">{{ tr('刷新','Refresh') }}</button>
    </div>
    <div :class="showFilters ? 'flex' : 'hidden sm:flex'" class="flex-wrap items-end gap-4 border-b border-gray-100 p-4 dark:border-dark-700/50 sm:p-6">
      <div v-if="admin" class="w-full sm:w-52"><label class="input-label">{{ tr('用户','User') }}</label><input v-model="names.user" list="media-users" class="input" :placeholder="tr('按邮箱搜索用户…','Search by email…')" @input="onSearch('user')" /><datalist id="media-users"><option v-for="s in suggestions.user" :key="s.id" :value="`${s.label} #${s.id}`" /></datalist></div>
      <div class="w-full sm:w-52"><label class="input-label">{{ tr('API 密钥','API key') }}</label><input v-model="names.key" list="media-keys" class="input" :placeholder="tr('按名称搜索密钥…','Search by name…')" @input="onSearch('key')" /><datalist id="media-keys"><option v-for="s in suggestions.key" :key="s.id" :value="`${s.label} #${s.id}`" /></datalist></div>
      <div class="w-full sm:w-60"><label class="input-label">{{ tr('模型','Model') }}</label><input v-model="filters.model" list="media-models" class="input" :placeholder="tr('全部模型','All models')" @change="apply" /><datalist id="media-models"><option v-for="model in modelOptions" :key="model" :value="model" /></datalist></div>
      <div v-if="admin" class="w-full sm:w-52"><label class="input-label">{{ tr('账号','Account') }}</label><input v-model="names.account" list="media-accounts" class="input" :placeholder="tr('按名称搜索账号…','Search accounts…')" @input="onSearch('account')" /><datalist id="media-accounts"><option v-for="s in suggestions.account" :key="s.id" :value="`${s.label} #${s.id}`" /></datalist></div>
      <div class="w-full sm:w-44"><label class="input-label">{{ tr('分组','Group') }}</label><Select v-model="filters.group_id" :options="[{value:'',label:tr('全部分组','All groups')},...groups]" searchable @change="apply" /></div>
      <div class="w-full sm:w-40"><label class="input-label">{{ tr('媒体类型','Media type') }}</label><Select v-model="filters.media_type" :options="typeOptions" @change="apply" /></div>
      <div class="w-full sm:w-40"><label class="input-label">{{ tr('任务状态','Task status') }}</label><Select v-model="filters.status" :options="statusOptions" @change="apply" /></div>
      <div class="w-full sm:w-40"><label class="input-label">{{ tr('账务状态','Billing state') }}</label><Select v-model="filters.settlement_state" :options="settlementOptions" @change="apply" /></div>
      <div class="w-full sm:w-60"><label class="input-label">{{ tr('任务 ID','Task ID') }}</label><input v-model="filters.task_id" class="input" :placeholder="tr('完整任务 ID','Exact task ID')" @change="apply" /></div>
      <div class="ml-auto flex items-center gap-2">
        <button class="btn btn-secondary" :disabled="loading" @click="refresh">{{ tr('刷新','Refresh') }}</button>
        <button class="btn btn-secondary" @click="reset">{{ tr('重置','Reset') }}</button>
        <div class="relative"><button class="btn btn-secondary" @click="showColumns = !showColumns">{{ tr('列设置','Columns') }}</button><div v-if="showColumns" class="absolute right-0 top-full z-30 mt-2 w-48 rounded-lg border bg-white p-2 shadow-lg dark:border-dark-600 dark:bg-dark-800"><label v-for="c in optionalColumns" :key="c.key" class="flex cursor-pointer items-center gap-2 p-2 text-sm"><input type="checkbox" :checked="!hidden.includes(c.key)" @change="toggleColumn(c.key)" />{{ c.label }}</label></div></div>
      </div>
    </div>
    <p v-if="error" role="alert" class="p-4 text-sm text-red-600">{{ error }} <button class="underline" @click="refresh">{{ tr('重试','Retry') }}</button></p>
    <DataTable :columns="columns" :data="tasks" :loading="loading" :row-key="r => r.task.id" :expandable-actions="false" :server-side-sort="true" default-sort-key="created_at" default-sort-order="desc" @sort="handleSort">
      <template #cell-user="{ row }"><span class="text-primary-600 dark:text-primary-400">{{ row.user_email || `#${row.user_id}` }}</span></template>
      <template #cell-account="{ row }">{{ row.account_name || `#${row.account_id}` }}</template>
      <template #cell-key="{ row }">{{ row.api_key_name || `#${row.api_key_id}` }}</template>
      <template #cell-model="{ row }"><p class="font-medium">{{ row.task.model }}</p><button class="mt-1 max-w-64 truncate text-xs text-gray-400 hover:text-primary-600" :title="row.task.id" @click="copyID(row.task.id)">{{ copied === row.task.id ? tr('已复制','Copied') : row.task.id }}</button></template>
      <template #cell-group="{ row }"><span class="rounded bg-primary-50 px-2 py-1 text-xs text-primary-600 dark:bg-primary-900/30 dark:text-primary-400">{{ row.group_name || `#${row.group_id}` }}</span></template>
      <template #cell-type="{ row }">{{ typeOptions.find(o => o.value === row.media_type)?.label || row.media_type }}</template>
      <template #cell-status="{ row }"><span class="rounded px-2 py-1 text-xs" :class="row.task.status === 'completed' ? 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-400' : row.task.status === 'failed' ? 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-400' : 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'">{{ statusLabel(row.task.status) }}</span><p class="mt-2 text-xs text-gray-400">{{ settlementLabel(row.task.billing.state) }}</p></template>
      <template #cell-costs="{ row }"><span class="font-medium text-green-600 dark:text-green-400">{{ money(String(Number(row.task.billing.accepted) + Number(row.task.billing.settled) - Number(row.task.billing.refunded))) }}</span><p v-if="Number(row.task.billing.refunded)" class="mt-1 text-xs text-gray-400">{{ tr('退款','Refund') }} {{ money(row.task.billing.refunded) }}</p></template>
      <template #cell-cost="{ row }">{{ money(row.cost_amount) }}<p class="text-xs text-gray-400">{{ row.cost_status || tr('未知','Unknown') }}</p></template>
      <template #cell-margin="{ row }">{{ money(row.margin_amount) }}</template>
      <template #cell-duration="{ row }">{{ elapsed(row) }}</template>
      <template #cell-created_at="{ row }"><span class="whitespace-nowrap text-xs">{{ new Date(row.task.created_at * 1000).toLocaleString() }}</span></template>
      <template #cell-actions="{ row }"><button class="text-sm text-primary-600 hover:underline dark:text-primary-400" @click="selected = row">{{ tr('详情','Details') }}</button></template>
    </DataTable>
    <Pagination v-if="total > 0" :page="page" :total="total" :page-size="pageSize" show-jump @update:page="page = $event; refresh()" @update:page-size="pageSize = $event; apply()" />
    <MediaTaskDetail :record="selected" :admin="admin" @close="selected = null" @updated="refresh" />
  </section>
</template>
