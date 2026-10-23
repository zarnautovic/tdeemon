import { queryOptions } from '@tanstack/react-query'

import { fetchClient } from './client'
import { HttpError } from './errors'

// sessionQuery is the signed-in user, or an HttpError 401 without a
// session (read from the answer's status, so an answer without a body, as
// a proxy's may be, still counts). The app's sections ask for it before
// they load (src/router.tsx), and take a minute-old answer as current, so
// moving between them does not wait for the server each time.
export const sessionQuery = queryOptions({
  queryKey: ['get', '/api/v1/auth/session'],
  queryFn: async ({ signal }) => {
    const { data, response } = await fetchClient.GET('/api/v1/auth/session', { signal })
    if (data) {
      return data
    }
    throw new HttpError(response.status)
  },
  staleTime: 60_000,
})

// maxTarget is the server's cap on return_to (internal/api/oidc.go).
const maxTarget = 1024

// safeTarget keeps a path on this app to go to after signing in, by the
// rules the server keeps return_to to (safeReturnTo in
// internal/api/oidc.go), and replaces anything else with the start page:
// another site ("//host", "/\host"), a backslash or a control character
// anywhere, which a browser may read as either, a target too long, and
// the sign-in page itself.
export function safeTarget(target: unknown): string {
  if (typeof target !== 'string' || !target.startsWith('/') || target.length > maxTarget) {
    return '/'
  }
  if (target.startsWith('//') || /^\/login(?:[/?#]|$)/i.test(target)) {
    return '/'
  }
  for (const c of target) {
    if (c === '\\' || c < ' ' || c === '\u007f') {
      return '/'
    }
  }
  return target
}
