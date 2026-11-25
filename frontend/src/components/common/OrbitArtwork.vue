<template>
  <picture class="orbit-picture" :class="{ 'orbit-animated': animated, 'orbit-paused': paused }" aria-hidden="true">
    <source media="(max-width: 720px)" :srcset="gatewayMobile" />
    <img :src="gatewayArt" alt="" width="1222" height="809" :fetchpriority="animated ? 'high' : 'auto'" />
  </picture>
</template>

<script setup lang="ts">
import gatewayArt from '@/assets/landing/gateway.webp'
import gatewayMobile from '@/assets/landing/gateway-mobile.webp'

defineProps<{ animated?: boolean; paused?: boolean }>()
</script>

<style scoped>
.orbit-picture {
  display: block;
  pointer-events: none;
  perspective: 780px;
}

.orbit-picture img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  filter: saturate(1.26) brightness(.84);
}

:global(.dark .orbit-picture img) {
  filter: brightness(1.21) saturate(1.12);
}

.orbit-animated img { animation: orbit-roll 18s ease-in-out infinite; }
.orbit-paused img { animation-play-state: paused; }

@keyframes orbit-roll {
  0%, 100% { transform: rotateZ(-5deg) rotateY(-14deg); }
  50% { transform: rotateZ(12deg) rotateY(19deg) scale(1.08); }
}

@media (prefers-reduced-motion: reduce) {
  .orbit-animated img { animation: none; }
}
</style>
