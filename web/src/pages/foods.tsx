import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { Plus, Search } from 'lucide-react'
import { useId } from 'react'

import { useFindFoods, useFoodList } from '@/api/foods'
import { sessionQuery } from '@/api/session'
import { FoodRow } from '@/components/foods/food-row'
import { LoadError } from '@/components/load-error'
import { Button, buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// FoodsPage is the user's foods: a search of them, or the list of those in
// use or archived, by name or by when they were eaten last, a page at a
// time; and a way to add one. What it shows is in the address (q,
// archived, order), so coming back from a food finds it as it was.
export function FoodsPage() {
  const t = useMessages()
  const id = useId()
  const { q = '', archived = false, order = 'name' } = useSearch({ from: '/app/foods' })
  const navigate = useNavigate({ from: '/foods' })
  const session = useQuery(sessionQuery)
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const searching = q.trim() !== ''

  return (
    <section className="space-y-4">
      <header className="flex items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold">{t.foods.title}</h1>
        <Link to="/foods/new" className={buttonVariants({ variant: 'outline' })}>
          <Plus aria-hidden />
          {t.foods.add}
        </Link>
      </header>
      <div className="relative">
        <label htmlFor={id} className="sr-only">
          {t.foods.search}
        </label>
        <Search
          aria-hidden
          className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <input
          id={id}
          type="search"
          value={q}
          placeholder={t.foods.searchHint}
          autoComplete="off"
          className={cn(inputClass, 'pl-9')}
          onChange={(event) => {
            const typed = event.currentTarget.value
            void navigate({
              search: (s) => ({ ...s, q: typed === '' ? undefined : typed }),
              replace: true,
            })
          }}
        />
      </div>
      {searching ? (
        <Found query={q} timeZone={timeZone} />
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <fieldset aria-label={t.foods.lists} className="flex gap-1">
              {([false, true] as const).map((a) => (
                <Button
                  key={String(a)}
                  variant={archived === a ? 'default' : 'outline'}
                  aria-pressed={archived === a}
                  onClick={() => {
                    void navigate({ search: (s) => ({ ...s, archived: a || undefined }) })
                  }}
                >
                  {a ? t.foods.archived : t.foods.inUse}
                </Button>
              ))}
            </fieldset>
            <label className="flex items-center gap-2 text-sm">
              <span className="text-muted-foreground">{t.foods.order}</span>
              <select
                value={order}
                className={cn(inputClass, 'h-8 w-auto')}
                onChange={(event) => {
                  const chosen = event.currentTarget.value === 'use' ? 'use' : undefined
                  void navigate({ search: (s) => ({ ...s, order: chosen }) })
                }}
              >
                <option value="name">{t.foods.byName}</option>
                <option value="use">{t.foods.byUse}</option>
              </select>
            </label>
          </div>
          <Listed archived={archived} order={order} timeZone={timeZone} />
        </>
      )}
    </section>
  )
}

// Found is what a search of the foods finds.
function Found({ query, timeZone }: { query: string; timeZone: string }) {
  const t = useMessages()
  const found = useFindFoods(query)

  if (found.isPending) {
    return <output className="block text-muted-foreground">{t.loading}</output>
  }
  if (found.isError) {
    return <LoadError error={found.error} onRetry={() => void found.refetch()} />
  }
  if (found.data.foods.length === 0) {
    return <p className="text-muted-foreground">{t.foods.noMatch}</p>
  }
  return (
    <ul aria-label={t.foods.found} className="divide-y">
      {found.data.foods.map((food) => (
        <FoodRow key={food.id} food={food} timeZone={timeZone} />
      ))}
    </ul>
  )
}

// Listed is the list of the foods in use or archived, with more to show
// while there are.
function Listed({
  archived,
  order,
  timeZone,
}: {
  archived: boolean
  order: 'name' | 'use'
  timeZone: string
}) {
  const t = useMessages()
  const list = useFoodList(archived, order)

  if (list.isPending) {
    return <output className="block text-muted-foreground">{t.loading}</output>
  }
  if (list.isLoadingError) {
    return <LoadError error={list.error} onRetry={() => void list.refetch()} />
  }
  const foods = list.data.pages.flatMap((p) => p.foods)
  const total = list.data.pages.at(-1)?.total ?? foods.length
  if (foods.length === 0) {
    return <p className="text-muted-foreground">{archived ? t.foods.noneArchived : t.foods.none}</p>
  }
  return (
    <div className="space-y-3">
      <ul aria-label={archived ? t.foods.archived : t.foods.inUse} className="divide-y">
        {foods.map((food) => (
          <FoodRow key={food.id} food={food} timeZone={timeZone} />
        ))}
      </ul>
      <p className="text-center text-sm text-muted-foreground">
        {t.foods.shown(foods.length, total)}
      </p>
      {list.hasNextPage && (
        <Button
          variant="outline"
          size="lg"
          className="w-full"
          disabled={list.isFetchingNextPage}
          onClick={() => void list.fetchNextPage()}
        >
          {list.isFetchingNextPage ? t.loading : t.foods.more}
        </Button>
      )}
      {list.isFetchNextPageError && (
        <LoadError error={list.error} onRetry={() => void list.fetchNextPage()} />
      )}
    </div>
  )
}
