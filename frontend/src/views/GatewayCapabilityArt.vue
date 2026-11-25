<template>
  <div class="capability-art" aria-hidden="true">
    <div v-if="kind === 'models'" class="capability-network">
      <svg viewBox="0 0 280 110" preserveAspectRatio="none">
        <path d="M32 18 C66 18 79 55 117 55 M32 55 H117 M32 92 C66 92 79 55 117 55 M161 55 H254 M202 55 C223 55 228 25 254 25 M202 55 C223 55 228 85 254 85" />
        <path class="capability-moving" d="M32 55 H254" pathLength="100" />
      </svg>
      <div class="capability-network-providers">
        <span v-for="model in featuredGatewayModels.slice(0, 3)" :key="model.id"><img :src="model.logo" alt="" width="19" height="19" /></span>
      </div>
      <span class="capability-network-hub"><img :src="siteLogo" alt="" width="44" height="44" /></span>
      <div class="capability-network-clients">
        <span><Icon name="terminal" size="sm" /></span>
        <span><Icon name="key" size="sm" /></span>
        <span><Icon name="cloud" size="sm" /></span>
      </div>
    </div>

    <div v-else-if="kind === 'media'" class="capability-media">
      <div class="capability-media-tile capability-media-image">
        <img src="../assets/landing/nebula-mobile.webp" alt="" width="540" height="540" loading="lazy" />
      </div>
      <div class="capability-media-tile capability-media-video">
        <Icon name="play" size="md" />
        <span class="capability-media-timeline"></span>
      </div>
      <div class="capability-media-tile capability-media-audio">
        <svg viewBox="0 0 120 28" preserveAspectRatio="none">
          <path d="M0 14h11l5-5 5 10 5-14 5 19 5-12 6 2h8l5-7 5 14 5-21 5 27 5-18 5 9 5-4h10l5-4 5 8 5-4h10" />
          <path class="capability-moving" pathLength="100" d="M0 14h11l5-5 5 10 5-14 5 19 5-12 6 2h8l5-7 5 14 5-21 5 27 5-18 5 9 5-4h10l5-4 5 8 5-4h10" />
        </svg>
      </div>
    </div>

    <div v-else class="capability-usage">
      <span class="capability-chart-label">{{ t('home.landing.visualExample') }}</span>
      <svg viewBox="0 0 280 110" preserveAspectRatio="none">
        <path class="capability-chart-grid" d="M6 28h268M6 48h268M6 68h268M6 88h268" />
        <path class="capability-chart-bars" d="M18 109V92h15v17 M47 109V82h15v27 M76 109V86h15v23 M105 109V67h15v42 M134 109V74h15v35 M163 109V54h15v55 M192 109V62h15v47 M221 109V43h15v66 M250 109V34h15v75" />
        <path class="capability-chart-line" d="M7 90 C33 94 42 68 67 76 S111 55 135 62 S172 42 194 47 S229 28 268 19" />
        <path class="capability-moving" d="M7 90 C33 94 42 68 67 76 S111 55 135 62 S172 42 194 47 S229 28 268 19" pathLength="100" />
        <circle cx="268" cy="19" r="4" />
      </svg>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { featuredGatewayModels } from '@/components/common/gatewayModelBrands'

defineProps<{ kind: 'models' | 'media' | 'usage'; siteLogo: string }>()
const { t } = useI18n()
</script>

<style scoped>
.capability-art {
  position: relative;
  isolation: isolate;
  display: grid;
  min-height: 131px;
  align-items: center;
  overflow: hidden;
  margin-bottom: 20px;
  border: 1px solid var(--border);
  border-radius: 11px;
  color: var(--accent);
  background: radial-gradient(ellipse at 52% 65%, var(--glow), transparent 75%), var(--surface);
}

.capability-art svg { width: 100%; height: auto; }
.capability-art path { fill: none; stroke: currentColor; stroke-width: 1.5; vector-effect: non-scaling-stroke; }

.capability-network,
.capability-usage {
  position: relative;
  margin: 0 6%;
}

.capability-network svg,
.capability-usage svg { position: absolute; inset: 0; width: 100%; height: 100%; }
.capability-network { height: 114px; }
.capability-network svg { color: color-mix(in srgb, var(--accent) 52%, var(--border)); }
.capability-network-hub,
.capability-network-providers span,
.capability-network-clients span {
  display: grid;
  place-items: center;
  border: 1px solid var(--border);
  border-radius: 7px;
  background: var(--surface);
}

.capability-network-hub {
  position: absolute;
  top: 50%;
  left: 49.5%;
  width: 54px;
  height: 54px;
  border-color: color-mix(in srgb, var(--accent) 62%, var(--border));
  box-shadow: 0 0 18px var(--action-shadow);
  transform: translate(-50%, -50%);
}
.capability-network-hub img { border-radius: 7px; }
.capability-network-providers, .capability-network-clients { position: absolute; inset-block: 4px; display: grid; align-items: center; }
.capability-network-providers { right: 2%; }
.capability-network-clients { left: 2%; }
.capability-network-providers span, .capability-network-clients span { width: 32px; height: 28px; }
.capability-network-providers span { background: #f8fcff; }
.capability-network-clients svg { position: static; width: 16px; height: 16px; color: var(--accent); }

.capability-media {
  position: relative;
  height: 116px;
  width: min(244px, 90%);
  justify-self: center;
}

.capability-media-tile {
  position: absolute;
  overflow: hidden;
  border: 1px solid #5b7fa864;
  border-radius: 8px;
  color: #d8f6ff;
  box-shadow: 0 7px 15px #05182b48;
}

.capability-media-image { top: 6px; left: 1%; width: 56%; height: 78px; background: #102445; transform: rotate(-6deg); }
.capability-media-image img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center; animation: capability-preview 13s ease-in-out infinite alternate; }
.capability-media-video { display: grid; top: 20px; right: 0; width: 55%; height: 72px; place-items: center; background: linear-gradient(125deg, #1d4a7e, #082448 74%); transform: rotate(5deg); }
.capability-media-video svg { width: 27px; height: 27px; border: 1px solid #aadcfa90; padding: 5px; border-radius: 50%; }
.capability-media-timeline { position: absolute; right: 9px; bottom: 8px; left: 9px; height: 2px; border-radius: 2px; background: linear-gradient(90deg, #a1f1e9 66%, #53779e 66%); }
.capability-media-timeline::after { position: absolute; inset-block: -1px; left: 0; width: 20%; border-radius: 3px; background: #d9fff6; content: ''; animation: capability-scan 4s linear infinite; }
.capability-media-audio { display: grid; bottom: 0; left: 24%; width: 52%; height: 34px; place-items: center; padding: 5px 8px; background: #091e37; }
.capability-media-audio path { stroke: #85e6e4; }

.capability-usage { height: 110px; }
.capability-chart-label { position: absolute; z-index: 1; top: 0; left: 3px; color: var(--muted); font-size: 10px; font-weight: 700; }
.capability-usage .capability-chart-grid { stroke: var(--border); }
.capability-usage .capability-chart-bars { fill: color-mix(in srgb, var(--accent) 16%, transparent); stroke: none; }
.capability-usage .capability-chart-line { stroke-width: 2; }
.capability-usage circle { fill: var(--accent); }

.capability-art path.capability-moving {
  stroke: var(--accent);
  stroke-width: 3;
  stroke-linecap: round;
  stroke-dasharray: 7 100;
  animation: capability-transmit 4s linear infinite;
}

@keyframes capability-transmit { from { stroke-dashoffset: 7; } to { stroke-dashoffset: -100; } }
@keyframes capability-preview { to { transform: scale(1.17) rotate(9deg); } }
@keyframes capability-scan { to { transform: translateX(400%); } }
.capability-media-audio path.capability-moving { stroke: #eefffb; }
.capability-art path.capability-moving,
.capability-media-image img,
.capability-media-timeline::after { animation-play-state: var(--landing-motion-state, running); }

@media (prefers-reduced-motion: reduce) {
  .capability-art path.capability-moving { display: none; }
  .capability-media-image img, .capability-media-timeline::after { animation: none; }
}
</style>
