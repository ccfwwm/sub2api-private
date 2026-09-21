<template>
  <ImageDeploymentPanel v-if="isAdmin" :version="currentVersion" :repository="appStore.updateRepository" />
  <span v-else-if="version" class="text-xs text-gray-500 dark:text-dark-400">v{{ version }}</span>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAuthStore, useAppStore } from '@/stores'
import ImageDeploymentPanel from './ImageDeploymentPanel.vue'
const props = defineProps<{ version?: string }>()
const appStore = useAppStore()
const authStore = useAuthStore()
const isAdmin = computed(() => authStore.isAdmin)
const currentVersion = computed(() => appStore.currentVersion || props.version || '')
onMounted(() => { if (isAdmin.value) void appStore.fetchVersion(false) })
</script>
