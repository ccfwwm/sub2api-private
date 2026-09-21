<template>
  <div class="relative text-xs">
    <button class="rounded-lg bg-gray-100 px-2 py-1 dark:bg-dark-800" @click="toggle">
      v{{ version || '…' }} · 私有版本
    </button>
    <div v-if="open" class="absolute bottom-full left-0 z-50 mb-2 w-80 space-y-3 rounded-xl border bg-white p-4 shadow-lg dark:border-dark-600 dark:bg-dark-800">
      <div class="flex items-center justify-between">
        <strong>镜像更新</strong>
        <button aria-label="关闭" @click="open = false">×</button>
      </div>
      <a :href="`https://github.com/${repository}/releases`" target="_blank" rel="noopener noreferrer" class="text-primary-600">查看私有发布版本 ↗</a>
      <p>构建不会自动上线。先准备版本并验证备份，再手动执行切换。</p>
      <p v-if="error" role="alert" class="break-words text-red-600">{{ error }}</p>
      <p v-if="state && !state.enabled" class="text-amber-700">生产部署尚未启用，请先由服务器管理员完成安装与授权。</p>
      <p v-if="releaseError" class="text-amber-700">{{ releaseError }}</p>
      <label class="block">已构建并验证的正式版本
        <select v-model="target" class="mt-1 w-full rounded border p-2 dark:bg-dark-900" :disabled="busy || !versions.length">
          <option value="">选择版本</option>
          <option v-for="release in versions" :key="release.version" :value="release.version">{{ release.version }}</option>
        </select>
      </label>
      <label class="flex gap-2">
        <input v-model="confirmed" type="checkbox" :disabled="busy" />
        <span>我已选择合适时段：准备会读取生产数据库并占用服务器资源；切换或回滚会短暂停服。</span>
      </label>
      <button class="btn btn-secondary w-full" :disabled="!canPrepare" @click="act('prepare')">准备版本：备份与隔离验证</button>
      <div v-if="state?.job" class="space-y-2 border-t pt-3">
        <p>版本：{{ state.job.version }}</p>
        <p role="status">状态：{{ statusText }}</p>
        <p v-if="state.job.error" class="text-red-600">{{ state.job.error }}</p>
        <p v-if="state.job.status === 'ready'">验证完成。准备结果一小时内有效；执行切换前仍会再次备份。</p>
        <button v-if="state.job.status === 'ready'" class="btn btn-primary w-full" :disabled="!confirmed || busy" @click="act('deploy')">确认切换生产版本</button>
        <button v-if="state.job.status === 'succeeded'" class="btn btn-secondary w-full" :disabled="!confirmed || busy" @click="act('rollback')">回滚到本次部署前的程序</button>
      </div>
      <button class="text-primary-600" :disabled="busy" @click="refresh">刷新部署状态</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { getImageDeployment, getImageReleases, requestImageDeployment, type ImageDeploymentState, type RollbackVersionInfo } from '@/api/admin/system'

withDefaults(defineProps<{ version?: string; repository?: string }>(), { repository: 'ccfwwm/sub2api-private' })
const open = ref(false)
const target = ref('')
const confirmed = ref(false)
const busy = ref(false)
const error = ref('')
const releaseError = ref('')
const versions = ref<RollbackVersionInfo[]>([])
const state = ref<ImageDeploymentState | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
const terminal = new Set(['ready', 'succeeded', 'failed', 'rolled_back', 'interrupted'])
const running = computed(() => !!state.value?.job && !terminal.has(state.value.job.status))
const canPrepare = computed(() => state.value?.enabled && confirmed.value && !busy.value && !running.value && /^v[1-9]\d*\.\d+\.\d+$/.test(target.value))
const labels: Record<string, string> = {
  queued_prepare: '准备任务已提交', validating: '校验版本', backing_up: '备份中',
  restoring_and_testing: '隔离恢复与启动验证', ready: '待手动切换', queued_deploy: '切换任务已提交',
  checking_cutover: '核对生产状态', final_backup: '切换前备份', draining: '等待在途请求结束',
  switching: '切换中', succeeded: '部署成功', queued_rollback: '回滚任务已提交',
  rolling_back: '程序回滚中', rolled_back: '已回滚程序，数据库未恢复覆盖',
  failed: '失败，请检查宿主机记录', interrupted: '任务被中断，需要管理员检查'
}
const statusText = computed(() => labels[state.value?.job?.status || ''] || state.value?.job?.status)

function message(err: unknown): string {
  const e = err as { response?: { data?: { message?: string } }; message?: string }
  return e.response?.data?.message || e.message || '部署服务暂时不可用'
}
async function refresh() {
  clearTimeout(timer)
  try {
    state.value = await getImageDeployment()
    error.value = ''
  } catch (err) { error.value = message(err) }
  // Keep retrying during the brief period when the application is being replaced.
  if (open.value && !disposed) timer = setTimeout(refresh, running.value ? 3000 : 15000)
}
async function toggle() {
  open.value = !open.value
  if (open.value) {
    void refresh()
    try {
      versions.value = (await getImageReleases()).versions
      releaseError.value = versions.value.length ? '' : '暂无已验证的正式版本'
    } catch { releaseError.value = '无法读取私有版本列表，请检查 GitHub 只读凭据。' }
  }
  else clearTimeout(timer)
}
async function act(action: 'prepare' | 'deploy' | 'rollback') {
  if (!confirmed.value || busy.value) return
  busy.value = true
  try {
    const result = await requestImageDeployment({ action, version: target.value, job_id: state.value?.job?.id, confirm: true })
    if (state.value) state.value.job = result.job
    confirmed.value = false
    await refresh()
  } catch (err) { error.value = message(err) }
  finally { busy.value = false }
}
onBeforeUnmount(() => { disposed = true; clearTimeout(timer) })
</script>
