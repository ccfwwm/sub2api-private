<template>
  <section class="dashboard-hero" aria-labelledby="dashboard-hero-title">
    <div class="dashboard-hero__art" aria-hidden="true">
      <span class="dashboard-hero__orbit"></span>
      <span class="dashboard-hero__orbit dashboard-hero__orbit--outer"></span>
      <img :src="siteLogo || '/logo.svg'" alt="" width="60" height="60" />
    </div>
    <div class="dashboard-hero__body">
      <h2 id="dashboard-hero-title">{{ title }}</h2>
      <div class="dashboard-hero__actions"><slot /></div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAppStore } from '@/stores/app'
import { sanitizeUrl } from '@/utils/url'

defineProps<{ title: string }>()
const appStore = useAppStore()
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
</script>

<style scoped>
.dashboard-hero__art {
  position: absolute;
  inset: 0 0 0 auto;
  width: min(57%, 620px);
  display: grid;
  place-items: center;
  overflow: hidden;
  pointer-events: none;
  background:
    radial-gradient(ellipse at 58% 46%, #1d286755 4%, transparent 62%),
    linear-gradient(90deg, #111b35 0%, #111b3500 56%),
    url('@/assets/landing/nebula-mobile.webp') center 48% / cover no-repeat;
  mask-image: linear-gradient(90deg, transparent, #000 24%);
}

.dashboard-hero__art img {
  position: relative;
  padding: 7px;
  border: 1px solid #b0eafa80;
  border-radius: 16px;
  background: #101c37e8;
  box-shadow: 0 0 40px #5ac9ff72;
  object-fit: contain;
}

.dashboard-hero__orbit {
  position: absolute;
  width: clamp(140px, 27vw, 355px);
  aspect-ratio: 3.2;
  border: 1px solid #a9ddf66e;
  border-radius: 50%;
  transform: rotate(-16deg);
}

.dashboard-hero__orbit::before {
  position: absolute;
  top: 0;
  left: 0;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #c0fbff;
  box-shadow: 0 0 11px 3px #65dfff;
  content: '';
  offset-path: ellipse(50% 50% at 50% 50%);
  animation: console-orbit-flow 6s linear infinite;
}

.dashboard-hero__orbit--outer {
  width: clamp(172px, 35vw, 465px);
  border-color: #b9c6fd4d;
}

.dashboard-hero__orbit--outer::before { animation-delay: -2.5s; animation-duration: 9s; }

@keyframes console-orbit-flow {
  to { offset-distance: 100%; }
}

@media (max-width: 639px) {
  .dashboard-hero__art { width: 49%; mask-image: linear-gradient(90deg, transparent, #000 40%); }
  .dashboard-hero__art img { width: 43px; height: 43px; }
}

@media (prefers-reduced-motion: reduce) {
  .dashboard-hero__orbit::before { animation: none; }
}
</style>
