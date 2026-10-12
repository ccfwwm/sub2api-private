<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { mediaAPI, type MediaTaskRecord } from '@/api/media'
const props = withDefaults(defineProps<{ admin?: boolean }>(), {admin:false})
const { locale } = useI18n()
const tr = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
const tasks = ref<MediaTaskRecord[]>([]), error = ref(''), selected = ref(''), busy = ref(false)
const action = ref('refund'), amount = ref(''), reason = ref(''), decision = ref(''), upstream = ref('')
const number = (v: unknown) => v == null ? '—' : String(v)
async function refresh() { try { tasks.value = await mediaAPI.tasks(props.admin); error.value = '' } catch (e) { error.value = (e as {message?:string}).message || String(e) } }
async function more() { try { const last = tasks.value.at(-1); if (last) tasks.value.push(...await mediaAPI.tasks(props.admin,last.task.id)) } catch (e) { error.value = String(e) } }
async function run(record: MediaTaskRecord) {
  if (!reason.value.trim()) { error.value = tr('请填写原因或核对凭据','Enter a reason or reconciliation evidence'); return }
  busy.value = true; error.value = ''
  try {
    const { task } = record
    if (action.value === 'refund') await mediaAPI.refund(task.id,amount.value,reason.value,task.billing.version,crypto.randomUUID())
    else if (action.value === 'cost') await mediaAPI.verifyCost(task.id,amount.value,reason.value,task.billing.version,crypto.randomUUID())
    else await mediaAPI.reconcile(task.id,decision.value,upstream.value,reason.value,task.billing.version)
    await refresh(); amount.value = ''; reason.value = ''
  } catch (e) { error.value = (e as {message?:string}).message || String(e) }
  finally { busy.value = false }
}
onMounted(refresh)
</script>
<template>
  <section class="space-y-3 rounded-xl border p-4 dark:border-dark-600">
    <div class="flex items-center justify-between"><h2 class="text-base font-semibold">{{ tr('媒体生成任务','Media generation tasks') }}</h2><button class="btn btn-secondary" @click="refresh">{{ tr('刷新','Refresh') }}</button></div>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p><p v-if="!tasks.length && !error" class="text-sm text-gray-500">{{ tr('暂无媒体任务','No media tasks yet') }}</p>
    <div v-for="record in tasks" :key="record.task.id" class="border-t py-3 dark:border-dark-600">
      <button class="flex w-full flex-wrap justify-between gap-2 text-left text-sm" @click="selected = selected === record.task.id ? '' : record.task.id"><span class="font-medium">{{ record.task.model }} <span class="ml-2 rounded bg-gray-100 px-2 py-1 text-xs dark:bg-dark-700">{{ record.task.status }}</span></span><span>{{ tr('结算','Settled') }} ${{ record.task.billing.settled }} · {{ tr('退款','Refunded') }} ${{ record.task.billing.refunded }}</span></button>
      <div v-if="selected === record.task.id" class="mt-4 space-y-4 text-sm">
        <p class="break-all text-xs text-gray-500">{{ record.task.id }} · {{ new Date(record.task.created_at * 1000).toLocaleString() }}</p>
        <div class="grid gap-3 sm:grid-cols-4"><div v-for="(label, key) in {reserved:tr('预留','Reserved'),accepted:tr('受理费','Acceptance fee'),released:tr('释放','Released'),maximum_charge:tr('最高收费','Maximum charge')}" :key="key" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800"><span class="text-xs text-gray-500">{{ label }}</span><p class="mt-1 font-medium">${{ record.task.billing[key] }}</p></div></div>
        <p class="text-xs text-gray-500">{{ tr('媒体倍率 / 分时倍率','Media / time multiplier') }} ×{{ record.task.billing.multiplier }} / ×{{ record.task.billing.time_multiplier }} · {{ record.task.billing.state }}</p>
        <div class="overflow-x-auto"><table class="w-full text-left text-xs"><thead><tr><th class="py-2">{{ tr('收费项','Charge') }}</th><th>{{ tr('单位','Unit') }}</th><th>{{ tr('原始数量','Original quantity') }}</th><th>{{ tr('收费数量','Billed quantity') }}</th><th>{{ tr('单价 USD','Price USD') }}</th><th>{{ tr('金额 USD','Amount USD') }}</th></tr></thead><tbody><tr v-for="(line,i) in record.task.billing.calculation?.items" :key="i" class="border-t dark:border-dark-600"><td class="py-2">{{ line.name }}</td><td>{{ line.unit }}</td><td>{{ Array.isArray(line.original) ? line.original.join(' + ') : number(line.original) }}</td><td>{{ number(line.quantity) }}</td><td>{{ number(line.unit_price) }}</td><td>{{ number(line.raw_amount) }}</td></tr></tbody></table></div>
        <div class="flex flex-wrap gap-2"><span v-for="(values,key) in record.task.billing.quantities" :key="key" class="rounded bg-gray-100 px-2 py-1 text-xs dark:bg-dark-800">{{ key }}: {{ values.join(' + ') }}</span></div>
        <p class="break-all text-xs text-gray-500">{{ tr('价格版本','Price version') }} {{ record.task.billing.price_version }}</p>
        <p v-if="record.task.diagnostic" class="text-amber-700 dark:text-amber-400">{{ record.task.diagnostic }}</p>
        <p v-if="admin" class="text-xs">{{ tr('账号','Account') }} {{ record.account_id }} · {{ tr('用户','User') }} {{ record.user_id }} · {{ tr('成本状态','Cost status') }} {{ record.cost_status || 'unknown' }} · {{ tr('成本','Cost') }} {{ record.cost_amount == null ? '—' : `$${record.cost_amount}` }} · {{ tr('毛利','Margin') }} {{ record.margin_amount == null ? '—' : `$${record.margin_amount}` }}</p>
        <details v-if="admin" class="rounded-lg border p-3 dark:border-dark-600"><summary class="cursor-pointer">{{ tr('核对与账务操作','Reconciliation and accounting') }}</summary><div class="mt-3 grid gap-3 sm:grid-cols-2"><select v-model="action" class="input"><option value="refund">{{ tr('退款','Refund') }}</option><option value="cost">{{ tr('核实供应商成本','Verify supplier cost') }}</option><option value="reconcile">{{ tr('核对任务受理状态','Reconcile acceptance') }}</option></select><input v-if="action !== 'reconcile'" v-model="amount" class="input" inputmode="decimal" :placeholder="tr('金额 USD','Amount USD')" /><template v-else><select v-model="decision" class="input"><option value="">{{ tr('选择核对结论','Choose a decision') }}</option><option value="query">{{ tr('确认已受理','Accepted') }}</option><option value="failed">{{ tr('确认未受理','Not accepted') }}</option></select><input v-model="upstream" class="input" :placeholder="tr('上游任务 ID（已受理时）','Upstream task ID (if accepted)')" /></template><input v-model="reason" class="input sm:col-span-2" :placeholder="tr('操作原因 / 核对凭据（必填）','Reason / evidence (required)')" /><button class="btn btn-primary" :disabled="busy" @click="run(record)">{{ tr('执行并刷新','Apply and refresh') }}</button></div></details>
      </div>
    </div>
    <button v-if="tasks.length >= 50" class="btn btn-secondary" @click="more">{{ tr('更早任务','Older tasks') }}</button>
  </section>
</template>
