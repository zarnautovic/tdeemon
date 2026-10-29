import {
  keepPreviousData,
  useQueries,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'

import { matches } from '@/lib/search'

import { $api, fetchClient } from './client'
import { HttpError } from './errors'
import type { components } from './schema'
import type { Food } from './types'

// A page of the food list.
export const foodPage = 50

// The lists of the user's foods: those in use, the archived ones, and the
// estimates in use, whose values are to check.
export type FoodFilter = 'inUse' | 'archived' | 'estimates'

// useFoodList is the user's foods of a list (filter), by name, by their
// last use or by how many of the latest meals had them, a page at a
// time: fetchNextPage asks for the next while there are more.
export function useFoodList(filter: FoodFilter, order: 'name' | 'use' | 'frequent') {
  const archived = filter === 'archived'
  return $api.useInfiniteQuery(
    'get',
    '/api/v1/foods/list',
    {
      params: {
        query: {
          archived,
          ...(filter === 'estimates' ? { source: 'estimate' as const } : {}),
          order,
          limit: foodPage,
        },
      },
    },
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

// useFrequentFoods are the user's foods the latest meals had most, 50 at
// most: what the day's chips and the quick add's Frequent offer, of one
// query. Asked only when enabled.
export function useFrequentFoods(enabled = true) {
  return $api.useQuery(
    'get',
    '/api/v1/foods/list',
    { params: { query: { order: 'frequent', limit: 50 } } },
    { enabled },
  )
}

// useFindFoods is the user's foods a search finds (find_foods), when
// there is something to search for: the archived ones, or the others.
export function useFindFoods(query: string, archived = false) {
  const q = query.trim()
  return $api.useQuery(
    'get',
    '/api/v1/foods',
    { params: { query: { query: [q], limit: 50, ...(archived ? { archived } : {}) } } },
    // While the next search comes, the last one's foods stay.
    { enabled: q !== '', placeholderData: keepPreviousData },
  )
}

// useFindEstimates is the user's estimates in use a search finds: every
// one of them, page after page of the list (a search of all the foods
// stops at its first 50, of which none may be one), narrowed as
// find_foods narrows, by the start of the words of a name, a brand or
// another name. A change of a food asks again for the list, and so for
// them.
export function useFindEstimates(query: string) {
  const q = query.trim()
  return useQuery({
    queryKey: ['get', '/api/v1/foods/list', 'estimates'],
    queryFn: async ({ signal }) => {
      const all: Food[] = []
      for (;;) {
        // One page after the other: each starts where the one before ended.
        // oxlint-disable-next-line no-await-in-loop -- each page's offset is the foods before it
        const { data, response } = await fetchClient.GET('/api/v1/foods/list', {
          params: {
            query: { source: 'estimate', order: 'name', limit: 200, offset: all.length },
          },
          signal,
        })
        if (data === undefined) {
          throw new HttpError(response.status)
        }
        all.push(...data.foods)
        if (data.foods.length === 0 || all.length >= data.total) {
          return all
        }
      }
    },
    select: (foods) => foods.filter((f) => matches(q, [f.name, f.brand, ...(f.aliases ?? [])])),
    enabled: q !== '',
  })
}

// useFoodUse is how many of the user's meals had a food, and the last of
// them, which a food's page says and logs again.
export function useFoodUse(id: string) {
  return $api.useQuery('get', '/api/v1/foods/{id}/use', { params: { path: { id } } })
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
