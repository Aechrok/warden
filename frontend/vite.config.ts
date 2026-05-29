import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { readFileSync } from 'node:fs'

const pkg = JSON.parse(readFileSync('./package.json', 'utf-8'))

// In the dev container, API_TARGET is set to http://warden:8080.
// Outside a container (local npm dev), it falls back to localhost.
const apiTarget = process.env.API_TARGET ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version ?? '0.1.0'),
  },
  server: {
    host: '0.0.0.0',
    proxy: {
      '/api': apiTarget,
      '/auth': apiTarget,
    },
    watch: {
      usePolling: true,
      interval: 300,
    },
  },
})
