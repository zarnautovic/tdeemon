import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Bot, Plus, Smartphone } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import { flushSync } from 'react-dom'

import { $api } from '@/api/client'
import { statusOf } from '@/api/errors'
import { sessionQuery } from '@/api/session'
import { SaveStatus } from '@/components/day/save-status'
import { LoadError } from '@/components/load-error'
import { NewToken } from '@/components/settings/new-token'
import { Sheet } from '@/components/sheet'
import { Skeleton } from '@/components/skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

const tokens = ['get', '/api/v1/tokens']

// ApiTokens are the devices and AI clients that sign in with a token of
// their own, a card each: its name, what it may do, and when it was used
// last. A new one is made in a sheet, which shows it once; one is
// revoked, or all at once, as after a password reset. A revoke cannot be
// taken back, so it asks first: the focus goes to its safe answer, and
// comes back to what asked it, or to the heading once the token is gone.
export function ApiTokens() {
  const t = useMessages()
  const f = useFormat()
  const queryClient = useQueryClient()
  const session = useQuery(sessionQuery)
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const list = $api.useQuery('get', '/api/v1/tokens')
  const asked = () => queryClient.invalidateQueries({ queryKey: tokens })
  const heading = useRef<HTMLHeadingElement>(null)
  // Each token's button that asks to revoke it, and the one for all.
  const openers = useRef(new Map<string, HTMLButtonElement>())
  const all = useRef<HTMLButtonElement>(null)
  // The button that opened the sheet of a new token, which the focus goes
  // back to.
  const adder = useRef<HTMLButtonElement>(null)
  const first = useRef<HTMLInputElement>(null)
  const [adding, setAdding] = useState(false)
  // The token asked to be revoked, until it is or is kept; all of them.
  const [asking, setAsking] = useState<string | undefined>(undefined)
  const [askingAll, setAskingAll] = useState(false)
  const done = () => {
    setAsking(undefined)
    setAskingAll(false)
    heading.current?.focus()
  }
  // What revoking all of them came to, until a new one is made.
  const revokeAll = $api.useMutation('delete', '/api/v1/tokens', {
    onSuccess: async () => {
      done()
      await asked()
    },
  })
  const revoke = $api.useMutation('delete', '/api/v1/tokens/{id}', {
    onSuccess: async () => {
      done()
      await asked()
    },
    // Revoked elsewhere already: it is gone all the same.
    onError: async (error) => {
      if (statusOf(error) === 404) {
        done()
        await asked()
      }
    },
  })

  // keep closes the question, and gives the focus to what focus finds
  // once the button that asked it is back.
  const keep = (focus: () => HTMLElement | null | undefined) => {
    flushSync(() => {
      setAsking(undefined)
      setAskingAll(false)
    })
    focus()?.focus()
  }

  const add = (
    <Button
      ref={adder}
      onClick={() => {
        setAdding(true)
      }}
    >
      <Plus aria-hidden />
      {t.settings.newToken}
    </Button>
  )
  const any = list.isSuccess && list.data.tokens.length > 0

  let body
  if (list.isPending) {
    body = <Skeleton blocks={['h-24', 'h-24']} />
  } else if (list.isError) {
    body = <LoadError error={list.error} onRetry={() => void list.refetch()} />
  } else if (list.data.tokens.length === 0) {
    body = (
      <div className="space-y-3 rounded-lg border border-dashed bg-card p-5">
        <p className="font-medium">{t.settings.noTokens}</p>
        <p className="text-small text-muted-foreground">{t.settings.noTokensHint}</p>
        {add}
      </div>
    )
  } else {
    body = (
      <ul className="space-y-3">
        {list.data.tokens.map((tok) => {
          const Icon = tok.scopes.every((s) => s === 'ingest') ? Smartphone : Bot
          return (
            <li key={tok.id} className="space-y-3 rounded-lg border bg-card p-4 shadow-card">
              <div className="flex items-start gap-3">
                <span
                  aria-hidden
                  className="flex size-9 shrink-0 items-center justify-center rounded-md bg-surface-2 text-muted-foreground"
                >
                  <Icon strokeWidth={1.75} className="size-4.5" />
                </span>
                <div className="min-w-0 flex-1">
                  <h3 className="truncate font-semibold">{tok.name}</h3>
                  <p className="text-small text-muted-foreground">
                    {tok.last_used_at
                      ? t.settings.lastUsed(
                          `${f.day(tok.last_used_at, timeZone)} ${f.time(tok.last_used_at, timeZone)}`,
                        )
                      : t.settings.neverUsed}
                  </p>
                </div>
                {asking === tok.id ? null : (
                  <Button
                    ref={(el) => {
                      if (el) {
                        openers.current.set(tok.id, el)
                      }
                      return () => {
                        openers.current.delete(tok.id)
                      }
                    }}
                    variant="ghost"
                    size="sm"
                    aria-label={t.settings.revoke(tok.name)}
                    onClick={() => {
                      setAsking(tok.id)
                      setAskingAll(false)
                    }}
                  >
                    {t.settings.yesRevoke}
                  </Button>
                )}
              </div>
              <ul aria-label={t.settings.scopes} className="flex flex-wrap gap-1.5">
                {tok.scopes.map((s) => (
                  <li key={s}>
                    <Badge tone="primary">{t.settings.scope[s]}</Badge>
                  </li>
                ))}
              </ul>
              {asking === tok.id && (
                <Ask
                  question={t.settings.confirmRevoke(tok.name)}
                  pending={revoke.isPending}
                  onYes={() => {
                    revoke.mutate({ params: { path: { id: tok.id } } })
                  }}
                  onKeep={() => {
                    keep(() => openers.current.get(tok.id) ?? heading.current)
                  }}
                />
              )}
            </li>
          )
        })}
      </ul>
    )
  }

  return (
    <section aria-labelledby="settings-tokens" className="space-y-3">
      <div className="flex min-h-11 items-center justify-between gap-2">
        <h2 id="settings-tokens" ref={heading} tabIndex={-1} className="text-h3 outline-none">
          {t.settings.tokens}
        </h2>
        {any && add}
      </div>
      {body}
      {revoke.isError && statusOf(revoke.error) !== 404 && <SaveStatus change={revoke} />}
      {any && (
        <div className="space-y-2 border-t pt-3">
          {askingAll ? (
            <Ask
              question={t.settings.confirmRevokeAll}
              pending={revokeAll.isPending}
              onYes={() => {
                revokeAll.mutate({})
              }}
              onKeep={() => {
                keep(() => all.current)
              }}
            />
          ) : (
            <Button
              ref={all}
              variant="destructive"
              onClick={() => {
                setAskingAll(true)
                setAsking(undefined)
              }}
            >
              {t.settings.revokeAll}
            </Button>
          )}
        </div>
      )}
      {/* There before what it says, so a screen reader says it. */}
      <output className="block text-sm">
        {revokeAll.isSuccess && t.settings.revoked(revokeAll.data.revoked)}
      </output>
      {revokeAll.isError && <SaveStatus change={revokeAll} />}
      <Sheet
        open={adding}
        onOpenChange={setAdding}
        title={t.settings.newToken}
        initialFocus={first}
        finalFocus={adder}
      >
        <NewToken
          first={first}
          onCreated={() => {
            revokeAll.reset()
            void asked()
          }}
          onDone={() => {
            setAdding(false)
          }}
        />
      </Sheet>
    </section>
  )
}

// Ask is the question before a revoke, which cannot be taken back: the
// focus goes to Keep, the safe answer, and the question describes both.
function Ask({
  question,
  pending,
  onYes,
  onKeep,
}: {
  question: string
  pending: boolean
  onYes: () => void
  onKeep: () => void
}) {
  const t = useMessages()
  const id = useId()
  const keep = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    keep.current?.focus()
  }, [])

  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      <span id={id} className="w-full">
        {question}
      </span>
      <Button
        variant="destructive"
        size="sm"
        aria-describedby={id}
        disabled={pending}
        onClick={onYes}
      >
        {t.settings.yesRevoke}
      </Button>
      <Button ref={keep} variant="ghost" size="sm" aria-describedby={id} onClick={onKeep}>
        {t.settings.keep}
      </Button>
    </div>
  )
}
