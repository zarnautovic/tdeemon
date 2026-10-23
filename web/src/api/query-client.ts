import { QueryClient } from '@tanstack/react-query'

import { retry } from './errors'

// createQueryClient is the app's: a query is asked again only when that
// may help (retry), so a refusal or a server that is down shows at once.
export function createQueryClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retry } } })
}
