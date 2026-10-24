import {
  keepPreviousData,
  useQueries,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'

import { $api } from './client'
import type { components } from './schema'
import type { Food } from './types'

// A page of the food list.
export const foodPage = 50

// useFoodList is the user's foods, the archived ones or the others, by
// name or by their last use, a page at a time: fetchNextPage asks for the
// next while there are more.
export function useFoodList(archived: boolean, order: 'name' | 'use') {
  return $api.useInfiniteQuery(
    'get',
    '/api/v1/foods/list',
    { params: { query: { archived, order, limit: foodPage } } },
    {
      pageParamName: 'offset',
      initialPageParam: 0,
      getNextPageParam: (last, pages) => {
        const shown = pages.reduce((n, p) => n + p.foods.length, 0)
        return shown < last.total ? shown : undefined
      },
    },
  )
}

// useFindFoods is the user's foods a search finds (find_foods), when
// there is something to search for; archived foods are left out.
export function useFindFoods(query: string) {
  const q = query.trim()
  return $api.useQuery(
    'get',
    '/api/v1/foods',
    { params: { query: { query: [q], limit: 50 } } },
    // While the next search comes, the last one's foods stay.
    { enabled: q !== '', placeholderData: keepPreviousData },
  )
}

// useFoods is the user's foods of ids, each as it comes: undefined until
// it has, or when it cannot.
export function useFoods(ids: readonly string[]): (Food | undefined)[] {
  return useQueries({
    queries: ids.map((id) =>
      $api.queryOptions('get', '/api/v1/foods/{id}', { params: { path: { id } } }),
    ),
    combine: (results) => results.map((r) => r.data),
  })
}

// keep keeps a food a save answered with, and asks the lists and the
// searches again.
async function keep(queryClient: QueryClient, food: Food) {
  queryClient.setQueryData(
    $api.queryOptions('get', '/api/v1/foods/{id}', { params: { path: { id: food.id } } }).queryKey,
    food,
  )
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/foods/list'] }),
    queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/foods'] }),
  ])
}

// useSaveFood saves a food the user has, corrected, archived or brought
// back.
export function useSaveFood() {
  const queryClient = useQueryClient()
  return $api.useMutation('post', '/api/v1/foods', {
    onSuccess: (food) => keep(queryClient, food),
  })
}

// useCreateFood makes a new food; a food of its name and brand, archived
// too, is in the way (409), not corrected.
export function useCreateFood() {
  const queryClient = useQueryClient()
  return $api.useMutation('post', '/api/v1/foods/new', {
    onSuccess: (food) => keep(queryClient, food),
  })
}

// fieldsOf is a food as the server keeps it, to save it again as it is:
// archived, or brought back.
export function fieldsOf(food: Food): components['schemas']['UpsertFoodInputBody'] {
  const { last_use: _lastUse, queries: _queries, ...fields } = food
  return fields
}
