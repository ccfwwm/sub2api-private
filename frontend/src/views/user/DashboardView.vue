<template>
  <AppLayout>
    <div class="space-y-5">
      <DashboardHero :title="t('dashboard.userOverview')">
        <router-link to="/keys"><Icon name="key" size="sm" />{{ t('dashboard.apiKeys') }}</router-link>
        <router-link v-if="modelPlazaEnabled" to="/model-plaza?embedded=1"><Icon name="grid" size="sm" />{{ t('nav.modelPlaza') }}</router-link>
      </DashboardHero>
      <div v-if="loading" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <template v-else-if="stats">
        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" :platform-quotas="platformQuotas" />
        <div class="console-chart-slot">
          <UserDashboardCharts
            v-model:start-date="startDate"
            v-model:end-date="endDate"
            v-model:granularity="granularity"
            :loading="loadingCharts"
            :trend="trendData"
            :models="modelStats"
            @date-range-change="loadCharts"
            @granularity-change="loadCharts"
            @refresh="refreshAll"
          />
        </div>
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div class="lg:col-span-2"><UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" /></div>
          <div class="lg:col-span-1"><UserDashboardQuickActions /></div>
        </div>
      </template>
      <div v-else class="card flex flex-wrap items-center justify-between gap-3 p-4" role="status">
        <p class="text-sm text-gray-700 dark:text-gray-200">{{ t('dashboard.loadFailed') }}</p>
        <button type="button" class="btn btn-secondary" @click="refreshAll">{{ t('common.refresh') }}</button>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import { getMyPlatformQuotas } from '@/api/user'
import AppLayout from '@/components/layout/AppLayout.vue'
import DashboardHero from '@/components/layout/DashboardHero.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'
import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import type { ModelStat, PlatformQuotaItem, TrendDataPoint, UsageLog } from '@/types'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import { formatDateLocalInput } from '@/utils/format'

const UserDashboardCharts = defineAsyncComponent({
  loader: () => import('@/components/user/dashboard/UserDashboardCharts.vue'),
  loadingComponent: LoadingSpinner
})
const { t } = useI18n()
const authStore = useAuthStore()
const user = computed(() => authStore.user)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))
const stats = ref<UserStatsType | null>(null)
const loading = ref(true)
const loadingUsage = ref(true)
const loadingCharts = ref(true)
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)

const recentStart = formatDateLocalInput(new Date(Date.now() - 6 * 86400000))
const recentEnd = formatDateLocalInput(new Date())
const startDate = ref(recentStart)
const endDate = ref(recentEnd)
const granularity = ref<'day' | 'hour'>('day')
let chartRequest = 0

async function loadStats() {
  loading.value = !stats.value
  try {
    stats.value = await usageAPI.getDashboardStats()
  } catch (error) {
    console.error('Failed to load dashboard stats:', error)
  } finally {
    loading.value = false
  }
}

async function loadCharts() {
  const request = ++chartRequest
  loadingCharts.value = true
  const range = { start_date: startDate.value, end_date: endDate.value }
  try {
    const snapshot = await usageAPI.getDashboardSnapshotV2({
      ...range,
      granularity: granularity.value,
      include_trend: true,
      include_model_stats: true,
      include_group_stats: false
    })
    if (request === chartRequest) {
      trendData.value = snapshot.trend || []
      modelStats.value = snapshot.models || []
    }
  } catch (error) {
    if (request === chartRequest) console.error('Failed to load charts:', error)
  } finally {
    if (request === chartRequest) loadingCharts.value = false
  }
}

async function loadRecent() {
  loadingUsage.value = true
  try {
    const result = await usageAPI.query({
      start_date: recentStart,
      end_date: recentEnd,
      page: 1,
      page_size: 5
    })
    recentUsage.value = result.items || []
  } catch (error) {
    console.error('Failed to load recent usage:', error)
  } finally {
    loadingUsage.value = false
  }
}

async function loadPlatformQuotas() {
  try {
    const data = await getMyPlatformQuotas()
    platformQuotas.value = data.platform_quotas ?? []
  } catch (error) {
    console.warn('Failed to load platform quotas:', error)
    platformQuotas.value = []
  }
}

function loadDashboard() {
  void loadStats()
  void loadCharts()
  void loadRecent()
  void loadPlatformQuotas()
}

function refreshAll() {
  void authStore.refreshUser().catch((error) => console.warn('Failed to refresh dashboard balance:', error))
  loadDashboard()
}

onMounted(loadDashboard)
</script>
