<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import MediaPublicLayout from '@/components/media/MediaPublicLayout.vue'
const route = useRoute()
const loading = ref(true)
const source = computed(() => (import.meta.env.DEV ? '/media-docs/index.html' : '/docs/index.html') + '?embedded=1' + route.hash)
</script>
<template>
  <MediaPublicLayout>
    <div class="relative">
    <p v-if="loading" role="status" class="absolute left-4 top-4 text-sm text-gray-500">正在加载文档…</p>
    <iframe :src="source" title="文档" class="w-full rounded-xl border bg-white dark:border-dark-700" style="height:calc(100dvh - 116px); min-height:480px" referrerpolicy="same-origin" @load="loading = false" />
    </div>
  </MediaPublicLayout>
</template>
