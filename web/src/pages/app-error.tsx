import { useRouter, type ErrorComponentProps } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { problem } from '@/i18n/problem'
import { narrowPage } from '@/lib/styles'

// AppError is what a page shows for an error it does not handle; mostly,
// that the server cannot say who is signed in, being down or failing.
export function AppError({ error }: ErrorComponentProps) {
  const t = useMessages()
  const router = useRouter()

  return (
    <main className={narrowPage}>
      <p role="alert" className="text-destructive">
        {problem(t, error)}
      </p>
      <Button variant="outline" onClick={() => void router.invalidate()}>
        {t.retry}
      </Button>
    </main>
  )
}
