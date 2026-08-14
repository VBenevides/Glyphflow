(() => {
  const key = 'glyphflow:theme'
  const saved = localStorage.getItem(key)
  const dark = saved === 'dark' || (!saved && matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
})()
