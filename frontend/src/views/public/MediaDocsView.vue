<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import MediaPublicLayout from '@/components/media/MediaPublicLayout.vue'
const route = useRoute()
const loading = ref(true)
const source = computed(() => (import.meta.env.DEV ? '/media-docs/index.html' : '/docs/imagevideo') + '?embedded=1' + route.hash)
</script>
<template>
  <MediaPublicLayout>
    <h1 class="mb-4 text-xl font-semibold">音视频接口文档</h1>
    <p v-if="loading" role="status" class="text-sm text-gray-500">正在加载文档…</p>
    <iframe :src="source" title="音视频接口文档与在线调试" class="w-full rounded-xl border bg-white dark:border-dark-700" style="height:calc(100dvh - 180px); min-height:600px" referrerpolicy="same-origin" @load="loading = false" />
  </MediaPublicLayout>
</template>
