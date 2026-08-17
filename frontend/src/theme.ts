export type Theme = 'light' | 'dark'

export const THEMES: Theme[] = ['light', 'dark']

export const THEME_KEY = 'glyphflow:theme'

function isTheme(value: string | null | undefined): value is Theme {
  return typeof value === 'string' && THEMES.includes(value as Theme)
}

export function resolveTheme(saved: string | null, prefersDark: boolean): Theme {
  if (isTheme(saved)) return saved
  return prefersDark ? 'dark' : 'light'
}

export function applyTheme(theme: Theme): void {
  const nextTheme = isTheme(theme) ? theme : 'light'
  document.documentElement.dataset.theme = nextTheme
  try {
    window.localStorage.setItem(THEME_KEY, nextTheme)
  } catch {
    // Private browsing can disable storage; the active theme still applies.
  }
}

export function currentTheme(): Theme {
  const theme = document.documentElement.dataset.theme
  return isTheme(theme) ? theme : 'light'
}
