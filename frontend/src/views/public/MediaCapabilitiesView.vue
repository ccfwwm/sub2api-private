<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import MediaPublicLayout from '@/components/media/MediaPublicLayout.vue'
import PlazaMediaPricing from '@/components/modelPlaza/PlazaMediaPricing.vue'
import { getPublicMediaCatalog, type ModelPlazaResponse } from '@/api/modelPlaza'
const data = ref<ModelPlazaResponse | null>(null), loading = ref(false), error = ref('')
const search = ref(''), group = ref('')
async function refresh() {
  loading.value = true; error.value = ''
  try { data.value = await getPublicMediaCatalog() }
  catch (e) { error.value = (e as Error).message || '模型能力加载失败，请重试。' }
  finally { loading.value = false }
}
const visible = computed(() => (data.value?.groups ?? []).filter(g => !group.value || String(g.id) === group.value).map(g => ({ ...g, media_models: g.media_models?.filter(m => m.model.toLowerCase().includes(search.value.trim().toLowerCase())) })).filter(g => g.media_models?.length))
onMounted(refresh)
</script>
<template>
  <MediaPublicLayout>
    <section class="space-y-5">
      <div class="flex flex-wrap gap-3"><input v-model="search" type="search" class="input max-w-sm" placeholder="搜索模型名称" aria-label="搜索模型" /><select v-model="group" class="input max-w-xs" aria-label="选择分组"><option value="">全部分组</option><option v-for="g in data?.groups" :key="g.id" :value="String(g.id)">{{ g.name }}</option></select><button class="btn btn-secondary" :disabled="loading" @click="refresh">刷新</button></div>
      <p v-if="error" role="alert" class="text-red-600">{{ error }}</p><p v-else-if="loading && !data" role="status">正在加载模型能力…</p>
      <p v-if="data?.notices?.length" class="text-sm text-amber-700">部分分组暂不可用，其余模型仍可查看。</p>
      <section v-for="g in visible" :key="g.id" class="rounded-xl border bg-white p-4 dark:border-dark-700 dark:bg-dark-900"><header class="mb-4"><h2 class="font-semibold">{{ g.name }}</h2></header><PlazaMediaPricing :models="g.media_models!" :group-id="g.id" :description-version="g.description_version" :multiplier="g.user_rate_multiplier ?? g.rate_multiplier" :image-multiplier="g.image_rate_independent ? g.image_rate_multiplier : undefined" :video-multiplier="g.video_rate_independent ? g.video_rate_multiplier : undefined" /></section>
      <p v-if="!loading && !error && !visible.length" class="py-8 text-center text-gray-500">暂无符合条件的模型。</p>
    </section>
  </MediaPublicLayout>
</template>
