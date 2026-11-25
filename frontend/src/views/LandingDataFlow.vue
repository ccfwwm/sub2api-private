<template>
  <svg class="landing-data-flow" :class="`landing-data-flow-${variant}`" viewBox="0 0 1100 340"
    preserveAspectRatio="xMidYMid slice" aria-hidden="true">
    <path v-for="path in paths[variant]" :key="path" class="data-flow-track" :d="path" />
    <path v-for="(path, index) in paths[variant]" :key="`packet-${index}`" class="data-flow-packet"
      :style="{ animationDelay: `-${index * 1.7}s` }" :d="path" pathLength="100" />
  </svg>
</template>

<script setup lang="ts">
defineProps<{ variant: 'quickstart' | 'science' }>()

const paths = {
  quickstart: [
    'M-80 80H130Q164 80 195 114L295 226Q321 255 360 255H1190',
    'M-80 302H216Q249 302 278 272L455 92Q482 62 525 62H1190',
    'M515 380L702 188Q727 162 770 162H1190',
  ],
  science: [
    'M-100 239C215 419 344 388 579 198S922 -110 1200 31',
    'M-80 95C220 23 313 267 591 242S982 44 1200 163',
    'M-70 316C176 391 383 354 541 137S983 22 1200 31',
    'M-70 362C225 417 405 337 592 173S1024 20 1200 144',
  ],
}
</script>

<style scoped>
.landing-data-flow {
  position: absolute;
  z-index: -1;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: hidden;
  border-radius: inherit;
  fill: none;
  color: var(--accent);
  pointer-events: none;
}
.landing-data-flow-science { color: var(--science-accent); }
.data-flow-track { stroke: currentColor; stroke-width: 1.15; opacity: .19; vector-effect: non-scaling-stroke; }
.data-flow-packet {
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-dasharray: 6 100;
  opacity: .64;
  vector-effect: non-scaling-stroke;
  animation: background-transmit 8s linear infinite;
}

@keyframes background-transmit { from { stroke-dashoffset: 6; } to { stroke-dashoffset: -100; } }
:global(.landing-motion-paused .data-flow-packet) { animation-play-state: paused; }

@media (prefers-reduced-motion: reduce) {
  .data-flow-packet { display: none; }
}
</style>
