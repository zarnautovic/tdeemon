import { fileURLToPath } from 'node:url'
import babel from '@rolldown/plugin-babel'
import tailwindcss from '@tailwindcss/vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The Go server in development (go run ./cmd/tdeemon); the dev server
// forwards API calls to it, so the app always talks to its own origin.
const server = process.env['TDEEMON_DEV_SERVER'] ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react(), babel({ presets: [reactCompilerPreset()] }), tailwindcss()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    proxy: { '/api': server, '/healthz': server },
  },
  build: {
    // internal/webui embeds this directory (go build -tags webui).
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
  },
})
