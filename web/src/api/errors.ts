// HttpError is an answer of the API that is not a success, by its HTTP
// status, and for 429 and 503 how many seconds to wait (Retry-After).
// What openapi-fetch gives as the error is the answer's body, which an
// answer from a proxy may lack, and openapi-react-query's hooks pass on
// nothing else; a request whose status matters reads the response and
// throws this instead.
export class HttpError extends Error {
  readonly status: number
  readonly retryAfter: number | undefined

  constructor(status: number, retryAfter?: number) {
    super(`the server answered ${String(status)}`)
    this.name = 'HttpError'
    this.status = status
    this.retryAfter = retryAfter
  }
}

// NoAnswerError is a request that got no answer: the server is down, or
// out of reach. fetchClient throws it for what fetch throws, so it is not
// taken for a bug of the app, which is often a TypeError too.
export class NoAnswerError extends Error {
  constructor(cause: unknown) {
    super('the server did not answer', { cause })
    this.name = 'NoAnswerError'
  }
}

// statusOf is the HTTP status a request failed with: an HttpError's, or
// the status of the API's problem (RFC 9457) that openapi-react-query
// throws; undefined for a request with no answer, or anything else.
export function statusOf(error: unknown): number | undefined {
  if (error instanceof HttpError) {
    return error.status
  }
  if (typeof error === 'object' && error !== null && 'status' in error) {
    return typeof error.status === 'number' ? error.status : undefined
  }
  return undefined
}

// retry asks a query again after a failure that may pass, once: no
// answer, or a server error. A refusal (4xx) would be answered the same.
export function retry(failures: number, error: unknown): boolean {
  if (failures >= 1) {
    return false
  }
  if (error instanceof NoAnswerError) {
    return true
  }
  const status = statusOf(error)
  return status !== undefined && status >= 500
}

// detailOf is what the API's problem (RFC 9457) says of a refusal, in
// the server's words, with what its errors say of each field: a request
// the schema refuses says only "validation failed" otherwise. Empty for
// anything else.
export function detailOf(error: unknown): string {
  if (
    typeof error !== 'object' ||
    error === null ||
    !('detail' in error) ||
    typeof error.detail !== 'string'
  ) {
    return ''
  }
  const errors: unknown[] = 'errors' in error && Array.isArray(error.errors) ? error.errors : []
  const said = errors.flatMap((e) => {
    if (typeof e !== 'object' || e === null || !('message' in e) || typeof e.message !== 'string') {
      return []
    }
    const at = 'location' in e && typeof e.location === 'string' ? e.location : ''
    return [at ? `${at.replace(/^body\./, '')}: ${e.message}` : e.message]
  })
  return said.length > 0 ? `${error.detail}: ${said.join('; ')}` : error.detail
}
