(() => {
  const key = 'glyphflow:theme'
  const themes = ['light', 'dark']
  let saved = null
  try {
    saved = localStorage.getItem(key)
  } catch {
    // Continue with the system preference when storage is unavailable.
  }
  const systemTheme = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  const theme = themes.includes(saved) ? saved : systemTheme
  document.documentElement.dataset.theme = theme
})()
