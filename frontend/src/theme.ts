export type Theme = 'light' | 'dark'

export const THEMES: Theme[] = ['light', 'dark']

export const THEME_KEY = 'glyphflow:theme'

export function resolveTheme(saved: string | null, prefersDark: boolean): Theme {
  if (saved && THEMES.includes(saved as Theme)) return saved as Theme
  return prefersDark ? 'dark' : 'light'
}

export function applyTheme(theme: Theme): void {
  if (!THEMES.includes(theme)) theme = 'light'
  document.documentElement.dataset.theme = theme
  window.localStorage.setItem(THEME_KEY, theme)
}

export function currentTheme(): Theme {
  const theme = document.documentElement.dataset.theme
  return THEMES.includes(theme as Theme) ? theme as Theme : 'light'
}
