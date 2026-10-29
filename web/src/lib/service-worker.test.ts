import { afterEach, expect, test, vi } from 'vitest'

import {
  checkEvery,
  watchUpdates,
  type AppWorker,
  type Browser,
  type Registration,
} from '@/lib/service-worker'

// A service worker and its registration as the browser gives them, whose
// state a test moves on.
class FakeWorker extends EventTarget implements AppWorker {
  state: ServiceWorkerState
  postMessage = vi.fn<(message: unknown) => void>()

  constructor(state: ServiceWorkerState) {
    super()
    this.state = state
  }

  become(state: ServiceWorkerState) {
    this.state = state
    this.dispatchEvent(new Event('statechange'))
  }
}

class FakeRegistration extends EventTarget implements Registration {
  installing: FakeWorker | null = null
  waiting: FakeWorker | null = null
  active: FakeWorker | null = null
  update = vi.fn<() => Promise<unknown>>(() => Promise.resolve())
}

function browserOf(reg: FakeRegistration) {
  return {
    serviceWorker: {
      register: vi.fn<(url: string) => Promise<Registration>>(() => Promise.resolve(reg)),
    },
    reload: vi.fn<() => void>(),
  } satisfies Browser
}

// The worker registers once a promise settles.
async function settled() {
  await new Promise((resolve) => {
    setTimeout(resolve, 0)
  })
}

afterEach(() => {
  vi.useRealTimers()
})

test('a new version that waits as the page loads is said, and its Reload loads the page once it is active', async () => {
  const reg = new FakeRegistration()
  reg.active = new FakeWorker('activated')
  const waiting = new FakeWorker('installed')
  reg.waiting = waiting
  const browser = browserOf(reg)
  const takes: (() => void)[] = []

  const stop = watchUpdates((take) => takes.push(take), browser)
  await settled()
  expect(browser.serviceWorker.register).toHaveBeenCalledWith('/sw.js')
  expect(takes).toHaveLength(1)

  takes[0]?.()
  expect(waiting.postMessage).toHaveBeenCalledWith({ type: 'SKIP_WAITING' })
  waiting.become('activating')
  expect(browser.reload).not.toHaveBeenCalled()
  waiting.become('activated')
  expect(browser.reload).toHaveBeenCalledOnce()
  stop()
})

test('the first worker installed is no new version, and one installed beside it is', async () => {
  const reg = new FakeRegistration()
  const first = new FakeWorker('installing')
  reg.installing = first
  const ready = vi.fn<(take: () => void) => void>()

  const stop = watchUpdates(ready, browserOf(reg))
  await settled()
  first.become('installed')
  reg.installing = null
  reg.active = first
  first.become('activated')
  expect(ready).not.toHaveBeenCalled()

  const next = new FakeWorker('installing')
  reg.installing = next
  reg.dispatchEvent(new Event('updatefound'))
  reg.installing = null
  reg.waiting = next
  next.become('installed')
  expect(ready).toHaveBeenCalledOnce()
  stop()
})

test('a Reload after another tab took the new version loads the page at once', async () => {
  const reg = new FakeRegistration()
  reg.active = new FakeWorker('activated')
  const waiting = new FakeWorker('installed')
  reg.waiting = waiting
  const browser = browserOf(reg)
  const takes: (() => void)[] = []

  const stop = watchUpdates((take) => takes.push(take), browser)
  await settled()
  reg.waiting = null
  reg.active = waiting
  expect(takes).toHaveLength(1)
  takes[0]?.()
  expect(waiting.postMessage).not.toHaveBeenCalled()
  expect(browser.reload).toHaveBeenCalledOnce()
  stop()
})

test('an app left open asks for a new version every hour and each time it is shown, until stopped', async () => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
  const reg = new FakeRegistration()
  // Without a network: the next check asks again.
  reg.update.mockRejectedValue(new Error('offline'))

  const stop = watchUpdates(() => undefined, browserOf(reg))
  await settled()
  expect(reg.update).not.toHaveBeenCalled()
  vi.advanceTimersByTime(checkEvery)
  expect(reg.update).toHaveBeenCalledTimes(1)
  document.dispatchEvent(new Event('visibilitychange'))
  expect(reg.update).toHaveBeenCalledTimes(2)

  stop()
  vi.advanceTimersByTime(checkEvery)
  document.dispatchEvent(new Event('visibilitychange'))
  expect(reg.update).toHaveBeenCalledTimes(2)
})

test('without service workers, or with the worker refused, the app runs without one', async () => {
  const ready = vi.fn<(take: () => void) => void>()
  watchUpdates(ready, { serviceWorker: undefined, reload: vi.fn<() => void>() })()

  const register = vi.fn<(url: string) => Promise<Registration>>(() =>
    Promise.reject(new Error('blocked')),
  )
  const stop = watchUpdates(ready, { serviceWorker: { register }, reload: vi.fn<() => void>() })
  await settled()
  expect(register).toHaveBeenCalledOnce()
  expect(ready).not.toHaveBeenCalled()
  stop()
})

test('stopped before the worker registers, it says nothing of it', async () => {
  const reg = new FakeRegistration()
  reg.active = new FakeWorker('activated')
  reg.waiting = new FakeWorker('installed')
  const ready = vi.fn<(take: () => void) => void>()

  watchUpdates(ready, browserOf(reg))()
  await settled()
  expect(ready).not.toHaveBeenCalled()
})
