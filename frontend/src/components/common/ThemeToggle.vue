<template>
  <button type="button" class="theme-toggle" :aria-label="label" :title="label" @click="toggleTheme">
    <Icon :name="isDark ? 'sun' : 'moon'" size="md" aria-hidden="true" />
  </button>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const isDark = ref(document.documentElement.classList.contains('dark'))
const label = computed(() => t(isDark.value ? 'home.switchToLight' : 'home.switchToDark'))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}
</script>

<style scoped>
.theme-toggle {
  display: grid;
  width: 40px;
  height: 40px;
  flex-shrink: 0;
  place-items: center;
  border: 1px solid transparent;
  border-radius: 9px;
  color: #405775;
  transition: color 180ms ease, background-color 180ms ease, border-color 180ms ease;
}

.theme-toggle:hover { border-color: #c2d3e9; color: #13508c; background: #e5f1ff; }
.theme-toggle:focus-visible { outline: 2px solid #2374bb; outline-offset: 2px; }
:global(.dark .theme-toggle) { color: #b9cbe2; }
:global(.dark .theme-toggle:hover) { border-color: #3d5777; color: #9ef7f1; background: #162941; }
:global(.dark .theme-toggle:focus-visible) { outline-color: #8cf1ee; }
</style>
