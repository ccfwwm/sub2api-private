<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import MediaAccountsManager from '@/components/media/MediaAccountsManager.vue'
import MediaProtocolManager from '@/components/media/MediaProtocolManager.vue'
import MediaDiagnostics from '@/components/media/MediaDiagnostics.vue'
import MediaTaskHistory from '@/components/media/MediaTaskHistory.vue'
const route = useRoute(), router = useRouter(), { locale } = useI18n()
const tr = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const tabs = computed(() => [
  { id: 'accounts', name: tr('账号模型', 'Account models') },
  { id: 'bindings', name: tr('模型绑定', 'Bindings') },
  { id: 'templates', name: tr('协议模板', 'Protocol templates') },
  { id: 'diagnostics', name: tr('联调检测', 'Diagnostics') },
  { id: 'tasks', name: tr('任务记录', 'Tasks') }
])
const tab = computed(() => tabs.value.some(item => item.id === route.query.tab) ? String(route.query.tab) : 'accounts')
function navigate(id: string) { void router.replace({ query: { ...route.query, tab: id } }) }
</script>
<template>
  <AppLayout>
    <div class="mx-auto max-w-[1600px] space-y-6 p-4 lg:p-8">
      <header class="flex flex-wrap items-start justify-between gap-3">
        <div><h1 class="text-2xl font-semibold">{{ tr('媒体管理', 'Media management') }}</h1><p class="mt-2 text-sm text-gray-500">{{ tr('拉取账号模型 → 绑定协议模板 → 配置请求能力 → 测试并启用。价格按公开模型名匹配渠道定价。', 'Discover models, bind templates, configure request capabilities, then test and enable. Public model names match channel pricing.') }}</p></div>
        <RouterLink class="btn btn-secondary" to="/admin/channels/pricing">{{ tr('渠道定价', 'Channel pricing') }}</RouterLink>
      </header>
      <div class="grid min-w-0 gap-5 lg:grid-cols-[180px_minmax(0,1fr)]">
        <nav :aria-label="tr('媒体管理分区', 'Media sections')" class="flex gap-2 overflow-auto rounded-xl border p-2 dark:border-dark-600 lg:sticky lg:top-4 lg:h-fit lg:flex-col">
          <button v-for="item in tabs" :key="item.id" class="rounded-lg px-4 py-3 text-left text-sm whitespace-nowrap" :class="tab === item.id ? 'bg-primary-50 text-primary-700 dark:bg-primary-950 dark:text-primary-300' : 'hover:bg-gray-50 dark:hover:bg-dark-800'" :aria-current="tab === item.id ? 'page' : undefined" @click="navigate(item.id)">{{ item.name }}</button>
        </nav>
        <main class="min-w-0">
          <MediaAccountsManager v-if="tab === 'accounts'" />
          <MediaProtocolManager v-else-if="tab === 'templates' || tab === 'bindings'" :key="tab" />
          <MediaDiagnostics v-else-if="tab === 'diagnostics'" />
          <MediaTaskHistory v-else admin />
        </main>
      </div>
    </div>
  </AppLayout>
</template>
