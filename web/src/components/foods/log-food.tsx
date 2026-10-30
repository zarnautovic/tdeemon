import { useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useRef, useState } from 'react'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import type { Food } from '@/api/types'
import { lastAmountOf } from '@/components/day/meal-entries'
import { QuickAdd } from '@/components/day/quick-add'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// LogFood logs a food today: it opens the meal to log (QuickAdd) with the
// food in it, in the amount it was eaten in last (Log 200 g), as the
// day's chips do, with what today has left. amount says that amount in
// other words than a food's (a recipe's share).
export function LogFood({
  food,
  amount: said,
  className,
}: {
  food: Food
  amount?: string | undefined
  className?: string
}) {
  const t = useMessages()
  const f = useFormat()
  const [open, setOpen] = useState(false)
  const button = useRef<HTMLButtonElement>(null)
  const day = $api.useQuery('get', '/api/v1/days/{date}', { params: { path: { date: 'today' } } })
  const me = $api.useQuery('get', '/api/v1/me')
  const session = useQuery(sessionQuery)
  // Today, as the day answered, else as the user's profile says: a tap
  // before either came opens the meal once one has.
  const date = day.data?.date ?? me.data?.today
  const amount = said ?? lastAmountOf(food, t, f)

  return (
    <>
      <Button
        ref={button}
        className={className}
        onClick={() => {
          setOpen(true)
        }}
      >
        <Plus aria-hidden />
        {amount === undefined ? t.foods.log : t.foods.logAmount(amount)}
      </Button>
      {date !== undefined && (
        <QuickAdd
          key={date}
          open={open}
          onOpenChange={setOpen}
          initial={food}
          opener={button}
          remaining={day.data?.remaining?.kcal}
          date={date}
          today
          timeZone={session.data?.user.timezone ?? 'UTC'}
          startHour={session.data?.user.day_start_hour ?? 0}
        />
      )}
    </>
  )
}
