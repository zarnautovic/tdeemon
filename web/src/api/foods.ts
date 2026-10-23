import { useQueryClient } from '@tanstack/react-query'

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
    { enabled: q !== '' },
  )
}

// useSaveFood saves a food, new or corrected (archived too, or brought
// back): the food itself is kept, and the lists and searches asked again.
export function useSaveFood() {
  const queryClient = useQueryClient()
  return $api.useMutation('post', '/api/v1/foods', {
    onSuccess: async (food) => {
      queryClient.setQueryData(
        $api.queryOptions('get', '/api/v1/foods/{id}', { params: { path: { id: food.id } } })
          .queryKey,
        food,
      )
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/foods/list'] }),
        queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/foods'] }),
      ])
    },
  })
}

// fieldsOf is a food as the server keeps it, to save it again as it is:
// archived, or brought back.
export function fieldsOf(food: Food): components['schemas']['UpsertFoodInputBody'] {
  const { last_use: _lastUse, queries: _queries, ...fields } = food
  return fields
}
