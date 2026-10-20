<script setup lang="ts">
import { onMounted } from 'vue'

// A router-link does not reach Nginx's independent docs route. Navigate the
// whole page so the docs container owns the response and no iframe is needed.
const destination = (import.meta.env.DEV ? '/media-docs/index.html' : '/docs/imagevideo')
  + window.location.search + window.location.hash
onMounted(() => window.location.replace(destination))
</script>
<template>
  <p class="p-6"><a :href="destination">正在打开词元互联接口文档…</a></p>
</template>
