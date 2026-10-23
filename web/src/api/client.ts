import createFetchClient from 'openapi-fetch'
import createClient from 'openapi-react-query'

import { NoAnswerError } from './errors'
import type { paths } from './schema'

// Requests go to the app's own origin: the Go server serves the app in
// production, and the Vite dev server forwards /api to it in development.
// The origin is explicit because openapi-fetch builds a Request, which
// outside a browser (tests) does not resolve a relative URL. fetch is
// looked up on every call, not captured once, so tests can stub it.
export const fetchClient = createFetchClient<paths>({
  baseUrl: globalThis.location.origin,
  fetch: (request) => globalThis.fetch(request),
})

// A request that got no answer fails with a NoAnswerError; one cancelled
// (a query no longer needed) keeps its AbortError.
fetchClient.use({
  onError: ({ error }) =>
    error instanceof DOMException && error.name === 'AbortError'
      ? undefined
      : new NoAnswerError(error),
})

// TanStack Query hooks typed by the OpenAPI document, e.g.
// $api.useQuery('get', '/api/v1/info').
export const $api = createClient(fetchClient)
