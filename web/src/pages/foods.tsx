import { useQuery } from '@tanstack/react-query'
import { Link, useLocation, useNavigate, useSearch } from '@tanstack/react-router'
import { Plus, Search } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'

import { useFindFoods, useFoodList } from '@/api/foods'
import { sessionQuery } from '@/api/session'
import { FoodRow } from '@/components/foods/food-row'
import { LoadError } from '@/components/load-error'
import { Button, buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'
import type { FoodsSearch } from '@/router'

// FoodsPage is the user's foods, those in use or the archived ones: a
// search of them, or their list, by name or by when they were eaten last,
// a page at a time; and a way to add one. What it shows is in the address (q,
// archived, order), so coming back from a food finds it as it was.
export function FoodsPage() {
  const t = useMessages()
  const id = useId()
  const search = useSearch({ from: '/app/foods' })
  const { q = '', archived = false, order = 'name' } = search
  const navigate = useNavigate({ from: '/foods' })
  const session = useQuery(sessionQuery)
  const timeZone = session.data?.user.timezone ?? 'UTC'
  // What is typed: the address follows it, but the field is not taken
  // from it as it is typed, which would move the caret to the end. A q
  // the field did not write (a link, back and forward) is put in it; one
  // it wrote comes later than the typing, and is passed over, with those
  // written before it, which it replaced (some never come: a navigation
  // that waits on the network gives way to the next).
  const [typed, setTyped] = useState(q)
  const [written, setWritten] = useState<readonly string[]>([])
  const [seenQ, setSeenQ] = useState(q)
  if (q !== seenQ) {
    setSeenQ(q)
    const at = written.indexOf(q)
    if (at >= 0) {
      setWritten(written.slice(at + 1))
    } else {
      setTyped(q)
    }
  }
  const searching = typed.trim() !== ''
  // The search takes the focus when the shortcut / brought the page.
  const field = useRef<HTMLInputElement>(null)
  const find = useLocation({ select: (l) => l.state.find === true })
  useEffect(() => {
    if (find) {
      field.current?.focus()
    }
  }, [find])

  return (
    <section className="max-w-3xl space-y-4">
      <header className="flex items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold">{t.foods.title}</h1>
        <Link
          to="/foods/new"
          state={{ foods: search }}
          className={buttonVariants({ variant: 'outline' })}
        >
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
          ref={field}
          id={id}
          type="search"
          // The shortcut / finds it.
          data-find
          value={typed}
          placeholder={t.foods.searchHint}
          autoComplete="off"
          className={cn(inputClass, 'pl-9')}
          onChange={(event) => {
            const now = event.currentTarget.value
            setTyped(now)
            setWritten((w) => [...w, now])
            void navigate({
              search: (s) => ({ ...s, q: now === '' ? undefined : now }),
              replace: true,
            })
          }}
        />
      </div>
      {/* The search too is of the foods in use or of the archived ones. */}
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
        {!searching && (
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
        )}
      </div>
      {searching ? (
        <Found query={typed} archived={archived} timeZone={timeZone} from={search} />
      ) : (
        <Listed archived={archived} order={order} timeZone={timeZone} from={search} />
      )}
    </section>
  )
}

// Found is what a search of the foods finds, of the archived ones or the
// others.
function Found({
  query,
  archived,
  timeZone,
  from,
}: {
  query: string
  archived: boolean
  timeZone: string
  from: FoodsSearch
}) {
  const t = useMessages()
  const found = useFindFoods(query, archived)

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
        <FoodRow key={food.id} food={food} timeZone={timeZone} from={from} />
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
  from,
}: {
  archived: boolean
  order: 'name' | 'use'
  timeZone: string
  from: FoodsSearch
}) {
  const t = useMessages()
  const list = useFoodList(archived, order)

  if (list.isPending) {
    return <output className="block text-muted-foreground">{t.loading}</output>
  }
  if (list.isLoadingError) {
    return <LoadError error={list.error} onRetry={() => void list.refetch()} />
  }
  // A food may come on two pages, when the list changed between them.
  const seen = new Set<string>()
  const foods = list.data.pages
    .flatMap((p) => p.foods)
    .filter((food) => !seen.has(food.id) && Boolean(seen.add(food.id)))
  const total = list.data.pages.at(-1)?.total ?? foods.length
  if (foods.length === 0) {
    return <p className="text-muted-foreground">{archived ? t.foods.noneArchived : t.foods.none}</p>
  }
  return (
    <div className="space-y-3">
      <ul aria-label={archived ? t.foods.archived : t.foods.inUse} className="divide-y">
        {foods.map((food) => (
          <FoodRow key={food.id} food={food} timeZone={timeZone} from={from} />
        ))}
      </ul>
      <p className="text-center text-sm text-muted-foreground">
        {t.foods.shown(foods.length, total)}
      </p>
      {list.hasNextPage && (
        <Button
          variant="outline"
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
