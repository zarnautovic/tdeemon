import { Bot, ChevronLeft, ChevronRight, KeyRound, Smartphone } from 'lucide-react'
import { useEffect, useId, useRef, useState, type RefObject, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { SaveStatus } from '@/components/day/save-status'
import {
  connectKinds,
  scopes,
  type ConnectKind,
  type Scope,
} from '@/components/settings/connect-kinds'
import { ConnectSteps } from '@/components/settings/connect-steps'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

const icons = { coach: Bot, phone: Smartphone, other: KeyRound } as const

// The scope a kind's access key takes: an AI coach's reads and logs
// through MCP, a phone's sends health data.
const scopeOf = { coach: 'tools', phone: 'ingest' } as const satisfies Record<
  Exclude<ConnectKind, 'other'>,
  Scope
>

function useCreateKey(onCreated: () => void) {
  return $api.useMutation('post', '/api/v1/tokens', { onSuccess: onCreated })
}
type CreateKey = ReturnType<typeof useCreateKey>

// Connect connects an AI coach or a phone, in the sheet it opens in: what
// it is (an AI coach, an iPhone with Health Auto Export, or another of
// the scopes the user picks), then its access key, named (an AI coach's
// Claude, a phone's iPhone) and with the scope its kind takes, and the
// steps to give it the key, shown this once (ConnectSteps): closed, the
// sheet forgets it. start opens it at a kind, else at the choice; the
// sheet focuses the choice's first button (choice) or the name (name) as
// it opens. The instance's address, which the steps give, is asked for
// at once.
export function Connect({
  start,
  choice,
  name,
  onCreated,
  onDone,
}: {
  start?: ConnectKind | undefined
  choice: RefObject<HTMLButtonElement | null>
  name: RefObject<HTMLInputElement | null>
  onCreated: () => void
  onDone: () => void
}) {
  const t = useMessages()
  const id = useId()
  const [kind, setKind] = useState<ConnectKind | undefined>(start)
  // The kind picked last, whose button the choice focuses again.
  const [picked, setPicked] = useState<ConnectKind | undefined>(undefined)
  const create = useCreateKey(onCreated)
  // The steps' address, there by the time they come.
  $api.useQuery('get', '/api/v1/info')

  let step
  if (create.isSuccess && kind !== undefined) {
    step = <ConnectSteps kind={kind} made={create.data} onDone={onDone} />
  } else if (kind === undefined) {
    step = (
      <Choice
        first={choice}
        focus={picked}
        onPick={(k) => {
          setKind(k)
          setPicked(k)
        }}
      />
    )
  } else {
    step = (
      <KeyForm
        key={kind}
        kind={kind}
        field={name}
        create={create}
        onBack={() => {
          setKind(undefined)
        }}
      />
    )
  }

  return (
    <div className="space-y-4">
      {/* There before the key comes, so a screen reader says that it
          did; the key itself it does not read out. */}
      <output id={`${id}-made`} className="block text-small">
        {create.isSuccess && t.settings.created}
      </output>
      {step}
    </div>
  )
}

// Choice is what to connect, a button each, with what it is; focus is
// the one to focus as it shows, picked before.
function Choice({
  first,
  focus,
  onPick,
}: {
  first: RefObject<HTMLButtonElement | null>
  focus: ConnectKind | undefined
  onPick: (kind: ConnectKind) => void
}) {
  const t = useMessages()
  const id = useId()
  const again = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    again.current?.focus()
  }, [])

  return (
    <section aria-labelledby={id} className="space-y-2">
      <h3 id={id} className="font-semibold">
        {t.connect.choose}
      </h3>
      <ul className="space-y-2">
        {connectKinds.map((k, i) => {
          const Icon = icons[k]
          return (
            <li key={k}>
              <button
                ref={k === focus ? again : i === 0 ? first : undefined}
                type="button"
                className="flex min-h-14 w-full items-center gap-3 rounded-lg border bg-card px-3 py-2.5 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => {
                  onPick(k)
                }}
              >
                <span
                  aria-hidden
                  className="flex size-9 shrink-0 items-center justify-center rounded-md bg-surface-2 text-muted-foreground"
                >
                  <Icon strokeWidth={1.75} className="size-4.5" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block font-medium">{t.connect.kinds[k]}</span>
                  <span className="block text-small text-muted-foreground">
                    {t.connect.kindHints[k]}
                  </span>
                </span>
                <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" />
              </button>
            </li>
          )
        })}
      </ul>
    </section>
  )
}

// KeyForm makes the access key of a kind: its name, which an AI coach's
// and a phone's start with, and what it may do, its kind's scope, or
// those picked for another. The name takes the focus as it shows.
function KeyForm({
  kind,
  field,
  create,
  onBack,
}: {
  kind: ConnectKind
  field: RefObject<HTMLInputElement | null>
  create: CreateKey
  onBack: () => void
}) {
  const t = useMessages()
  const id = useId()
  const [name, setName] = useState(kind === 'other' ? '' : t.connect.names[kind])
  const [chosen, setChosen] = useState<ReadonlySet<Scope>>(() => new Set())
  const given: Scope[] = kind === 'other' ? scopes.filter((s) => chosen.has(s)) : [scopeOf[kind]]
  useEffect(() => {
    field.current?.focus()
  }, [field])

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    create.mutate({ body: { name: name.trim(), scopes: given } })
  }

  return (
    <form onSubmit={submit} aria-labelledby={id} className="space-y-4">
      <h3 id={id} className="font-semibold">
        {t.connect.kinds[kind]}
      </h3>
      <label className="block space-y-1">
        <span className={cn(fieldLabel, 'block')}>{t.settings.tokenName}</span>
        <input
          ref={field}
          value={name}
          maxLength={100}
          autoComplete="off"
          placeholder={t.settings.tokenPlaceholder}
          className={inputClass}
          onChange={(e) => {
            setName(e.currentTarget.value)
          }}
        />
      </label>
      {kind === 'other' ? (
        <fieldset className="space-y-1">
          <legend className={cn(fieldLabel, 'mb-1')}>{t.settings.scopes}</legend>
          {scopes.map((s) => (
            <label key={s} className="flex min-h-11 items-center gap-3 lg:min-h-9">
              <input
                type="checkbox"
                className="size-4.5 accent-primary"
                checked={chosen.has(s)}
                onChange={(e) => {
                  const next = new Set(chosen)
                  if (e.currentTarget.checked) {
                    next.add(s)
                  } else {
                    next.delete(s)
                  }
                  setChosen(next)
                }}
              />
              {t.settings.scope[s]}
            </label>
          ))}
        </fieldset>
      ) : (
        <div className="space-y-1">
          <p id={`${id}-scopes`} className={fieldLabel}>
            {t.settings.scopes}
          </p>
          <ul aria-labelledby={`${id}-scopes`} className="flex flex-wrap gap-1.5">
            {given.map((s) => (
              <li key={s}>
                <Badge tone="primary">{t.settings.scope[s]}</Badge>
              </li>
            ))}
          </ul>
        </div>
      )}
      <Button
        type="submit"
        className="w-full"
        disabled={create.isPending || name.trim() === '' || given.length === 0}
      >
        {t.settings.create}
      </Button>
      <SaveStatus change={create} />
      <Button type="button" variant="ghost" className="-ml-3" onClick={onBack}>
        <ChevronLeft aria-hidden />
        {t.connect.back}
      </Button>
    </form>
  )
}
