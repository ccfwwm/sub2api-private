<template>
  <section id="gateway-flow" class="flow" aria-labelledby="gateway-flow-title">
    <div class="flow-heading">
      <div>
        <h2 id="gateway-flow-title">{{ t('home.flow.title') }}</h2>
        <p>{{ t('home.flow.instructions') }}</p>
      </div>
      <div class="flow-modes" role="group" :aria-label="t('home.flow.direction')">
        <button type="button" :aria-pressed="!returning" @click="returning = false">
          <Icon name="arrowRight" size="xs" aria-hidden="true" /> {{ t('home.flow.request') }}
        </button>
        <button type="button" :aria-pressed="returning" @click="returning = true">
          <Icon name="arrowLeft" size="xs" aria-hidden="true" /> {{ t('home.flow.response') }}
        </button>
      </div>
    </div>

    <div class="flow-board">
      <div class="flow-labels">
        <span id="flow-tools-label">{{ t('home.flow.tools') }}</span>
        <span id="flow-models-label">{{ t('home.flow.models') }}</span>
      </div>
      <div class="flow-diagram" :class="{ 'flow-returning': returning, 'flow-paused': paused }">
        <svg class="flow-wires" viewBox="0 0 1000 300" preserveAspectRatio="none" aria-hidden="true">
          <path v-for="(tool, index) in tools" :key="tool.label" :d="toolPath(index)" />
          <path v-for="(model, index) in models" :key="model.id" :d="modelPath(index)" />
          <g :key="`${toolIndex}-${modelIndex}-${returning}`">
            <path class="flow-selected" :d="toolPath(toolIndex)" />
            <path class="flow-selected" :d="modelPath(modelIndex)" />
            <path class="flow-pulse flow-pulse-tool" :d="toolPath(toolIndex)" pathLength="100" />
            <path class="flow-pulse flow-pulse-model" :d="modelPath(modelIndex)" pathLength="100" />
          </g>
        </svg>

        <div class="flow-nodes" role="group" aria-labelledby="flow-tools-label">
          <button v-for="(tool, index) in tools" :key="tool.label" type="button" :aria-pressed="toolIndex === index"
            @click="toolIndex = index">
            <span class="flow-brand-icon"><img :src="tool.logo" alt="" width="22" height="22" /></span>
            <span>{{ tool.label }}</span>
          </button>
        </div>

        <div class="flow-hub">
          <span class="flow-hub-orbit" aria-hidden="true"></span>
          <div class="flow-hub-mark"><img :src="siteLogo" alt="" width="72" height="72" /></div>
          <span>{{ siteName }}</span>
        </div>

        <div class="flow-nodes" role="group" aria-labelledby="flow-models-label">
          <button v-for="(model, index) in models" :key="model.id" type="button" :aria-pressed="modelIndex === index"
            @click="modelIndex = index">
            <span class="flow-brand-icon"><img :src="model.logo" alt="" width="22" height="22" /></span>
            <span>{{ model.label }}</span>
          </button>
        </div>
      </div>

      <div class="flow-summary">
        <p aria-live="polite">
          <span>{{ t('home.flow.demo') }}</span>
          <strong>{{ t(returning ? 'home.flow.responsePath' : 'home.flow.requestPath', { tool: tools[toolIndex].label, model: models[modelIndex].label, gateway: siteName }) }}</strong>
        </p>
        <router-link v-if="showModelPlazaEntry" to="/model-plaza?embedded=1">
          {{ t('nav.modelPlaza') }} <Icon name="arrowRight" size="xs" aria-hidden="true" />
        </router-link>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { featuredGatewayModels, gatewayModels as models } from '@/components/common/gatewayModelBrands'
import codexLogo from '@/assets/landing/logos/codex.webp'
import workbuddyLogo from '@/assets/landing/logos/workbuddy.svg'

defineProps<{ siteName: string; siteLogo: string; showModelPlazaEntry: boolean; paused: boolean }>()

const { t } = useI18n()
const tools = [
  { label: 'Claude Code', logo: featuredGatewayModels[0].logo },
  { label: 'Codex', logo: codexLogo },
  { label: 'ZeroWall Science', logo: '/zerowall-icon.png' },
  { label: 'WorkBuddy', logo: workbuddyLogo },
] as const

const toolIndex = ref(1)
const modelIndex = ref(1)
const returning = ref(false)

function toolPath(index: number) {
  const y = (index + 0.5) * 300 / tools.length
  return `M 175 ${y} C 345 ${y}, 335 150, 500 150`
}

function modelPath(index: number) {
  const y = (index + 0.5) * 300 / models.length
  return `M 500 150 C 665 150, 655 ${y}, 825 ${y}`
}
</script>

<style scoped>
.flow {
  --flow-surface: #f4f9ff;
  --flow-node: #fff;
  --flow-active: #e0f4ff;
  --flow-line: #bbcee4;
  --flow-route: #2380c4;
  --flow-pulse: #0050b0;
  --flow-grid: #a6bfe320;
  scroll-margin-top: 22px;
  margin-block: 57px 0;
}

:global(.dark .flow) {
  --flow-surface: #101c2f;
  --flow-node: #142339;
  --flow-active: #153753;
  --flow-line: #39516d;
  --flow-route: #508ce0;
  --flow-pulse: #9df8ff;
  --flow-grid: #6587c018;
}

.flow-heading {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: end;
  gap: 14px 24px;
  margin-bottom: 21px;
}

.flow-heading h2 {
  margin: 0;
  font-size: clamp(28px, 3.1vw, 44px);
  font-weight: 750;
  letter-spacing: -.033em;
  line-height: 1.2;
  text-wrap: balance;
}
.flow-heading p { margin: 10px 0 0; color: var(--muted); font-size: 14px; line-height: 1.65; }

.flow-modes {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
  padding: 4px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--flow-surface);
}

.flow-modes button {
  display: inline-flex;
  min-height: 36px;
  align-items: center;
  gap: 6px;
  padding: 0 12px;
  border-radius: 7px;
  color: var(--muted);
  font-size: 13px;
  font-weight: 700;
}

.flow-modes button:hover,
.flow-modes button[aria-pressed='true'] { color: var(--accent); background: var(--flow-active); }

.flow-board {
  padding: 22px clamp(10px, 3vw, 32px) 0;
  border: 1px solid var(--border);
  border-radius: 18px;
  background:
    radial-gradient(ellipse at center, var(--flow-active), transparent 64%),
    repeating-linear-gradient(0deg, transparent 0 27px, var(--flow-grid) 27px 28px),
    repeating-linear-gradient(90deg, transparent 0 27px, var(--flow-grid) 27px 28px),
    var(--flow-surface);
}

.flow-labels {
  display: flex;
  justify-content: space-between;
  margin-bottom: 12px;
  padding-inline: 5px;
  color: var(--muted);
  font-size: 12px;
  font-weight: 700;
}

.flow-diagram {
  position: relative;
  isolation: isolate;
  display: grid;
  min-height: 348px;
  grid-template-columns: minmax(0, 21%) minmax(0, 1fr) minmax(0, 21%);
}

.flow-wires {
  position: absolute;
  z-index: -1;
  inset: 0;
  width: 100%;
  height: 100%;
  fill: none;
  pointer-events: none;
}

.flow-wires path { stroke: var(--flow-line); stroke-width: 1.35; vector-effect: non-scaling-stroke; }
.flow-wires .flow-selected { stroke: var(--flow-route); stroke-width: 2; }

.flow-wires .flow-pulse {
  stroke: var(--flow-pulse);
  stroke-width: 3;
  stroke-linecap: round;
  stroke-dasharray: 9 191;
  animation: flow-transmit 3s linear infinite;
}

.flow-wires .flow-pulse-model { animation-delay: 1.5s; }
.flow-returning .flow-wires .flow-pulse { animation-name: flow-receive; }
.flow-returning .flow-wires .flow-pulse-model { animation-delay: 0s; }
.flow-returning .flow-wires .flow-pulse-tool { animation-delay: 1.5s; }
.flow-paused .flow-wires .flow-pulse { animation-play-state: paused; }

@keyframes flow-transmit {
  from { stroke-dashoffset: 9; }
  50%, to { stroke-dashoffset: -100; }
}

@keyframes flow-receive {
  from { stroke-dashoffset: -100; }
  50%, to { stroke-dashoffset: 9; }
}

.flow-nodes {
  display: grid;
  grid-auto-rows: 1fr;
  align-items: center;
}

.flow-nodes button {
  display: flex;
  min-height: 47px;
  align-items: center;
  gap: 10px;
  padding: 7px 11px;
  border: 1px solid var(--flow-line);
  border-radius: 10px;
  background: var(--flow-node);
  color: var(--text);
  font-size: 14px;
  font-weight: 700;
  line-height: 1.25;
  text-align: left;
  transition: border-color 180ms, background-color 180ms;
}

.flow-nodes button span { overflow-wrap: anywhere; }
.flow-brand-icon {
  display: grid;
  width: 31px;
  height: 31px;
  flex-shrink: 0;
  place-items: center;
  border-radius: 8px;
  background: #f7fbff;
}
.flow-brand-icon img { width: 22px; height: 22px; object-fit: contain; }
.flow-nodes button:hover,
.flow-nodes button[aria-pressed='true'] { border-color: var(--flow-route); background: var(--flow-active); }

.flow-hub { display: grid; position: relative; place-self: center; justify-items: center; }

.flow-hub-orbit {
  position: absolute;
  z-index: -1;
  inset: -8px;
  border: 1px solid var(--flow-line);
  border-top-color: var(--flow-route);
  border-bottom-color: var(--flow-route);
  border-radius: 50%;
  opacity: .8;
  animation: flow-hub-rotate 10s linear infinite;
}

@keyframes flow-hub-rotate { to { transform: rotate(360deg); } }
.flow-paused .flow-hub-orbit { animation-play-state: paused; }

.flow-hub-mark {
  display: grid;
  width: 94px;
  height: 94px;
  place-items: center;
  border: 1px solid var(--flow-route);
  border-radius: 23px;
  background: var(--flow-node);
  box-shadow: 0 0 0 8px var(--flow-grid), 0 0 34px var(--flow-line);
}

.flow-hub img { width: 72px; height: 72px; object-fit: contain; border-radius: 16px; }

.flow-hub > span {
  position: absolute;
  top: calc(100% + 13px);
  max-width: 180%;
  overflow: hidden;
  color: var(--accent);
  font-size: 12px;
  font-weight: 800;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.flow-summary {
  display: flex;
  min-height: 57px;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: 4px 14px;
  margin-top: 11px;
  padding-block: 8px;
  border-top: 1px solid var(--border);
  font-size: 13px;
}

.flow-summary p { display: flex; flex-wrap: wrap; align-items: baseline; gap: 3px 12px; margin: 0; }
.flow-summary p span { color: var(--muted); }
.flow-summary p strong { color: var(--text); font-weight: 650; overflow-wrap: anywhere; }
.flow-summary a { display: inline-flex; min-height: 36px; align-items: center; gap: 6px; color: var(--accent); font-weight: 700; }
.flow-summary a:hover { text-decoration: underline; }

@media (max-width: 720px) {
  .flow { margin-top: 52px; }
  .flow-diagram { grid-template-columns: minmax(0, 35%) minmax(0, 1fr) minmax(0, 35%); }
  .flow-nodes button { gap: 5px; padding-inline: 6px; font-size: 12px; }
  .flow-brand-icon { width: 19px; height: 19px; border-radius: 4px; }
  .flow-brand-icon img { width: 15px; height: 15px; }
  .flow-hub-mark { width: 60px; height: 60px; border-radius: 15px; }
  .flow-hub img { width: 44px; height: 44px; border-radius: 9px; }
  .flow-hub > span { font-size: 11px; }
}

@media (max-width: 350px) {
  .flow-nodes button { gap: 4px; padding-inline: 4px; font-size: 11px; }
  .flow-brand-icon { width: 16px; height: 16px; }
  .flow-brand-icon img { width: 13px; height: 13px; }
  .flow-hub-mark { width: 51px; height: 51px; }
  .flow-hub img { width: 39px; height: 39px; }
}

@media (prefers-reduced-motion: reduce) {
  .flow-wires .flow-pulse { display: none; }
  .flow-hub-orbit { animation: none; }
  .flow-nodes button { transition: none; }
}
</style>
