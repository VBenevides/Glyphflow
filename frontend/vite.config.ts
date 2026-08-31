import { readFileSync } from 'node:fs'
import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

const version = readFileSync(new URL('../VERSION', import.meta.url), 'utf8').trim()

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  return {
    plugins: [react()],
    define: { 'import.meta.env.VITE_APP_VERSION': JSON.stringify(version) },
    server: { proxy: { '/api': { target: process.env.VITE_BACKEND_URL ?? env.VITE_BACKEND_URL ?? 'http://localhost:8080', changeOrigin: true } } },
  }
})
