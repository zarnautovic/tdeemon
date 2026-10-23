import { fetchClient } from './client'
import type { components } from './schema'

export type Session = components['schemas']['SessionBody']

// SignInError is a sign-in the server refused: its status, and for 429
// and 503 how many seconds to wait (Retry-After), which the hooks of
// openapi-react-query do not pass on.
export class SignInError extends Error {
  readonly status: number
  readonly retryAfter: number | undefined

  constructor(status: number, retryAfter: number | undefined) {
    super(`signing in failed with ${String(status)}`)
    this.name = 'SignInError'
    this.status = status
    this.retryAfter = retryAfter
  }
}

// parseRetryAfter reads Retry-After in seconds, the form the API sends.
function parseRetryAfter(header: string | null): number | undefined {
  if (header === null || !/^\d+$/.test(header)) {
    return undefined
  }
  return Number(header)
}

// signIn starts a web session with an email and a password; the answer
// sets the session cookie.
export async function signIn(body: components['schemas']['LoginInputBody']): Promise<Session> {
  const { data, response } = await fetchClient.POST('/api/v1/auth/login', { body })
  if (data) {
    return data
  }
  throw new SignInError(response.status, parseRetryAfter(response.headers.get('Retry-After')))
}
