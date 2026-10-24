import { Link } from '@tanstack/react-router'
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

// RepeatStatus says where a repeat stands: logged today, with what the
// server warned of it and the way to today from another day; or why not.
export function RepeatStatus({
  log,
  isToday,
}: {
  log: ReturnType<typeof useLogMeal>
  isToday: boolean
}) {
  const t = useMessages()
  const status = statusOf(log.error)
  if (log.isError && (status === 409 || status === 422)) {
    return (
      <p role="alert" className="text-xs text-destructive">
        {t.log.refused(detailOf(log.error))}
      </p>
    )
  }
  if (!log.isSuccess) {
    return log.isIdle ? null : <SaveStatus change={log} />
  }
  return (
    <output className="block text-sm">
      {t.meal.repeated}{' '}
      {!isToday && (
        <Link to="/" search={{}} className="underline underline-offset-2">
          {t.meal.seeToday}
        </Link>
      )}
      {log.data.warnings.length > 0 && (
        <ul className="text-amber-700 dark:text-amber-400">
          {log.data.warnings.map((w, i) => (
            <li key={`${w.code} ${String(i)}`}>{t.meal.warnings[w.code]}</li>
          ))}
        </ul>
      )}
    </output>
  )
}
