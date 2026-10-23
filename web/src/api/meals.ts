import { useQueryClient } from '@tanstack/react-query'

import { $api } from './client'
import { dayScope } from './day'
import { statusOf } from './errors'

// The days' queries, by the prefix of their keys.
const days = ['get', '/api/v1/days/{date}']

// useUpdateMeal changes a meal of day date by its ops, in order. Its
// answer has the day's totals but not its meals, so the days are asked
// again; the change stands done once they came. So they are for a meal no
// longer there (404), deleted elsewhere.
export function useUpdateMeal(date: string) {
  const queryClient = useQueryClient()
  return $api.useMutation('patch', '/api/v1/meals/{id}', {
    scope: dayScope(date),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: days }),
    onError: (error) =>
      statusOf(error) === 404 ? queryClient.invalidateQueries({ queryKey: days }) : undefined,
  })
}

// useDeleteMeal deletes a meal of day date. A meal no longer there (404)
// was deleted already, by a delete whose answer was lost or elsewhere:
// the days are asked again all the same.
export function useDeleteMeal(date: string) {
  const queryClient = useQueryClient()
  return $api.useMutation('delete', '/api/v1/meals/{id}', {
    scope: dayScope(date),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: days }),
    onError: (error) =>
      statusOf(error) === 404 ? queryClient.invalidateQueries({ queryKey: days }) : undefined,
  })
}

// useUndo takes back the user's latest change, made anywhere: after the
// changes of day date made before it. What it took back may be of any
// day, a food or the notes, so everything is asked again.
export function useUndo(date: string) {
  const queryClient = useQueryClient()
  return $api.useMutation('post', '/api/v1/undo', {
    scope: dayScope(date),
    onSuccess: () => queryClient.invalidateQueries(),
  })
}
