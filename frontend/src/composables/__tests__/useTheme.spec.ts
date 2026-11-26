import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { initTheme, useTheme } from '../useTheme'

describe('shared appearance', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    localStorage.removeItem('theme')
  })

  afterEach(() => {
    vi.useRealTimers()
    document.documentElement.classList.remove('dark')
    document.documentElement.style.removeProperty('color-scheme')
  })

  it('switches at 07:00 and 19:00 Beijing time without a reload', () => {
    vi.setSystemTime(new Date('2026-11-26T22:59:59Z'))
    localStorage.setItem('theme', 'light')
    initTheme()
    expect(useTheme().isDark.value).toBe(true)

    vi.advanceTimersByTime(1_000)
    expect(useTheme().isDark.value).toBe(false)
    vi.advanceTimersByTime(12 * 60 * 60 * 1_000)
    expect(useTheme().isDark.value).toBe(true)
  })

  it('shares a manual selection across controls and tabs until the next boundary', () => {
    vi.setSystemTime(new Date('2026-11-26T10:59:59Z'))
    initTheme()
    const header = useTheme()
    const homepage = useTheme()
    header.toggleTheme()
    expect(homepage.isDark.value).toBe(true)
    initTheme()
    expect(header.isDark.value).toBe(true)

    const otherTab = JSON.stringify({ theme: 'light', until: Date.parse('2026-11-26T11:00:00Z') })
    window.dispatchEvent(new StorageEvent('storage', { key: 'theme', newValue: otherTab }))
    expect(header.isDark.value).toBe(false)
    expect(document.documentElement.style.colorScheme).toBe('light')

    vi.advanceTimersByTime(1_000)
    expect(homepage.isDark.value).toBe(true)
    expect(document.documentElement.style.colorScheme).toBe('dark')
  })
})
