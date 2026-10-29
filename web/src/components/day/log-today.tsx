import { useQuery } from '@tanstack/react-query'
import type { RefObject } from 'react'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import { kcalLeft } from '@/components/day/left'
import { QuickAdd } from '@/components/day/quick-add'

// LogToday is the app's Log a meal: the meal to log (QuickAdd) on today,
// with what today's target leaves, as its ring says it, over whatever
// screen it was asked on, which stays as it was once the meal is logged.
// Today is asked for once it opens; a tap before today is known opens it
// once it is. opener is what opened it, which the focus goes back to.
export function LogToday({
  open,
  onOpenChange,
  opener,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  opener: RefObject<HTMLElement | null>
}) {
  const day = $api.useQuery(
    'get',
    '/api/v1/days/{date}',
    { params: { path: { date: 'today' } } },
    { enabled: open },
  )
  const me = $api.useQuery('get', '/api/v1/me', {}, { enabled: open })
  const session = useQuery(sessionQuery)
  // Today, as the day answered, else as the user's profile says.
  const date = day.data?.date ?? me.data?.today
  const target = day.data?.target

  if (date === undefined) {
    return null
  }
  return (
    <QuickAdd
      // Each day's meal of its own.
      key={date}
      open={open}
      onOpenChange={onOpenChange}
      opener={opener}
      remaining={
        target === undefined ? undefined : kcalLeft(day.data?.total.kcal ?? 0, target.kcal)
      }
      date={date}
      today
      timeZone={session.data?.user.timezone ?? 'UTC'}
      startHour={session.data?.user.day_start_hour ?? 0}
    />
  )
}
