import { Repeat2 } from 'lucide-react'

import { detailOf, statusOf } from '@/api/errors'
import type { useLogMeal } from '@/api/meals'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// RepeatButton is the button that repeats a meal, today at this time
// (repeatOf).
export function RepeatButton({
  label,
  pending,
  onClick,
}: {
  label: string
  pending: boolean
  onClick: () => void
}) {
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={label}
      disabled={pending}
      className="self-center"
      onClick={onClick}
    >
      <Repeat2 aria-hidden />
    </Button>
  )
}

// RepeatStatus says why a repeat was not logged: the server refused it,
// or the way to send it again. One logged says so in a toast.
export function RepeatStatus({ log }: { log: ReturnType<typeof useLogMeal> }) {
  const t = useMessages()
  const status = statusOf(log.error)
  if (log.isError && (status === 409 || status === 422)) {
    return (
      <p role="alert" className="text-xs text-destructive">
        {t.log.refused(detailOf(log.error))}
      </p>
    )
  }
  return <SaveStatus change={log} />
}
