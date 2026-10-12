<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { mediaAPI, type MediaTaskRecord } from '@/api/media'
const { locale } = useI18n()
const text = (zh: string, en: string) => (locale?.value ?? 'zh').startsWith('zh') ? zh : en
const tasks = ref<MediaTaskRecord[]>([])
const error = ref('')
const selected = ref('')
async function refresh() { try { tasks.value = await mediaAPI.tasks(); error.value = '' } catch (cause) { error.value = (cause as { message?: string }).message ?? String(cause) } }
async function more() { try { const last = tasks.value.at(-1); if (last) tasks.value.push(...await mediaAPI.tasks(false,last.task.id)) } catch (cause) { error.value = String(cause) } }
onMounted(refresh)
</script>
<template>
  <section class="space-y-3 rounded-xl border p-4 dark:border-gray-700"><div class="flex items-center justify-between"><h2 class="text-base font-semibold">{{ text('媒体生成任务', 'Media generation tasks') }}</h2><button class="btn btn-secondary" @click="refresh">{{ text('刷新', 'Refresh') }}</button></div><p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p><p v-if="!tasks.length && !error" class="text-sm text-gray-500">{{ text('暂无媒体任务', 'No media tasks yet') }}</p><div v-for="{ task } in tasks" :key="task.id" class="border-t py-3 dark:border-gray-700"><button class="flex w-full flex-wrap justify-between gap-2 text-left text-sm" @click="selected = selected === task.id ? '' : task.id"><span>{{ task.model }} · {{ task.status }}<span class="ml-2 text-xs text-gray-500">{{ task.id }}</span></span><span>USD {{ task.billing.accepted }} + {{ task.billing.settled }} · {{ text('退款', 'Refunded') }} {{ task.billing.refunded }}</span></button><div v-if="selected === task.id" class="mt-3 space-y-2 text-xs text-gray-500"><p>{{ text('价卡版本', 'Price version') }} {{ task.billing.price_version }} · {{ text('预留', 'Reserved') }} ${{ task.billing.reserved }} · {{ text('释放', 'Released') }} ${{ task.billing.released }} · {{ task.billing.state }}</p><p>{{ text('媒体倍率 / 分时倍率', 'Media / time multiplier') }} {{ task.billing.multiplier }} / {{ task.billing.time_multiplier }}</p><pre class="overflow-auto">{{ JSON.stringify(task.billing.quantities, null, 2) }}</pre><pre v-if="task.billing.calculation" class="overflow-auto">{{ JSON.stringify(task.billing.calculation, null, 2) }}</pre><p v-if="task.diagnostic">{{ task.diagnostic }}</p></div></div><button v-if="tasks.length >= 50" class="btn btn-secondary" @click="more">{{ text('更早任务', 'Older tasks') }}</button></section>
</template>
