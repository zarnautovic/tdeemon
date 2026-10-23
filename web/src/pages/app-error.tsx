import { useRouter, type ErrorComponentProps } from '@tanstack/react-router'

import { NoAnswerError, statusOf } from '@/api/errors'
import { Button } from '@/components/ui/button'
import type { Messages } from '@/i18n/en'
import { useMessages } from '@/i18n/messages'

// problem says what failed: no answer from the server, an answer that is
// an error, or the app itself.
function problem(t: Messages, error: unknown): string {
  if (error instanceof NoAnswerError) {
    return t.unreachable
  }
  return statusOf(error) === undefined ? t.failed : t.serverFailed
}

// AppError is what a page shows for an error it does not handle; mostly,
// that the server cannot say who is signed in, being down or failing.
export function AppError({ error }: ErrorComponentProps) {
  const t = useMessages()
  const router = useRouter()

  return (
    <main className="space-y-4">
      <p role="alert" className="text-destructive">
        {problem(t, error)}
      </p>
      <Button variant="outline" onClick={() => void router.invalidate()}>
        {t.retry}
      </Button>
    </main>
  )
}
