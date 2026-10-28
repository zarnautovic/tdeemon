import { Accordion } from '@base-ui/react/accordion'
import { useQueryClient } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
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
import { Loosen } from '@/components/plan/loosen'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

const groups: readonly Group[] = ['rates', 'food', 'autopilot']

// namedIn is the first limit, in the form's order, a refusal of the
// server's names in its detail.
function namedIn(detail: string): LimitName | undefined {
  return limitFields.find((field) => new RegExp(`\\b${field.name}\\b`).test(detail))?.name
}

// LimitsForm changes the user's limits of a plan, each against the
// default and the instance's: a limit may be tightened freely, and
// loosened, no further than the instance's, once the user says so. Only
// the limits typed differently are sent. The limits come in groups, the
// pace, the food and the autopilot, each a panel that opens; the pace's
// is open. A limit that cannot be saved says why under its field, and the
// first such field, its panel opened, takes the focus and comes into
// view. labelledBy is the id of what names the form, the page's title.
export function LimitsForm({ body, labelledBy }: { body: LimitsBody; labelledBy: string }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const queryClient = useQueryClient()
  const limitsKey = $api.queryOptions('get', '/api/v1/limits').queryKey
  // What was typed in each field, by name; a field not typed shows the
  // user's limit.
  const [typed, setTyped] = useState<Partial<Record<LimitName, string>>>({})
  // The groups open, and each field, by name, to take the focus.
  const [open, setOpen] = useState<Group[]>(['rates'])
  const inputs = useRef(new Map<LimitName, HTMLInputElement>())
  // Whether the user is asked to loosen what the change loosens, and
  // whether the change was tried with nothing in it.
  const [asking, setAsking] = useState(false)
  const [unchanged, setUnchanged] = useState(false)

  // The field to take the focus once the form shows it, each ask new and
  // taken once.
  const [focus, setFocus] = useState<{ name: LimitName } | undefined>(undefined)
  const focused = useRef<{ name: LimitName } | undefined>(undefined)

  // reveal opens the group of a field, which then takes the focus.
  function reveal(name: LimitName) {
    const group = limitFields.find((field) => field.name === name)?.group
    if (group !== undefined) {
      setOpen((gs) => (gs.includes(group) ? gs : [...gs, group]))
    }
    setFocus({ name })
  }

  const save = $api.useMutation('put', '/api/v1/limits', {
    onSuccess: (answer) => {
      queryClient.setQueryData(limitsKey, answer)
      sayChange({ title: t.toast.saved })
      // A proposal's preview says what the limits refuse of it.
      void queryClient.invalidateQueries({
        queryKey: $api.queryOptions('get', '/api/v1/plan').queryKey,
      })
      setTyped({})
      setAsking(false)
    },
    onError: async (error) => {
      // Refused: the question is answered, and the refusal says why, at
      // the field it names.
      if (statusOf(error) === 422) {
        setAsking(false)
        const name = namedIn(detailOf(error))
        if (name !== undefined) {
          reveal(name)
        }
      }
      // The limits were changed elsewhere, so this loosens one after all:
      // read them again, and ask.
      if (statusOf(error) === 409) {
        setAsking(true)
        await queryClient.invalidateQueries({ queryKey: limitsKey })
      }
    },
  })
  // A field asked to take the focus takes it once its group is open and
  // the form no longer held while a change is sent: in the middle of the
  // screen, clear of a phone's bar.
  useEffect(() => {
    if (focus === undefined || focus === focused.current || save.isPending) {
      return
    }
    focused.current = focus
    const input = inputs.current.get(focus.name)
    input?.focus({ preventScroll: true })
    input?.scrollIntoView({ block: 'center' })
  }, [focus, save.isPending])
  const value = (field: LimitField, v: number) => f.field(shown(field, v))
  const label = (name: LimitName) => t.plan.limit[name]
  const result = check(body, typed, f.parse)
  const status = statusOf(save.error)
  const refused = save.isError && (status === 409 || status === 422)
  const refusal = refused ? t.plan.refused(named(detailOf(save.error), label)) : ''
  // The field a refusal of the server's names, which says it there.
  const refusedAt = save.isError && status === 422 ? namedIn(detailOf(save.error)) : undefined
  const loosening = result.loosened.length > 0

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
    const wrong = limitFields.find((field) => result.problems[field.name] !== undefined)
    if (wrong !== undefined) {
      setUnchanged(false)
      reveal(wrong.name)
      return
    }
    if (Object.keys(result.changes).length === 0) {
      setUnchanged(true)
      return
    }
    setUnchanged(false)
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
    <form onSubmit={submit} noValidate className="space-y-4" aria-labelledby={labelledBy}>
      <fieldset disabled={save.isPending} className="min-w-0">
        <Accordion.Root
          multiple
          value={open}
          onValueChange={(gs: Group[]) => {
            setOpen(gs)
          }}
          className="divide-y overflow-hidden rounded-lg border bg-card shadow-card"
        >
          {groups.map((group) => {
            const fields = limitFields.filter((field) => field.group === group)
            const wrong = fields.filter(
              (field) => result.problems[field.name] !== undefined || refusedAt === field.name,
            ).length
            const changed = fields.filter(
              (field) => body.limits[field.name] !== body.defaults[field.name],
            ).length
            return (
              <Accordion.Item key={group} value={group}>
                <Accordion.Header>
                  <Accordion.Trigger className="group flex min-h-14 w-full items-center gap-3 px-4 py-3 text-left outline-none hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset lg:px-5">
                    <span className="min-w-0 flex-1">
                      <span className="block font-semibold">{t.plan.groups[group]}</span>
                      <span className="block text-small text-muted-foreground">
                        {t.plan.groupHints[group]}
                      </span>
                    </span>
                    <span
                      className={cn(
                        'shrink-0 text-small',
                        wrong > 0 ? 'text-destructive' : 'text-muted-foreground',
                      )}
                    >
                      {wrong > 0 ? t.plan.toFix(wrong) : t.plan.changed(changed)}
                    </span>
                    <ChevronDown
                      aria-hidden
                      className="size-4 shrink-0 text-muted-foreground transition-transform group-data-panel-open:rotate-180"
                    />
                  </Accordion.Trigger>
                </Accordion.Header>
                <Accordion.Panel
                  keepMounted
                  className="h-(--accordion-panel-height) overflow-hidden transition-[height] duration-(--duration-base) ease-out data-ending-style:h-0 data-starting-style:h-0"
                >
                  <fieldset className="grid min-w-0 gap-5 border-t px-4 pt-4 pb-5 lg:grid-cols-2 lg:gap-x-8 lg:px-5">
                    <legend className="sr-only">{t.plan.groups[group]}</legend>
                    {fields.map((field) => {
                      const mine = body.limits[field.name] ?? 0
                      const text = typed[field.name] ?? value(field, mine)
                      const instance = body.instance[field.name]
                      const holds = body.effective[field.name]
                      const problem = result.problems[field.name]
                      const wrongHere = problem !== undefined || refusedAt === field.name
                      const hint = `${id}-${field.name}-hint`
                      const why = `${id}-${field.name}-wrong`
                      return (
                        <div key={field.name} className="space-y-1">
                          <label className="block space-y-1">
                            <span className={cn(fieldLabel, 'block text-foreground')}>
                              {label(field.name)}
                            </span>
                            <span className="flex items-center gap-2">
                              <input
                                ref={(el) => {
                                  if (el) {
                                    inputs.current.set(field.name, el)
                                  }
                                  return () => {
                                    inputs.current.delete(field.name)
                                  }
                                }}
                                value={text}
                                inputMode={
                                  field.signed ? 'text' : field.whole ? 'numeric' : 'decimal'
                                }
                                autoComplete="off"
                                aria-invalid={wrongHere || undefined}
                                aria-describedby={wrongHere ? `${hint} ${why}` : hint}
                                className={cn(inputClass, 'w-28 tabular-nums')}
                                onChange={(e) => {
                                  const next = e.currentTarget.value
                                  setTyped((ts) => ({ ...ts, [field.name]: next }))
                                  // What was sent, and said of it, is no
                                  // longer what is typed.
                                  setAsking(false)
                                  setUnchanged(false)
                                  save.reset()
                                }}
                              />
                              <span className="text-sm text-muted-foreground">
                                {unitOf(field, text)}
                              </span>
                            </span>
                          </label>
                          <p id={hint} className="text-caption text-muted-foreground">
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
                            <p id={why} className="text-sm text-destructive">
                              {problemText(field, problem)}
                            </p>
                          )}
                          {!problem && refusedAt === field.name && (
                            <p id={why} role="alert" className="text-sm text-destructive">
                              {refusal}
                            </p>
                          )}
                        </div>
                      )
                    })}
                  </fieldset>
                </Accordion.Panel>
              </Accordion.Item>
            )
          })}
        </Accordion.Root>
      </fieldset>
      {unchanged && (
        <p role="alert" className="text-sm">
          {t.plan.unchanged}
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
      {/* A refusal says why, at its field when it names one; a 409 is
          asked instead, unless the limits read again do not explain it. */}
      {refused && refusedAt === undefined && !(asking && loosening) && (
        <p role="alert" className="text-sm text-destructive">
          {refusal}
        </p>
      )}
      <Button type="submit" disabled={save.isPending}>
        {t.plan.save}
      </Button>
      {!refused && <SaveStatus change={save} />}
    </form>
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
