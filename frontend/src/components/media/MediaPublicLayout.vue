<script setup lang="ts">
import { onMounted } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import PlazaNavBar from '@/components/modelPlaza/PlazaNavBar.vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
const auth = useAuthStore()
const app = useAppStore()
onMounted(() => { void app.fetchPublicSettings() })
</script>
<template>
  <AppLayout v-if="auth.isAuthenticated"><slot /></AppLayout>
  <div v-else class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <PlazaNavBar />
    <div class="mx-auto flex max-w-screen-2xl flex-col gap-5 p-4 lg:flex-row lg:p-6">
      <nav aria-label="媒体导航" class="flex shrink-0 gap-2 lg:w-44 lg:flex-col">
        <router-link to="/media-capabilities" class="rounded-lg px-4 py-3 text-sm" active-class="bg-primary-100 text-primary-700">多媒体模型能力</router-link>
        <router-link to="/media-docs" class="rounded-lg px-4 py-3 text-sm" active-class="bg-primary-100 text-primary-700">多媒体接口文档</router-link>
        <router-link v-if="app.cachedPublicSettings?.model_plaza_enabled" to="/model-plaza" class="rounded-lg px-4 py-3 text-sm" active-class="bg-primary-100 text-primary-700">模型广场</router-link>
      </nav>
      <main class="min-w-0 flex-1"><slot /></main>
    </div>
  </div>
</template>
