import { detailOf, statusOf } from '@/api/errors'
import type { useLogMeal } from '@/api/meals'
import { SaveStatus } from '@/components/day/save-status'
import { useMessages } from '@/i18n/messages'

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
