(() => {
  const key = 'glyphflow:theme'
  const themes = ['light', 'dark']
  let saved = null
  try {
    saved = localStorage.getItem(key)
  } catch {
    // Continue with the system preference when storage is unavailable.
  }
  const theme = themes.includes(saved)
    ? saved
    : matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  document.documentElement.dataset.theme = theme
})()
