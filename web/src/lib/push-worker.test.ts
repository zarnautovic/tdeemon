import { expect, test, vi } from 'vitest'

import {
  bodyOf,
  keyBytes,
  noteOf,
  open,
  resubscribe,
  showOptions,
  skipsWaiting,
  urlOf,
  type PushManager,
  type WindowClient,
} from './push-worker'

const data = (v: unknown) => ({ json: () => v })

test("a push's note is the server's, and anything else the app's name", () => {
  expect(
    noteOf(
      data({
        title: 'Morning summary',
        body: 'Yesterday 1980 kcal',
        url: '/?summary=2026-10-30',
        tag: 'morning',
      }),
    ),
  ).toEqual({
    title: 'Morning summary',
    body: 'Yesterday 1980 kcal',
    url: '/?summary=2026-10-30',
    tag: 'morning',
  })
  // A page of another site is no page of the app's.
  expect(noteOf(data({ title: 'x', url: 'https://example.com/' }))).toEqual({
    title: 'x',
    body: '',
    url: '/',
    tag: undefined,
  })
  for (const d of [null, data(42), data({ body: 'no title' })]) {
    expect(noteOf(d)).toEqual({ title: 'tdeemon', body: '', url: '/' })
  }
  expect(
    noteOf({
      json: () => {
        throw new SyntaxError('not JSON')
      },
    }),
  ).toEqual({ title: 'tdeemon', body: '', url: '/' })
})

test('a note shows with its page, for a tap', () => {
  expect(showOptions({ title: 't', body: 'b', url: '/settings', tag: 'test' })).toEqual({
    body: 'b',
    tag: 'test',
    icon: '/pwa-192.png',
    badge: '/pwa-192.png',
    data: { url: '/settings' },
  })
  expect(showOptions({ title: 't', body: 'b', url: '/' })).not.toHaveProperty('tag')
  const origin = 'https://tdeemon.example.com'
  expect(urlOf({ url: '/settings/devices' }, origin)).toBe('/settings/devices')
  expect(urlOf({ url: '/?summary=2026-10-30' }, origin)).toBe('/?summary=2026-10-30')
  expect(urlOf({ url: `${origin}/foods#x` }, origin)).toBe('/foods#x')
  // What a browser reads as another site.
  for (const d of [
    undefined,
    null,
    {},
    { url: '//evil.example/' },
    { url: '/\\evil.example/' },
    { url: '/\n/evil.example/' },
    { url: 'https://evil.example/' },
    { url: 'javascript:alert(1)' },
  ]) {
    expect(urlOf(d, origin)).toBe('/')
  }
})

function window(url: string): WindowClient {
  return {
    url,
    focus: vi.fn<() => Promise<unknown>>(() => Promise.resolve()),
    navigate: vi.fn<(url: string) => Promise<unknown>>(() => Promise.resolve()),
  }
}

test("a tap takes the app's window there, or opens one", async () => {
  const other = window('https://example.com/')
  const app = window('https://tdeemon.example.com/foods')
  const openWindow = vi.fn<(url: string) => Promise<unknown>>(() => Promise.resolve())
  await open(
    { matchAll: () => Promise.resolve([other, app]), openWindow },
    'https://tdeemon.example.com',
    '/?summary=2026-10-30',
  )
  expect(app.focus).toHaveBeenCalled()
  expect(app.navigate).toHaveBeenCalledWith('https://tdeemon.example.com/?summary=2026-10-30')
  expect(other.navigate).not.toHaveBeenCalled()
  expect(openWindow).not.toHaveBeenCalled()

  await open(
    { matchAll: () => Promise.resolve([other]), openWindow },
    'https://tdeemon.example.com',
    '/settings',
  )
  expect(openWindow).toHaveBeenCalledWith('/settings')

  // A window the worker does not control yet cannot be navigated.
  openWindow.mockClear()
  const uncontrolled = window('https://tdeemon.example.com/')
  uncontrolled.navigate = () => Promise.reject(new TypeError('not controlled'))
  await open(
    { matchAll: () => Promise.resolve([uncontrolled]), openWindow },
    'https://tdeemon.example.com',
    '/settings',
  )
  expect(openWindow).toHaveBeenCalledWith('/settings')
})

test("the server's key decodes to its bytes", () => {
  expect([...keyBytes('BAEC_-8')]).toEqual([4, 1, 2, 255, 239])
})

test('a renewed subscription is told to the server', async () => {
  const subscribe = vi.fn<PushManager['subscribe']>(() =>
    Promise.resolve({
      toJSON: () => ({
        endpoint: 'https://web.push.apple.com/x',
        keys: { p256dh: 'k', auth: 'a' },
        expirationTime: null,
      }),
    }),
  )
  const fetcher = vi.fn<(input: string, init?: RequestInit) => Promise<Response>>((input) =>
    Promise.resolve(
      input === '/api/v1/push/key' ? Response.json({ public_key: 'BAEC' }) : new Response(null),
    ),
  )
  await resubscribe({ subscribe }, fetcher)
  expect(subscribe).toHaveBeenCalledWith({
    userVisibleOnly: true,
    applicationServerKey: keyBytes('BAEC'),
  })
  expect(fetcher).toHaveBeenLastCalledWith('/api/v1/push/subscriptions', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      endpoint: 'https://web.push.apple.com/x',
      keys: { p256dh: 'k', auth: 'a' },
    }),
  })

  // Signed out, or a key that is none: nothing to subscribe with.
  subscribe.mockClear()
  await Promise.all(
    [new Response(null, { status: 401 }), Response.json({})].map((answer) =>
      resubscribe({ subscribe }, () => Promise.resolve(answer)),
    ),
  )
  expect(subscribe).not.toHaveBeenCalled()
  expect(bodyOf({})).toEqual({ endpoint: '', keys: { p256dh: '', auth: '' } })
})

test("the app's Reload is a message of its origin's alone", () => {
  const origin = 'https://tdeemon.example.com'
  expect(skipsWaiting({ type: 'SKIP_WAITING' }, origin, origin)).toBe(true)
  expect(skipsWaiting({ type: 'SKIP_WAITING' }, 'https://evil.example', origin)).toBe(false)
  expect(skipsWaiting({ type: 'SKIP_WAITING' }, '', origin)).toBe(false)
  for (const message of [null, 'SKIP_WAITING', { type: 'OTHER' }]) {
    expect(skipsWaiting(message, origin, origin)).toBe(false)
  }
})
