import { fetchClient } from './client'
import { HttpError } from './errors'
import type { components } from './schema'

export type Session = components['schemas']['SessionBody']

// parseRetryAfter reads Retry-After in seconds, the form the API sends.
function parseRetryAfter(header: string | null): number | undefined {
  if (header === null || !/^\d+$/.test(header)) {
    return undefined
  }
  return Number(header)
}

// signIn starts a web session with an email and a password; the answer
// sets the session cookie. A refusal is an HttpError, with Retry-After.
export async function signIn(body: components['schemas']['LoginInputBody']): Promise<Session> {
  const { data, response } = await fetchClient.POST('/api/v1/auth/login', { body })
  if (data) {
    return data
  }
  throw new HttpError(response.status, parseRetryAfter(response.headers.get('Retry-After')))
}
