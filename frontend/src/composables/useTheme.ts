import { readonly, ref } from 'vue'

type Theme = 'light' | 'dark'
const storageKey = 'theme'
const systemQuery = '(prefers-color-scheme: dark)'
const dark = ref(false)
let systemTheme: MediaQueryList | undefined
let preference: Theme | null = null

function applyTheme() {
  dark.value = preference === 'dark' ||
    (preference === null && Boolean(systemTheme?.matches))
  document.documentElement.classList.toggle('dark', dark.value)
  document.documentElement.style.colorScheme = dark.value ? 'dark' : 'light'
}

function parseTheme(value: string | null): Theme | null {
  return value === 'dark' || value === 'light' ? value : null
}

export function initTheme() {
  try {
    preference = parseTheme(localStorage.getItem(storageKey))
  } catch {
    preference = null
  }
  systemTheme = window.matchMedia?.(systemQuery)
  applyTheme()
  systemTheme?.addEventListener?.('change', applyTheme)
  window.addEventListener('storage', (event) => {
    if (event.key !== storageKey && event.key !== null) return
    preference = parseTheme(event.newValue)
    applyTheme()
  })
}

const isDark = readonly(dark)

function toggleTheme() {
  preference = dark.value ? 'light' : 'dark'
  applyTheme()
  try {
    localStorage.setItem(storageKey, preference)
  } catch {
    // The current tab can still switch themes if browser storage is blocked.
  }
}

export function useTheme() {
  return { isDark, toggleTheme }
}
