import { useRouter, type ErrorComponentProps } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// AppError is what the app's sections show when the server cannot say
// who is signed in: it is down or unreachable, or failed.
export function AppError({ error }: ErrorComponentProps) {
  const t = useMessages()
  const router = useRouter()

  return (
    <section className="space-y-4">
      <p role="alert" className="text-destructive">
        {error instanceof TypeError ? t.unreachable : t.serverFailed}
      </p>
      <Button variant="outline" onClick={() => void router.invalidate()}>
        {t.retry}
      </Button>
    </section>
  )
}
