import { useQuery } from '@tanstack/react-query'

import { $api } from '@/api/client'
import { dayQuery } from '@/api/day'
import { useFrequentFoods } from '@/api/foods'
import { useRecentMeals } from '@/api/meals'
import type { Day } from '@/api/types'
import { weekOf } from '@/components/day/week'
import { addDays } from '@/i18n/format'

// useDayReady tells whether what a day's page shows above its meals has
// come, or failed to, once the day did: the page shows it all at once,
// and nothing comes in above the meals afterwards to push them down
// under a finger. It is who the user is, the summary and the plan (the
// hero and its TDEE, the phase, a proposal that waits, the first steps),
// the history of the day's week (the week, a day before's TDEE), and of
// today what its chips offer (the day before, the foods logged most and
// the meals of before) and, before a plan, the first steps' access keys.
// Each is the query of the part that shows it, asked as that part is: so
// it is asked once, the part shown hidden until the page is ready.
export function useDayReady(date: string | undefined, day: Day | undefined): boolean {
  const came = day !== undefined
  const me = $api.useQuery('get', '/api/v1/me')
  const summary = $api.useQuery('get', '/api/v1/summary', undefined, { enabled: came })
  const plan = $api.useQuery('get', '/api/v1/plan', undefined, { enabled: came })
  // Today is the day / answered, once it has; else the user's.
  const today = date === undefined && day !== undefined ? day.date : me.data?.today
  const shown = date ?? today
  const isToday = date === undefined || date === today
  const week = shown === undefined || today === undefined ? undefined : weekOf(shown, today)
  const history = $api.useQuery(
    'get',
    '/api/v1/history',
    { params: { query: week ?? { from: '' } } },
    { enabled: came && week !== undefined && week.from <= week.to },
  )
  const chips = came && isToday
  const yesterday = useQuery({
    ...dayQuery(day === undefined ? '' : addDays(day.date, -1)),
    enabled: chips,
  })
  const frequent = useFrequentFoods(chips)
  const recent = useRecentMeals(chips)
  // Before a plan, today has the first steps in its hero's place.
  const steps =
    chips &&
    day.target === undefined &&
    plan.data?.phases.length === 0 &&
    plan.data.targets.length === 0 &&
    plan.data.pending === undefined
  const keys = $api.useQuery('get', '/api/v1/tokens', undefined, { enabled: steps })

  return ![me, summary, plan, history, yesterday, frequent, recent, keys].some((q) => q.isLoading)
}
