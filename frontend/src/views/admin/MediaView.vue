<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { mediaAPI, type ConfigKind, type MediaConfigRecord, type MediaTaskRecord } from '@/api/media'
import capabilityTemplates from '@/data/media-capabilities.json'
import MediaPriceEditor from '@/components/media/MediaPriceEditor.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { useStepUp, isStepUpCancelled } from '@/composables/useStepUp'

const { locale } = useI18n()
const stepUp = useStepUp()
const text = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
const kind = ref<ConfigKind>('capability')
const capabilities = ref<MediaConfigRecord[]>([])
const prices = ref<MediaConfigRecord[]>([])
const tasks = ref<MediaTaskRecord[]>([])
const error = ref('')
const busy = ref(false)
const selected = ref<MediaConfigRecord | null>(null)
const editor = ref('{}')
const histories = ref<MediaConfigRecord[]>([])
const templateID = ref('')
const simulator = ref('{\n  "operation": "video",\n  "request": {"model": "00sd2.5", "prompt": "A quiet valley", "seconds": 30},\n  "identity": {"Multiplier": "1", "TimeMultiplier": "1"}\n}')
const simulation = ref<Record<string, unknown> | null>(null)
const publishPreview = ref(false)
const taskID = ref('')
const refundAmount = ref('')
const costAmount = ref('')
const reason = ref('')
const upstreamID = ref('')
const decision = ref('query')
const newOperationKey = () => crypto.randomUUID()
const rows = computed(() => kind.value === 'capability' ? capabilities.value : prices.value)
const draft = computed<Record<string, unknown> | null>(() => { try { return JSON.parse(editor.value) } catch { return null } })
const changes = computed(() => {
  const before = selected.value?.config ?? {}
  const after = draft.value ?? {}
  return [...new Set([...Object.keys(before), ...Object.keys(after)])].filter(key => JSON.stringify(before[key]) !== JSON.stringify(after[key]))
})
const task = computed(() => tasks.value.find(record => record.task.id === taskID.value)?.task)
async function action(run: () => Promise<void>) {
  busy.value = true; error.value = ''
  try { await stepUp.run(run) } catch (cause) { if (!isStepUpCancelled(cause)) error.value = (cause as { message?: string }).message ?? String(cause) }
  finally { busy.value = false }
}
async function refresh() {
  const [caps, cards, jobs] = await Promise.all([mediaAPI.configs('capability'), mediaAPI.configs('price'), mediaAPI.tasks(true)])
  capabilities.value = caps; prices.value = cards; tasks.value = jobs
}
async function select(record: MediaConfigRecord) {
  selected.value = record; editor.value = JSON.stringify(record.config, null, 2); publishPreview.value = false
  histories.value = await mediaAPI.history(kind.value, record.id)
}
function useTemplate() {
  const template = capabilityTemplates.find(value => value.profile_id === templateID.value)
  if (!template) return
  kind.value = 'capability'; selected.value = null; histories.value = []; publishPreview.value = false
  editor.value = JSON.stringify(template, null, 2)
}
function createPrice() {
  const cap = draft.value
  const model = typeof cap?.model === 'string' ? cap.model : ''
  const groupIDs = cap?.group_ids as number[] | undefined
  const price = {
    price_id: `sale:${model || 'model'}:${crypto.randomUUID().slice(0, 8)}`, version: '1', purpose: 'sale', status: 'draft',
    currency: 'USD', media_type: cap?.media_type ?? 'video', operation: (cap?.operations as string[] | undefined)?.[0] ?? 'video',
    model_match: { kind: 'exact', value: model }, scope: { kind: 'group', reference: String(groupIDs?.[0] ?? '') },
    effective_at: null, test_only: false, multiplier_policy: 'effective_media_then_time', success_policy: 'any_output',
    rules: [{ priority: 100, when: {}, charges: [{ name: 'generation', unit: 'request', unit_price: null,
      quantity_source: 'task_count', fallback_source: null, usage_field: null, trigger: 'success', refundable: true,
      rounding: { mode: 'none', step: '1', minimum: '0', scope: 'per_output' }, method: 'linear', package_quantity: null, tiers: [] }] }],
    reservation: { maximum_charge: null, max_quantities: {}, overrun: 'cap_user_and_alert' }, notes: ''
  }
  kind.value = 'price'; selected.value = null; histories.value = []; editor.value = JSON.stringify(price, null, 2)
}
function editField(field: string, value: unknown) {
  if (!draft.value) return
  editor.value = JSON.stringify({ ...draft.value, [field]: value }, null, 2)
}
function setPurpose(value: string) {
  if (!draft.value) return
  const current = draft.value.scope as { kind: string; reference: string }
  editor.value = JSON.stringify({ ...draft.value, purpose: value, multiplier_policy: value === 'cost' ? 'account_cost_once' : 'effective_media_then_time', scope: { ...current, kind: value === 'cost' ? 'account' : 'group' } }, null, 2)
}
function numberIDs(value: string) { return value.split(',').map(value => Number(value.trim())).filter(value => Number.isInteger(value) && value > 0) }
async function save(publish: boolean) {
  if (!draft.value) throw new Error(text('JSON 格式错误', 'Invalid JSON'))
  const config = draft.value
  const id = String(config[kind.value === 'capability' ? 'profile_id' : 'price_id'] ?? '')
  if (!id) throw new Error(text('请输入配置 ID', 'Configuration ID required'))
  const expected = selected.value?.id === id ? selected.value.version : 0
  const record = await mediaAPI.save(kind.value, id, expected, config, publish)
  await refresh(); await select(record)
}
async function simulate() {
  const input = JSON.parse(simulator.value)
  if (!input.capability) input.capability = kind.value === 'capability' ? draft.value : capabilities.value.find(value => value.config.model === input.request?.model)?.config
  if (!input.price) input.price = kind.value === 'price' ? draft.value : prices.value.find(value => (value.config.model_match as { value?: string })?.value === input.request?.model && value.config.purpose === 'sale')?.config
  if (!input.capability || !input.price) throw new Error(text('选择能力配置并填写价卡后再模拟', 'Choose a capability and a configured price card'))
  simulation.value = await mediaAPI.simulate(input)
}
function download() {
  const blob = new Blob([JSON.stringify({ capabilities: capabilities.value.map(value => value.config), prices: prices.value.map(value => value.config) }, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob); const link = document.createElement('a'); link.href = url; link.download = 'media-configurations.json'; link.click(); URL.revokeObjectURL(url)
}
async function importDrafts(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  const bundle = JSON.parse(await file.text()) as { capabilities?: Record<string, unknown>[]; prices?: Record<string, unknown>[] }
  for (const [type, list] of [['capability', bundle.capabilities ?? []], ['price', bundle.prices ?? []]] as const) {
    for (const config of list) {
      const id = String(config[type === 'capability' ? 'profile_id' : 'price_id'])
      const existing = (type === 'capability' ? capabilities.value : prices.value).find(value => value.id === id)
      if (existing) throw new Error(text(`已存在 ${id}，请单独审阅更新`, `${id} exists; review it individually`))
      await mediaAPI.save(type, id, 0, config, false)
    }
  }
  await refresh()
}
onMounted(() => action(refresh))
</script>

<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6 p-4 lg:p-8">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div><h1 class="text-2xl font-semibold">{{ text('媒体模型与计费', 'Media models and billing') }}</h1><p class="mt-2 text-sm text-gray-500">{{ text('售价与成本独立配置。草稿不会开放模型，发布需要真实渠道验证和完整价格。', 'Retail prices and supplier costs are independent. Drafts stay closed; publishing requires provider verification and configured prices.') }}</p></div>
        <div class="flex gap-2"><button class="btn btn-secondary" @click="action(refresh)">{{ text('刷新', 'Refresh') }}</button><button class="btn btn-secondary" @click="download">{{ text('导出配置', 'Export') }}</button><label class="btn btn-secondary cursor-pointer">{{ text('导入草稿', 'Import drafts') }}<input class="hidden" type="file" accept="application/json" @change="action(() => importDrafts($event))" /></label></div>
      </div>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-red-700 dark:bg-red-950 dark:text-red-300">{{ error }}</p>
      <div class="grid gap-6 lg:grid-cols-[300px_1fr]">
        <aside class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-gray-700">
          <div class="flex gap-2"><button class="btn flex-1" :class="kind === 'capability' ? 'btn-primary' : 'btn-secondary'" @click="kind = 'capability'">{{ text('能力', 'Capabilities') }}</button><button class="btn flex-1" :class="kind === 'price' ? 'btn-primary' : 'btn-secondary'" @click="kind = 'price'">{{ text('价卡', 'Prices') }}</button></div>
          <select v-model="templateID" class="input w-full" :aria-label="text('模型模板', 'Model template')"><option value="">{{ text('38 个来源模型模板', '38 source model templates') }}</option><option v-for="value in capabilityTemplates" :key="value.profile_id" :value="value.profile_id">{{ value.provider }} · {{ value.model }}</option></select>
          <div class="flex gap-2"><button class="btn btn-secondary flex-1" @click="useTemplate">{{ text('新建能力', 'New capability') }}</button><button class="btn btn-secondary flex-1" @click="createPrice">{{ text('新建价卡', 'New price') }}</button></div>
          <button v-for="record in rows" :key="record.id" class="block w-full rounded-lg border p-3 text-left text-sm dark:border-gray-700" :class="selected?.id === record.id ? 'border-blue-500 bg-blue-50 dark:bg-blue-950' : ''" @click="action(() => select(record))"><span class="break-all font-medium">{{ record.id }}</span><span class="mt-1 block text-xs text-gray-500">{{ record.status }} · v{{ record.version }} · {{ text('发布版本', 'Published') }} {{ record.published_version ?? '—' }}</span></button>
          <p v-if="!rows.length" class="text-sm text-gray-500">{{ text('暂无配置。先从模板创建草稿。', 'No configuration. Start with a template.') }}</p>
        </aside>
        <section class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-gray-700">
          <div v-if="draft && kind === 'capability'" class="grid gap-3 sm:grid-cols-2">
            <label class="text-sm">{{ text('精确模型 ID', 'Exact model ID') }}<input class="input mt-1 w-full" :value="draft.model" @change="editField('model', ($event.target as HTMLInputElement).value)" /></label>
            <label class="text-sm">{{ text('上游模型 ID', 'Upstream model ID') }}<input class="input mt-1 w-full" :value="draft.upstream_model" @change="editField('upstream_model', ($event.target as HTMLInputElement).value)" /></label>
            <label class="text-sm">{{ text('绑定分组 ID，逗号分隔', 'Group IDs, comma separated') }}<input class="input mt-1 w-full" :value="(draft.group_ids as number[] | undefined)?.join(',')" @change="editField('group_ids', numberIDs(($event.target as HTMLInputElement).value))" /></label>
            <label class="text-sm">{{ text('绑定账号 ID，逗号分隔', 'Account IDs, comma separated') }}<input class="input mt-1 w-full" :value="(draft.account_ids as number[] | undefined)?.join(',')" @change="editField('account_ids', numberIDs(($event.target as HTMLInputElement).value))" /></label>
          </div>
          <div v-if="draft && kind === 'price'" class="grid gap-3 sm:grid-cols-2">
            <label class="text-sm">{{ text('用途', 'Purpose') }}<select class="input mt-1 w-full" :value="draft.purpose" @change="setPurpose(($event.target as HTMLSelectElement).value)"><option value="sale">{{ text('用户售价', 'Retail price') }}</option><option value="cost">{{ text('上游成本', 'Supplier cost') }}</option></select></label>
            <label class="text-sm">{{ text('接口用途', 'Operation') }}<select class="input mt-1 w-full" :value="draft.operation" @change="editField('operation', ($event.target as HTMLSelectElement).value)"><option v-for="op in ['video', 'image', 'image_edit', 'gemini_image', 'audio', 'chat_music']" :key="op">{{ op }}</option></select></label>
            <p class="text-sm text-gray-500 sm:col-span-2">{{ text('金额使用 USD 十进制字符串。null 表示未配置，"0" 表示免费。支持按次、产物、秒、分钟、token 和输入素材收费；组合收费、档位与取整在下方规则中配置。', 'Use USD decimal strings. null means unconfigured; "0" means free. Charge per request, output, second, minute, token or reference input. Configure combinations, tiers and rounding below.') }}</p>
          </div>
          <MediaPriceEditor v-if="draft && kind === 'price'" :config="draft" @update="editor = JSON.stringify($event, null, 2)" />
          <label class="block text-sm font-medium">{{ text('完整配置 · JSON', 'Full configuration · JSON') }}<textarea v-model="editor" class="input mt-2 min-h-[420px] w-full font-mono text-xs" spellcheck="false" /></label>
          <p class="text-xs text-gray-500">{{ text('JSON 中可配置时长来源、条件档位、组合收费、失败受理费、预留上限、独立音乐倍率、凭证所在账号和下载能力。请使用账号 ID；不要填入密钥。', 'JSON supports duration sources, conditions, composite fees, acceptance fees, reservation ceilings, music multipliers and delivery capabilities. Bind account IDs; do not enter credentials.') }}</p>
          <div class="flex flex-wrap gap-2"><button class="btn btn-primary" :disabled="busy || !draft" @click="action(() => save(false))">{{ text('保存草稿', 'Save draft') }}</button><button class="btn btn-secondary" :disabled="busy || !draft" @click="publishPreview = true">{{ text('审阅并发布', 'Review and publish') }}</button><button v-if="selected" class="btn btn-secondary" :disabled="busy" @click="action(async () => { await mediaAPI.disable(kind, selected!.id, selected!.version); await refresh() })">{{ text('停止新任务', 'Stop new tasks') }}</button></div>
          <div v-if="publishPreview" class="space-y-3 rounded-lg border border-blue-300 bg-blue-50 p-4 dark:bg-blue-950"><p class="text-sm">{{ text('本次变更字段：', 'Changed fields: ') }}{{ changes.join(', ') || '—' }}</p><div v-for="field in changes" :key="field" class="grid gap-2 text-xs sm:grid-cols-2"><pre class="overflow-auto rounded bg-white p-2 dark:bg-gray-900">{{ field }}: {{ JSON.stringify(selected?.config[field] ?? null, null, 2) }}</pre><pre class="overflow-auto rounded bg-white p-2 dark:bg-gray-900">{{ field }}: {{ JSON.stringify(draft?.[field] ?? null, null, 2) }}</pre></div><p class="text-sm">{{ text('发布影响后续新任务；在途任务保持原价卡与倍率。受理费仅在明确配置并公示后启用。', 'Publishing affects future tasks. Existing tasks retain their snapshots. Acceptance fees require explicit configuration and disclosure.') }}</p><button class="btn btn-primary" :disabled="busy" @click="action(() => save(true))">{{ text('发布此版本', 'Publish this version') }}</button></div>
          <details v-if="histories.length"><summary class="cursor-pointer text-sm">{{ text('版本历史', 'Version history') }} ({{ histories.length }})</summary><div v-for="history in histories" :key="history.version" class="mt-2"><p class="text-sm">v{{ history.version }} · {{ history.status }}</p><pre class="max-h-52 overflow-auto text-xs">{{ JSON.stringify(history.config, null, 2) }}</pre></div></details>
        </section>
      </div>
      <section class="space-y-4 rounded-xl border p-4 dark:border-gray-700"><h2 class="text-lg font-semibold">{{ text('计费模拟器', 'Billing simulator') }}</h2><p class="text-sm text-gray-500">{{ text('与实际结算共用计算器；不调用上游、不冻结、不扣费。可填写 observed_quantities 核对最终用量。', 'Uses the settlement calculator. No provider call, hold or debit. Supply observed_quantities to verify final usage.') }}</p><textarea v-model="simulator" class="input min-h-44 w-full font-mono text-xs" spellcheck="false" /><button class="btn btn-primary" :disabled="busy" @click="action(simulate)">{{ text('模拟', 'Simulate') }}</button><pre v-if="simulation" class="overflow-auto rounded-lg bg-gray-50 p-4 text-xs dark:bg-gray-900">{{ JSON.stringify(simulation, null, 2) }}</pre></section>
      <section class="space-y-4 rounded-xl border p-4 dark:border-gray-700"><h2 class="text-lg font-semibold">{{ text('任务与账务核对', 'Tasks and reconciliation') }}</h2><div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th>ID / {{ text('模型', 'Model') }}</th><th>{{ text('状态', 'Status') }}</th><th>{{ text('预留 / 收费 / 退款 USD', 'Held / charged / refunded USD') }}</th><th>{{ text('成本', 'Cost') }}</th></tr></thead><tbody><tr v-for="record in tasks" :key="record.task.id" class="border-t dark:border-gray-700"><td class="p-2"><button class="text-blue-600" @click="taskID = record.task.id">{{ record.task.id }}</button><div>{{ record.task.model }}</div></td><td>{{ record.task.status }} / {{ record.task.billing.state }}</td><td>{{ record.task.billing.reserved }} / {{ record.task.billing.accepted }} + {{ record.task.billing.settled }} / {{ record.task.billing.refunded }}</td><td>{{ record.cost_status ?? text('未知', 'Unknown') }} · {{ record.cost_amount ?? '—' }}<p class="text-xs text-gray-500">{{ record.cost_status === 'estimated' ? text('估算毛利', 'Estimated margin') : text('毛利', 'Margin') }} · {{ record.margin_amount ?? '—' }}</p></td></tr></tbody></table></div>
        <button v-if="tasks.length >= 50" class="btn btn-secondary" @click="action(async () => { tasks.push(...await mediaAPI.tasks(true,tasks.at(-1)?.task.id)) })">{{ text('更早任务', 'Older tasks') }}</button>
        <div v-if="task" class="space-y-3 rounded-lg bg-gray-50 p-4 dark:bg-gray-900"><pre class="max-h-80 overflow-auto text-xs">{{ JSON.stringify(task, null, 2) }}</pre><label class="block text-sm">{{ text('核对证据 / 退款原因', 'Evidence / refund reason') }}<input v-model="reason" class="input mt-1 w-full" /></label><div v-if="task.billing.state === 'manual_review'" class="flex flex-wrap gap-2"><select v-model="decision" class="input"><option value="query">{{ text('绑定已存在的上游任务继续查询', 'Bind existing upstream task') }}</option><option value="failed">{{ text('已核实失败，释放预留', 'Confirmed failure; release hold') }}</option></select><input v-model="upstreamID" class="input" :placeholder="text('上游任务 ID', 'Upstream task ID')" /><button class="btn btn-primary" :disabled="busy || !reason" @click="action(async () => { await mediaAPI.reconcile(task!.id, decision, upstreamID, reason, task!.billing.version); await refresh() })">{{ text('记录核对结果', 'Record reconciliation') }}</button></div><div class="flex gap-2"><input v-model="costAmount" class="input" inputmode="decimal" :placeholder="text('账单核实成本 USD', 'Verified bill cost USD')" /><button class="btn btn-secondary" :disabled="busy || !reason || !costAmount" @click="action(async () => { await mediaAPI.verifyCost(task!.id,costAmount,reason,task!.billing.version,newOperationKey()); await refresh() })">{{ text('记录成本对账', 'Record bill reconciliation') }}</button></div><div v-if="task.billing.state === 'settled'" class="flex gap-2"><input v-model="refundAmount" class="input" inputmode="decimal" :placeholder="text('退款金额 USD', 'Refund USD')" /><button class="btn btn-primary" :disabled="busy || !reason || !refundAmount" @click="action(async () => { await mediaAPI.refund(task!.id, refundAmount, reason, task!.billing.version, newOperationKey()); await refresh() })">{{ text('追加退款记录', 'Append refund') }}</button></div></div>
      </section>
    </div>
    <TotpStepUpDialog :controller="stepUp" />
  </AppLayout>
</template>
