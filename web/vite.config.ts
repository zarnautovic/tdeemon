import { fileURLToPath } from 'node:url'
import babel from '@rolldown/plugin-babel'
import tailwindcss from '@tailwindcss/vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The Go server in development (go run ./cmd/tdeemon); the dev server
// forwards API calls to it, so the app always talks to its own origin.
const server = process.env.TDEEMON_DEV_SERVER ?? 'http://localhost:8080'

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
    // internal/webui caches files here for a year, as their names carry a
    // content hash; its assetsDir must match.
    assetsDir: 'assets',
  },
  test: {
    environment: 'jsdom',
    coverage: {
      provider: 'v8',
      include: ['src/**'],
      // Generated types, the entry point that mounts the app, and the
      // components copied from the shadcn registry.
      exclude: ['src/api/schema.d.ts', 'src/main.tsx', 'src/components/ui/**'],
      reporter: ['text-summary', 'lcov'],
      // No branch threshold: the React Compiler adds a cache branch to
      // every memoised value, which a test takes only one way.
      thresholds: { lines: 75, statements: 75, functions: 75 },
    },
  },
})
