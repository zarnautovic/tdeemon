import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useId, useRef, useState, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import type { components } from '@/api/schema'
import { phaseFieldOf, useViolationAt } from '@/components/plan/plan-edit'
import { useDate, useViolationText, violationsOf } from '@/components/plan/proposal-text'
import { Button } from '@/components/ui/button'
import { addDays, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { problem } from '@/i18n/problem'
import limits from '@/lib/limits.json'
import { fieldLabel, inputClass, linkTarget } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

type Plan = components['schemas']['PlanBody']
type Phase = components['schemas']['PhaseBody']
type PhaseType = Phase['type']

const types: readonly PhaseType[] = ['cut', 'maintain', 'bulk', 'recomp']

// The weights a goal or a milestone may be, as a weigh-in's.
const { least: leastKg, most: mostKg } = limits.metrics.weight

// The form's fields, as typed.
type Field = 'start' | 'rate' | 'end' | 'goal' | 'milestone'

// PhaseForm changes a phase of the plan, or plans the next one when phase
// is undefined: its type and first day (a new one's alone: a phase that
// has started keeps them), its rate, its end, and its goal and milestone
// weight. A next phase that starts before the planned end of the one
// before ends that one the day before. A rate is typed as how fast for a
// cut or a bulk, and with its sign for a recomp. What the limits refuse
// the server says at its field, and one of the user's own may be
// loosened in the limits, as their form asks. Saved, the plan has a
// revision of the user's, which the history takes back.
export function PhaseForm({
  plan,
  phase,
  onDone,
}: {
  plan: Plan
  phase?: Phase | undefined
  onDone: () => void
}) {
  const t = useMessages()
  const f = useFormat()
  const date = useDate()
  const id = useId()
  const queryClient = useQueryClient()
  const at = useViolationAt()
  const violation = useViolationText()
  const e = t.plan.edit
  const last = plan.phases.at(-1)
  const [type, setType] = useState<PhaseType>(phase?.type ?? 'maintain')
  const [typed, setTyped] = useState<Record<Field, string>>(() => ({
    start: phase?.start ?? nextStart(last, plan.today),
    rate:
      phase === undefined || phase.type === 'maintain'
        ? ''
        : f.field(phase.type === 'recomp' ? phase.rate_pct_week : Math.abs(phase.rate_pct_week)),
    end: phase?.end ?? '',
    goal: phase?.goal_weight === undefined ? '' : f.field(phase.goal_weight),
    milestone: phase?.milestone_weight === undefined ? '' : f.field(phase.milestone_weight),
  }))
  const [checked, setChecked] = useState(false)
  const form = useRef<HTMLFormElement>(null)
  const change = $api.useMutation('post', '/api/v1/plan/changes', {
    onSuccess: () => {
      void queryClient.invalidateQueries()
      sayChange({ title: phase === undefined ? e.saved.next : e.saved.phase })
      onDone()
    },
  })

  const rate = signed(typed.rate, f.parse)
  const weight = (s: string) => (s.trim() === '' ? null : f.parse(s))
  const goal = weight(typed.goal)
  const milestone = weight(typed.milestone)
  const start = phase?.start ?? typed.start
  const yesterday = addDays(plan.today, -1)
  // What is wrong of each field, before it is sent.
  const wrong: Partial<Record<Field, string>> = {}
  if (phase === undefined && (typed.start === '' || typed.start < plan.today)) {
    wrong.start = e.problem.startPast
  }
  if (type !== 'maintain' && rate === undefined) {
    wrong.rate = e.problem.number
  }
  if (typed.end !== '' && typed.end < start) {
    wrong.end = e.problem.endBefore
  } else if (typed.end !== '' && typed.end < yesterday) {
    wrong.end = e.problem.endPast
  }
  for (const [k, kg] of [
    ['goal', goal],
    ['milestone', milestone],
  ] as const) {
    if (kg === undefined || (kg !== null && (kg < leastKg || kg > mostKg))) {
      wrong[k] = e.problem.range(f.number(leastKg), f.number(mostKg))
    }
  }
  const refused = change.isError ? violationsOf(change.error) : []
  const refusedAt = (k: Field) => refused.filter((v) => phaseFieldOf(v) === k)
  const refusedForm = refused.filter((v) => phaseFieldOf(v) === undefined)
  const loosen = refused.some((v) => v.level === 'user')

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    setChecked(true)
    if (Object.keys(wrong).length > 0) {
      requestAnimationFrame(() => {
        form.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus()
      })
      return
    }
    const sign = type === 'cut' ? -1 : 1
    const set = {
      start,
      type,
      rate_pct_week:
        type === 'maintain' ? 0 : sign * (type === 'recomp' ? (rate ?? 0) : Math.abs(rate ?? 0)),
      ...(typed.end === '' ? {} : { end: typed.end }),
      ...(goal === null || goal === undefined ? {} : { goal_weight: goal }),
      ...(milestone === null || milestone === undefined ? {} : { milestone_weight: milestone }),
    }
    // A next phase ends the one before it the day before it starts, if
    // that one was to run on.
    const before = phase === undefined ? plan.phases.findLast((p) => p.start < start) : undefined
    const ended =
      before?.end !== undefined && before.end >= start
        ? [{ ...before, end: addDays(start, -1) }]
        : []
    change.mutate({ body: { phases: [...ended, set] } })
  }

  // field is a field's input and what is wrong of it: the form's check
  // once it was saved, and the server's refusal.
  const field = (k: Field, label: string, input: { type?: string; hint?: string } = {}) => {
    const said = [...(checked && wrong[k] !== undefined ? [wrong[k]] : []), ...refusedAt(k).map(at)]
    const errorId = `${id}-${k}`
    const hintId = `${id}-${k}-hint`
    return (
      <div className="space-y-1">
        <label className="block space-y-1">
          <span className={cn(fieldLabel, 'block')}>{label}</span>
          <input
            type={input.type ?? 'text'}
            value={typed[k]}
            {...(input.type === undefined ? { inputMode: 'decimal' as const } : {})}
            autoComplete="off"
            aria-invalid={said.length > 0}
            aria-describedby={
              [...(said.length > 0 ? [errorId] : []), ...(input.hint ? [hintId] : [])].join(' ') ||
              undefined
            }
            className={cn(inputClass, input.type === undefined && 'tabular-nums')}
            onChange={(ev) => {
              const value = ev.currentTarget.value
              change.reset()
              setTyped((was) => ({ ...was, [k]: value }))
            }}
          />
        </label>
        {input.hint !== undefined && (
          <p id={hintId} className="text-caption text-muted-foreground">
            {input.hint}
          </p>
        )}
        {said.length > 0 && (
          <p id={errorId} className="text-caption text-destructive">
            {said.join(' ')}
          </p>
        )}
      </div>
    )
  }

  return (
    <form ref={form} onSubmit={submit} noValidate className="space-y-4">
      {phase === undefined ? (
        <>
          <label className="block space-y-1">
            <span className={cn(fieldLabel, 'block')}>{e.type}</span>
            <select
              value={type}
              className={inputClass}
              onChange={(ev) => {
                const value = types.find((ty) => ty === ev.currentTarget.value)
                if (value !== undefined) {
                  change.reset()
                  setType(value)
                }
              }}
            >
              {types.map((ty) => (
                <option key={ty} value={ty}>
                  {t.plan.change.types[ty]}
                </option>
              ))}
            </select>
          </label>
          {field('start', e.start, { type: 'date' })}
        </>
      ) : (
        <p className="font-medium">{e.since(t.plan.change.types[phase.type], date(phase.start))}</p>
      )}
      {type !== 'maintain' &&
        field('rate', type === 'cut' ? e.loss : type === 'bulk' ? e.gain : e.rate)}
      {field('end', e.end, { type: 'date', hint: e.endHint })}
      <div className="grid grid-cols-2 gap-2">
        {field('goal', e.goal)}
        {field('milestone', e.milestone)}
      </div>
      {change.isError && (
        <div role="alert" className="space-y-1 text-sm text-destructive">
          {refused.length > 0 ? (
            <>
              <p>{e.refused}</p>
              {refusedForm.map((v) => (
                <p key={`${v.rule} ${v.day_type ?? ''}`}>{violation(v)}</p>
              ))}
            </>
          ) : (
            <p>
              {statusOf(change.error) === 422
                ? t.plan.refused(detailOf(change.error))
                : problem(t, change.error)}
            </p>
          )}
        </div>
      )}
      {loosen && (
        <Link to="/plan/limits" className={cn(linkTarget, 'block text-sm text-primary underline')}>
          {e.loosen}
        </Link>
      )}
      <Button type="submit" className="w-full" disabled={change.isPending}>
        {change.isPending ? e.saving : e.save}
      </Button>
    </form>
  )
}

// nextStart is the day a next phase starts by default: the day after the
// last phase's planned end, or after its first day for one that runs on;
// never before tomorrow.
function nextStart(last: Phase | undefined, today: string): string {
  const after = last === undefined ? today : addDays(last.end ?? last.start, 1)
  const tomorrow = addDays(today, 1)
  return after > tomorrow ? after : tomorrow
}

// signed reads a number typed with its sign, a minus or a minus sign
// before it: parse reads the rest.
function signed(s: string, parse: (s: string) => number | undefined): number | undefined {
  const typed = s.trim()
  const minus = /^[-−]/.test(typed)
  const n = parse(minus ? typed.slice(1) : typed)
  return n === undefined ? undefined : minus ? -n : n
}
