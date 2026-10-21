<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import MediaTaskHistory from '@/components/media/MediaTaskHistory.vue'

const { t } = useI18n()
</script>

<template>
  <AppLayout>
    <div class="space-y-6">
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('nav.mediaTasks') }}</h1>
      <div class="card overflow-hidden">
        <MediaTaskHistory />
      </div>
    </div>
  </AppLayout>
</template>
