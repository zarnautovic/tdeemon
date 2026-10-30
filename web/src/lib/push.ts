// This device's notifications by Web Push: whether the browser can take
// them, the subscription turned on (the permission asked for at a tap,
// as Safari asks), turned off, and told to the server again when the app
// opens, as iOS can drop one without a word.
import { fetchClient } from '@/api/client'
import { HttpError } from '@/api/errors'
import { bodyOf, keyBytes, type SubscriptionJSON } from '@/lib/push-worker'

// What of the browser's push this takes, so a test gives its own.
export interface Subscription {
  endpoint: string
  toJSON: () => SubscriptionJSON
  unsubscribe: () => Promise<boolean>
}

export interface Browser {
  // Undefined where the browser has no push: an origin without HTTPS, an
  // iPhone's Safari outside the app installed.
  push:
    | {
        getSubscription: () => Promise<Subscription | null>
        subscribe: (options: {
          userVisibleOnly: true
          applicationServerKey: Uint8Array<ArrayBuffer>
        }) => Promise<Subscription>
      }
    | undefined
  permission: () => NotificationPermission
  requestPermission: () => Promise<NotificationPermission>
  // iOS, where push needs the app installed on the home screen.
  ios: boolean
  standalone: boolean
  // Whether the user turned them off on this device, which the
  // permission given does not say: kept in the browser.
  turnedOff: () => boolean
  setTurnedOff: (off: boolean) => void
}

const offKey = 'tdeemon.push.off'

function thisBrowser(): Browser {
  const ios =
    /iPhone|iPad|iPod/.test(navigator.userAgent) ||
    // An iPad asks for the desktop site, as a Mac with a touch screen.
    (navigator.userAgent.includes('Macintosh') && navigator.maxTouchPoints > 1)
  const standalone =
    (typeof globalThis.matchMedia === 'function' &&
      globalThis.matchMedia('(display-mode: standalone)').matches) ||
    ('standalone' in navigator && navigator.standalone === true)
  const can =
    'serviceWorker' in navigator && 'PushManager' in globalThis && 'Notification' in globalThis
  return {
    push: can
      ? {
          // The registration there is, not ready's, which waits for one
          // forever where the worker did not register (a test's browser,
          // a policy's).
          getSubscription: async () =>
            (await navigator.serviceWorker.getRegistration())?.pushManager.getSubscription() ??
            null,
          subscribe: async (options) => {
            const reg = await navigator.serviceWorker.getRegistration()
            if (reg === undefined) {
              throw new Error('no service worker to subscribe with')
            }
            return reg.pushManager.subscribe(options)
          },
        }
      : undefined,
    permission: () => (can ? Notification.permission : 'denied'),
    requestPermission: () => Notification.requestPermission(),
    ios,
    standalone,
    // Storage that throws (a private window) forgets it: the device is
    // then as the permission says.
    turnedOff: () => {
      try {
        return localStorage.getItem(offKey) === '1'
      } catch {
        return false
      }
    },
    setTurnedOff: (off) => {
      try {
        if (off) {
          localStorage.setItem(offKey, '1')
        } else {
          localStorage.removeItem(offKey)
        }
      } catch {
        // Not kept; see turnedOff.
      }
    },
  }
}

// State is this device's notifications:
//   - unsupported: the browser has no push;
//   - install: iOS outside the app installed, which must be added to the
//     home screen first;
//   - blocked: the user refused them, which only the browser's settings
//     take back;
//   - off, or on under the server's id of this device.
export type State =
  | { kind: 'unsupported' }
  | { kind: 'install' }
  | { kind: 'blocked' }
  | { kind: 'off' }
  | { kind: 'on'; id: string }

// save tells the server of sub, and returns its id there.
async function save(sub: Subscription): Promise<string> {
  const { data, response } = await fetchClient.PUT('/api/v1/push/subscriptions', {
    body: bodyOf(sub.toJSON()),
  })
  if (data === undefined) {
    throw new HttpError(response.status)
  }
  return data.subscription.id
}

async function subscribeWith(push: NonNullable<Browser['push']>): Promise<Subscription> {
  const { data, response } = await fetchClient.GET('/api/v1/push/key')
  if (data === undefined) {
    throw new HttpError(response.status)
  }
  return push.subscribe({ userVisibleOnly: true, applicationServerKey: keyBytes(data.public_key) })
}

// sync reads this device's state, and when it is on tells the server of
// the subscription again: one the browser kept, or, the permission given
// and the subscription lost, a new one, unless the user turned them off
// here.
export async function sync(from = thisBrowser()): Promise<State> {
  if (from.push === undefined) {
    return from.ios && !from.standalone ? { kind: 'install' } : { kind: 'unsupported' }
  }
  switch (from.permission()) {
    case 'denied':
      return { kind: 'blocked' }
    case 'default':
      return { kind: 'off' }
    case 'granted':
      break
  }
  const kept = await from.push.getSubscription()
  if (kept === null && from.turnedOff()) {
    return { kind: 'off' }
  }
  const sub = kept ?? (await subscribeWith(from.push))
  // Turned off meanwhile, in another tab: the one just made goes.
  if (kept === null && from.turnedOff()) {
    await sub.unsubscribe()
    return { kind: 'off' }
  }
  return { kind: 'on', id: await save(sub) }
}

// turnOn asks for the permission, at the user's tap, and subscribes.
export async function turnOn(from = thisBrowser()): Promise<State> {
  if (from.push === undefined) {
    return sync(from)
  }
  const permission = await from.requestPermission()
  if (permission !== 'granted') {
    return permission === 'denied' ? { kind: 'blocked' } : { kind: 'off' }
  }
  from.setTurnedOff(false)
  return sync(from)
}

// turnOff unsubscribes this device, at the server first: a subscription
// the server keeps of a device that dropped it is deleted at its next
// push all the same.
export async function turnOff(id: string, from = thisBrowser()): Promise<State> {
  const { response } = await fetchClient.DELETE('/api/v1/push/subscriptions/{id}', {
    params: { path: { id } },
  })
  // One deleted already is gone all the same.
  if (!response.ok && response.status !== 404) {
    throw new HttpError(response.status)
  }
  from.setTurnedOff(true)
  await (await from.push?.getSubscription())?.unsubscribe()
  return { kind: 'off' }
}

// leave ends this device's notifications as its user signs out, so the
// next to sign in here gets none of theirs: at the server, in the
// session that is ending, and in the browser, the device turned off
// until someone turns it on. It never fails the sign-out: a subscription
// the server keeps of it is deleted at its next push.
export async function leave(state: State | undefined, from = thisBrowser()): Promise<void> {
  try {
    if (state?.kind === 'on') {
      await turnOff(state.id, from)
      return
    }
    from.setTurnedOff(true)
    await (await from.push?.getSubscription())?.unsubscribe()
  } catch {
    // See above.
  }
}
