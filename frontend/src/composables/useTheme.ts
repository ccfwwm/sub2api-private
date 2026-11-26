import { readonly, ref } from 'vue'

type Theme = 'light' | 'dark'
type ThemeOverride = { theme: Theme; until: number }

const storageKey = 'theme'
const hourMs = 60 * 60 * 1000
const halfDayMs = 12 * hourMs
// Beijing uses UTC+08:00. Its 07:00 and 19:00 boundaries are at 23:00 and 11:00 UTC.
const scheduleOffsetMs = hourMs
const dark = ref(false)
let override: ThemeOverride | null = null
let timer: number | undefined

function themePeriod(now: number) {
  const period = Math.floor((now + scheduleOffsetMs) / halfDayMs)
  return {
    dark: period % 2 !== 0,
    until: (period + 1) * halfDayMs - scheduleOffsetMs
  }
}

function updateTheme() {
  const now = Date.now()
  const period = themePeriod(now)
  if (override && override.until !== period.until) override = null
  dark.value = override ? override.theme === 'dark' : period.dark
  document.documentElement.classList.toggle('dark', dark.value)
  document.documentElement.style.colorScheme = dark.value ? 'dark' : 'light'
  window.clearTimeout(timer)
  timer = window.setTimeout(updateTheme, period.until - now)
}

function readOverride(value: string | null): ThemeOverride | null {
  try {
    const parsed = value ? JSON.parse(value) : null
    return parsed && (parsed.theme === 'light' || parsed.theme === 'dark') &&
      Number.isFinite(parsed.until) ? parsed : null
  } catch {
    return null
  }
}

function onStorage(event: StorageEvent) {
  if (event.key !== storageKey && event.key !== null) return
  override = readOverride(event.newValue)
  updateTheme()
}

export function initTheme() {
  try {
    override = readOverride(localStorage.getItem(storageKey))
  } catch {
    override = null
  }
  window.addEventListener('storage', onStorage)
  window.addEventListener('pageshow', updateTheme)
  document.addEventListener('visibilitychange', updateTheme)
  updateTheme()
}

const isDark = readonly(dark)

function toggleTheme() {
  updateTheme()
  override = { theme: dark.value ? 'light' : 'dark', until: themePeriod(Date.now()).until }
  updateTheme()
  try {
    localStorage.setItem(storageKey, JSON.stringify(override))
  } catch {
    // The current tab can still switch themes if browser storage is blocked.
  }
}

export function useTheme() {
  return { isDark, toggleTheme }
}
