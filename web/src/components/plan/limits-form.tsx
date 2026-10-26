import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import { SaveStatus } from '@/components/day/save-status'
import {
  check,
  limitFields,
  shown,
  type Group,
  type LimitField,
  type LimitName,
  type LimitsBody,
  type Problem,
} from '@/components/plan/limit-fields'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

const groups: readonly Group[] = ['rates', 'food', 'autopilot']

// LimitsForm changes the user's limits of a plan, each against the
// default and the instance's: a limit may be tightened freely, and
// loosened, no further than the instance's, once the user says so. Only
// the limits typed differently are sent.
export function LimitsForm({ body }: { body: LimitsBody }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const queryClient = useQueryClient()
  const limitsKey = $api.queryOptions('get', '/api/v1/limits').queryKey
  // What was typed in each field, by name; a field not typed shows the
  // user's limit.
  const [typed, setTyped] = useState<Partial<Record<LimitName, string>>>({})
  // Whether the user is asked to loosen what the change loosens, and
  // whether the change was tried with nothing in it, or with a problem.
  const [asking, setAsking] = useState(false)
  const [tried, setTried] = useState<'unchanged' | 'wrong' | undefined>(undefined)
  const save = $api.useMutation('put', '/api/v1/limits', {
    onSuccess: (answer) => {
      queryClient.setQueryData(limitsKey, answer)
      // A proposal's preview says what the limits refuse of it.
      void queryClient.invalidateQueries({
        queryKey: $api.queryOptions('get', '/api/v1/plan').queryKey,
      })
      setTyped({})
      setAsking(false)
    },
    onError: async (error) => {
      // Refused: the question is answered, and the refusal says why.
      if (statusOf(error) === 422) {
        setAsking(false)
      }
      // The limits were changed elsewhere, so this loosens one after all:
      // read them again, and ask.
      if (statusOf(error) === 409) {
        setAsking(true)
        await queryClient.invalidateQueries({ queryKey: limitsKey })
      }
    },
  })
  const result = check(body, typed, f.parse)
  const status = statusOf(save.error)
  const refused = save.isError && (status === 409 || status === 422)
  const loosening = result.loosened.length > 0

  const value = (field: LimitField, v: number) => f.field(shown(field, v))
  const label = (name: LimitName) => t.plan.limit[name]
  // unitOf is a field's unit, which agrees with the number typed in it:
  // 1 day, 2 days.
  const unitOf = (field: LimitField, typedIn: string) => {
    const unit = t.plan.unit[field.unit]
    return typeof unit === 'string' ? unit : unit(f.parse(typedIn) ?? Number.NaN)
  }

  function send(confirm: boolean) {
    save.mutate({ body: { limits: result.changes, ...(confirm ? { confirm: true } : {}) } })
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (Object.keys(result.problems).length > 0) {
      setTried('wrong')
      return
    }
    if (Object.keys(result.changes).length === 0) {
      setTried('unchanged')
      return
    }
    setTried(undefined)
    if (loosening) {
      setAsking(true)
      return
    }
    send(false)
  }

  function problemText(field: LimitField, p: Problem): string {
    if (p.kind === 'held') {
      return t.plan.problem[p.to](label(p.other), value(field, p.value))
    }
    if (p.kind === 'order') {
      return t.plan.problem.order(label(p.other))
    }
    if (p.kind === 'range') {
      return t.plan.problem.range(f.number(p.min), f.number(p.max))
    }
    if (p.kind === 'instance') {
      return t.plan.problem.instance(value(field, p.value))
    }
    return t.plan.problem[p.kind]
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-4" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className="font-medium">
        {t.plan.limits}
      </h2>
      <p className="text-sm text-muted-foreground">{t.plan.limitsHint}</p>
      <fieldset disabled={save.isPending} className="min-w-0 space-y-4">
        {groups.map((group) => (
          <fieldset key={group} className="min-w-0 space-y-3">
            <legend className="mb-2 font-medium">{t.plan.groups[group]}</legend>
            {limitFields
              .filter((field) => field.group === group)
              .map((field) => {
                const mine = body.limits[field.name] ?? 0
                const text = typed[field.name] ?? value(field, mine)
                const instance = body.instance[field.name]
                const holds = body.effective[field.name]
                const problem = result.problems[field.name]
                const hint = `${id}-${field.name}-hint`
                const wrong = `${id}-${field.name}-wrong`
                return (
                  <div key={field.name} className="space-y-1">
                    <label className="block space-y-1">
                      <span className={cn(fieldLabel, 'block')}>{label(field.name)}</span>
                      <span className="flex items-center gap-2">
                        <input
                          value={text}
                          inputMode={field.signed ? 'text' : field.whole ? 'numeric' : 'decimal'}
                          autoComplete="off"
                          aria-invalid={problem ? true : undefined}
                          aria-describedby={problem ? `${hint} ${wrong}` : hint}
                          className={cn(inputClass, 'w-28 tabular-nums')}
                          onChange={(e) => {
                            const next = e.currentTarget.value
                            setTyped((ts) => ({ ...ts, [field.name]: next }))
                            // What was sent, and said of it, is no longer
                            // what is typed.
                            setAsking(false)
                            setTried(undefined)
                            save.reset()
                          }}
                        />
                        <span className="text-sm text-muted-foreground">{unitOf(field, text)}</span>
                      </span>
                    </label>
                    <p id={hint} className="text-xs text-muted-foreground">
                      {[
                        t.plan.defaults(value(field, body.defaults[field.name] ?? 0)),
                        ...(instance === undefined
                          ? []
                          : [t.plan.instance(value(field, instance))]),
                      ].join(' · ')}
                      {holds !== undefined && holds !== mine && (
                        <>
                          {' · '}
                          <strong className="font-medium">
                            {t.plan.holds(value(field, holds))}
                          </strong>
                        </>
                      )}
                    </p>
                    {problem && (
                      <p id={wrong} className="text-sm text-destructive">
                        {problemText(field, problem)}
                      </p>
                    )}
                  </div>
                )
              })}
          </fieldset>
        ))}
      </fieldset>
      {tried && (
        <p role="alert" className={cn('text-sm', tried === 'wrong' && 'text-destructive')}>
          {tried === 'wrong' ? t.plan.wrong : t.plan.unchanged}
        </p>
      )}
      {asking && loosening && (
        <Loosen
          names={result.loosened.map(label).join(', ')}
          pending={save.isPending}
          onYes={() => {
            send(true)
          }}
          onBack={() => {
            setAsking(false)
            save.reset()
          }}
        />
      )}
      {/* A refusal says why; a 409 is asked instead, unless the limits
          read again do not explain it. */}
      {refused && !(asking && loosening) && (
        <p role="alert" className="text-sm text-destructive">
          {t.plan.refused(named(detailOf(save.error), label))}
        </p>
      )}
      {!refused && <SaveStatus change={save} />}
      <Button type="submit" disabled={save.isPending}>
        {t.plan.save}
      </Button>
    </form>
  )
}

// Loosen asks the user to loosen what a change loosens: the focus goes
// to Back, the safe answer, and the question describes both answers.
function Loosen({
  names,
  pending,
  onYes,
  onBack,
}: {
  names: string
  pending: boolean
  onYes: () => void
  onBack: () => void
}) {
  const t = useMessages()
  const id = useId()
  const back = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    back.current?.focus()
  }, [])

  return (
    <div className="space-y-2 rounded-lg border border-amber-500/50 p-3 text-sm">
      <p id={id}>{t.plan.loosen(names)}</p>
      <div className="flex gap-2">
        <Button variant="destructive" aria-describedby={id} disabled={pending} onClick={onYes}>
          {t.plan.yesLoosen}
        </Button>
        {/* Not while the loosening is sent: it may be taken all the same. */}
        <Button
          ref={back}
          variant="ghost"
          aria-describedby={id}
          disabled={pending}
          onClick={onBack}
        >
          {t.plan.keep}
        </Button>
      </div>
    </div>
  )
}

// named is the server's detail with the limits' names in it given as
// the page calls them.
function named(detail: string, label: (name: LimitName) => string): string {
  return limitFields.reduce(
    (s, field) => s.replaceAll(new RegExp(`\\b${field.name}\\b`, 'g'), `"${label(field.name)}"`),
    detail,
  )
}
