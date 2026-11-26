<template>
  <div class="landing" data-testid="default-home">
    <a href="#landing-main" class="landing-skip">{{ t('home.landing.skip') }}</a>
    <header class="landing-header">
      <div class="landing-shell landing-header-inner">
        <router-link to="/" class="landing-brand">
          <img :src="siteLogo" alt="" width="34" height="34" />
          <span>{{ siteName }}</span>
        </router-link>
        <nav class="landing-nav" :aria-label="t('home.landing.navigation')">
          <router-link v-if="showModelPlazaEntry" to="/model-plaza?embedded=1">{{ t('nav.modelPlaza') }}</router-link>
          <a href="#gateway-flow">{{ t('home.flow.navigation') }}</a>
          <router-link to="/media-capabilities">{{ t('home.landing.media') }}</router-link>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.landing.developerDocs') }}</a>
          <router-link v-else to="/media-docs">{{ t('nav.imagevideoDocs') }}</router-link>
          <a href="#science">ZeroWall Science</a>
        </nav>
        <div class="landing-header-actions">
          <ThemeToggle />
          <LocaleSwitcher :show-flags="false" class="landing-locale" />
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="landing-button landing-button-outline landing-login">
            {{ t(isAuthenticated ? 'home.dashboard' : 'home.login') }}
            <Icon name="arrowRight" size="xs" aria-hidden="true" />
          </router-link>
        </div>
      </div>
    </header>

    <main id="landing-main" tabindex="-1">
      <section class="landing-hero" aria-labelledby="gateway-title">
        <OrbitArtwork class="landing-hero-art" />
        <div class="landing-shell landing-hero-inner">
          <p class="landing-label"><span class="landing-signal"></span>{{ t('home.landing.gatewayLabel') }}</p>
          <h1 id="gateway-title">
            {{ t('home.landing.heroFirst') }}
            <span>{{ t('home.landing.heroSecond') }}</span>
          </h1>
          <p class="landing-hero-description">{{ t('home.landing.heroDescription') }}</p>
          <div class="landing-actions">
            <router-link to="/keys" class="landing-button landing-button-primary">
              <Icon name="key" size="sm" aria-hidden="true" />
              {{ t('home.landing.getKey') }}
              <Icon name="arrowRight" size="sm" aria-hidden="true" />
            </router-link>
            <a href="#quickstart" class="landing-button landing-button-outline">
              <Icon name="terminal" size="sm" aria-hidden="true" />
              {{ t('home.landing.quickstart') }}
            </a>
          </div>
        </div>
      </section>

      <GatewayFlow class="landing-shell" :site-name="siteName" :site-logo="siteLogo"
        :show-model-plaza-entry="showModelPlazaEntry" />

      <section id="capabilities" class="landing-shell landing-section" aria-labelledby="capabilities-title">
        <p class="landing-label">{{ t('home.landing.capabilitiesLabel') }}</p>
        <h2 id="capabilities-title">{{ t('home.landing.capabilitiesTitle') }}</h2>
        <div class="landing-cards">
          <router-link v-for="card in gatewayCards" :key="card.key" :to="card.to" class="landing-card">
            <GatewayCapabilityArt :kind="card.key" :site-logo="siteLogo" />
            <h3>{{ t(`home.landing.cards.${card.key}.title`) }} <Icon name="arrowRight" size="sm" aria-hidden="true" /></h3>
            <p>{{ t(`home.landing.cards.${card.key}.description`) }}</p>
          </router-link>
        </div>
      </section>

      <section id="quickstart" class="landing-shell landing-quickstart" aria-labelledby="quickstart-title">
        <LandingDataFlow variant="quickstart" />
        <div class="landing-quickstart-copy">
          <p class="landing-label">{{ t('home.landing.quickstart') }}</p>
          <h2 id="quickstart-title">{{ t('home.landing.quickTitle') }}</h2>
          <p>{{ t('home.landing.quickDescription') }}</p>
          <router-link to="/keys" class="landing-link">
            {{ t('home.landing.getKey') }} <Icon name="arrowRight" size="sm" aria-hidden="true" />
          </router-link>
        </div>
        <div class="landing-code">
          <div class="landing-code-header">
            <span><Icon name="terminal" size="sm" aria-hidden="true" /> GET /v1/models</span>
            <button type="button" class="landing-copy" @click="copyToClipboard(quickstartExample)">
              <Icon :name="copied ? 'check' : 'clipboard'" size="sm" aria-hidden="true" />
              {{ t(copied ? 'home.landing.copied' : 'home.landing.copy') }}
            </button>
          </div>
          <pre aria-label="cURL"><code><span class="landing-code-accent">curl</span>{{ ' ' }}<span class="landing-code-url">'{{ quickstartEndpoint }}'</span>{{ quickstartAuthHeader }}</code></pre>
        </div>
      </section>

      <section id="science" class="landing-shell landing-section landing-science" aria-labelledby="science-title">
        <div class="landing-science-intro">
          <LandingDataFlow variant="science" />
          <div class="landing-science-copy">
            <p class="landing-science-brand">
              <img src="/zerowall-icon.png" alt="" width="36" height="36" loading="lazy" />
              ZeroWall Science
            </p>
            <h2 id="science-title">{{ t('home.landing.scienceTitle') }}</h2>
            <p class="landing-science-description">{{ t('home.landing.scienceDescription') }}</p>
            <a :href="scienceUrl" target="_blank" rel="noopener noreferrer" class="landing-button landing-button-science">
              {{ t('home.landing.scienceVisit') }} <Icon name="externalLink" size="sm" aria-hidden="true" />
            </a>
          </div>
          <figure class="landing-science-figure">
            <img src="../assets/landing/science-workspace.webp" :alt="t('home.landing.scienceImageAlt')" width="1248" height="780" loading="lazy" decoding="async" />
            <figcaption>{{ t('home.landing.scienceImageCaption') }}</figcaption>
          </figure>
        </div>
        <ul class="landing-science-features">
          <li v-for="feature in scienceFeatures" :key="feature.key">
            <a :href="`${scienceUrl}#${feature.section}`" target="_blank" rel="noopener noreferrer">
              <span class="landing-science-feature-mark">
                <Icon :name="feature.icon" size="lg" aria-hidden="true" />
                <svg viewBox="0 0 100 45" fill="none" aria-hidden="true">
                  <path :d="feature.art" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round" stroke-linecap="round" />
                  <path class="landing-science-feature-pulse" :d="feature.art" pathLength="100" />
                </svg>
              </span>
              <Icon class="landing-science-feature-arrow" name="externalLink" size="xs" aria-hidden="true" />
              <h3>{{ t(`home.landing.scienceFeatures.${feature.key}.title`) }}</h3>
              <p>{{ t(`home.landing.scienceFeatures.${feature.key}.description`) }}</p>
            </a>
          </li>
        </ul>
      </section>
    </main>

    <footer class="landing-footer">
      <div class="landing-shell landing-footer-inner">
        <span>&copy; {{ currentYear }} {{ siteName }}</span>
        <nav :aria-label="t('home.landing.footerNavigation')">
          <router-link to="/media-docs">{{ t('nav.imagevideoDocs') }}</router-link>
          <a :href="scienceUrl" target="_blank" rel="noopener noreferrer">ZeroWall Science <Icon name="externalLink" size="xs" aria-hidden="true" /></a>
        </nav>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import OrbitArtwork from '@/components/common/OrbitArtwork.vue'
import ThemeToggle from '@/components/common/ThemeToggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { sanitizeUrl } from '@/utils/url'
import GatewayCapabilityArt from './GatewayCapabilityArt.vue'
import GatewayFlow from './GatewayFlow.vue'
import LandingDataFlow from './LandingDataFlow.vue'

const props = defineProps<{
  siteName: string
  siteLogo: string
  docUrl: string
  apiBaseUrl: string
  showModelPlazaEntry: boolean
  isAuthenticated: boolean
  dashboardPath: string
  currentYear: number
}>()

const { t } = useI18n()
const { copied, copyToClipboard } = useClipboard()


const gatewayCards = computed(() => [
  { key: 'models', to: props.showModelPlazaEntry ? '/model-plaza?embedded=1' : '/keys' },
  { key: 'media', to: '/media-capabilities' },
  { key: 'usage', to: '/usage' },
] as const)

const scienceUrl = 'https://zerowallscience.org/'
const scienceFeatures = [
  { key: 'agent', icon: 'sparkles', section: 'agent', art: 'M8 22h24m14 0h18m0 0v-12h16M64 22v12h16M8 18h8v8H8zm26-3h12v14H34zm46-9h9v8h-9zm0 24h9v8h-9z' },
  { key: 'singleCell', icon: 'beaker', section: 'knockout', art: 'M22 14l18 16 15-19 20 9-13 15-22-5M55 11l7 24M22 10a4 4 0 1 0 0 8 4 4 0 0 0 0-8zm18 16a4 4 0 1 0 0 8 4 4 0 0 0 0-8zm35-10a4 4 0 1 0 0 8 4 4 0 0 0 0-8z' },
  { key: 'medical', icon: 'chart', section: 'medical', art: 'M9 24h18l8-10 8 17 8-23 9 26 6-10h20M19 34h65M19 14h66' },
  { key: 'reproduction', icon: 'book', section: 'reproduce', art: 'M13 12h25v22H13zm49 0h25v22H62zM19 18h13m-13 5h11m-11 5h13m36-8 4 4 8-8M42 23h16m-5-4 5 4-5 4' },
  { key: 'presentation', icon: 'document', section: 'presentation', art: 'M28 7h51v26H28zM22 13v26h52M34 13h39m-39 7h17m-17 5h13m12 1v-5m6 5v-8m6 8v-4' },
] as const

const quickstartEndpoint = computed(() => {
  const base = new URL(sanitizeUrl(props.apiBaseUrl) || window.location.origin)
  const basePath = base.pathname.replace(/\/+$/, '')
  const apiPath = basePath.endsWith('/v1') ? basePath : `${basePath}/v1`
  return `${base.origin}${apiPath}/models`.replace(/'/g, '%27')
})
const quickstartAuthHeader = ' -H "Authorization: Bearer $SUB2API_KEY"'
const quickstartExample = computed(() => `curl '${quickstartEndpoint.value}'${quickstartAuthHeader}`)
</script>

<style scoped src="./home-landing.css"></style>
