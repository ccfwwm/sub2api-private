<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { mediaAPI, type MediaTaskReview, type MediaTaskRecord } from '@/api/media'
const props = defineProps<{ record: MediaTaskRecord | null; admin?: boolean }>()
const emit = defineEmits<{ close: []; updated: [] }>()
const { locale } = useI18n()
const tr = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const error = ref(''), busy = ref(false), action = ref('review'), amount = ref(''), reason = ref(''), decision = ref('failed'), upstream = ref('')
const disposition = ref('refund')
const operationKey = ref(crypto.randomUUID())
const number = (v: unknown) => v == null ? '—' : String(v)
const ledgerUnits = (v: string | undefined) => { const [whole = '0', fraction = ''] = (v || '0').split('.'); return BigInt(whole) * 100000000n + BigInt(fraction.padEnd(8, '0').slice(0, 8)) }
const netCharge = (task: MediaTaskRecord['task']) => { const units = ledgerUnits(task.billing.accepted) + ledgerUnits(task.billing.settled) - ledgerUnits(task.billing.refunded); return `${units / 100000000n}.${String(units % 100000000n).padStart(8, '0')}` }
const review = ref<MediaTaskReview | null>(null), loading = ref(false)
const current = computed(() => review.value?.record || props.record)
const copied = ref('')
async function copyEvidence(id: string, value: unknown) {
  try { await navigator.clipboard.writeText(JSON.stringify(value, null, 2)); copied.value = id }
  catch { error.value = tr('复制失败，请在下方选中内容手动复制。','Copy failed. Select the content below to copy it manually.') }
}
const access = ref<Record<string, {url: string; expires_at: number}>>({})
let sequence = 0
onBeforeUnmount(() => { sequence++ })
watch(() => props.record, async () => {
  const generation = ++sequence
  review.value = null; access.value = {}; copied.value = ''; error.value = ''; amount.value = ''; reason.value = ''; decision.value = 'failed'; upstream.value = ''; action.value = 'review'; disposition.value = 'refund'; operationKey.value = crypto.randomUUID(); busy.value = false; loading.value = false
  if (!props.admin || !props.record) return
  loading.value = true
  try { const result = await mediaAPI.detail(props.record.task.id); if (generation === sequence) review.value = result }
  catch (e) { if (generation === sequence) error.value = (e as Error).message || tr('读取最新任务失败','Failed to read latest task') }
  finally { if (generation === sequence) loading.value = false }
}, { immediate: true })
async function preview(key: string) {
  if (!current.value) return
  const id = current.value.task.id
  const generation = sequence
  try { const result = await mediaAPI.artifactAccess(id, key); if (generation === sequence) access.value[key] = result }
  catch (e) { if (generation === sequence) error.value = (e as Error).message || String(e) }
}
async function exportReview() {
  if (!current.value) return
  const generation = sequence, id = current.value.task.id
  try {
    const blob = await mediaAPI.exportTask(id)
    if (generation !== sequence) return
    const url = URL.createObjectURL(blob), link = document.createElement('a')
    link.href = url; link.download = `${id}.json`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (e) { if (generation === sequence) error.value = (e as Error).message || String(e) }
}

async function run(record: MediaTaskRecord) {
  if (!reason.value.trim()) { error.value = tr('请填写原因或核对凭据','Enter a reason or reconciliation evidence'); return }
  busy.value = true; error.value = ''
  const generation = sequence
  try {
    const { task } = record
    if (action.value === 'review') {
      const net = netCharge(task)
      await mediaAPI.review(task.id, decision.value, disposition.value === 'refund' ? '0' : disposition.value === 'keep' ? net : amount.value, reason.value, task.billing.version, operationKey.value)
    }
    else if (action.value === 'refund') await mediaAPI.refund(task.id,amount.value,reason.value,task.billing.version,crypto.randomUUID())
    else if (action.value === 'cost') await mediaAPI.verifyCost(task.id,amount.value,reason.value,task.billing.version,crypto.randomUUID())
    else await mediaAPI.reconcile(task.id,'query',upstream.value,reason.value,task.billing.version)
    emit('updated')
    if (generation !== sequence) return
    operationKey.value = crypto.randomUUID()
    const latest = await mediaAPI.detail(task.id)
    if (generation !== sequence) return
    review.value = latest; amount.value = ''; reason.value = ''
  } catch (e) {
    if (generation !== sequence) return
    const failure = e as {message?:string; status?:number; error?:{message?:string}; response?:{data?:{error?:{message?:string}}}}
    error.value = failure.error?.message || failure.response?.data?.error?.message || failure.message || String(e)
    if (failure.status === 409 || error.value.includes('409') || error.value.includes('version_conflict')) {
      try { const latest = await mediaAPI.detail(record.task.id); if (generation !== sequence) return; review.value = latest; error.value = tr('任务状态已更新，已刷新最新数据。请核对后重新执行。','Task changed. Latest data loaded; review before retrying.'); operationKey.value = crypto.randomUUID() } catch { /* Keep the original error. */ }
    }
  }
  finally { if (generation === sequence) busy.value = false }
}
</script>
<template>
  <BaseDialog :show="!!record" width="extra-wide" :title="tr('媒体任务详情','Media task details')" @close="emit('close')">
    <p v-if="loading" class="text-sm text-gray-500">{{ tr('正在读取最新状态…','Loading latest status…') }}</p>
    <p v-if="error" role="alert" class="mb-3 text-sm text-red-600">{{ error }}</p>
      <div v-if="current" class="mt-4 space-y-4 text-sm">
        <p class="break-all text-xs text-gray-500">{{ current.task.id }} · {{ new Date(current.task.created_at * 1000).toLocaleString() }}</p>
        <p class="break-all font-medium">{{ current.task.model }} · {{ current.media_type }} · {{ current.task.status }}</p>
        <div class="grid gap-3 sm:grid-cols-4"><div v-for="(label, key) in {reserved:tr('预留','Reserved'),accepted:tr('受理费','Acceptance fee'),settled:tr('结算','Settled'),released:tr('释放','Released'),refunded:tr('退款','Refunded'),maximum_charge:tr('最高收费','Maximum charge')}" :key="key" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800"><span class="text-xs text-gray-500">{{ label }}</span><p class="mt-1 font-medium">${{ current.task.billing[key] }}</p></div></div>
        <p class="text-xs text-gray-500">{{ tr('媒体倍率 / 分时倍率','Media / time multiplier') }} ×{{ current.task.billing.multiplier }} / ×{{ current.task.billing.time_multiplier }} · {{ current.task.billing.state }}</p>
        <div class="overflow-x-auto"><table class="w-full text-left text-xs"><thead><tr><th class="py-2">{{ tr('收费项','Charge') }}</th><th>{{ tr('单位','Unit') }}</th><th>{{ tr('原始数量','Original quantity') }}</th><th>{{ tr('收费数量','Billed quantity') }}</th><th>{{ tr('单价 USD','Price USD') }}</th><th>{{ tr('金额 USD','Amount USD') }}</th></tr></thead><tbody><tr v-for="(line,i) in current.task.billing.calculation?.items" :key="i" class="border-t dark:border-dark-600"><td class="py-2">{{ line.name }}</td><td>{{ line.unit }}</td><td>{{ Array.isArray(line.original) ? line.original.join(' + ') : number(line.original) }}</td><td>{{ number(line.quantity) }}</td><td>{{ number(line.unit_price) }}</td><td>{{ number(line.raw_amount) }}</td></tr></tbody></table></div>
        <div class="flex flex-wrap gap-2"><span v-for="(values,key) in current.task.billing.quantities" :key="key" class="rounded bg-gray-100 px-2 py-1 text-xs dark:bg-dark-800">{{ key }}: {{ values.join(' + ') }}</span></div>
        <p class="break-all text-xs text-gray-500">{{ tr('价格版本','Price version') }} {{ current.task.billing.price_version }}</p>
        <p v-if="current.task.diagnostic" class="text-amber-700 dark:text-amber-400">{{ current.task.diagnostic }}</p>
        <p v-if="admin" class="text-xs">{{ tr('账号','Account') }} {{ current.account_name }} (#{{ current.account_id }}) · {{ tr('用户','User') }} {{ current.user_email }} (#{{ current.user_id }}) · {{ tr('成本状态','Cost status') }} {{ current.cost_status || 'unknown' }} · {{ tr('成本','Cost') }} {{ current.cost_amount == null ? '—' : `$${current.cost_amount}` }} · {{ tr('毛利','Margin') }} {{ current.margin_amount == null ? '—' : `$${current.margin_amount}` }}</p>
        <div v-if="admin" class="flex gap-3"><button class="btn btn-secondary" @click="exportReview">{{ tr('导出审查 JSON','Export review JSON') }}</button></div>
        <details v-if="admin" open class="rounded-lg border p-3 dark:border-dark-600"><summary class="cursor-pointer">{{ tr('人工核对与账务操作','Manual review and accounting') }}</summary>
          <p class="mt-2 text-xs text-gray-500">{{ tr('适用于所有任务。金额为最终实收总额，不是追加金额；全额退回会释放预留并退还已扣费用。核对后停止自动处理，原始账务记录保留。','Available for every task. Amount is the final net charge. Full return releases holds and refunds charges. Review stops automatic processing and preserves the ledger.') }}</p>
          <div class="mt-3 grid gap-3 sm:grid-cols-2">
            <select v-model="action" class="input"><option value="review">{{ tr('核对任务状态与费用','Review status and charge') }}</option><option value="cost">{{ tr('核实供应商成本','Verify supplier cost') }}</option><option value="reconcile">{{ tr('绑定原任务继续查询','Bind original task and resume') }}</option><option value="refund">{{ tr('部分退款（已结算任务）','Partial refund (settled tasks)') }}</option></select>
            <template v-if="action === 'review'">
              <select v-model="decision" class="input" aria-label="任务状态"><option value="failed">{{ tr('失败','Failed') }}</option><option value="completed">{{ tr('已完成','Completed') }}</option></select>
              <select v-model="disposition" class="input" aria-label="账务处理"><option value="refund">{{ tr('全额退回 / 释放预留','Return all / release hold') }}</option><option value="charge">{{ tr('按核对金额扣费','Set final charge') }}</option><option value="keep">{{ tr('保留已扣费用，释放剩余预留','Keep net charge; release hold') }}</option></select>
              <input v-if="disposition === 'charge'" v-model="amount" class="input" inputmode="decimal" :placeholder="tr('最终实收金额（不超过最高收费）','Final net charge (within task maximum)')" />
            </template>
            <template v-else-if="action === 'reconcile'"><input v-model="upstream" class="input" :placeholder="tr('上游任务 ID（已受理时）','Existing upstream task ID')" /></template>
            <input v-else v-model="amount" class="input" inputmode="decimal" :placeholder="tr('金额 USD','Amount USD')" />
            <input v-model="reason" class="input sm:col-span-2" :placeholder="tr('操作原因 / 核对凭据（必填）','Reason / evidence (required)')" />
            <button class="btn btn-primary" :disabled="busy || loading" data-testid="accounting-submit" @click="run(current)">{{ tr('执行并刷新','Apply and refresh') }}</button>
          </div>
        </details>
        <div v-if="current.task.data.length" class="space-y-2"><h4 class="font-medium">{{ tr('生成产物','Artifacts') }}</h4><div v-for="artifact in current.task.data" :key="artifact.key" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800"><p>{{ artifact.key }} · {{ artifact.mime_type }} · {{ artifact.available ? tr('可用','Available') : tr('已失效','Unavailable') }}</p><p v-if="artifact.size_bytes != null" class="text-xs text-gray-500">{{ tr('文件大小','File size') }}: {{ artifact.size_bytes.toLocaleString() }} B</p><p v-if="artifact.duration_seconds">{{ tr('实际时长','Actual duration') }}: {{ artifact.duration_seconds }} s</p><div v-if="admin && artifact.available" class="mt-2 space-y-2">
          <button class="btn btn-secondary" @click="preview(artifact.key)">{{ tr('预览 / 更新下载链接','Preview / renew download link') }}</button>
          <template v-if="access[artifact.key]">
            <img v-if="artifact.type === 'image'" :src="access[artifact.key]!.url" class="max-h-96 max-w-full rounded" :alt="artifact.key" referrerpolicy="no-referrer" />
            <video v-else-if="artifact.type === 'video'" :src="access[artifact.key]!.url" controls preload="metadata" class="max-h-96 w-full" />
            <audio v-else-if="artifact.type === 'audio'" :src="access[artifact.key]!.url" controls preload="metadata" />
            <a :href="access[artifact.key]!.url + '&download=1'" class="btn btn-secondary" referrerpolicy="no-referrer">{{ tr('下载原文件','Download original') }}</a>
            <p class="text-xs text-gray-500">{{ tr('链接 5 分钟内有效；过期后可重新获取','Link expires in 5 minutes; renew after expiry') }}</p>
          </template>
        </div><p v-if="!admin" class="text-xs text-gray-500">{{ tr('下载需携带创建任务时的 API Key','Download with the API key that created this task') }}</p></div></div>
        <p v-if="admin && review" data-testid="submission-stage">{{ tr('实际上游提交','Upstream submission') }}: {{ review.submission?.stage === 'accepted' ? tr('上游已受理','Accepted') : review.submission?.stage === 'attempted' ? tr('已尝试发送','Attempted') : review.submission?.stage === 'prepared' ? tr('准备发送','Prepared') : tr('未记录','Not recorded') }}<span v-if="review.submission"> · {{ JSON.stringify(review.submission.references) }}</span></p>
        <section v-if="admin && review" class="space-y-3 rounded-lg border p-3 dark:border-dark-600" aria-label="管理员请求与响应证据">
          <div><h4 class="font-medium">{{ tr('请求与响应证据','Request and response evidence') }}</h4><p class="mt-1 text-xs text-gray-500">{{ tr('按阶段分开显示：用户入站、归一化参数、模板生成的准备请求、实际发给上游的请求，以及上游响应。媒体正文和带签名的素材 URL 对管理员完整保留，可复制复测；账号认证密钥和本地文件路径仍会保护。','Stages are separated so the administrator can reproduce the call. Media payloads and signed media URLs remain available for reproduction; account credentials and local paths stay protected.') }}</p></div>
          <details open class="rounded border p-3"><summary class="cursor-pointer font-medium">{{ tr('1. 用户入站请求','1. Incoming request') }}</summary><button type="button" class="btn btn-secondary mt-2" data-testid="copy-incoming" @click="copyEvidence('incoming', review.incoming_request ?? {})">{{ copied === 'incoming' ? tr('已复制','Copied') : tr('复制 JSON','Copy JSON') }}</button><pre class="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-all text-xs">{{ JSON.stringify(review.incoming_request ?? {}, null, 2) }}</pre></details>
          <details class="rounded border p-3"><summary class="cursor-pointer font-medium">{{ tr('2. 归一化与计费前参数','2. Normalized parameters') }}</summary><pre class="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-all text-xs">{{ JSON.stringify(review.normalized_request ?? review.request ?? {}, null, 2) }}</pre></details>
          <details class="rounded border p-3"><summary class="cursor-pointer font-medium">{{ tr('3. 模板准备请求','3. Template prepared request') }}</summary><pre class="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-all text-xs">{{ JSON.stringify(review.prepared_request ?? {}, null, 2) }}</pre></details>
          <details class="rounded border p-3"><summary class="cursor-pointer font-medium">{{ tr('4. 实际上游请求','4. Actual upstream request') }}</summary><button type="button" class="btn btn-secondary mt-2" data-testid="copy-outgoing" @click="copyEvidence('outgoing', review.upstream_request?.body ?? review.submission?.request ?? {})">{{ copied === 'outgoing' ? tr('已复制','Copied') : tr('复制 JSON','Copy JSON') }}</button><div class="mt-2 text-xs"><p>{{ review.upstream_request?.method || review.submission?.method || '—' }} {{ review.upstream_request?.path || review.submission?.path || '—' }} · {{ review.upstream_request?.encoding || review.submission?.encoding || '—' }}</p><pre class="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-all">{{ JSON.stringify({ body: review.upstream_request?.body ?? review.submission?.request ?? {}, files: review.upstream_request?.files ?? review.submission?.files ?? [] }, null, 2) }}</pre></div></details>
          <details class="rounded border p-3"><summary class="cursor-pointer font-medium">{{ tr('5. 上游提交与响应','5. Upstream submission and response') }}</summary><button type="button" class="btn btn-secondary mt-2" data-testid="copy-response" @click="copyEvidence('response', review.submission?.response ?? review.upstream_response ?? review.response ?? {})">{{ copied === 'response' ? tr('已复制','Copied') : tr('复制 JSON','Copy JSON') }}</button><div class="mt-2 text-xs"><p>{{ tr('提交状态','Submission stage') }}: {{ review.submission?.stage || tr('未记录','Not recorded') }} · HTTP {{ review.submission?.response_status_code || '—' }} · {{ review.submission?.response_content_type || '—' }}</p><pre class="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-all">{{ JSON.stringify(review.submission?.response ?? review.upstream_response ?? review.response ?? {}, null, 2) }}</pre></div></details>
        </section>
      </div>
  </BaseDialog>
</template>
