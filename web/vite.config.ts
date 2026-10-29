import { fileURLToPath } from 'node:url'
import babel from '@rolldown/plugin-babel'
import tailwindcss from '@tailwindcss/vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'
import { defineConfig } from 'vitest/config'

// The Go server in development (go run ./cmd/tdeemon); the dev server
// forwards API calls to it, so the app always talks to its own origin.
const server = process.env.TDEEMON_DEV_SERVER ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [
    react(),
    babel({ presets: [reactCompilerPreset()] }),
    tailwindcss(),
    // The app installed on a phone or a desktop: its manifest, and a
    // service worker that keeps the app's shell (its page, scripts,
    // styles, fonts and icons) to open without a network. The data are
    // never kept there: the API, MCP and the health check go to the
    // server, and a sign-in's redirects with them. The app registers the
    // worker itself (src/lib/service-worker.ts), and a new version waits
    // for the user's Reload (UpdatePrompt), not reloading the page under
    // them. web/icons/generate.sh draws the icons.
    VitePWA({
      registerType: 'prompt',
      injectRegister: false,
      manifest: {
        id: '/',
        name: 'tdeemon',
        short_name: 'tdeemon',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        // --background in src/index.css, light; index.html's theme-color
        // follows the theme chosen.
        background_color: '#f5f6f8',
        theme_color: '#f5f6f8',
        icons: [
          { src: '/pwa-192.png', sizes: '192x192', type: 'image/png' },
          { src: '/pwa-512.png', sizes: '512x512', type: 'image/png' },
          { src: '/maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      // The icons are among the PNGs below.
      includeManifestIcons: false,
      workbox: {
        globPatterns: ['**/*.{html,js,css,woff2,svg,png}'],
        // A file over it is left out of the precache with only a warning,
        // and the app would not open without a network: the main chunk,
        // about 1.2 MB, is near Workbox's 2 MiB.
        maximumFileSizeToCacheInBytes: 3 * 1024 * 1024,
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api\//, /^\/mcp/, /^\/healthz/],
        cleanupOutdatedCaches: true,
      },
    }),
  ],
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
