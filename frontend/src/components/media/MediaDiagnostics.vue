<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import { mediaProtocolsAPI } from '@/api/media-protocols'
const route = useRoute()
const group = ref(String(route.query.group_id || '')), key = ref(''), account = ref(String(route.query.account_id || ''))
const request = ref('{"model":"","prompt":"A quiet valley","seconds":5}'), operation = ref('video')
const result = ref<unknown>(), busy = ref(false), error = ref('')
async function run(effective: boolean) {
  busy.value = true; error.value = ''
  try {
    if (effective) {
      if (!Number.isSafeInteger(Number(group.value)) || Number(group.value) <= 0) throw new Error('请输入有效分组 ID。')
      result.value = await mediaProtocolsAPI.effective(Number(group.value))
    } else {
      if (!Number.isSafeInteger(Number(key.value)) || Number(key.value) <= 0) throw new Error('请输入有效 API Key ID。')
      result.value = await mediaProtocolsAPI.preview({ mode: 'effective', api_key_id: Number(key.value), ...(account.value ? { account_id: Number(account.value) } : {}), operation: operation.value, request: JSON.parse(request.value), measure_media: false })
    }
  } catch (cause) { error.value = (cause as Error).message } finally { busy.value = false }
}
</script>
<template>
  <section class="space-y-4 rounded-xl border p-5 dark:border-dark-600">
    <h2 class="text-lg font-semibold">联调检测</h2><p class="text-sm text-gray-500">读取分组有效配置，定位模板绑定问题；预览请求映射与估价。生成测试请在账号模型中选择“测试 / 查看”，结果未知的任务先到任务记录核对。</p>
    <div class="flex flex-wrap items-end gap-2"><label class="text-sm">分组 ID<input v-model="group" type="number" min="1" class="input block" /></label><button class="btn btn-secondary" :disabled="busy" @click="run(true)">检查实际绑定</button></div>
    <div class="grid gap-3 sm:grid-cols-3"><label class="text-sm">API Key ID<input v-model="key" type="number" min="1" class="input block w-full" /></label><label class="text-sm">候选账号 ID<input v-model="account" type="number" min="1" class="input block w-full" /></label><label class="text-sm">请求操作<select v-model="operation" class="input block w-full"><option v-for="op in ['image','image_edit','gemini_image','video','audio','speech','chat_music']" :key="op">{{ op }}</option></select></label></div>
    <label class="block text-sm">请求样例<textarea v-model="request" class="input mt-1 min-h-32 w-full font-mono text-xs" /></label><button class="btn btn-primary" :disabled="busy" @click="run(false)">预览映射与渠道估价</button>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p><pre v-if="result" class="max-h-[600px] overflow-auto rounded-lg bg-gray-50 p-4 text-xs dark:bg-dark-800">{{ JSON.stringify(result,null,2) }}</pre>
  </section>
</template>
