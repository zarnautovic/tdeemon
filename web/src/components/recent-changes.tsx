import { useQuery } from '@tanstack/react-query'
import { Undo2 } from 'lucide-react'
import { useState } from 'react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { sessionQuery } from '@/api/session'
import { itemName } from '@/components/day/meal-entries'
import { longName, namedBy } from '@/components/day/meal-slot'
import { metricKinds, type MetricKind } from '@/components/day/metric-kinds'
import { LoadError } from '@/components/load-error'
import { Sheet } from '@/components/sheet'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { Messages } from '@/i18n/en'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { useUndoLatest } from '@/lib/undo'

type Change = components['schemas']['ChangeBody']

// How many of the latest changes the sheet lists.
const listed = 10

// The clients a model works through: their changes are the AI coach's.
const models: ReadonlySet<string> = new Set(['mcp', 'chat', 'telegram'])

function isKind(kind: string | undefined): kind is MetricKind {
  return metricKinds.some((k) => k === kind)
}

// said is what a change did, of what, in the words its toast said it
// in: a meal by its name (its description, or what was in it), a food by
// its name and brand, a measurement by its kind; a change of the plan by
// what it did alone.
function said(change: Change, t: Messages): string {
  const meal =
    change.name !== undefined && change.name !== '' && !longName(change.name)
      ? change.name
      : namedBy(
          change.items.map((name) => itemName({ name }, t)),
          t,
        )
  const food = change.name
    ? itemName({ name: change.name, ...(change.brand !== undefined && { brand: change.brand }) }, t)
    : t.changes.aFood
  const metric = isKind(change.metric) ? t.day.metric[change.metric] : undefined
  switch (change.action) {
    case 'meal.create':
      return t.changes.logged(meal)
    case 'meal.replace':
      return t.toast.changed(meal)
    case 'meal.delete':
      return t.toast.deleted(meal)
    case 'food.save':
      return t.toast.savedOf(food)
    case 'food.archive':
      return change.archived === false ? t.toast.inUse(food) : t.toast.archived(food)
    case 'metric.set':
      return metric === undefined ? t.toast.saved : t.toast.savedOf(metric)
    case 'metric.delete':
      return t.toast.deleted(metric ?? t.undo.change)
    case 'day.checkin':
      return t.toast.checkIn
    case 'day.training':
      return t.toast.dayType
    case 'day.exclude':
      return t.toast.excluded
    case 'day.include':
      return t.toast.included
    case 'notes.set':
      return t.toast.notes
    case 'proposal.status':
      return t.changes.proposal[change.status ?? ''] ?? t.changes.other
    default:
      return t.changes.actions[change.action] ?? t.changes.other
  }
}

// RecentChanges is the user's latest changes, in a sheet: what each did
// and of what, when and through whom (you, your AI coach, a device), and
// whether an undo took it back; with Undo on the one undo takes back
// next, so a change is taken back after its toast is gone too. The toast
// says how the undo went, and the list follows it.
export function RecentChanges({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const t = useMessages()

  return (
    <Sheet open={open} onOpenChange={onOpenChange} title={t.changes.title}>
      <List />
    </Sheet>
  )
}

function List() {
  const t = useMessages()
  const changes = $api.useQuery('get', '/api/v1/changes', {
    params: { query: { limit: listed } },
  })
  // When the list opened, which a change of today is told by.
  const [now] = useState(() => new Date().toISOString())

  if (changes.isPending) {
    return <output className="block text-muted-foreground">{t.loading}</output>
  }
  if (changes.isError) {
    return <LoadError error={changes.error} onRetry={() => void changes.refetch()} />
  }
  if (changes.data.changes.length === 0) {
    return <p className="text-sm text-muted-foreground">{t.changes.none}</p>
  }
  return (
    <ol className="-my-2 divide-y">
      {changes.data.changes.map((change) => (
        <Row key={change.id} change={change} now={now} />
      ))}
    </ol>
  )
}

function Row({ change, now }: { change: Change; now: string }) {
  const t = useMessages()
  const f = useFormat()
  const timeZone = useQuery(sessionQuery).data?.user.timezone ?? 'UTC'
  const what = said(change, t)
  const time = f.time(change.at, timeZone)
  // The day too, of a change made on another than today.
  const day = f.day(change.at, timeZone)
  const when = day === f.day(now, timeZone) ? time : t.changes.at(day, time)
  let who = t.changes.device
  if (change.client === 'web') {
    who = t.changes.you
  } else if (models.has(change.client)) {
    who = t.changes.coach
  }

  return (
    <li className="flex items-center gap-3 py-3">
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="text-sm font-medium break-words">{what}</p>
        <p className="text-small text-muted-foreground">{t.changes.by(when, who)}</p>
      </div>
      {change.undone && <Badge>{t.changes.undone}</Badge>}
      {change.next && <UndoNext change={change} what={what} />}
    </li>
  )
}

// UndoNext takes back the change undo takes back next, by its name
// (last): only while it is the latest, so a change made since elsewhere
// is not taken back in its place.
function UndoNext({ change, what }: { change: Change; what: string }) {
  const t = useMessages()
  const latest = useUndoLatest()

  return (
    <Button
      variant="outline"
      size="sm"
      aria-label={t.changes.undoOf(what)}
      disabled={latest.pending}
      onClick={() => {
        latest.undo({ action: change.action, entity: change.entity })
      }}
    >
      <Undo2 aria-hidden />
      {t.toast.undo}
    </Button>
  )
}
