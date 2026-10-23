import { NoAnswerError, statusOf } from '@/api/errors'

import type { Messages } from './en'

// problem says what failed: no answer from the server, an answer that is
// an error, or the app itself.
export function problem(t: Messages, error: unknown): string {
  if (error instanceof NoAnswerError) {
    return t.unreachable
  }
  return statusOf(error) === undefined ? t.failed : t.serverFailed
}
