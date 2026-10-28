import { useLocation, useNavigate, useParams, useRouter, useSearch } from '@tanstack/react-router'

import { $api } from '@/api/client'
import { statusOf } from '@/api/errors'
import { BackToFoods } from '@/components/foods/back-to-foods'
import { FoodDetail } from '@/components/foods/food-detail'
import { FoodForm } from '@/components/foods/food-form'
import { LoadError } from '@/components/load-error'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import type { FoodsSearch } from '@/router'

// FoodPage is a food of the user's, read (FoodDetail); with edit=true in
// the address, its form, to correct it. The form goes back to the food
// once saved, or given up.
export function FoodPage() {
  const t = useMessages()
  const { id } = useParams({ from: '/app/foods/$id' })
  const { edit } = useSearch({ from: '/app/foods/$id' })
  const fromFood = useLocation({ select: (l) => l.state.fromFood === true })
  const router = useRouter()
  const navigate = useNavigate()
  const food = $api.useQuery('get', '/api/v1/foods/{id}', { params: { path: { id } } })

  // read goes back to the food from its form: back in the history when
  // the food opened it, else to the food in its place.
  function read() {
    if (fromFood) {
      router.history.back()
      return
    }
    void navigate({
      to: '.',
      search: (s: FoodsSearch & { edit?: true | undefined }) => ({ ...s, edit: undefined }),
      replace: true,
    })
  }

  let body
  if (food.isPending) {
    body = <Skeleton />
  } else if (food.isError && [404, 422].includes(statusOf(food.error) ?? 0)) {
    // 422: the address names no food's id at all.
    body = <p className="text-muted-foreground">{t.foods.notFound}</p>
  } else if (food.isError) {
    body = <LoadError error={food.error} onRetry={() => void food.refetch()} />
  } else if (edit) {
    body = (
      <>
        <header className="flex items-center justify-between gap-2">
          <h1 className="text-h2">{t.foods.editTitle}</h1>
          <Button variant="ghost" size="sm" onClick={read}>
            {t.foods.cancel}
          </Button>
        </header>
        {/* A food corrected elsewhere, or archived, starts the form again
            only when it is another food. */}
        <div className="rounded-lg bg-card p-4 shadow-card lg:p-6">
          <FoodForm key={food.data.id} food={food.data} onSaved={read} />
        </div>
      </>
    )
  } else {
    body = <FoodDetail key={food.data.id} food={food.data} />
  }

  return (
    <div className="space-y-3">
      {!edit && <BackToFoods />}
      {body}
    </div>
  )
}

// Skeleton is a food on its way, of the shape of its page: its name, its
// energy, its buttons and its table.
function Skeleton() {
  const t = useMessages()
  const block = 'animate-pulse rounded-lg bg-surface-2 motion-reduce:animate-none'

  return (
    <div className="space-y-4">
      <output className="sr-only">{t.loading}</output>
      <div aria-hidden className="flex items-center gap-3">
        <span className={`size-14 shrink-0 ${block}`} />
        <span className={`h-8 w-1/2 ${block}`} />
      </div>
      <div aria-hidden className={`h-40 ${block}`} />
      <div aria-hidden className={`h-11 ${block}`} />
      <div aria-hidden className={`h-64 ${block}`} />
    </div>
  )
}
