import { beforeEach, describe, expect, it } from 'vitest'
import { THEME_KEY, applyTheme, currentTheme, resolveTheme } from './theme'

describe('theme', () => {
  beforeEach(() => {
    const values = new Map<string, string>()
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: {
        clear: () => values.clear(),
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => values.set(key, value),
      },
    })
    window.localStorage.clear()
    document.documentElement.dataset.theme = ''
  })

  it('resolves only supported saved themes before system preference', () => {
    expect(resolveTheme('light', true)).toBe('light')
    expect(resolveTheme('dark', false)).toBe('dark')
    expect(resolveTheme('neon', false)).toBe('light')
    expect(resolveTheme('neon', true)).toBe('dark')
    expect(resolveTheme(null, true)).toBe('dark')
    expect(resolveTheme('other', false)).toBe('light')
    expect(resolveTheme('other', true)).toBe('dark')
  })

  it('applies and persists every supported theme', () => {
    applyTheme('dark')
    expect(currentTheme()).toBe('dark')
    expect(window.localStorage.getItem(THEME_KEY)).toBe('dark')
    for (const theme of ['light'] as const) {
      applyTheme(theme)
      expect(currentTheme()).toBe(theme)
      expect(window.localStorage.getItem(THEME_KEY)).toBe(theme)
    }
  })

  it('keeps the active theme when storage is unavailable', () => {
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: { setItem: () => { throw new Error('storage disabled') } },
    })
    applyTheme('dark')
    expect(currentTheme()).toBe('dark')
  })
})
