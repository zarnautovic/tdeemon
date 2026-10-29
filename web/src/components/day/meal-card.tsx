import { ChevronDown, Pencil, Repeat2, Trash2 } from 'lucide-react'
import { useId, useRef, useState, type RefObject } from 'react'

import { statusOf } from '@/api/errors'
import { useDeleteMeal, useLogMeal } from '@/api/meals'
import type { Item, Meal } from '@/api/types'
import { MacroSplit } from '@/components/day/macro-split'
import { MealEditor } from '@/components/day/meal-editor'
import { itemName, repeatOf, shownAmount } from '@/components/day/meal-entries'
import { longName, mealName } from '@/components/day/meal-slot'
import { RepeatStatus } from '@/components/day/repeat-meal'
import { SaveStatus } from '@/components/day/save-status'
import { Sheet } from '@/components/sheet'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

// The clients a model logs through: its meals are marked AI.
const models: ReadonlySet<Meal['client']> = new Set(['mcp', 'chat', 'telegram'])

// together is values said each with its label or unit ("P 47", "248
// kcal"), in a line that breaks between them, after a "·", and never
// within one: no space in them, nor before a "·", is one a line breaks
// at.
function together(pairs: readonly string[]): string {
  return pairs.map((pair) => pair.replaceAll(' ', '\u00a0')).join('\u00a0· ')
}

// MealCard is a meal as a row of the day: when it was eaten, its name and
// kcal (whose unit the day's meals say once, above them), its macros as a
// bar and in grams, whether a model logged it and whether it holds an
// estimate, and how many items. A tap shows its items with how much of
// each and what each brought, and the ways to change it (MealEditor, in a
// sheet), repeat it today at this time, or delete it; a toast says what
// was done, with what the server warned of it, and Undo. Deleted, in its
// editor or from the card, the focus goes to afterDelete, as the card
// goes with it.
export function MealCard({
  meal,
  date,
  today,
  timeZone,
  startHour,
  afterDelete,
}: {
  meal: Meal
  date: string
  // The user's logical today, which a repeat is of.
  today: string
  // The user's clock: its time zone and the hour a day starts at.
  timeZone: string
  startHour: number
  afterDelete?: RefObject<HTMLElement | null>
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const time = f.time(meal.eaten_at, timeZone)
  const long = longName(meal.description)
  const name = mealName(meal, t)
  const kcal = f.number(meal.total.kcal)
  // A repeat is today's, but goes in turn with the changes of the day
  // shown: a change of the day made meanwhile waits for it.
  const repeat = useLogMeal(date, (answer) => {
    sayChange(
      {
        title: t.meal.repeated(
          mealName(answer.meal, t),
          t.unit.kcal(f.number(answer.meal.total.kcal)),
        ),
        lines: answer.warnings.map((w) => t.meal.warnings[w.code]),
        // Today's, which another day's page does not show.
        to: date === today ? undefined : today,
      },
      { last: { action: 'meal.create', entity: answer.meal.id }, date },
    )
  })
  const remove = useDeleteMeal(date, () => {
    sayChange(
      { title: t.toast.deleted(name) },
      { last: { action: 'meal.delete', entity: meal.id }, date },
    )
    // The card goes with its Delete, and the focus in it.
    afterDelete?.current?.focus()
  })
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  // Deleted in the editor, which closes on it.
  const [deleted, setDeleted] = useState(false)
  // The button that opens the editor, which takes the focus back when it
  // closes, and the editor's first field.
  const change = useRef<HTMLButtonElement>(null)
  const first = useRef<HTMLInputElement>(null)

  // amount is how much of an item, in its unit; of a named serving, as
  // it was given, with that: "2 slices (80 g)", as the editor has it.
  function amount(item: Item): string {
    let inUnit
    if (item.unit === 'portion') {
      // The word's form follows the number shown: 0.999 is "1 portion".
      const shown = Math.round(item.amount * 100) / 100
      inUnit = t.day.portions(f.number(shown, 2), shown)
    } else {
      inUnit = t.unit[item.unit](f.number(item.amount))
    }
    const { servings, serving } = item.quantity
    // Rounded as the editor's field has it, but not to none.
    return servings !== undefined && serving !== undefined
      ? `${shownAmount(servings, (n, digits = 2) => f.number(n, digits))} ${serving} (${inUnit})`
      : inUnit
  }

  return (
    <article aria-labelledby={`${id}-name`} className="rounded-lg border bg-card">
      <h3>
        <button
          type="button"
          aria-expanded={open}
          aria-controls={`${id}-more`}
          onClick={() => {
            setOpen((o) => !o)
          }}
          // The time's column as wide as the language's times, so the
          // name has the rest of the line but the energy.
          className={cn(
            'grid min-h-14 w-full items-baseline gap-x-2.5 gap-y-1.5 rounded-lg px-4 py-3.5 text-left outline-none hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
            f.hour12
              ? 'grid-cols-[3.75rem_minmax(0,1fr)_auto]'
              : 'grid-cols-[2.5rem_minmax(0,1fr)_auto]',
          )}
        >
          <time
            dateTime={meal.eaten_at}
            className="text-small whitespace-nowrap text-muted-foreground tabular-nums"
          >
            {time}
          </time>
          <span id={`${id}-name`} className="truncate font-semibold">
            {name}
          </span>
          {/* The energy on one line, whatever the name's length; its unit
              is the meals', said above them, and to a screen reader here. */}
          <span className="font-semibold whitespace-nowrap tabular-nums">
            <span aria-hidden>{kcal}</span>
            <span className="sr-only">{t.unit.kcal(kcal)}</span>
          </span>
          {long && (
            <span className="col-span-2 col-start-2 line-clamp-2 text-small text-muted-foreground">
              {meal.description}
            </span>
          )}
          <span className="col-span-2 col-start-2 flex flex-wrap items-center gap-x-2.5 gap-y-1 text-caption text-muted-foreground">
            <MacroSplit nutrients={meal.total} />
            <span>
              {together([
                `${t.day.short.protein} ${f.grams(meal.total.protein)}`,
                `${t.day.short.carbs} ${f.grams(meal.total.carbs)}`,
                `${t.day.short.fat} ${f.grams(meal.total.fat)}`,
              ])}
            </span>
            <span className="ml-auto flex items-center gap-1.5">
              {models.has(meal.client) && <Badge>{t.meal.ai}</Badge>}
              {meal.items.some((item) => item.source === 'estimate') && (
                <Badge tone="warning">{t.meal.estimate}</Badge>
              )}
              {/* The items, and that a tap shows them. */}
              <span>{t.meal.items(meal.items.length)}</span>
              <ChevronDown
                aria-hidden
                className={cn(
                  'size-4 shrink-0 transition-transform motion-reduce:transition-none',
                  open && 'rotate-180',
                )}
              />
            </span>
          </span>
        </button>
      </h3>
      {open && (
        <div id={`${id}-more`} className="space-y-3 border-t px-4 pt-3 pb-4">
          {meal.note && (
            <p className="text-sm whitespace-pre-line text-muted-foreground">{meal.note}</p>
          )}
          <ul className="divide-y text-sm">
            {meal.items.map((item) => (
              <li key={item.id} className="flex items-baseline justify-between gap-2 py-1.5">
                {/* Its name, and its brand after it, as the foods are shown. */}
                <span>
                  {itemName({ name: item.name }, t)}
                  {item.brand && (
                    <span className="text-muted-foreground"> · {item.brand}</span>
                  )}{' '}
                  <span className="text-muted-foreground">{amount(item)}</span>
                </span>
                <span className="text-right text-muted-foreground">
                  {together([
                    t.unit.kcal(f.number(item.nutrients.kcal)),
                    `${t.day.short.protein} ${f.grams(item.nutrients.protein)}`,
                    `${t.day.short.carbs} ${f.grams(item.nutrients.carbs)}`,
                    `${t.day.short.fat} ${f.grams(item.nutrients.fat)}`,
                  ])}
                </span>
              </li>
            ))}
          </ul>
          <div className="flex flex-wrap gap-2">
            <Button
              ref={change}
              variant="outline"
              size="sm"
              aria-label={t.meal.edit(name)}
              onClick={() => {
                setEditing(true)
                setDeleted(false)
              }}
            >
              <Pencil aria-hidden />
              {t.meal.change}
            </Button>
            <Button
              variant="outline"
              size="sm"
              aria-label={t.meal.repeat(name)}
              disabled={repeat.isPending}
              onClick={() => {
                repeat.mutate({ body: repeatOf(meal) })
              }}
            >
              <Repeat2 aria-hidden />
              {t.meal.repeatToday}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              aria-label={t.meal.deleteOf(name)}
              disabled={remove.isPending}
              onClick={() => {
                remove.mutate({ params: { path: { id: meal.id } } })
              }}
            >
              <Trash2 aria-hidden />
              {t.meal.deleteShort}
            </Button>
          </div>
        </div>
      )}
      <Sheet
        open={editing}
        onOpenChange={setEditing}
        title={t.meal.editTitle}
        initialFocus={first}
        finalFocus={deleted && afterDelete ? afterDelete : change}
        tall
        bare
      >
        <MealEditor
          meal={meal}
          date={date}
          today={today}
          timeZone={timeZone}
          startHour={startHour}
          first={first}
          onDone={() => {
            setEditing(false)
          }}
          onDeleted={() => {
            setDeleted(true)
            setEditing(false)
          }}
        />
      </Sheet>
      <div className="px-4 pb-3 empty:hidden">
        <RepeatStatus log={repeat} />
        {/* A meal no longer there (404) was deleted: by this delete, its
            answer lost, or elsewhere. */}
        {remove.isError && statusOf(remove.error) !== 404 && <SaveStatus change={remove} />}
      </div>
    </article>
  )
}
