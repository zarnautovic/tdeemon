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
    // The end-to-end tests in e2e/ are Playwright's (npm run e2e).
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['src/test/setup.ts'],
    coverage: {
      provider: 'v8',
      include: ['src/**'],
      // Generated types, the entry point that mounts the app, the
      // components copied from the shadcn registry, and the tests' setup.
      exclude: ['src/api/schema.d.ts', 'src/main.tsx', 'src/components/ui/**', 'src/test/**'],
      reporter: ['text-summary', 'lcov'],
      // Lines and functions only: the React Compiler adds a cache branch,
      // and the statements around it, to every memoised value, and a test
      // takes it only one way.
      thresholds: { lines: 75, functions: 75 },
    },
  },
})
