<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="hasHomeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Compact Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-9 w-9 shrink-0 rounded-lg object-contain" />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img :src="siteLogo || '/logo.svg'" alt="Logo" class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain" />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <!-- Default Home Page -->
  <div v-else ref="homeRoot" data-testid="default-home" class="token-home">
    <canvas ref="heroCanvas" class="token-field" aria-hidden="true"></canvas>
    <div class="token-rings" aria-hidden="true">
      <span class="token-ring token-ring-outer"></span>
      <span class="token-ring token-ring-middle"></span>
      <span class="token-ring token-ring-inner"></span>
      <span class="token-orbit-dot token-orbit-dot-a"></span>
      <span class="token-orbit-dot token-orbit-dot-b"></span>
      <span class="token-orbit-dot token-orbit-dot-c"></span>
    </div>

    <header class="token-header">
      <div class="token-shell token-header-inner">
        <router-link to="/" class="token-brand" aria-label="词元互联，返回首页">
          <img :src="siteLogo || '/logo.svg'" alt="词元互联" class="token-brand-mark" />
          <span>词元互联</span>
        </router-link>
        <nav class="token-nav" aria-label="首页导航">
          <router-link v-if="showModelPlazaEntry" :to="modelPlazaRoute" class="token-nav-link">
            模型能力
          </router-link>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="token-nav-action">
            进入平台 <span aria-hidden="true">↗</span>
          </router-link>
        </nav>
      </div>
    </header>

    <main class="token-main">
      <section class="token-hero" aria-labelledby="token-title">
        <p class="token-kicker">AI MODELS <span aria-hidden="true">×</span> RESEARCH</p>
        <h1 id="token-title">词元互联<span>GPT · Claude · DeepSeek</span></h1>
        <p class="token-tagline">统一的模型入口与科研工作台。</p>
        <div class="token-actions">
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="token-button token-button-primary">
            进入平台 <span aria-hidden="true">↗</span>
          </router-link>
          <a href="https://zerowallscience.org/" target="_blank" rel="noopener noreferrer" class="token-button token-button-secondary">
            ZeroWall Science <span aria-hidden="true">↗</span>
          </a>
        </div>
      </section>

      <section class="token-science token-shell" aria-labelledby="science-title">
        <div class="token-science-logo-wrap">
          <img src="/zerowall-icon.png" alt="ZeroWall Science" class="token-science-logo" />
        </div>
        <div class="token-science-copy">
          <p class="token-kicker">ZERO WALL SCIENCE</p>
          <h2 id="science-title">科研过程，有迹可循。</h2>
          <p>项目工作区 · 科研工具 · 可恢复任务 · 证据追溯</p>
          <a href="https://zerowallscience.org/" target="_blank" rel="noopener noreferrer" class="token-science-link">
            访问官网 <span aria-hidden="true">↗</span>
          </a>
        </div>
      </section>
    </main>

    <footer class="token-footer">
      <span>&copy; {{ currentYear }} 词元互联</span>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const modelPlazaRequiresAuth = computed(() => appStore.cachedPublicSettings?.model_plaza_require_auth === true)
const showModelPlazaEntry = computed(() => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value))
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const currentYear = computed(() => new Date().getFullYear())
const modelPlazaRoute = '/model-plaza?embedded=1'
const heroCanvas = ref<HTMLCanvasElement | null>(null)
let stopHeroField: (() => void) | null = null

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

function startHeroField() {
  const canvas = heroCanvas.value
  const root = canvas?.closest<HTMLElement>('.token-home')
  if (!canvas || !root) return () => undefined
  if (window.navigator.userAgent.toLowerCase().includes('jsdom')) return () => undefined

  let context: CanvasRenderingContext2D | null = null
  try {
    context = canvas.getContext('2d')
  } catch {
    return () => undefined
  }
  if (!context) return () => undefined

  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const canAnimate = !reduceMotion && typeof window.requestAnimationFrame === 'function'
  const particles = Array.from({ length: 48 }, (_, index) => ({
    x: ((index * 83) % 101) / 100,
    y: ((index * 47 + 13) % 97) / 100,
    radius: index % 8 === 0 ? 2.4 : 1.1,
    speed: 0.0006 + (index % 5) * 0.00016,
    phase: index * 1.7,
    color: index % 3 === 0 ? '#7184e7' : index % 3 === 1 ? '#70cbbd' : '#c58fdb',
  }))
  let width = 1
  let height = 1
  let frame = 0

  const resize = () => {
    const ratio = Math.min(window.devicePixelRatio || 1, 2)
    width = root.clientWidth || 1024
    height = root.clientHeight || 720
    canvas.width = Math.floor(width * ratio)
    canvas.height = Math.floor(height * ratio)
    context?.setTransform(ratio, 0, 0, ratio, 0, 0)
  }

  const draw = (time: number) => {
    if (!context) return
    context.clearRect(0, 0, width, height)
    const centerX = width * 0.5
    const centerY = height * 0.43
    const baseRadius = Math.min(width, height) * 0.37
    context.save()
    context.translate(centerX, centerY)
    particles.forEach((particle) => {
      const angle = particle.phase + time * particle.speed
      const x = particle.x * width - centerX + Math.cos(angle) * baseRadius * 0.07
      const y = particle.y * height - centerY + Math.sin(angle * 1.13) * baseRadius * 0.07
      context.beginPath()
      context.fillStyle = particle.color
      context.globalAlpha = particle.radius > 2 ? 0.72 : 0.34
      context.arc(x, y, particle.radius, 0, Math.PI * 2)
      context.fill()
    })
    context.restore()
    if (canAnimate) frame = window.requestAnimationFrame(draw)
  }

  resize()
  draw(0)
  window.addEventListener('resize', resize, { passive: true })
  return () => {
    window.removeEventListener('resize', resize)
    if (frame) window.cancelAnimationFrame(frame)
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
  stopHeroField = startHeroField()
})

onUnmounted(() => {
  stopHeroField?.()
  stopHeroField = null
})
</script>

<style scoped>
.token-home {
  position: relative;
  isolation: isolate;
  min-height: 100vh;
  overflow: hidden;
  color: #17243d;
  background:
    radial-gradient(circle at 50% 38%, rgba(229, 220, 255, 0.8), transparent 28%),
    radial-gradient(circle at 7% 76%, rgba(255, 226, 237, 0.5), transparent 26%),
    radial-gradient(circle at 94% 20%, rgba(210, 220, 255, 0.7), transparent 30%),
    linear-gradient(115deg, #fbfdff 0%, #f5faff 48%, #f2f5ff 100%);
}

.token-home::before,
.token-home::after {
  position: absolute;
  inset: 0;
  z-index: -2;
  pointer-events: none;
  content: '';
}

.token-home::before {
  opacity: 0.54;
  background-image: radial-gradient(rgba(105, 128, 183, 0.3) 0.8px, transparent 0.8px);
  background-size: 34px 34px;
  mask-image: linear-gradient(to bottom, #000, rgba(0, 0, 0, 0.7) 72%, transparent);
  animation: token-dots 18s linear infinite;
}

.token-home::after {
  z-index: -3;
  background: radial-gradient(circle at 50% 40%, rgba(118, 137, 225, 0.13), transparent 32%);
  animation: token-glow 12s ease-in-out infinite alternate;
}

.token-field,
.token-rings {
  position: absolute;
  inset: 0;
  z-index: -1;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

.token-ring {
  position: absolute;
  left: 50%;
  top: 42%;
  aspect-ratio: 1;
  border: 1px solid rgba(116, 139, 196, 0.13);
  border-radius: 50%;
  transform: translate(-50%, -50%);
}

.token-ring-outer { width: min(82vw, 920px); animation: token-ring-spin 34s linear infinite; }
.token-ring-middle { width: min(64vw, 700px); border-style: dashed; animation: token-ring-spin-reverse 25s linear infinite; }
.token-ring-inner { width: min(44vw, 470px); animation: token-ring-spin 18s linear infinite; }

.token-orbit-dot {
  position: absolute;
  left: 50%;
  top: 42%;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  box-shadow: 0 0 0 5px rgba(255, 255, 255, 0.42), 0 0 18px currentColor;
}

.token-orbit-dot-a { color: #7184e7; animation: token-orbit-a 12s linear infinite; }
.token-orbit-dot-b { color: #70cbbd; animation: token-orbit-b 17s linear infinite; }
.token-orbit-dot-c { color: #c58fdb; animation: token-orbit-c 21s linear infinite; }

.token-shell { width: min(1180px, calc(100% - 48px)); margin-inline: auto; }
.token-header { position: relative; z-index: 2; height: 76px; border-bottom: 1px solid rgba(223, 232, 241, 0.76); background: rgba(255, 255, 255, 0.72); backdrop-filter: blur(18px); }
.token-header-inner { display: flex; height: 100%; align-items: center; justify-content: space-between; gap: 24px; }
.token-brand { display: inline-flex; align-items: center; gap: 11px; color: #17243d; font-size: 18px; font-weight: 800; letter-spacing: -0.04em; }
.token-brand-mark { width: 38px; height: 38px; border-radius: 12px; object-fit: contain; box-shadow: 0 7px 16px rgba(54, 117, 209, 0.18); }
.token-nav { display: flex; align-items: center; gap: 25px; }
.token-nav-link { color: #58677d; font-size: 13px; font-weight: 700; }
.token-nav-link:hover { color: #2869db; }
.token-nav-action,
.token-button { display: inline-flex; align-items: center; justify-content: center; gap: 8px; border-radius: 11px; font-size: 13px; font-weight: 750; transition: transform 0.2s, box-shadow 0.2s, background 0.2s; }
.token-nav-action { min-height: 40px; padding: 0 17px; color: #fff; background: #203553; }
.token-nav-action:hover,
.token-button:hover { transform: translateY(-2px); }
.token-main { position: relative; z-index: 1; }
.token-hero { display: flex; min-height: calc(100vh - 76px); flex-direction: column; align-items: center; justify-content: center; padding: 72px 24px 116px; text-align: center; }
.token-kicker { margin: 0 0 22px; color: #3364ba; font-size: 12px; font-weight: 800; letter-spacing: 0.13em; }
.token-kicker::before { display: inline-block; width: 18px; height: 2px; margin: 0 9px 4px 0; border-radius: 2px; background: currentColor; content: ''; }
.token-hero h1 { margin: 0; color: #101b32; font-size: clamp(60px, 10vw, 128px); font-weight: 850; letter-spacing: -0.09em; line-height: 1.02; }
.token-hero h1 span { display: block; margin-top: 13px; color: #5665d5; font-size: 0.31em; font-weight: 650; letter-spacing: 0.02em; line-height: 1.6; }
.token-tagline { margin: 20px 0 0; color: #5c6b82; font-size: 15px; line-height: 1.8; }
.token-actions { display: flex; flex-wrap: wrap; justify-content: center; gap: 11px; margin-top: 30px; }
.token-button { min-height: 48px; padding: 0 23px; }
.token-button-primary { color: #fff; background: linear-gradient(105deg, #2466d2, #5475e4); box-shadow: 0 13px 26px rgba(51, 108, 213, 0.2); }
.token-button-secondary { color: #274a7b; border: 1px solid #c7d9ee; background: rgba(255, 255, 255, 0.84); }
.token-science { display: grid; grid-template-columns: 180px 1fr; align-items: center; gap: 36px; margin-bottom: 88px; padding: 30px 36px; border: 1px solid rgba(181, 204, 234, 0.86); border-radius: 22px; background: rgba(255, 255, 255, 0.57); box-shadow: 0 24px 56px rgba(68, 104, 162, 0.08); backdrop-filter: blur(16px); }
.token-science-logo-wrap { display: grid; place-items: center; }
.token-science-logo { width: 130px; height: 130px; border-radius: 25px; object-fit: contain; background: #fff; box-shadow: 0 18px 35px rgba(67, 108, 181, 0.14); }
.token-science-copy .token-kicker { margin-bottom: 12px; }
.token-science-copy h2 { margin: 0 0 10px; color: #17243d; font-size: clamp(28px, 4vw, 44px); letter-spacing: -0.06em; line-height: 1.2; }
.token-science-copy p:not(.token-kicker) { margin: 0; color: #607089; font-size: 14px; line-height: 1.8; }
.token-science-link { display: inline-flex; gap: 8px; margin-top: 18px; color: #3d66af; font-size: 12px; font-weight: 750; }
.token-footer { position: relative; z-index: 1; padding: 26px 24px 34px; border-top: 1px solid rgba(223, 232, 241, 0.76); color: #8491a4; font-size: 12px; text-align: center; }

@keyframes token-dots { from { background-position: 0 0; } to { background-position: 34px 34px; } }
@keyframes token-glow { from { opacity: 0.58; transform: scale(0.96); } to { opacity: 1; transform: scale(1.05); } }
@keyframes token-ring-spin { from { transform: translate(-50%, -50%) rotate(0deg); } to { transform: translate(-50%, -50%) rotate(360deg); } }
@keyframes token-ring-spin-reverse { from { transform: translate(-50%, -50%) rotate(360deg); } to { transform: translate(-50%, -50%) rotate(0deg); } }
@keyframes token-orbit-a { from { transform: translate(-50%, -50%) rotate(0deg) translateX(min(32vw, 350px)); } to { transform: translate(-50%, -50%) rotate(360deg) translateX(min(32vw, 350px)); } }
@keyframes token-orbit-b { from { transform: translate(-50%, -50%) rotate(180deg) translateX(min(23vw, 250px)); } to { transform: translate(-50%, -50%) rotate(-180deg) translateX(min(23vw, 250px)); } }
@keyframes token-orbit-c { from { transform: translate(-50%, -50%) rotate(70deg) translateX(min(16vw, 175px)); } to { transform: translate(-50%, -50%) rotate(430deg) translateX(min(16vw, 175px)); } }

@media (max-width: 700px) {
  .token-shell { width: min(100% - 36px, 520px); }
  .token-header { height: 68px; }
  .token-brand { font-size: 16px; }
  .token-brand-mark { width: 32px; height: 32px; border-radius: 10px; }
  .token-nav { gap: 10px; }
  .token-nav-link { font-size: 12px; }
  .token-nav-action { min-height: 36px; padding: 0 12px; font-size: 12px; }
  .token-hero { min-height: 610px; padding: 70px 18px 88px; }
  .token-hero h1 { font-size: clamp(58px, 18vw, 92px); }
  .token-hero h1 span { font-size: 0.34em; }
  .token-tagline { font-size: 14px; }
  .token-science { grid-template-columns: 1fr; gap: 21px; margin-bottom: 58px; padding: 26px 23px 30px; text-align: center; }
  .token-science-logo { width: 112px; height: 112px; border-radius: 22px; }
  .token-kicker::before { margin-left: 0; }
  .token-science-link { justify-content: center; }
  .token-ring-outer { width: 112vw; }
  .token-ring-middle { width: 88vw; }
  .token-ring-inner { width: 62vw; }
}

@media (prefers-reduced-motion: reduce) {
  .token-home::before,
  .token-home::after,
  .token-ring,
  .token-orbit-dot { animation: none; }
  .token-nav-action,
  .token-button { transition: none; }
}
</style>
