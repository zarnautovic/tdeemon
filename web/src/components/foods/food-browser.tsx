import { Menu } from '@base-ui/react/menu'
import { Link, useLocation, useNavigate, useSearch } from '@tanstack/react-router'
import { Check, ChevronDown, Plus, Search } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'

import { $api } from '@/api/client'
import { useFindEstimates, useFindFoods, useFoodList, type FoodFilter } from '@/api/foods'
import { eatenThisWeek } from '@/components/foods/eaten'
import { FoodRow } from '@/components/foods/food-row'
import { LoadError } from '@/components/load-error'
import { Button, buttonVariants } from '@/components/ui/button'
import { ChipGroup } from '@/components/ui/chip-group'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import type { FoodsSearch } from '@/router'

const filters = ['inUse', 'archived', 'estimates'] as const

// The orders of the list, the first when the address names none.
const orders = ['frequent', 'use', 'name'] as const
type Order = (typeof orders)[number]

// A list's card, which its rows divide.
const card = 'divide-y overflow-hidden rounded-lg bg-card shadow-card'

// FoodBrowser is the user's foods: a search of them, or a list (those in
// use, the archived ones, the estimates in use, as chips, the one chosen
// with how many foods it has), by how many of the latest meals had them
// (those eaten this week first), by when they were eaten last or by name
// (a menu beside the chips), a page at a time; and a way to add one. A
// search is of the list chosen. What it shows is in the address (q,
// filter, order), which a food opened from it keeps, so the list beside a
// food, and back from it, is as it was.
export function FoodBrowser() {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const search = useSearch({ from: '/app/foods' })
  const { q = '', filter = 'inUse', order = 'frequent' } = search
  // The address's own path stays: a food open beside the list stays open.
  const navigate = useNavigate()
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
  // The list as the address has it, which a food opened from it keeps.
  const from: FoodsSearch = {
    q: search.q,
    filter: search.filter,
    order: search.order,
  }
  // How many foods the list chosen has, once it came (the list's own
  // request), a search of it too.
  const total = useFoodList(filter, order).data?.pages.at(-1)?.total

  return (
    <section aria-labelledby={`${id}-title`} className="min-w-0 space-y-3">
      <header className="flex items-center justify-between gap-2">
        <h1 id={`${id}-title`} className="text-title">
          {t.foods.title}
        </h1>
        {/* Without a food in use, the list's own button is the one way to
            add one. */}
        {!(filter === 'inUse' && total === 0) && (
          <Link
            to="/foods/new"
            search={from}
            // Short to the eye, whole to a screen reader.
            aria-label={t.foods.newSaid}
            className={buttonVariants({ size: 'sm' })}
          >
            <Plus aria-hidden />
            {t.foods.new}
          </Link>
        )}
      </header>
      <label className="flex h-12 items-center gap-2.5 rounded-lg border-2 border-input bg-card px-3.5 focus-within:border-ring">
        <Search aria-hidden className="size-4.5 shrink-0 text-muted-foreground" />
        <span className="sr-only">{t.foods.search}</span>
        <input
          ref={field}
          type="search"
          // The shortcut / finds it.
          data-find
          value={typed}
          placeholder={t.foods.searchHint}
          autoComplete="off"
          className="h-full min-w-0 flex-1 bg-transparent text-base outline-none"
          onChange={(event) => {
            const now = event.currentTarget.value
            setTyped(now)
            setWritten((w) => [...w, now])
            void navigate({
              to: '.',
              search: (s: FoodsSearch) => ({ ...s, q: now === '' ? undefined : now }),
              replace: true,
            })
          }}
        />
      </label>
      {/* The search too is of the list chosen, and ordered by how well a
          food matches. Rows of chips apart, a chip's target clear of the
          next row's. */}
      <div className="flex flex-wrap items-center gap-x-2 gap-y-3">
        <ChipGroup
          legend={t.foods.lists}
          options={filters.map((value) => ({
            value,
            label:
              value === filter && total !== undefined
                ? t.foods.counted(t.foods.filter[value], f.number(total))
                : t.foods.filter[value],
          }))}
          value={filter}
          onChange={(chosen) => {
            void navigate({
              to: '.',
              search: (s: FoodsSearch) => ({
                ...s,
                filter: chosen === 'inUse' ? undefined : chosen,
              }),
            })
          }}
        />
        {!searching && (
          <OrderMenu
            order={order}
            onChange={(chosen) => {
              void navigate({
                to: '.',
                search: (s: FoodsSearch) => ({
                  ...s,
                  order: chosen === 'frequent' ? undefined : chosen,
                }),
              })
            }}
          />
        )}
      </div>
      {searching ? (
        <Found query={typed} filter={filter} from={from} />
      ) : (
        <Listed filter={filter} order={order} from={from} />
      )}
    </section>
  )
}

// OrderMenu is the order of a list, as a menu whose button says the one
// chosen, in short.
function OrderMenu({ order, onChange }: { order: Order; onChange: (order: Order) => void }) {
  const t = useMessages()

  return (
    <Menu.Root>
      <Menu.Trigger
        aria-label={t.foods.order(t.foods.orderShort[order])}
        className="relative ml-auto flex h-8 shrink-0 items-center gap-0.5 rounded-md px-1.5 text-small font-semibold whitespace-nowrap text-primary outline-none after:absolute after:inset-x-0 after:-inset-y-1.5 after:content-[''] hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
      >
        {t.foods.orderShort[order]}
        <ChevronDown aria-hidden className="size-3.5" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Positioner sideOffset={6} align="end" className="z-40">
          <Menu.Popup className="min-w-48 rounded-lg border bg-card p-1 shadow-sheet outline-none">
            <Menu.RadioGroup
              value={order}
              onValueChange={(value: Order) => {
                if (value !== order) {
                  onChange(value)
                }
              }}
            >
              {orders.map((o) => (
                <Menu.RadioItem
                  key={o}
                  value={o}
                  closeOnClick
                  className="flex h-11 cursor-default items-center gap-2 rounded-md pr-3 pl-2 text-sm outline-none select-none data-highlighted:bg-muted"
                >
                  <span className="flex size-4 items-center justify-center">
                    <Menu.RadioItemIndicator>
                      <Check aria-hidden className="size-4" />
                    </Menu.RadioItemIndicator>
                  </span>
                  {t.foods.orders[o]}
                </Menu.RadioItem>
              ))}
            </Menu.RadioGroup>
          </Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
  )
}

// Found is what a search of a list finds: of the archived foods, or of
// those in use, all or their estimates.
function Found({ query, filter, from }: { query: string; filter: FoodFilter; from: FoodsSearch }) {
  const t = useMessages()
  const today = $api.useQuery('get', '/api/v1/me').data?.today
  const estimates = filter === 'estimates'
  // The estimates are searched in their own list: a search of all the
  // foods finds 50 at most, which may leave them out.
  const all = useFindFoods(estimates ? '' : query, filter === 'archived')
  const guessed = useFindEstimates(estimates ? query : '')
  const found = estimates ? guessed : all

  if (found.isPending) {
    return <Skeleton />
  }
  if (found.isError) {
    return <LoadError error={found.error} onRetry={() => void found.refetch()} />
  }
  const foods = estimates ? (guessed.data ?? []) : (all.data?.foods ?? [])
  if (foods.length === 0) {
    return <Empty words={t.foods.noMatch} from={from} />
  }
  return (
    <ul aria-label={t.foods.found} className={card}>
      {foods.map((food) => (
        <FoodRow key={food.id} food={food} from={from} today={today} />
      ))}
    </ul>
  )
}

// Listed is a list of the foods, with more to show while there are. By
// how many meals had them, those eaten in the last 7 days come first, as
// this week's.
function Listed({ filter, order, from }: { filter: FoodFilter; order: Order; from: FoodsSearch }) {
  const t = useMessages()
  const list = useFoodList(filter, order)
  const today = $api.useQuery('get', '/api/v1/me').data?.today

  if (list.isPending) {
    return <Skeleton />
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
    return (
      <Empty
        words={
          { inUse: t.foods.none, archived: t.foods.noneArchived, estimates: t.foods.noneEstimates }[
            filter
          ]
        }
        from={from}
        add={filter === 'inUse'}
      />
    )
  }
  const name = t.foods.filter[filter]
  const groups =
    order === 'frequent' && today !== undefined
      ? [
          { title: t.foods.thisWeek, foods: foods.filter((food) => eatenThisWeek(food, today)) },
          { title: t.foods.earlier, foods: foods.filter((food) => !eatenThisWeek(food, today)) },
        ].filter((g) => g.foods.length > 0)
      : [{ title: undefined, foods }]
  return (
    <div className="space-y-3">
      {groups.map((group) => (
        <section key={group.title ?? 'all'} className="space-y-1.5">
          {group.title !== undefined && (
            <h2 className="px-1 text-small font-semibold text-muted-foreground">{group.title}</h2>
          )}
          <ul
            aria-label={group.title === undefined ? name : `${name}: ${group.title}`}
            className={card}
          >
            {group.foods.map((food) => (
              <FoodRow key={food.id} food={food} from={from} today={today} />
            ))}
          </ul>
        </section>
      ))}
      <p className="text-center text-small text-muted-foreground">
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

// Empty is what a list or a search without foods says, with the way to
// add one.
function Empty({ words, from, add = true }: { words: string; from: FoodsSearch; add?: boolean }) {
  const t = useMessages()

  return (
    <div className="space-y-3 rounded-lg bg-card p-4 text-center shadow-card lg:p-6">
      <p className="text-muted-foreground">{words}</p>
      {add && (
        <Link to="/foods/new" search={from} className={buttonVariants()}>
          <Plus aria-hidden />
          {t.foods.add}
        </Link>
      )}
    </div>
  )
}

// Skeleton is a list on its way: rows of a food's shape, so what comes
// takes their place without moving the page.
function Skeleton() {
  const t = useMessages()

  return (
    <div>
      <output className="sr-only">{t.loading}</output>
      <ul aria-hidden className={card}>
        {[0, 1, 2, 3, 4].map((i) => (
          <li key={i} className="flex min-h-16 items-center gap-3 px-3 py-2.5 lg:px-4">
            <span className="size-10 shrink-0 animate-pulse rounded-md bg-surface-2 motion-reduce:animate-none" />
            <span className="flex-1 space-y-2">
              <span className="block h-3.5 w-2/3 animate-pulse rounded-sm bg-surface-2 motion-reduce:animate-none" />
              <span className="block h-2.5 w-1/2 animate-pulse rounded-sm bg-surface-2 motion-reduce:animate-none" />
            </span>
            <span className="h-3.5 w-14 animate-pulse rounded-sm bg-surface-2 motion-reduce:animate-none" />
          </li>
        ))}
      </ul>
    </div>
  )
}
