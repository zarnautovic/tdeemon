import { afterEach, expect, test, vi } from 'vitest'

import { fakeServer, problem } from '@/test/app'

import { leave, sync, turnOff, turnOn, type Browser, type Subscription } from './push'

afterEach(() => {
  vi.unstubAllGlobals()
})

function subscription(endpoint = 'https://web.push.apple.com/kept'): Subscription {
  return {
    endpoint,
    toJSON: () => ({ endpoint, keys: { p256dh: 'k', auth: 'a' } }),
    unsubscribe: vi.fn<() => Promise<boolean>>(() => Promise.resolve(true)),
  }
}

// browser is a browser with push whose permission and subscription are
// given; over is laid over it.
function browser(
  permission: NotificationPermission,
  kept: Subscription | null,
  over: Partial<Browser> = {},
): Browser & { off: boolean } {
  const b: Browser & { off: boolean } = {
    off: false,
    push: {
      getSubscription: () => Promise.resolve(kept),
      subscribe: vi.fn<NonNullable<Browser['push']>['subscribe']>(() =>
        Promise.resolve(subscription('https://fcm.googleapis.com/fcm/send/new')),
      ),
    },
    permission: () => permission,
    requestPermission: () => Promise.resolve(permission),
    ios: false,
    standalone: false,
    turnedOff: () => b.off,
    setTurnedOff: (off: boolean) => {
      b.off = off
    },
    ...over,
  }
  return b
}

// pushServer keeps what was put, and answers the id of this device.
function pushServer() {
  const put: unknown[] = []
  const deleted: string[] = []
  fakeServer({
    'GET /api/v1/push/key': () => Response.json({ public_key: 'BAEC' }),
    'PUT /api/v1/push/subscriptions': async (r) => {
      put.push(await r.json())
      return Response.json({
        subscription: {
          id: 'this-device',
          device: 'Mac · Chrome',
          created_at: '2026-10-30T08:00:00Z',
        },
      })
    },
    'DELETE /api/v1/push/subscriptions/this-device': () => {
      deleted.push('this-device')
      return new Response(null, { status: 204 })
    },
    'DELETE /api/v1/push/subscriptions/gone': () => problem(404),
    'DELETE /api/v1/push/subscriptions/down': () => problem(503),
  })
  return { put, deleted }
}

test('a browser without push cannot have them, and an iPhone needs the app installed', async () => {
  const none = { push: undefined, ios: false, standalone: false }
  expect(await sync({ ...browser('default', null), ...none })).toEqual({ kind: 'unsupported' })
  expect(await sync({ ...browser('default', null), ...none, ios: true })).toEqual({
    kind: 'install',
  })
  expect(await sync({ ...browser('default', null), ...none, ios: true, standalone: true })).toEqual(
    {
      kind: 'unsupported',
    },
  )
  expect(await sync(browser('denied', null))).toEqual({ kind: 'blocked' })
  expect(await sync(browser('default', null))).toEqual({ kind: 'off' })
})

test('a subscription kept is told to the server again, and one lost made anew', async () => {
  const server = pushServer()
  expect(await sync(browser('granted', subscription()))).toEqual({ kind: 'on', id: 'this-device' })
  expect(server.put).toEqual([
    { endpoint: 'https://web.push.apple.com/kept', keys: { p256dh: 'k', auth: 'a' } },
  ])

  const lost = browser('granted', null)
  expect(await sync(lost)).toEqual({ kind: 'on', id: 'this-device' })
  expect(lost.push?.subscribe).toHaveBeenCalledWith({
    userVisibleOnly: true,
    applicationServerKey: new Uint8Array([4, 1, 2]),
  })

  // Turned off here: the permission stays, the subscription does not
  // come back.
  const off = browser('granted', null)
  off.off = true
  expect(await sync(off)).toEqual({ kind: 'off' })
  expect(off.push?.subscribe).not.toHaveBeenCalled()
})

test('turned on at a tap, as the permission is answered; turned off, at the server first', async () => {
  const server = pushServer()
  expect(await turnOn(browser('denied', null))).toEqual({ kind: 'blocked' })
  expect(await turnOn(browser('default', null))).toEqual({ kind: 'off' })
  const b = browser('granted', null)
  b.off = true
  expect(await turnOn(b)).toEqual({ kind: 'on', id: 'this-device' })
  expect(b.off).toBe(false)

  const kept = subscription()
  const on = browser('granted', kept)
  expect(await turnOff('this-device', on)).toEqual({ kind: 'off' })
  expect(server.deleted).toEqual(['this-device'])
  expect(kept.unsubscribe).toHaveBeenCalled()
  expect(on.off).toBe(true)
  // Deleted elsewhere already: off all the same; a server down: not.
  expect(await turnOff('gone', browser('granted', subscription()))).toEqual({ kind: 'off' })
  await expect(turnOff('down', browser('granted', subscription()))).rejects.toThrow('503')
})

test('a server that refuses the key or the subscription fails the sync', async () => {
  fakeServer({ 'GET /api/v1/push/key': () => problem(503) })
  await expect(sync(browser('granted', null))).rejects.toThrow('503')
  fakeServer({ 'PUT /api/v1/push/subscriptions': () => problem(422) })
  await expect(sync(browser('granted', subscription()))).rejects.toThrow('422')
})

test('turned off in another tab while one was made, the one made goes', async () => {
  const server = pushServer()
  const made = subscription('https://fcm.googleapis.com/fcm/send/new')
  const b = browser('granted', null)
  b.push = {
    getSubscription: () => Promise.resolve(null),
    subscribe: () => {
      b.off = true
      return Promise.resolve(made)
    },
  }
  expect(await sync(b)).toEqual({ kind: 'off' })
  expect(made.unsubscribe).toHaveBeenCalled()
  expect(server.put).toEqual([])
})

test("signing out ends this device's notifications, and never fails", async () => {
  const server = pushServer()
  const kept = subscription()
  const on = browser('granted', kept)
  await leave({ kind: 'on', id: 'this-device' }, on)
  expect(server.deleted).toEqual(['this-device'])
  expect(kept.unsubscribe).toHaveBeenCalled()
  expect(on.off).toBe(true)

  // Off already, the browser's subscription goes all the same.
  const stray = subscription()
  const off = browser('granted', stray)
  await leave({ kind: 'off' }, off)
  expect(stray.unsubscribe).toHaveBeenCalled()
  expect(off.off).toBe(true)
  await expect(
    leave({ kind: 'on', id: 'down' }, browser('granted', subscription())),
  ).resolves.toBeUndefined()
})
