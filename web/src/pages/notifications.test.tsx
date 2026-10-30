import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { fakeServer, renderAt, signedIn } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  Reflect.deleteProperty(navigator, 'serviceWorker')
  localStorage.clear()
  setLocale('en')
})

type Device = components['schemas']['PushSubscriptionBody']

// withPush gives the browser push, its permission not yet asked for,
// which a tap grants.
function withPush() {
  const unsubscribe = vi.fn<() => Promise<boolean>>(() => Promise.resolve(true))
  let kept: { endpoint: string; toJSON: () => unknown; unsubscribe: typeof unsubscribe } | null =
    null
  const pushManager = {
    getSubscription: () => Promise.resolve(kept),
    subscribe: () => {
      kept = {
        endpoint: 'https://fcm.googleapis.com/fcm/send/this',
        toJSON: () => ({
          endpoint: 'https://fcm.googleapis.com/fcm/send/this',
          keys: { p256dh: 'k', auth: 'a' },
        }),
        unsubscribe,
      }
      return Promise.resolve(kept)
    },
  }
  Object.defineProperty(navigator, 'serviceWorker', {
    configurable: true,
    value: { getRegistration: () => Promise.resolve({ pushManager }) },
  })
  // Only that there is one counts.
  vi.stubGlobal('PushManager', Object)
  const notification = {
    permission: 'default' as NotificationPermission,
    requestPermission: () => {
      notification.permission = 'granted'
      return Promise.resolve(notification.permission)
    },
  }
  vi.stubGlobal('Notification', notification)
  return { unsubscribe }
}

// notifyServer keeps the user's devices as the server does.
function notifyServer(devices: Device[] = [], locale?: 'hr') {
  let list = [...devices]
  const tests: number[] = []
  fakeServer({
    ...signedIn(locale),
    'GET /api/v1/push/key': () => Response.json({ public_key: 'BAEC' }),
    'GET /api/v1/push/subscriptions': () => Response.json({ subscriptions: list }),
    'PUT /api/v1/push/subscriptions': () => {
      const d: Device = {
        id: 'this-device',
        device: 'Mac · Chrome',
        created_at: '2026-10-30T08:00:00Z',
      }
      list = [d, ...list.filter((x) => x.id !== d.id)]
      return Response.json({ subscription: d })
    },
    'DELETE /api/v1/push/subscriptions/this-device': () => {
      list = list.filter((x) => x.id !== 'this-device')
      return new Response(null, { status: 204 })
    },
    'POST /api/v1/push/test': () => {
      tests.push(list.length)
      return Response.json({ sent: list.length, gone: 0, failed: 0 })
    },
  })
  return { tests }
}

const phone: Device = {
  id: 'phone',
  device: 'iPhone · Safari',
  created_at: '2026-10-29T08:00:00Z',
  last_sent_at: '2026-10-30T17:00:00Z',
}

test('a browser without push says so; the devices notified are listed all the same', async () => {
  notifyServer([phone])
  renderAt('/settings/notifications')

  expect(await screen.findByRole('heading', { name: 'Notifications' })).toBeTruthy()
  expect(await screen.findByText('This browser cannot get notifications.')).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Turn on' })).toBeNull()
  const card = (await screen.findByText('iPhone · Safari')).closest('li')
  // The user's clock, Los Angeles'.
  expect(card?.textContent).toContain('Last notified')
  expect(card?.textContent).toContain('10:00')
})

test('turned on at a tap, this device is listed as such, gets the test, and is turned off', async () => {
  const { unsubscribe } = withPush()
  const server = notifyServer([phone])
  renderAt('/settings/notifications')

  fireEvent.click(await screen.findByRole('button', { name: 'Turn on' }))
  expect(await screen.findByText('On')).toBeTruthy()
  const mine = (await screen.findByText('Mac · Chrome')).closest('li')
  expect(mine && within(mine).getByText('This device')).toBeTruthy()

  fireEvent.click(screen.getByRole('button', { name: 'Send a test' }))
  expect(await screen.findByText('Sent to 2 devices.')).toBeTruthy()
  expect(server.tests).toEqual([2])

  fireEvent.click(screen.getByRole('button', { name: 'Turn off' }))
  expect(await screen.findByText('Off')).toBeTruthy()
  expect(unsubscribe).toHaveBeenCalled()
  expect(screen.queryByText('Mac · Chrome')).toBeNull()
})

test('this device removed from the list is off here too, and stays so, in Croatian', async () => {
  const { unsubscribe } = withPush()
  notifyServer([], 'hr')
  renderAt('/settings/notifications')

  fireEvent.click(await screen.findByRole('button', { name: 'Uključi' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Prestani slati obavijesti na Mac · Chrome' }),
  )
  expect(await screen.findByText('Isključene')).toBeTruthy()
  expect(await screen.findByText('Još ih nijedan uređaj ne prima.')).toBeTruthy()
  expect(unsubscribe).toHaveBeenCalled()
  expect(localStorage.getItem('tdeemon.push.off')).toBe('1')
})

test('signing out turns this device off at the server first', async () => {
  const { unsubscribe } = withPush()
  const calls: string[] = []
  notifyServer()
  const fetch = globalThis.fetch
  vi.stubGlobal('fetch', async (input: Request) => {
    calls.push(`${input.method} ${new URL(input.url).pathname}`)
    return new URL(input.url).pathname === '/api/v1/auth/logout'
      ? new Response(null, { status: 204 })
      : fetch(input)
  })
  renderAt('/settings/notifications')
  fireEvent.click(await screen.findByRole('button', { name: 'Turn on' }))
  expect(await screen.findByText('On')).toBeTruthy()

  renderAt('/settings')
  fireEvent.click(await screen.findByRole('button', { name: /Sign out/ }))
  await waitFor(() => {
    expect(calls).toContain('POST /api/v1/auth/logout')
  })
  expect(calls.indexOf('DELETE /api/v1/push/subscriptions/this-device')).toBeGreaterThan(-1)
  expect(calls.indexOf('DELETE /api/v1/push/subscriptions/this-device')).toBeLessThan(
    calls.indexOf('POST /api/v1/auth/logout'),
  )
  expect(unsubscribe).toHaveBeenCalled()
})

test("the settings' row says how many devices get them", async () => {
  notifyServer([phone])
  renderAt('/settings')
  const row = await screen.findByRole('link', { name: /Notifications/ })
  expect(await within(row).findByText('1 device')).toBeTruthy()
})
