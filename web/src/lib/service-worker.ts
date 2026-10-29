// The app's service worker, which vite.config.ts writes: it keeps the
// app's shell, so the app opens without a network, and a new version of
// it waits for the user to take it, so the page is never reloaded under
// their hands, a meal half typed.

// What of the browser's service workers this takes, so a test gives its
// own.
export interface AppWorker extends EventTarget {
  readonly state: ServiceWorkerState
  postMessage: (message: unknown) => void
}

export interface Registration extends EventTarget {
  readonly installing: AppWorker | null
  readonly waiting: AppWorker | null
  readonly active: AppWorker | null
  update: () => Promise<unknown>
}

export interface Browser {
  serviceWorker: { register: (url: string) => Promise<Registration> } | undefined
  reload: () => void
}

function thisBrowser(): Browser {
  return {
    // Undefined where the browser has none: an origin without HTTPS.
    serviceWorker: 'serviceWorker' in navigator ? navigator.serviceWorker : undefined,
    reload: () => {
      globalThis.location.reload()
    },
  }
}

// How often an app left open asks whether a new version is out, besides
// each time it comes back into view: a browser asks of itself only when a
// page loads, and an app on a phone brought back, or a tab left open, has
// no load.
export const checkEvery = 60 * 60 * 1000

// watchUpdates registers the worker once the page has loaded, so it takes
// nothing from the page's own first requests, and calls ready when a new
// version waits, found then or later, with what takes it. It returns what
// stops the watch.
export function watchUpdates(ready: (take: () => void) => void, from = thisBrowser()): () => void {
  const container = from.serviceWorker
  if (container === undefined) {
    return () => undefined
  }
  let registration: Registration | undefined
  let stopped = false

  const check = () => {
    // Without a network it asks again at the next check.
    registration?.update().catch(() => undefined)
  }
  const shown = () => {
    if (document.visibilityState === 'visible') {
      check()
    }
  }
  const register = async () => {
    let reg: Registration
    try {
      reg = await container.register('/sw.js')
    } catch {
      // A browser that refuses it (a policy, a test's) runs the app
      // without it.
      return
    }
    if (stopped) {
      return
    }
    registration = reg
    watch(reg, () => {
      if (!stopped) {
        ready(() => {
          take(reg, from.reload)
        })
      }
    })
  }
  const loaded = () => {
    void register()
  }

  if (document.readyState === 'complete') {
    loaded()
  } else {
    globalThis.addEventListener('load', loaded, { once: true })
  }
  const timer = setInterval(check, checkEvery)
  document.addEventListener('visibilitychange', shown)
  return () => {
    stopped = true
    globalThis.removeEventListener('load', loaded)
    clearInterval(timer)
    document.removeEventListener('visibilitychange', shown)
  }
}

// watch calls waits when a worker waits: one already, and each one
// installed beside an active one after. The first worker installed is
// active at once, and no new version.
function watch(reg: Registration, waits: () => void) {
  const installing = (sw: AppWorker | null) => {
    if (sw === null) {
      return
    }
    sw.addEventListener('statechange', () => {
      if (sw.state === 'installed' && reg.active !== null) {
        waits()
      }
    })
  }
  installing(reg.installing)
  reg.addEventListener('updatefound', () => {
    installing(reg.installing)
  })
  if (reg.waiting !== null) {
    waits()
  }
}

// take makes the worker that waits the active one, and then loads this
// page again, this page alone: another tab keeps its page and its toast,
// whose Reload then finds no worker waiting and loads its page at once.
function take(reg: Registration, reload: () => void) {
  const waiting = reg.waiting
  if (waiting === null) {
    reload()
    return
  }
  waiting.addEventListener('statechange', () => {
    if (waiting.state === 'activated') {
      reload()
    }
  })
  // The worker's own message (vite-plugin-pwa's prompt): it skips waiting.
  // oxlint-disable-next-line unicorn/require-post-message-target-origin -- a worker's postMessage takes no origin, a window's does
  waiting.postMessage({ type: 'SKIP_WAITING' })
}
