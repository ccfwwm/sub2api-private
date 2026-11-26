import { afterEach, describe, expect, it, vi } from 'vitest'
import { initTheme, useTheme } from '../useTheme'

describe('shared appearance', () => {
  afterEach(() => {
    localStorage.removeItem('theme')
    document.documentElement.classList.remove('dark')
    document.documentElement.style.removeProperty('color-scheme')
    vi.unstubAllGlobals()
  })

  it('keeps every control in sync and persists the selected appearance', () => {
    const listeners = new EventTarget()
    const system = {
      matches: true,
      addEventListener: listeners.addEventListener.bind(listeners)
    }
    vi.stubGlobal('matchMedia', vi.fn(() => system))
    initTheme()

    const header = useTheme()
    const homepage = useTheme()
    expect(header.isDark.value).toBe(true)
    header.toggleTheme()
    expect(homepage.isDark.value).toBe(false)
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem('theme')).toBe('light')

    system.matches = false
    listeners.dispatchEvent(new Event('change'))
    system.matches = true
    listeners.dispatchEvent(new Event('change'))
    expect(header.isDark.value).toBe(false)

    window.dispatchEvent(new StorageEvent('storage', { key: 'theme', newValue: 'dark' }))
    expect(header.isDark.value).toBe(true)
    expect(homepage.isDark.value).toBe(true)
    expect(document.documentElement.style.colorScheme).toBe('dark')
  })
})
