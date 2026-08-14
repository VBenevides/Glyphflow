export type Theme = 'light' | 'dark'

export const THEME_KEY = 'glyphflow:theme'

export function resolveTheme(saved: string | null, prefersDark: boolean): Theme {
  return saved === 'dark' || (saved !== 'light' && prefersDark) ? 'dark' : 'light'
}

export function applyTheme(theme: Theme): void {
  document.documentElement.dataset.theme = theme
  window.localStorage.setItem(THEME_KEY, theme)
}

export function currentTheme(): Theme {
  return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light'
}
