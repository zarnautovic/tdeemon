// What the app's service worker (src/sw.ts) does with a notification the
// server pushes (internal/push): shows it, opens the app at its page when
// tapped, and subscribes again when the browser renews the subscription.
// It takes what it needs of the worker, so a test gives its own.

// skipsWaiting reports whether a message to the worker is the app's
// Reload of a new version (lib/service-worker.ts): SKIP_WAITING, from a
// page of the app's origin.
export function skipsWaiting(data: unknown, from: string, origin: string): boolean {
  return (
    from === origin &&
    typeof data === 'object' &&
    data !== null &&
    'type' in data &&
    data.type === 'SKIP_WAITING'
  )
}

// Note is what the server pushes: push.Notification.
export interface Note {
  title: string
  body: string
  url: string
  tag?: string | undefined
}

// noteOf reads a push's data; a push the server did not send (none, not
// JSON, not its shape) is shown as the app's name, as a browser requires
// a notification for each push.
export function noteOf(data: { json: () => unknown } | null): Note {
  let v: unknown
  try {
    v = data?.json()
  } catch {
    v = undefined
  }
  if (typeof v === 'object' && v !== null && 'title' in v && typeof v.title === 'string') {
    const body = 'body' in v && typeof v.body === 'string' ? v.body : ''
    // A path of the app's; the tap reads it again (urlOf).
    const url = 'url' in v && typeof v.url === 'string' && v.url.startsWith('/') ? v.url : '/'
    const tag = 'tag' in v && typeof v.tag === 'string' && v.tag !== '' ? v.tag : undefined
    return { title: v.title, body, url, tag }
  }
  return { title: 'tdeemon', body: '', url: '/' }
}

// ShowOptions are those of showNotification the app gives.
export interface ShowOptions {
  body: string
  tag?: string
  icon: string
  badge: string
  data: { url: string }
}

// showOptions are a note's options: its page goes with it, for a tap.
export function showOptions(n: Note): ShowOptions {
  return {
    body: n.body,
    ...(n.tag !== undefined && { tag: n.tag }),
    icon: '/pwa-192.png',
    badge: '/pwa-192.png',
    data: { url: n.url },
  }
}

// urlOf is the page a notification's tap opens, a path of the app at
// origin: its own, as the URL reads it, or the app's start for one that
// leads elsewhere (another site, or a path a browser reads as one, such
// as //site or /\site).
export function urlOf(data: unknown, origin: string): string {
  if (
    typeof data !== 'object' ||
    data === null ||
    !('url' in data) ||
    typeof data.url !== 'string'
  ) {
    return '/'
  }
  let u: URL
  try {
    u = new URL(data.url, origin)
  } catch {
    return '/'
  }
  return u.origin === origin ? u.pathname + u.search + u.hash : '/'
}

// WindowClient is what of an open window of the app a tap takes.
export interface WindowClient {
  url: string
  focus: () => Promise<unknown>
  navigate: (url: string) => Promise<unknown>
}

// Clients are what of the worker's clients a tap takes.
export interface Clients {
  matchAll: (options: {
    type: 'window'
    includeUncontrolled: boolean
  }) => Promise<readonly WindowClient[]>
  openWindow: (url: string) => Promise<unknown>
}

// open takes a tap to url in the app: a window of the app's already open
// goes there and comes to the front, or a new one opens, as for a window
// the worker does not control yet (the first one after it installed),
// which cannot be navigated.
export async function open(clients: Clients, origin: string, url: string): Promise<void> {
  const windows = await clients.matchAll({ type: 'window', includeUncontrolled: true })
  const app = windows.find((w) => new URL(w.url).origin === origin)
  if (app !== undefined) {
    try {
      await app.navigate(new URL(url, origin).href)
      await app.focus()
      return
    } catch {
      // A new window, below.
    }
  }
  await clients.openWindow(url)
}

// SubscriptionJSON is a subscription as PushSubscription.toJSON() gives
// it.
export interface SubscriptionJSON {
  endpoint?: string
  keys?: { p256dh?: string; auth?: string }
}

// bodyOf is a subscription as the server takes it (PUT
// /api/v1/push/subscriptions).
export function bodyOf(j: SubscriptionJSON): {
  endpoint: string
  keys: { p256dh: string; auth: string }
} {
  return {
    endpoint: j.endpoint ?? '',
    keys: { p256dh: j.keys?.p256dh ?? '', auth: j.keys?.auth ?? '' },
  }
}

// PushManager is what of the worker's push manager resubscribing takes.
export interface PushManager {
  subscribe: (options: {
    userVisibleOnly: true
    applicationServerKey: Uint8Array<ArrayBuffer>
  }) => Promise<{ toJSON: () => SubscriptionJSON }>
}

// keyBytes decodes the server's key, base64url, as subscribe takes it.
export function keyBytes(key: string): Uint8Array<ArrayBuffer> {
  const b64 = key.replaceAll('-', '+').replaceAll('_', '/')
  const raw = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4))
  const out = new Uint8Array(new ArrayBuffer(raw.length))
  for (let i = 0; i < raw.length; i++) {
    out[i] = raw.charCodeAt(i)
  }
  return out
}

// resubscribe subscribes again when the browser dropped or renewed the
// subscription (pushsubscriptionchange), with the instance's key, and
// tells the server, in the session of the user signed in: a device
// signed out, or a server down, subscribes again when the app opens.
export async function resubscribe(
  push: PushManager,
  fetcher: (input: string, init?: RequestInit) => Promise<Response>,
): Promise<void> {
  const keyAnswer = await fetcher('/api/v1/push/key')
  if (!keyAnswer.ok) {
    return
  }
  const answer: unknown = await keyAnswer.json()
  if (
    typeof answer !== 'object' ||
    answer === null ||
    !('public_key' in answer) ||
    typeof answer.public_key !== 'string'
  ) {
    return
  }
  const sub = await push.subscribe({
    userVisibleOnly: true,
    applicationServerKey: keyBytes(answer.public_key),
  })
  await fetcher('/api/v1/push/subscriptions', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(bodyOf(sub.toJSON())),
  })
}
