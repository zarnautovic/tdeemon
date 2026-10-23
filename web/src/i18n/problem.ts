import { NoAnswerError, statusOf } from '@/api/errors'

import type { Messages } from './en'

// problem says what failed: no answer from the server, the server, or
// the app itself (a bug, or a request the server refused).
export function problem(t: Messages, error: unknown): string {
  if (error instanceof NoAnswerError) {
    return t.unreachable
  }
  const status = statusOf(error)
  // A refusal (4xx) is the app's request, not the server failing.
  return status !== undefined && status >= 500 ? t.serverFailed : t.failed
}
