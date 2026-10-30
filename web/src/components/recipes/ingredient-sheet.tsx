import { useQuery } from '@tanstack/react-query'
import { useRef, useState, type RefObject } from 'react'
import { flushSync } from 'react-dom'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import { Finder } from '@/components/day/finder'
import { entryValues, type Entry, type FoodEntry } from '@/components/day/meal-entries'
import { slotAt } from '@/components/day/meal-slot'
import { NewEntry, type Making } from '@/components/day/new-entry'
import { ScanEntry } from '@/components/day/scan-entry'
import { entryUnitLabel } from '@/components/recipes/recipe-values'
import { Sheet } from '@/components/sheet'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { touchScreen } from '@/lib/media'

// IngredientSheet adds a recipe's ingredients, in a sheet that stays open
// for the next until Done: the foods of the user's that the search of
// Log a meal finds, each in an amount of its own; a new food or a
// barcode's is a step of its own, saved as Log a meal saves it. What was
// added is listed at its foot, each taken out again in a tap (onRemove).
// recipe is the recipe's name, and exclude its food, which is never its
// own ingredient; count is how many ingredients it has already.
export function IngredientSheet({
  open,
  onOpenChange,
  opener,
  ...picker
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  opener?: RefObject<HTMLElement | null>
  recipe: string
  exclude: string | undefined
  count: number
  onAdd: (entry: FoodEntry) => void
  onRemove: (key: string) => void
}) {
  const t = useMessages()
  const search = useRef<HTMLInputElement>(null)

  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title={t.recipes.addIngredients}
      initialFocus={search}
      tapToType
      finalFocus={opener}
      tall
      bare
    >
      <Picker
        {...picker}
        search={search}
        onDone={() => {
          onOpenChange(false)
        }}
      />
    </Sheet>
  )
}

// Picker is the sheet's search and its foot: what was added since it
// opened, and Done.
function Picker({
  recipe,
  exclude,
  count,
  onAdd,
  onRemove,
  search,
  onDone,
}: {
  recipe: string
  exclude: string | undefined
  count: number
  onAdd: (entry: FoodEntry) => void
  onRemove: (key: string) => void
  search: RefObject<HTMLInputElement | null>
  onDone: () => void
}) {
  const t = useMessages()
  const f = useFormat()
  const session = useQuery(sessionQuery)
  const today = $api.useQuery('get', '/api/v1/me').data?.today
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const [now] = useState(Date.now)
  const [query, setQuery] = useState('')
  const [making, setMaking] = useState<Making | undefined>(undefined)
  const [added, setAdded] = useState<FoodEntry[]>([])
  const maker = useRef<HTMLElement | null>(null)
  const done = useRef<HTMLButtonElement>(null)
  const most = limits.recipe.maxItems
  const full = count >= most

  // add takes an ingredient into the recipe, and readies the search for
  // the next: emptied, with the focus; on a touch screen, whose keyboard
  // would cover the lists, Done has it.
  function add(entry: Entry) {
    if (entry.kind !== 'food' || full) {
      return
    }
    flushSync(() => {
      onAdd(entry)
      setAdded((a) => [...a, entry])
      setQuery('')
      setMaking(undefined)
    })
    const next = touchScreen() ? done.current : search.current
    next?.focus()
  }

  function back() {
    flushSync(() => {
      setMaking(undefined)
    })
    maker.current?.focus()
  }

  return (
    <>
      {making === 'scan' ? (
        <ScanEntry ingredient onBack={back} onAdd={add} />
      ) : (
        making === 'food' && (
          <NewEntry making="food" ingredient name={query.trim()} onBack={back} onAdd={add} />
        )
      )}
      <div hidden={making !== undefined} className="contents">
        <p className="px-4 pt-2 text-small text-muted-foreground">
          {t.recipes.into(recipe.trim() || t.recipes.intoNew)} ·{' '}
          {t.recipes.count(f.number(count), f.number(most))}
        </p>
        <div className="min-h-0 flex-1 overflow-y-auto p-4">
          <Finder
            ingredient={{ exclude }}
            search={search}
            query={query}
            onQuery={setQuery}
            at={{ slot: slotAt(now, timeZone), now, timeZone, date: today ?? '' }}
            onAdd={add}
            onMake={(m, button) => {
              maker.current = button
              setMaking(m)
            }}
          />
        </div>
        <div className="space-y-2 border-t p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
          {added.length > 0 ? (
            <ul aria-label={t.recipes.added} className="flex flex-wrap gap-1.5">
              {added.map((entry) => {
                const n = f.parse(entry.amount) ?? Number.NaN
                const v = entryValues(entry, f.parse)
                return (
                  <li
                    key={entry.key}
                    className="flex items-center gap-0.5 rounded-full bg-surface-2 pl-3 text-small"
                  >
                    <span>
                      {[
                        entry.food.name,
                        `${entry.amount} ${entryUnitLabel(entry.food, entry.unit, n, t)}`,
                        ...(v === undefined ? [] : [t.unit.kcal(f.number(v.kcal))]),
                      ].join(' · ')}
                    </span>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      className="rounded-full"
                      aria-label={t.log.remove(entry.food.name)}
                      onClick={() => {
                        onRemove(entry.key)
                        setAdded((a) => a.filter((e) => e.key !== entry.key))
                      }}
                    >
                      <span aria-hidden>×</span>
                    </Button>
                  </li>
                )
              })}
            </ul>
          ) : (
            <p className="text-small text-muted-foreground">{t.recipes.addedHint}</p>
          )}
          {full && (
            <output className="block text-small text-muted-foreground">
              {t.recipes.mostIngredients(f.number(most))}
            </output>
          )}
          <Button ref={done} className="h-13 w-full text-base" onClick={onDone}>
            {added.length > 0 ? t.recipes.doneAdded(added.length) : t.recipes.done}
          </Button>
        </div>
      </div>
    </>
  )
}
