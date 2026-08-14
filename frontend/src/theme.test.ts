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

  it('resolves saved themes before system preference', () => {
    expect(resolveTheme('light', true)).toBe('light')
    expect(resolveTheme('dark', false)).toBe('dark')
    expect(resolveTheme(null, true)).toBe('dark')
    expect(resolveTheme('other', false)).toBe('light')
  })

  it('applies and persists both supported themes', () => {
    applyTheme('dark')
    expect(currentTheme()).toBe('dark')
    expect(window.localStorage.getItem(THEME_KEY)).toBe('dark')
    applyTheme('light')
    expect(currentTheme()).toBe('light')
    expect(window.localStorage.getItem(THEME_KEY)).toBe('light')
  })
})
