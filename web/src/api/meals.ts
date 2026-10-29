import { useQueryClient, type QueryClient } from '@tanstack/react-query'

import { $api } from './client'
import { dayScope } from './day'
import { statusOf } from './errors'
import type { components } from './schema'

// A meal's change answers with the meal and its day; a delete, the day.
type MealAnswer = components['schemas']['MealAnswerBody']
type DeleteAnswer = components['schemas']['DeleteMealOutputBody']

// The days' queries, by the prefix of their keys.
const days = ['get', '/api/v1/days/{date}']

// meals are asked again after a meal's change: the days, the meals of
// before, and the lists of foods by their use and a food's use, which a
// meal changes.
function meals(queryClient: QueryClient) {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: days }),
    queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/meals/recent'] }),
    queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/foods/list'] }),
    queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/foods/{id}/use'] }),
  ])
}

// useLogMeal logs a meal of day date. Its answer has the day's totals but
// not its meals, so the days are asked again, and the meals of before and
// the foods by their use; the meal stands logged once they came. They are
// when it failed too: without an answer it may have been logged all the
// same, and the day shows whether it was. Each change here runs done once
// the server took it, whether the page that made it is still shown or
// not: what the app says of it (a toast).
export function useLogMeal(date: string, done?: (answer: MealAnswer) => void) {
  const queryClient = useQueryClient()
  return $api.useMutation('post', '/api/v1/meals', {
    scope: dayScope(date),
    onSuccess: (answer) => {
      done?.(answer)
    },
    onSettled: () => meals(queryClient),
  })
}

// useUpdateMeal changes a meal of day date by its ops, in order. Its
// answer has the day's totals but not its meals, so the days are asked
// again, with the meals of before and the foods by their use; the change
// stands done once they came. The days are for a meal no longer there
// (404) too, deleted elsewhere. A change that saved a food asks the
// foods' searches again too.
export function useUpdateMeal(date: string, done?: (answer: MealAnswer) => void) {
  const queryClient = useQueryClient()
  return $api.useMutation('patch', '/api/v1/meals/{id}', {
    scope: dayScope(date),
    onSuccess: (answer, { body }) => {
      done?.(answer)
      return Promise.all([
        meals(queryClient),
        ...(body.ops?.some((op) => op.item?.food !== undefined)
          ? [queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/foods'] })]
          : []),
      ])
    },
    onError: (error) =>
      statusOf(error) === 404 ? queryClient.invalidateQueries({ queryKey: days }) : undefined,
  })
}

// useDeleteMeal deletes a meal of day date; the days are asked again,
// with the meals of before and the foods by their use. A meal no longer
// there (404) was deleted already, by a delete whose answer was lost or
// elsewhere: the days are asked again all the same.
export function useDeleteMeal(date: string, done?: (answer: DeleteAnswer) => void) {
  const queryClient = useQueryClient()
  return $api.useMutation('delete', '/api/v1/meals/{id}', {
    scope: dayScope(date),
    onSuccess: (answer) => {
      done?.(answer)
      return meals(queryClient)
    },
    onError: (error) =>
      statusOf(error) === 404 ? queryClient.invalidateQueries({ queryKey: days }) : undefined,
  })
}

// useRecentMeals are the meals of before, the latest 50 that are not the
// same, each with when it was eaten every time: what the quick add and
// the day's chips offer, of one query. Asked only when enabled.
export function useRecentMeals(enabled = true) {
  return $api.useQuery(
    'get',
    '/api/v1/meals/recent',
    { params: { query: { limit: 50 } } },
    { enabled },
  )
}

// useUndo takes back the user's latest change, made anywhere, or the one
// it names (last) while it is the latest: after the changes of day date
// made before it. What it took back may be of any day, a food or the
// notes, so everything is asked again, whatever the answer.
export function useUndo(date: string) {
  const queryClient = useQueryClient()
  return $api.useMutation('post', '/api/v1/undo', {
    scope: dayScope(date),
    // Failed too: without an answer it may have taken back a change all
    // the same, and the day shows whether it did.
    onSettled: () => queryClient.invalidateQueries(),
  })
}
