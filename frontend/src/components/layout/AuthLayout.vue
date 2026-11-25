<template>
  <div :class="portal ? 'auth-portal' : 'relative flex min-h-screen items-center justify-center overflow-hidden p-4'">
    <header v-if="portal" class="auth-portal-header">
      <router-link to="/" class="auth-portal-brand">
        <img :src="portalLogo" alt="" width="36" height="36" />
        <span>{{ portalSiteName }}</span>
      </router-link>
      <div class="auth-portal-tools">
        <ThemeToggle />
        <LocaleSwitcher :show-flags="false" />
      </div>
    </header>
    <AuthNebula v-if="portal" :site-name="portalSiteName" :subtitle="customSubtitle" />
    <!-- Background -->
    <div
      v-if="!portal"
      class="absolute inset-0 bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"
    ></div>

    <!-- Decorative Elements -->
    <div v-if="!portal" class="pointer-events-none absolute inset-0 overflow-hidden">
      <!-- Gradient Orbs -->
      <div
        class="absolute -right-40 -top-40 h-80 w-80 rounded-full bg-primary-400/20 blur-3xl"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 h-80 w-80 rounded-full bg-primary-500/15 blur-3xl"
      ></div>
      <div
        class="absolute left-1/2 top-1/2 h-96 w-96 -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary-300/10 blur-3xl"
      ></div>

      <!-- Grid Pattern -->
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"
      ></div>
    </div>

    <!-- Content Container -->
    <component :is="portal ? 'main' : 'div'" :class="portal ? 'auth-portal-form' : 'relative z-10 w-full max-w-md'">
      <!-- Logo/Brand -->
      <div v-if="!portal" class="mb-8 text-center">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div
            class="mb-4 inline-flex h-16 w-16 items-center justify-center overflow-hidden rounded-2xl shadow-lg shadow-primary-500/30"
          >
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="text-gradient mb-2 text-3xl font-bold">
            {{ siteName }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div :class="portal ? 'auth-portal-card' : 'card-glass rounded-2xl p-8 shadow-glass'">
        <slot />
      </div>

      <!-- Footer Links -->
      <div v-if="!portal || $slots.footer" class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center text-xs text-gray-500 dark:text-dark-400">
        &copy; {{ currentYear }} {{ portal ? portalSiteName : siteName }}
        <template v-if="!portal">. All rights reserved.</template>
      </div>
    </component>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import ThemeToggle from '@/components/common/ThemeToggle.vue'
import AuthNebula from './AuthNebula.vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import { gatewayBrandLogo } from '@/utils/branding'

const appStore = useAppStore()
const { t } = useI18n()
defineProps<{ portal?: boolean }>()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const portalSiteName = computed(() => siteName.value === 'Sub2API' ? t('home.landing.brand') : siteName.value)
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const portalLogo = computed(() => gatewayBrandLogo(siteName.value, window.location.origin, siteLogo.value))
const customSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle)
const siteSubtitle = computed(() => customSubtitle.value || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
}

.auth-portal {
  --auth-ink: #f0f5fe;
  --auth-surface: #fff;
  --auth-border: #cfddf0;
  --auth-text: #142b49;
  --auth-muted: #536981;
  --auth-accent: #125a9d;
  --auth-hover: #104b82;
  --auth-button-text: #fff;
  --auth-shadow: #243b6020;
  display: grid;
  width: min(1460px, calc(100% - 64px));
  min-height: 100vh;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, .95fr);
  grid-template-rows: auto 1fr;
  gap: 0 22px;
  margin-inline: auto;
  padding-bottom: 30px;
  color: var(--auth-text);
}

:global(.dark .auth-portal) {
  --auth-ink: #101d30;
  --auth-surface: #0c1525;
  --auth-border: #30425e;
  --auth-text: #f0f5fc;
  --auth-muted: #a2b3cb;
  --auth-accent: #8cebe9;
  --auth-hover: #b4faf7;
  --auth-button-text: #0b2930;
  --auth-shadow: #02071070;
}

.auth-portal-header,
.auth-portal-brand,
.auth-portal-tools {
  display: flex;
  align-items: center;
}

.auth-portal-header {
  min-height: 72px;
  grid-column: 1 / -1;
  justify-content: space-between;
  gap: 12px;
}

.auth-portal-brand { min-width: 0; gap: 12px; font-size: 17px; font-weight: 750; overflow-wrap: anywhere; }
.auth-portal-brand img { flex-shrink: 0; object-fit: contain; border-radius: 9px; }
.auth-portal-tools { gap: 6px; }

.auth-portal-form {
  width: 100%;
  max-width: 530px;
  align-self: center;
  justify-self: center;
  padding: 25px 7px;
}

.auth-portal-card {
  padding: clamp(23px, 3.3vw, 46px);
  border: 1px solid var(--auth-border);
  border-radius: 22px;
  background: var(--auth-surface);
  box-shadow: 0 20px 62px var(--auth-shadow);
}

.auth-portal-card :deep(h1) { font-size: clamp(27px, 3.2vw, 38px); letter-spacing: -.045em; line-height: 1.2; }
.auth-portal-card :deep(.input) { min-height: 46px; border-color: var(--auth-border); background: var(--auth-ink); }
.auth-portal-card :deep(.input::placeholder) { color: var(--auth-muted); }
.auth-portal-card :deep(.input:focus) { border-color: var(--auth-accent); }
.auth-portal-card :deep(.input.input-error) { border-color: #e46767; }
.auth-portal-card :deep(.btn-primary) { min-height: 46px; color: var(--auth-button-text); background: var(--auth-accent); }
.auth-portal-card :deep(.btn-primary:not(:disabled):hover) { background: var(--auth-hover); }
.auth-portal-card :deep(.btn-primary:disabled) { opacity: .58; }
.auth-portal-card :deep(.btn-primary .animate-spin) { color: inherit; }
.auth-portal-form :deep(a.text-primary-600) { color: var(--auth-accent); }
.auth-portal-form :deep(a.text-primary-600:hover) { color: var(--auth-hover); text-decoration: underline; }
.auth-portal :deep(:is(a, button):focus-visible) { outline: 2px solid var(--auth-accent); outline-offset: 3px; }

@media (max-width: 800px) {
  .auth-portal { width: min(560px, calc(100% - 36px)); grid-template-columns: minmax(0, 1fr); grid-template-rows: auto auto 1fr; }
  .auth-portal-header { min-height: 63px; }
  .auth-portal-form { padding: 18px 0 0; }
  .auth-portal-card { border-radius: 16px; }
}
</style>
