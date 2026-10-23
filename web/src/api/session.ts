import { $api } from './client'

// sessionQuery is the signed-in user, or a 401 without a session. The
// app's sections ask for it before they load (src/router.tsx), and take
// a minute-old answer as current, so moving between them does not wait
// for the server each time.
export const sessionQuery = {
  ...$api.queryOptions('get', '/api/v1/auth/session'),
  staleTime: 60_000,
}

// isStatus tells whether a request failed with this HTTP status: the API
// answers an error as a problem (RFC 9457) with its status.
export function isStatus(error: unknown, status: number): boolean {
  return typeof error === 'object' && error !== null && 'status' in error && error.status === status
}

// safeTarget keeps a path on this app to go to after signing in, and
// replaces anything else (another site, "//host", the sign-in page
// itself) with the start page.
export function safeTarget(target: unknown): string {
  if (typeof target !== 'string' || !target.startsWith('/')) {
    return '/'
  }
  if (target.startsWith('//') || target.startsWith('/\\') || /^\/login(?:[/?#]|$)/.test(target)) {
    return '/'
  }
  return target
}
