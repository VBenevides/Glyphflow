(() => {
  const key = 'glyphflow:theme'
  const saved = localStorage.getItem(key)
  const themes = ['light', 'dark', 'neon']
  const theme = themes.includes(saved)
    ? saved
    : matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  document.documentElement.dataset.theme = theme
})()
