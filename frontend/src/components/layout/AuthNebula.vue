<template>
  <aside class="auth-nebula" :aria-label="siteName">
    <div class="auth-nebula-space" aria-hidden="true">
      <picture class="auth-nebula-photo">
        <source media="(max-width: 800px)" :srcset="nebulaMobile" />
        <img :src="nebula" alt="" width="1120" height="1120" />
      </picture>
      <div class="auth-nebula-dust"></div>
    </div>

    <ul class="auth-nebula-models" :aria-label="t('home.flow.models')">
      <li v-for="model in featuredGatewayModels" :key="model.id" :class="`auth-nebula-model-${model.id}`">
        <span class="auth-nebula-icon"><img :src="model.logo" alt="" width="20" height="20" /></span>
        <span class="auth-nebula-model-name">{{ model.label }}</span>
      </li>
    </ul>

    <div class="auth-nebula-copy">
      <p class="auth-nebula-label">{{ t('home.landing.gatewayLabel') }}</p>
      <p class="auth-nebula-headline">{{ t('auth.portal.headline') }}</p>
      <p class="auth-nebula-caption">{{ subtitle || t('auth.portal.caption') }}</p>
    </div>

  </aside>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { featuredGatewayModels } from '@/components/common/gatewayModelBrands'
import nebula from '@/assets/landing/nebula.webp'
import nebulaMobile from '@/assets/landing/nebula-mobile.webp'

defineProps<{ siteName: string; subtitle?: string }>()

const { t } = useI18n()
</script>

<style scoped>
.auth-nebula {
  position: relative;
  isolation: isolate;
  display: flex;
  min-height: 560px;
  align-items: end;
  overflow: hidden;
  padding: clamp(29px, 4.8vw, 68px);
  border: 1px solid #294568;
  border-radius: 24px;
  color: #f1f7ff;
  background: radial-gradient(ellipse at 48% 32%, #143859, transparent 70%), #071428;
}

.auth-nebula-space,
.auth-nebula-space::after {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.auth-nebula-space { z-index: -1; }

.auth-nebula-space::after {
  background: linear-gradient(0deg, #071428 1%, #071428ef 16%, transparent 47%);
  content: '';
}

.auth-nebula-photo {
  position: absolute;
  top: 39%;
  left: 50%;
  display: block;
  width: 108%;
  aspect-ratio: 1;
  mix-blend-mode: screen;
  transform: translate(-50%, -50%);
}

.auth-nebula-photo img {
  width: 100%;
  height: 100%;
  mask-image: radial-gradient(circle, #000 48%, transparent 69%);
  animation: nebula-rotate 22s linear infinite;
}

.auth-nebula-dust {
  position: absolute;
  inset: 0;
  opacity: .53;
  background:
    radial-gradient(circle at 19% 17%, #8bdaff 0 1.5px, transparent 4px),
    radial-gradient(circle at 81% 16%, #a4ceff 0 1px, transparent 5px),
    radial-gradient(circle at 74% 47%, #f5e3ff 0 1.5px, transparent 5px),
    radial-gradient(circle at 29% 49%, #64e4ff 0 1px, transparent 4px),
    radial-gradient(circle at 49% 8%, #7d9fff 0 1px, transparent 4px),
    radial-gradient(circle at 88% 48%, #89ffff 0 1px, transparent 4px);
  animation: nebula-drift 7s ease-in-out infinite alternate;
}

@keyframes nebula-rotate { to { transform: rotate(360deg); } }
@keyframes nebula-drift { to { opacity: .9; transform: translate(2%, -2%) scale(1.085); } }


.auth-nebula-models {
  position: absolute;
  inset: 0 0 24%;
  margin: 0;
  padding: 0;
  list-style: none;
  pointer-events: none;
}

.auth-nebula-models li {
  position: absolute;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 5px 9px 5px 5px;
  border: 1px solid #8ab5ee80;
  border-radius: 10px;
  color: #eaf6ff;
  background: #0b1b35e8;
  box-shadow: 0 8px 23px #020b1cad, 0 0 17px #2277c02b;
  font-size: 12px;
  font-weight: 650;
  animation: nebula-model-drift 7s ease-in-out infinite alternate;
}

.auth-nebula-models .auth-nebula-model-claude { top: 19%; left: 13%; animation-delay: -3s; }
.auth-nebula-models .auth-nebula-model-gpt { top: 15%; right: 18%; animation-delay: -4.5s; }
.auth-nebula-models .auth-nebula-model-gemini { top: 49%; right: 9%; animation-delay: -6s; }
.auth-nebula-models .auth-nebula-model-deepseek { top: 56%; left: 9%; animation-delay: -1.6s; }
.auth-nebula-models .auth-nebula-model-kimi { top: 77%; right: 26%; animation-delay: -5.2s; }

@keyframes nebula-model-drift { to { transform: translate(3px, -8px); } }

.auth-nebula-icon {
  display: grid;
  width: 29px;
  height: 29px;
  flex-shrink: 0;
  place-items: center;
  border-radius: 6px;
  background: #f8fcff;
}

.auth-nebula-copy { position: relative; max-width: 460px; }
.auth-nebula-label { color: #9fecf7; font-size: 13px; font-weight: 750; letter-spacing: .13em; text-transform: uppercase; }
.auth-nebula-headline { margin: 8px 0 12px; font-size: clamp(30px, 4.1vw, 60px); font-weight: 800; line-height: 1.13; letter-spacing: -.04em; text-wrap: balance; }
.auth-nebula-caption { max-width: 350px; color: #b9cbe1; font-size: 14px; line-height: 1.7; overflow-wrap: anywhere; }

@media (max-width: 800px) {
  .auth-nebula { min-height: 212px; padding: 22px 22px 64px; border-radius: 16px; }
  .auth-nebula-photo { top: 47%; left: 70%; width: 92%; }
  .auth-nebula-space::after { background: linear-gradient(90deg, #071428 3%, #071428a8 41%, transparent 81%); }
  .auth-nebula-copy { max-width: min(240px, 83%); }
  .auth-nebula-headline { margin-bottom: 0; font-size: clamp(23px, 5.3vw, 31px); }
  .auth-nebula-caption { display: none; }
  .auth-nebula-models { display: flex; align-items: center; gap: 6px; inset: auto 22px 18px; }
  .auth-nebula-models li { position: static; padding: 3px; border-radius: 7px; animation: none; }
  .auth-nebula-icon { width: 25px; height: 25px; }
  .auth-nebula-icon img { width: 16px; height: 16px; }
  .auth-nebula-model-name { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
}

@media (prefers-reduced-motion: reduce) {
  .auth-nebula-photo img, .auth-nebula-dust, .auth-nebula-models li { animation: none; }
}
</style>
