<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { mediaAPI, type MediaTaskRecord } from '@/api/media'
const props = defineProps<{ record: MediaTaskRecord | null; admin?: boolean }>()
const emit = defineEmits<{ close: []; updated: [] }>()
const { locale } = useI18n()
const tr = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const error = ref(''), busy = ref(false), action = ref('refund'), amount = ref(''), reason = ref(''), decision = ref(''), upstream = ref('')
const number = (v: unknown) => v == null ? '—' : String(v)
watch(() => props.record?.task.id, () => { error.value = ''; amount.value = ''; reason.value = ''; decision.value = ''; upstream.value = ''; action.value = 'refund' })
async function run(record: MediaTaskRecord) {
  if (!reason.value.trim()) { error.value = tr('请填写原因或核对凭据','Enter a reason or reconciliation evidence'); return }
  busy.value = true; error.value = ''
  try {
    const { task } = record
    if (action.value === 'refund') await mediaAPI.refund(task.id,amount.value,reason.value,task.billing.version,crypto.randomUUID())
    else if (action.value === 'cost') await mediaAPI.verifyCost(task.id,amount.value,reason.value,task.billing.version,crypto.randomUUID())
    else await mediaAPI.reconcile(task.id,decision.value,upstream.value,reason.value,task.billing.version)
    emit('updated'); amount.value = ''; reason.value = ''
  } catch (e) { error.value = (e as {message?:string}).message || String(e) }
  finally { busy.value = false }
}
</script>
<template>
  <BaseDialog :show="!!record" width="extra-wide" :title="tr('媒体任务详情','Media task details')" @close="emit('close')">
    <p v-if="error" role="alert" class="mb-3 text-sm text-red-600">{{ error }}</p>
      <div v-if="record" class="mt-4 space-y-4 text-sm">
        <p class="break-all text-xs text-gray-500">{{ record.task.id }} · {{ new Date(record.task.created_at * 1000).toLocaleString() }}</p>
        <p class="break-all font-medium">{{ record.task.model }} · {{ record.media_type }} · {{ record.task.status }}</p>
        <div class="grid gap-3 sm:grid-cols-4"><div v-for="(label, key) in {reserved:tr('预留','Reserved'),accepted:tr('受理费','Acceptance fee'),settled:tr('结算','Settled'),released:tr('释放','Released'),refunded:tr('退款','Refunded'),maximum_charge:tr('最高收费','Maximum charge')}" :key="key" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800"><span class="text-xs text-gray-500">{{ label }}</span><p class="mt-1 font-medium">${{ record.task.billing[key] }}</p></div></div>
        <p class="text-xs text-gray-500">{{ tr('媒体倍率 / 分时倍率','Media / time multiplier') }} ×{{ record.task.billing.multiplier }} / ×{{ record.task.billing.time_multiplier }} · {{ record.task.billing.state }}</p>
        <div class="overflow-x-auto"><table class="w-full text-left text-xs"><thead><tr><th class="py-2">{{ tr('收费项','Charge') }}</th><th>{{ tr('单位','Unit') }}</th><th>{{ tr('原始数量','Original quantity') }}</th><th>{{ tr('收费数量','Billed quantity') }}</th><th>{{ tr('单价 USD','Price USD') }}</th><th>{{ tr('金额 USD','Amount USD') }}</th></tr></thead><tbody><tr v-for="(line,i) in record.task.billing.calculation?.items" :key="i" class="border-t dark:border-dark-600"><td class="py-2">{{ line.name }}</td><td>{{ line.unit }}</td><td>{{ Array.isArray(line.original) ? line.original.join(' + ') : number(line.original) }}</td><td>{{ number(line.quantity) }}</td><td>{{ number(line.unit_price) }}</td><td>{{ number(line.raw_amount) }}</td></tr></tbody></table></div>
        <div class="flex flex-wrap gap-2"><span v-for="(values,key) in record.task.billing.quantities" :key="key" class="rounded bg-gray-100 px-2 py-1 text-xs dark:bg-dark-800">{{ key }}: {{ values.join(' + ') }}</span></div>
        <p class="break-all text-xs text-gray-500">{{ tr('价格版本','Price version') }} {{ record.task.billing.price_version }}</p>
        <p v-if="record.task.diagnostic" class="text-amber-700 dark:text-amber-400">{{ record.task.diagnostic }}</p>
        <p v-if="admin" class="text-xs">{{ tr('账号','Account') }} {{ record.account_id }} · {{ tr('用户','User') }} {{ record.user_id }} · {{ tr('成本状态','Cost status') }} {{ record.cost_status || 'unknown' }} · {{ tr('成本','Cost') }} {{ record.cost_amount == null ? '—' : `$${record.cost_amount}` }} · {{ tr('毛利','Margin') }} {{ record.margin_amount == null ? '—' : `$${record.margin_amount}` }}</p>
        <details v-if="admin" class="rounded-lg border p-3 dark:border-dark-600"><summary class="cursor-pointer">{{ tr('核对与账务操作','Reconciliation and accounting') }}</summary><div class="mt-3 grid gap-3 sm:grid-cols-2"><select v-model="action" class="input"><option value="refund">{{ tr('退款','Refund') }}</option><option value="cost">{{ tr('核实供应商成本','Verify supplier cost') }}</option><option value="reconcile">{{ tr('核对任务受理状态','Reconcile acceptance') }}</option></select><input v-if="action !== 'reconcile'" v-model="amount" class="input" inputmode="decimal" :placeholder="tr('金额 USD','Amount USD')" /><template v-else><select v-model="decision" class="input"><option value="">{{ tr('选择核对结论','Choose a decision') }}</option><option value="query">{{ tr('确认已受理','Accepted') }}</option><option value="failed">{{ tr('确认未受理','Not accepted') }}</option></select><input v-model="upstream" class="input" :placeholder="tr('上游任务 ID（已受理时）','Upstream task ID (if accepted)')" /></template><input v-model="reason" class="input sm:col-span-2" :placeholder="tr('操作原因 / 核对凭据（必填）','Reason / evidence (required)')" /><button class="btn btn-primary" :disabled="busy" @click="run(record)">{{ tr('执行并刷新','Apply and refresh') }}</button></div></details>
        <div v-if="record.task.data.length" class="space-y-2"><h4 class="font-medium">{{ tr('生成产物','Artifacts') }}</h4><div v-for="artifact in record.task.data" :key="artifact.key" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800"><p>{{ artifact.key }} · {{ artifact.mime_type }} · {{ artifact.available ? tr('可用','Available') : tr('已失效','Unavailable') }}</p><p v-if="artifact.size_bytes != null" class="text-xs text-gray-500">{{ tr('文件大小','File size') }}: {{ artifact.size_bytes.toLocaleString() }} B</p><p v-if="artifact.duration_seconds">{{ tr('实际时长','Actual duration') }}: {{ artifact.duration_seconds }} s</p><p class="mt-1 break-all text-xs text-gray-500">{{ artifact.content_url }}</p><p class="text-xs text-gray-500">{{ tr('下载需携带创建任务时的 API Key','Download with the API key that created this task') }}</p></div></div>
      </div>
  </BaseDialog>
</template>
