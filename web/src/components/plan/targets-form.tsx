import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useId, useRef, useState, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import type { components } from '@/api/schema'
import { useWarningText } from '@/components/plan/change-words'
import { targetNow } from '@/components/plan/phases'
import {
  targetFieldOf,
  useViolationAt,
  type DayType,
  type TargetField,
} from '@/components/plan/plan-edit'
import { useViolationText, violationsOf } from '@/components/plan/proposal-text'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { problem } from '@/i18n/problem'
import limits from '@/lib/limits.json'
import { fieldLabel, inputClass, linkTarget } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

type Plan = components['schemas']['PlanBody']
type DayTarget = components['schemas']['DayTargetBody']

const days: readonly DayType[] = ['training', 'rest']
const fields: readonly TargetField[] = ['kcal', 'protein', 'fat']

type Typed = Record<DayType, Record<TargetField, string>>

// TargetsForm changes the plan's targets by type of day from today on:
// the kcal, the protein and the fat of each, the carbs following what the
// kcal leave. What it can check it checks before it sends; what the
// limits refuse the server says, each at its field, and one of the
// user's own may be loosened in the limits, as their form asks. Saved, the
// plan has a revision of the user's, which the history takes back.
export function TargetsForm({ plan, onDone }: { plan: Plan; onDone: () => void }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const queryClient = useQueryClient()
  const at = useViolationAt()
  const violation = useViolationText()
  const warning = useWarningText()
  const e = t.plan.edit
  const now = targetNow(plan.targets, plan.today)
  const [typed, setTyped] = useState<Typed>(() => ({
    training: typedOf(now?.training, f.field),
    rest: typedOf(now?.rest, f.field),
  }))
  // Whether a wrong field says so: from the first save on, so typing does
  // not scold.
  const [checked, setChecked] = useState(false)
  const form = useRef<HTMLFormElement>(null)
  const change = $api.useMutation('post', '/api/v1/plan/changes', {
    onSuccess: (answer) => {
      void queryClient.invalidateQueries()
      sayChange({
        title: e.saved.targets,
        lines: answer.preview.checks.flatMap((ck) => ck.warnings.map(warning)),
      })
      onDone()
    },
  })

  const parsed = (d: DayType) => ({
    kcal: f.parse(typed[d].kcal),
    protein: f.parse(typed[d].protein),
    fat: f.parse(typed[d].fat),
  })
  const values = { training: parsed('training'), rest: parsed('rest') }
  // A day's target as it is sent: the carbs are what the kcal leave, and
  // the server takes no other.
  const sent = (d: DayType) => ({
    kcal: values[d].kcal ?? 0,
    protein: values[d].protein ?? 0,
    fat: values[d].fat ?? 0,
  })
  const wrong = (d: DayType, k: TargetField): string | undefined => {
    const n = values[d][k]
    if (n === undefined) {
      return e.problem.number
    }
    if (k === 'kcal' && n <= 0) {
      return e.problem.positive
    }
    if (k === 'kcal' && n > limits.plan.maxKcal) {
      return e.problem.most(f.number(limits.plan.maxKcal))
    }
    return undefined
  }
  const carbsOf = (d: DayType): number | undefined => {
    const { kcal, protein, fat } = values[d]
    return kcal === undefined || protein === undefined || fat === undefined
      ? undefined
      : (kcal - 4 * protein - 9 * fat) / 4
  }
  // What the server refused, each at its field, and the rest under the
  // form; forgotten as soon as a field changes.
  const refused = change.isError ? violationsOf(change.error) : []
  const refusedAt = (d: DayType, k: TargetField) =>
    refused.filter((v) => {
      const of = targetFieldOf(v)
      return of?.field === k && of.days.includes(d)
    })
  const refusedForm = refused.filter((v) => targetFieldOf(v) === undefined)
  const loosen = refused.some((v) => v.level === 'user')

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    setChecked(true)
    const bad = days.some(
      (d) => fields.some((k) => wrong(d, k) !== undefined) || (carbsOf(d) ?? 0) < 0,
    )
    if (bad) {
      // The first wrong field takes the focus, after the render that marks it.
      requestAnimationFrame(() => {
        form.current?.querySelector<HTMLInputElement>('[aria-invalid="true"]')?.focus()
      })
      return
    }
    change.mutate({
      body: { targets: [{ from: plan.today, training: sent('training'), rest: sent('rest') }] },
    })
  }

  return (
    <form
      ref={form}
      onSubmit={submit}
      noValidate
      className="space-y-4"
      aria-describedby={`${id}-from`}
    >
      <p id={`${id}-from`} className="text-small text-muted-foreground">
        {e.fromToday}
      </p>
      {days.map((d) => {
        const carbs = carbsOf(d)
        return (
          <fieldset key={d} className="min-w-0 space-y-3 rounded-lg border p-3">
            <legend className="px-1 font-medium">{t.plan.targets[d]}</legend>
            <div className="grid grid-cols-3 gap-2">
              {fields.map((k) => {
                const local = checked ? wrong(d, k) : undefined
                const server = refusedAt(d, k).map(at)
                const said = [...(local === undefined ? [] : [local]), ...server]
                const errorId = `${id}-${d}-${k}`
                return (
                  <div key={k} className="min-w-0 space-y-1">
                    <label className="block space-y-1">
                      <span className={cn(fieldLabel, 'block')}>{e[k]}</span>
                      <input
                        value={typed[d][k]}
                        inputMode="decimal"
                        autoComplete="off"
                        aria-invalid={said.length > 0}
                        aria-describedby={said.length > 0 ? errorId : undefined}
                        className={cn(inputClass, 'tabular-nums')}
                        onChange={(ev) => {
                          const value = ev.currentTarget.value
                          change.reset()
                          setTyped((was) => ({ ...was, [d]: { ...was[d], [k]: value } }))
                        }}
                      />
                    </label>
                    {said.length > 0 && (
                      <p id={errorId} className="text-caption text-destructive">
                        {said.join(' ')}
                      </p>
                    )}
                  </div>
                )
              })}
            </div>
            <p
              className={cn(
                'text-small',
                carbs !== undefined && carbs < 0 && checked
                  ? 'text-destructive'
                  : 'text-muted-foreground',
              )}
            >
              {carbs !== undefined && carbs < 0 && checked
                ? e.problem.over
                : e.carbs(carbs === undefined ? '–' : f.grams(Math.max(carbs, 0)))}
            </p>
          </fieldset>
        )
      })}
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

// typedOf is a day's target as its fields start: each value as it is
// stored, or empty for none.
function typedOf(
  d: DayTarget | undefined,
  field: (n: number, digits?: number) => string,
): Record<TargetField, string> {
  return {
    kcal: d === undefined ? '' : field(d.kcal, 0),
    protein: d === undefined ? '' : field(d.protein, 1),
    fat: d === undefined ? '' : field(d.fat, 1),
  }
}
