import { useQueryClient } from '@tanstack/react-query'
import { Ban, Check, Hourglass, ShieldCheck } from 'lucide-react'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { DriftMeter } from '@/components/autopilot/drift-meter'
import { week } from '@/components/autopilot/weekday'
import { SaveStatus } from '@/components/day/save-status'
import { ListGroup } from '@/components/list-group'
import { ListRow } from '@/components/list-row'
import { LoadError } from '@/components/load-error'
import { useDate } from '@/components/plan/proposal-text'
import { Skeleton } from '@/components/skeleton'
import { SubPage } from '@/components/sub-page'
import { Button } from '@/components/ui/button'
import { ChipGroup } from '@/components/ui/chip-group'
import { SegmentedControl } from '@/components/ui/segmented'
import { useFormat } from '@/i18n/format'
import { useLocale } from '@/i18n/locale'
import { useMessages } from '@/i18n/messages'
import { sayChange } from '@/lib/toast'

type Autopilot = components['schemas']['AutopilotBody']
type Change = components['schemas']['SetAutopilotInputBody']

// AutopilotPage is the autopilot's settings, a web session's only (an AI
// coach cannot turn it on): whether it applies its weekly step itself or
// asks first, which turned on asks first, saying in numbers what that
// means; the day of its weekly review; in autopilot the drift so far
// against the limit; what it may change, what it never changes and when
// it waits, by the user's limits; and those limits, a row to their page.
export function AutopilotPage() {
  const t = useMessages()
  const id = useId()
  const autopilot = $api.useQuery('get', '/api/v1/autopilot')
  const limits = $api.useQuery('get', '/api/v1/limits')

  let body
  if (autopilot.data !== undefined && limits.data !== undefined) {
    body = <Settings autopilot={autopilot.data} holds={limits.data.effective} />
  } else if (autopilot.isError || limits.isError) {
    body = (
      <LoadError
        title={t.loadFailed.autopilot}
        error={autopilot.error ?? limits.error}
        onRetry={() => {
          void autopilot.refetch()
          void limits.refetch()
        }}
      />
    )
  } else {
    body = <Skeleton blocks={['h-40', 'h-32', 'h-80']} />
  }

  return (
    <SubPage title={t.autopilot.name} titleId={id} from="plan" hint={t.autopilot.hint}>
      {body}
    </SubPage>
  )
}

const card = 'space-y-3 rounded-lg border bg-card p-4 shadow-card lg:p-5'

function Settings({ autopilot, holds }: { autopilot: Autopilot; holds: Record<string, number> }) {
  const t = useMessages()
  const f = useFormat()
  const date = useDate()
  const locale = useLocale()
  const id = useId()
  const queryClient = useQueryClient()
  const a = t.autopilot
  // Whether turning it on is asked about, before it is.
  const [asking, setAsking] = useState(false)
  const notNow = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (asking) {
      notNow.current?.focus()
    }
  }, [asking])
  const save = $api.useMutation('put', '/api/v1/autopilot', {
    onSuccess: async (answer, sent) => {
      queryClient.setQueryData($api.queryOptions('get', '/api/v1/autopilot').queryKey, answer)
      setAsking(false)
      // Undo leaves it: the toast says it, without an Undo.
      sayChange({ title: sent.body.mode === undefined ? a.savedDay : a.saved[answer.mode] })
      // The plan says the mode, the next review and the drift too.
      await queryClient.invalidateQueries({
        queryKey: $api.queryOptions('get', '/api/v1/plan').queryKey,
      })
    },
  })
  const send = (body: Change) => {
    save.mutate({ body })
  }
  const step = holds.step_kcal ?? 0
  const driftPct = holds.drift_pct ?? autopilot.drift.limit_pct
  const limitKcal = autopilot.drift.limit_kcal
  const pct = (n: number) => f.percent(n, 0)
  const on = autopilot.mode === 'autopilot'
  // The day chosen shows at once, while it is on its way.
  const weekday = (save.isPending ? save.variables.body.weekday : undefined) ?? autopilot.weekday
  const weekdayName = new Intl.DateTimeFormat(locale, { weekday: 'short', timeZone: 'UTC' })
  const shown = asking ? 'autopilot' : autopilot.mode

  return (
    <>
      <div className={card}>
        <SegmentedControl
          legend={a.modeLegend}
          options={[
            { value: 'propose', label: a.modes.propose },
            { value: 'autopilot', label: a.modes.autopilot },
          ]}
          value={shown}
          describedBy={`${id}-hint`}
          onChange={(mode) => {
            if (mode === 'autopilot') {
              save.reset()
              setAsking(true)
            } else {
              setAsking(false)
              send({ mode: 'propose' })
            }
          }}
        >
          <p id={`${id}-hint`} className="text-sm text-muted-foreground">
            {a.modeHints[shown]}
          </p>
        </SegmentedControl>
        {asking && (
          <section
            aria-labelledby={`${id}-ask`}
            className="space-y-2 rounded-lg border border-primary/40 bg-primary-soft/40 p-3 text-sm"
          >
            <p id={`${id}-ask`} className="font-semibold">
              {a.turnOn}
            </p>
            <p>
              {limitKcal === undefined
                ? a.turnOnTextNoBase(f.number(step), pct(driftPct))
                : a.turnOnText(f.number(step), f.number(limitKcal))}
            </p>
            <div className="flex gap-2">
              <Button
                disabled={save.isPending}
                onClick={() => {
                  send({ mode: 'autopilot' })
                }}
              >
                {a.turnOnYes}
              </Button>
              <Button
                ref={notNow}
                variant="ghost"
                disabled={save.isPending}
                onClick={() => {
                  setAsking(false)
                }}
              >
                {a.turnOnNo}
              </Button>
            </div>
          </section>
        )}
        <SaveStatus change={save} />
      </div>

      <section aria-labelledby={`${id}-day`} className={card}>
        <h2 id={`${id}-day`} className="font-semibold">
          {a.day}
        </h2>
        <ChipGroup
          legend={a.day}
          options={week.map((d, i) => ({
            value: d,
            // 2026-10-26 is a Monday.
            label: weekdayName.format(Date.UTC(2026, 9, 26 + i)),
          }))}
          value={weekday}
          onChange={(d) => {
            setAsking(false)
            send({ weekday: d })
          }}
        />
        <p className="text-sm text-muted-foreground">
          {a.dayHint(a.nextOn(f.weekday(autopilot.next), date(autopilot.next)))}
        </p>
      </section>

      {on && (
        <section aria-labelledby={`${id}-drift`} className={card}>
          <h2 id={`${id}-drift`} className="font-semibold">
            {a.drift}
          </h2>
          <DriftMeter
            used={autopilot.drift.used_kcal}
            limit={limitKcal}
            step={autopilot.step_kcal}
          />
          <p className="text-sm text-muted-foreground">
            {autopilot.drift.agreed_kcal === undefined
              ? a.driftHintNoTarget
              : a.driftHint(f.number(autopilot.drift.agreed_kcal), date(autopilot.drift.agreed_on))}
          </p>
        </section>
      )}

      <section className={`${card} space-y-4`}>
        <Rules
          title={a.mayTitle}
          icon={<Check aria-hidden className="mt-0.5 size-4 shrink-0 text-success" />}
          items={[
            a.mayStep(f.number(step), pct(holds.step_pct ?? 0)),
            a.mayCarbs,
            limitKcal === undefined
              ? a.mayDriftNoBase(pct(driftPct))
              : a.mayDrift(f.number(limitKcal), pct(driftPct)),
          ]}
        />
        <Rules
          title={a.neverTitle}
          icon={<Ban aria-hidden className="mt-0.5 size-4 shrink-0 text-muted-foreground" />}
          items={a.never}
        />
        <Rules
          title={a.waitsTitle}
          icon={<Hourglass aria-hidden className="mt-0.5 size-4 shrink-0 text-muted-foreground" />}
          items={[
            a.waitsData(holds.logged_days ?? 0, holds.weigh_ins ?? 0),
            a.waitsTrust,
            a.waitsDays(holds.first_step_days ?? 0, holds.interval_days ?? 0),
            a.waitsPending,
          ]}
        />
        <p className="border-t pt-3 text-sm text-muted-foreground">{a.undoNote}</p>
      </section>

      <ListGroup>
        <ListRow
          icon={ShieldCheck}
          title={a.limitsRow}
          to="/plan/limits"
          summary={a.limitsSummary(f.number(step), pct(driftPct))}
        />
      </ListGroup>
    </>
  )
}

// Rules are a list of what the autopilot does, under its title.
function Rules({
  title,
  icon,
  items,
}: {
  title: string
  icon: ReactNode
  items: readonly string[]
}) {
  const id = useId()

  return (
    <section aria-labelledby={id} className="space-y-1.5">
      <h2 id={id} className="font-semibold">
        {title}
      </h2>
      <ul className="space-y-1.5">
        {items.map((item) => (
          <li key={item} className="flex gap-2 text-sm">
            {icon}
            <span>{item}</span>
          </li>
        ))}
      </ul>
    </section>
  )
}
