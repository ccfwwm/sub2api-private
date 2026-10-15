<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { apiClient } from '@/api/client'
import type { DocSection } from '@/data/imagevideoDocs'

type DocumentState = { published_version: number; published_content: DocSection[]; draft_version: number; draft_content: DocSection[] }
type HistoryItem = { version: number; action: string; created_at: string }
const state = ref<DocumentState | null>(null), history = ref<HistoryItem[]>([])
const source = ref('[]'), message = ref(''), busy = ref(false), preview = ref(false)
const previewFrame = ref<HTMLIFrameElement | null>(null)
const parsed = computed<DocSection[] | null>(() => {
  try {
    const value = JSON.parse(source.value)
    return Array.isArray(value) ? value as DocSection[] : null
  } catch { return null }
})
const editorURL = '/api/v1/admin/docs/imagevideo'
function updatePreview() {
  previewFrame.value?.contentWindow?.postMessage({ type: 'imagevideo-docs-preview', content: parsed.value, version: state.value?.draft_version }, window.location.origin)
}
watch(source, updatePreview)
async function load(successMessage = '') {
  busy.value = true
  try {
    const [current, versions] = await Promise.all([apiClient.get(editorURL), apiClient.get(`${editorURL}/history`)] )
    state.value = current.data
    source.value = JSON.stringify(state.value?.draft_content ?? [], null, 2)
    history.value = versions.data ?? []
    message.value = successMessage
  } catch (error) { message.value = `读取失败：${String(error)}` }
  finally { busy.value = false }
}
async function save() {
  if (!state.value || !parsed.value) { message.value = 'JSON 格式不正确'; return }
  busy.value = true
  try {
    await apiClient.put(`${editorURL}/draft`, { expected_version: state.value.draft_version, content: parsed.value })
    await load('草稿已保存')
  } catch (error) { message.value = `保存失败（版本可能已变化），请刷新后核对：${String(error)}` }
  finally { busy.value = false }
}
async function publish() {
  if (!state.value || !parsed.value) { message.value = 'JSON 格式不正确'; return }
  busy.value = true
  try {
    await apiClient.post(`${editorURL}/publish`, { expected_version: state.value.draft_version, content: parsed.value })
    await load('新文档版本已发布')
  } catch (error) { message.value = `发布失败（版本可能已变化），请刷新后核对：${String(error)}` }
  finally { busy.value = false }
}
async function rollback(version: number) {
  if (!state.value) return
  busy.value = true
  try {
    await apiClient.post(`${editorURL}/rollback`, { expected_version: state.value.draft_version, version })
    await load(`已回滚到内容版本 ${version}`)
  } catch (error) { message.value = `回滚失败：${String(error)}` }
  finally { busy.value = false }
}
onMounted(() => load())
</script>

<template>
  <AppLayout>
    <div class="space-y-5">
      <header class="flex flex-wrap items-start justify-between gap-3">
        <div><h1 class="text-2xl font-bold text-gray-900 dark:text-white">媒体接口文档编辑</h1><p class="mt-1 text-sm text-gray-500">修改结构化 JSON；草稿不会出现在公开页面，发布后数据库即时生效。</p></div>
        <a href="/docs/imagevideo" target="_blank" class="rounded-lg border px-3 py-2 text-sm">打开公开文档 ↗</a>
      </header>
      <div class="flex flex-wrap items-center gap-2 rounded-xl border bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
        <span class="mr-auto text-sm text-gray-500">已发布 v{{ state?.published_version ?? '—' }} · 草稿版本 {{ state?.draft_version ?? '—' }}</span>
        <button class="rounded-lg border px-3 py-2 text-sm" :disabled="busy" @click="load()">刷新</button>
        <button class="rounded-lg border px-3 py-2 text-sm" :disabled="busy" @click="preview = !preview">{{ preview ? '关闭预览' : '预览草稿' }}</button>
        <button class="rounded-lg bg-primary-600 px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="busy || !parsed" @click="save">保存草稿</button>
        <button class="rounded-lg bg-emerald-600 px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="busy || !parsed" @click="publish">发布文档</button>
      </div>
      <p v-if="message" role="status" class="rounded-lg bg-gray-100 px-3 py-2 text-sm dark:bg-dark-800">{{ message }}</p>
      <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
        <textarea v-model="source" spellcheck="false" aria-label="媒体文档 JSON 草稿" class="min-h-[70vh] w-full rounded-xl border bg-gray-950 p-4 font-mono text-xs leading-6 text-emerald-100 dark:border-dark-700" />
        <aside class="rounded-xl border bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <h2 class="font-semibold">版本历史</h2><p class="my-2 text-xs text-gray-500">回滚会生成新的公开版本，历史记录保留。</p>
          <ul class="space-y-2"> <li v-for="(item,index) in history" :key="`${item.version}-${item.action}-${index}`" class="flex items-center justify-between gap-2 border-t py-2 text-xs"><span>v{{ item.version }} · {{ item.action }}<br><span class="text-gray-500">{{ item.created_at }}</span></span><button v-if="['publish', 'initial', 'rollback'].includes(item.action)" class="rounded border px-2 py-1" :disabled="busy" @click="rollback(item.version)">回滚到此内容</button></li></ul>
        </aside>
      </div>
      <iframe
        v-if="preview"
        ref="previewFrame"
        src="/docs/imagevideo?preview=1"
        title="媒体文档草稿预览"
        class="h-[80vh] w-full rounded-xl border dark:border-dark-700"
        @load="updatePreview"
      />
    </div>
  </AppLayout>
</template>
