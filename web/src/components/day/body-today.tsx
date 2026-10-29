import { Plus } from 'lucide-react'
import { useRef, useState, type ReactNode, type Ref } from 'react'
import { flushSync } from 'react-dom'

import { $api } from '@/api/client'
import type { Metric } from '@/api/types'
import { DayMetrics } from '@/components/day/day-metrics'
import type { MetricKind } from '@/components/day/metric-kinds'
import { useNothingMeasured } from '@/components/day/nothing-measured'
import { usePreviousWeight } from '@/components/day/previous-weight'
import { weekOf } from '@/components/day/week'
import { WeighIn } from '@/components/day/weigh-in'
import { Sheet } from '@/components/sheet'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { linkTarget } from '@/lib/styles'
import { cn } from '@/lib/utils'

// BodyToday is what the day measured at a glance, in four tiles: the
// weight (and the trend calibration smooths it to: today's, the
// summary's; of a day before, the one it had then, from the history of
// its week, which the week strip has asked for), the night's sleep, the
// steps and the resting heart rate, each a dash while the day has none.
// A tile is a tap from changing its measurement: the weight in a field in
// its place, hinting with the day before's, and the others in the sheet
// of all the measurements, open at theirs. All of them, to correct,
// delete or add, are a sheet away too. A day to come takes none (canLog).
// today is the user's, once the app knows it; another day's is the day's
// body, not today's. While the user has measured nothing yet, today has
// no empty tiles, only the way to a first weigh-in.
export function BodyToday({
  date,
  today,
  metrics,
  canLog = true,
}: {
  date: string
  today: string | undefined
  metrics: Metric[]
  canLog?: boolean
}) {
  const t = useMessages()
  const f = useFormat()
  // The sheet of the measurements, open at a kind's, or at none.
  const [sheet, setSheet] = useState<{ at?: MetricKind } | undefined>(undefined)
  const field = useRef<HTMLInputElement>(null)
  // The weight's field, in place of its tile, and what gives it the
  // focus back once it goes.
  const [weighing, setWeighing] = useState(false)
  const opener = useRef<HTMLButtonElement>(null)
  const nothing = useNothingMeasured(date, metrics)
  const previous = usePreviousWeight(date, weighing)
  const summary = $api.useQuery('get', '/api/v1/summary')
  const before = today !== undefined && date < today
  const history = $api.useQuery(
    'get',
    '/api/v1/history',
    { params: { query: weekOf(date, today ?? date) } },
    { enabled: before },
  )
  const trend =
    date === today
      ? summary.data?.trend?.kg
      : before
        ? history.data?.days.find((d) => d.date === date)?.trend_kg
        : undefined
  const value = (kind: Metric['kind']) => metrics.find((m) => m.kind === kind)?.value

  const weight = value('weight')
  const sleep = value('sleep_h')
  const steps = value('steps')
  const rhr = value('rhr')
  const tiles: { kind: MetricKind; value: string | undefined; note?: string | undefined }[] = [
    {
      kind: 'weight',
      value: weight === undefined ? undefined : `${f.number(weight, 1)} kg`,
      note: trend === undefined ? undefined : t.day.body.trend(f.number(trend, 1)),
    },
    {
      kind: 'sleep_h',
      value: sleep === undefined ? undefined : t.unit.hm(...f.hm(sleep)),
    },
    { kind: 'steps', value: steps === undefined ? undefined : f.number(steps) },
    { kind: 'rhr', value: rhr === undefined ? undefined : `${f.number(rhr)} bpm` },
  ]

  // weighed closes the weight's field, and gives the focus to what opened
  // it.
  const weighed = () => {
    flushSync(() => {
      setWeighing(false)
    })
    opener.current?.focus()
  }
  const weighIn = (
    <WeighIn
      date={date}
      label={t.day.metric.weight}
      initial={weight === undefined ? '' : f.field(weight, 1)}
      placeholder={previous?.weight === undefined ? undefined : f.field(previous.weight.value, 1)}
      focus
      onSaved={weighed}
      onCancel={weighed}
    />
  )

  let body: ReactNode
  if (nothing && canLog) {
    body = weighing ? (
      weighIn
    ) : (
      <Button
        ref={opener}
        variant="outline"
        className="w-full border-dashed"
        onClick={() => {
          setWeighing(true)
        }}
      >
        <Plus aria-hidden />
        {t.day.body.addWeight}
      </Button>
    )
  } else {
    body = (
      <ul className="grid grid-cols-2 gap-2">
        {weighing && <li className="col-span-2 rounded-md bg-surface-2 p-3">{weighIn}</li>}
        {tiles.map((tile) =>
          weighing && tile.kind === 'weight' ? null : (
            <li key={tile.kind}>
              <Tile
                label={t.day.metric[tile.kind]}
                value={tile.value}
                note={tile.note}
                buttonRef={tile.kind === 'weight' ? opener : undefined}
                onOpen={
                  canLog
                    ? () => {
                        if (tile.kind === 'weight') {
                          setWeighing(true)
                        } else {
                          setSheet({ at: tile.kind })
                        }
                      }
                    : undefined
                }
              />
            </li>
          ),
        )}
      </ul>
    )
  }

  return (
    <section aria-labelledby="day-body" className="space-y-3 rounded-lg border bg-card p-4">
      <div className="flex items-baseline justify-between gap-2">
        <h2 id="day-body" className="text-h3">
          {date === today ? t.day.body.title : t.day.body.titleOfDay}
        </h2>
        {!(nothing && canLog) && (
          <Button
            variant="link"
            className={cn(linkTarget, 'h-auto p-0 text-small font-semibold')}
            onClick={() => {
              setSheet({})
            }}
          >
            {t.day.body.all}
          </Button>
        )}
      </div>
      {body}
      <Sheet
        open={sheet !== undefined}
        onOpenChange={(open) => {
          if (!open) {
            setSheet(undefined)
          }
        }}
        title={t.day.measurements}
        initialFocus={sheet?.at === undefined ? undefined : field}
      >
        <DayMetrics date={date} metrics={metrics} canLog={canLog} at={sheet?.at} field={field} />
      </Sheet>
    </section>
  )
}

const tileClass = 'block h-full w-full space-y-0.5 rounded-md bg-surface-2 p-3 text-left'

// Tile is a measurement of the day's in short: its name, its value or a
// dash, and a note under it (the trend). With onOpen, a button that opens
// its change.
function Tile({
  label,
  value,
  note,
  buttonRef,
  onOpen,
}: {
  label: string
  value: string | undefined
  note: string | undefined
  buttonRef: Ref<HTMLButtonElement> | undefined
  onOpen: (() => void) | undefined
}) {
  const t = useMessages()
  const content = (
    <>
      <span className="block text-caption text-muted-foreground">{label}</span>
      <span className="block text-h2">{value ?? t.day.body.none}</span>
      {note !== undefined && <span className="block text-caption text-success">{note}</span>}
    </>
  )
  if (onOpen === undefined) {
    return <div className={tileClass}>{content}</div>
  }
  return (
    <button
      ref={buttonRef}
      type="button"
      className={cn(
        tileClass,
        'outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring',
      )}
      onClick={onOpen}
    >
      {content}
    </button>
  )
}
