// The app's service worker, which vite-plugin-pwa builds into /sw.js
// (injectManifest): it keeps the app's shell (its page, scripts, styles,
// fonts and icons) to open without a network, and never the data; a new
// version waits for the user's Reload (lib/service-worker.ts). It shows
// the notifications the server pushes (lib/push-worker.ts).
import {
  cleanupOutdatedCaches,
  createHandlerBoundToURL,
  precacheAndRoute,
} from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'

import { noteOf, open, resubscribe, showOptions, urlOf } from './lib/push-worker'

declare const self: ServiceWorkerGlobalScope

// oxlint-disable-next-line eslint/no-underscore-dangle -- Workbox's name, which the build replaces with the precache's list
precacheAndRoute(self.__WB_MANIFEST)
cleanupOutdatedCaches()
// A page of the app opens its shell; the API, MCP and the health check
// go to the server, and a sign-in's redirects with them.
registerRoute(
  new NavigationRoute(createHandlerBoundToURL('/index.html'), {
    denylist: [/^\/api\//, /^\/mcp/, /^\/healthz/],
  }),
)

// The app's Reload of a new version (lib/service-worker.ts).
self.addEventListener('message', (event) => {
  const data: unknown = event.data
  if (typeof data === 'object' && data !== null && 'type' in data && data.type === 'SKIP_WAITING') {
    void self.skipWaiting()
  }
})

self.addEventListener('push', (event) => {
  const note = noteOf(event.data)
  event.waitUntil(self.registration.showNotification(note.title, showOptions(note)))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  event.waitUntil(open(self.clients, self.location.origin, urlOf(event.notification.data)))
})

// Not in TypeScript's lib: Chrome and Firefox fire it when they renew a
// subscription, Safari not yet.
self.addEventListener('pushsubscriptionchange', (event) => {
  ;(event as ExtendableEvent).waitUntil(
    resubscribe(self.registration.pushManager, (input, init) => fetch(input, init)),
  )
})
