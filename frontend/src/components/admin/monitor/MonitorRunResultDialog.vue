<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.runResultTitle')"
    width="normal"
    @close="$emit('close')"
  >
    <div class="space-y-2">
      <div
        v-for="r in results"
        :key="r.model"
        class="flex flex-col gap-3 rounded-lg border border-gray-200 px-3 py-2 text-sm sm:flex-row sm:items-center sm:justify-between dark:border-dark-600"
      >
        <div class="flex min-w-0 flex-col">
          <span class="font-medium text-gray-900 dark:text-white">{{ formatMonitorModel(r.model) }}</span>
          <span v-if="r.message" class="break-words text-xs text-gray-500 [overflow-wrap:anywhere] dark:text-gray-400">{{ r.message }}</span>
          <MonitorQuotaView :snapshot="r.quota" class="mt-1" />
          <button
            v-if="editable && canReset(r)"
            type="button"
            class="mt-1 self-start text-xs font-medium text-amber-700 underline dark:text-amber-300"
            @click="$emit('reset', r.model)"
          >
            {{ locale.startsWith('zh') ? '已核对上游任务，确认后允许重试' : 'Confirm upstream check and allow retry' }}
          </button>
        </div>
        <div class="flex shrink-0 items-center gap-2 whitespace-nowrap">
          <span
            class="inline-flex items-center rounded-full px-2 py-0.5 text-[11px]"
            :class="statusBadgeClass(r.status)"
          >
            {{ statusLabel(r.status) }}
          </span>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatLatency(r.latency_ms) }} ms</span>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex flex-wrap justify-end gap-2">
        <button v-if="editable" class="btn btn-secondary" @click="$emit('edit')">
          {{ t('admin.channelMonitor.editDetectionSettings') }}
        </button>
        <button @click="$emit('close')" class="btn btn-primary">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { CheckResult } from '@/api/admin/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'
import MonitorQuotaView from '@/components/common/MonitorQuotaView.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'

defineProps<{
  show: boolean
  results: CheckResult[]
  editable?: boolean
}>()

defineEmits<{
  (e: 'close'): void
  (e: 'edit'): void
  (e: 'reset', model: string): void
}>()

const { t, locale } = useI18n()
const { statusLabel, statusBadgeClass, formatLatency, formatMonitorModel } = useChannelMonitorFormat()
const canReset = (result: CheckResult) => result.message.includes('acceptance uncertain') || result.message.includes('reset task')
</script>
