<template>
  <div class="orbit-art" aria-hidden="true">
    <picture>
      <source media="(max-width: 720px)" :srcset="gatewayMobile" />
      <img :src="gatewayArt" alt="" width="1222" height="809" fetchpriority="high" />
    </picture>
    <svg class="orbit-stream" viewBox="0 0 900 596" focusable="false">
      <path class="orbit-halo" :d="outerOrbit" pathLength="100" />
      <path class="orbit-halo orbit-halo-inner" :d="innerOrbit" pathLength="100" />
      <path class="orbit-trail" :d="outerOrbit" pathLength="100" />
      <path class="orbit-spark" :d="outerOrbit" pathLength="100" />
      <path class="orbit-trail orbit-trail-inner" :d="innerOrbit" pathLength="100" />
      <path class="orbit-spark orbit-spark-inner" :d="innerOrbit" pathLength="100" />
    </svg>
  </div>
</template>

<script setup lang="ts">
import gatewayArt from '@/assets/landing/gateway.webp'
import gatewayMobile from '@/assets/landing/gateway-mobile.webp'

// Follow the filaments in the projected artwork so the tilt stays fixed through each revolution.
const outerOrbit = 'M112 380C82 347 167 285 296 218S654 74 764 92C855 107 836 208 753 271C656 352 477 410 322 434C226 448 145 435 112 380Z'
const innerOrbit = 'M222 363C185 332 254 266 364 208S606 109 684 114C756 118 766 171 711 235C651 305 510 360 383 385C313 399 245 383 222 363Z'
</script>

<style scoped>
.orbit-art {
  pointer-events: none;
  mask-image: linear-gradient(to bottom, transparent, #000 7% 93%, transparent);
}

.orbit-art picture,
.orbit-stream { position: absolute; inset: 0; width: 100%; height: 100%; }

.orbit-art img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  filter: saturate(1.26) brightness(.84);
}

:global(.dark .orbit-art img) { filter: brightness(1.21) saturate(1.12); }

.orbit-stream { fill: none; color: #007bd8; }
:global(.dark .orbit-stream) { color: #79eeff; }

.orbit-halo,
.orbit-trail,
.orbit-spark {
  stroke: currentColor;
  stroke-linecap: round;
  animation: orbit-packets 7s linear infinite;
}

.orbit-halo {
  stroke-width: 28;
  stroke-dasharray: 15 35;
  animation-duration: 11s;
  filter: blur(9px);
  opacity: .29;
}
.orbit-halo-inner { stroke-width: 18; animation-delay: -2.7s; animation-duration: 14s; }

.orbit-trail {
  stroke-width: 5;
  stroke-dasharray: 7.5 25.8333;
  filter: blur(1.15px);
  opacity: .65;
}
.orbit-spark { stroke-width: 4.5; stroke-dasharray: .34 32.9933; }
.orbit-trail-inner, .orbit-spark-inner { animation-duration: 10s; animation-delay: -3s; }
.orbit-trail-inner { stroke-width: 3; opacity: .46; }
.orbit-spark-inner { stroke-width: 2.8; }

@keyframes orbit-packets { to { stroke-dashoffset: -100; } }

@media (prefers-reduced-motion: reduce) {
  .orbit-stream { display: none; }
}
</style>
