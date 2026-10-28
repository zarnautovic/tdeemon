import { useQueryClient } from '@tanstack/react-query'

import { $api } from './client'
import type { Day } from './types'

// dayQuery is the query of a day of the user's: a date, YYYY-MM-DD, or
// today.
export function dayQuery(date: string) {
  return $api.queryOptions('get', '/api/v1/days/{date}', { params: { path: { date } } })
}

// useKeepDay returns what keeps the day a change answered with: as its
// date's query, and as today's when today's holds that day, so the page
// shows it without asking the server again. The histories, which have
// the day as a row (its trend weight, which a weigh-in moves), are asked
// again.
function useKeepDay() {
  const queryClient = useQueryClient()
  return (answer: Day) => {
    queryClient.setQueryData(dayQuery(answer.date).queryKey, answer)
    const today = dayQuery('today').queryKey
    if (queryClient.getQueryData<Day>(today)?.date === answer.date) {
      queryClient.setQueryData(today, answer)
    }
    void queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/history'] })
  }
}

// The changes of a day run one at a time, in the order they were made, so
// the day the last of them answers with holds them all.
export function dayScope(date: string) {
  return { id: `day ${date}` }
}

// Each change below runs done once the server took it, whether the page
// that made it is still shown or not: what the app says of it (a toast).

// useCheckIn says what the user says of day date (PATCH, the fields given
// only).
export function useCheckIn(date: string, done?: (answer: Day) => void) {
  const keep = useKeepDay()
  return $api.useMutation('patch', '/api/v1/days/{date}', {
    scope: dayScope(date),
    onSuccess: (answer) => {
      keep(answer)
      done?.(answer)
    },
  })
}

// useLogMetric stores a measurement of day date of the user's.
export function useLogMetric(date: string, done?: (answer: Day) => void) {
  const keep = useKeepDay()
  return $api.useMutation('put', '/api/v1/days/{date}/metrics/{kind}', {
    scope: dayScope(date),
    onSuccess: (answer) => {
      keep(answer)
      done?.(answer)
    },
  })
}

// useDeleteMetric deletes a measurement of day date.
export function useDeleteMetric(date: string, done?: (answer: Day) => void) {
  const keep = useKeepDay()
  return $api.useMutation('delete', '/api/v1/days/{date}/metrics/{kind}', {
    scope: dayScope(date),
    onSuccess: (answer) => {
      keep(answer)
      done?.(answer)
    },
  })
}
