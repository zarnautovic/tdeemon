import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState, type SubmitEvent } from 'react'
import { flushSync } from 'react-dom'

import { $api } from '@/api/client'
import { statusOf } from '@/api/errors'
import { sessionQuery } from '@/api/session'
import { SaveStatus } from '@/components/day/save-status'
import { LoadError } from '@/components/load-error'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

const scopes = ['ingest', 'read', 'tools'] as const
type Scope = (typeof scopes)[number]

const tokens = ['get', '/api/v1/tokens']

// useCreateToken makes a token, and asks for the list again.
function useCreateToken() {
  const queryClient = useQueryClient()
  return $api.useMutation('post', '/api/v1/tokens', {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: tokens }),
  })
}

// ApiTokens are the user's tokens, with when each was used last: one is
// made, shown once, or revoked, and all at once, as after a password
// reset. A question before a revoke takes the focus to its safe answer,
// and the focus comes back to what asked it, or to the heading once the
// token is gone.
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
  // The token asked to be revoked, until it is or is kept; all of them.
  const [asking, setAsking] = useState<string | undefined>(undefined)
  const [askingAll, setAskingAll] = useState(false)
  const done = () => {
    setAsking(undefined)
    setAskingAll(false)
    heading.current?.focus()
  }
  // The token just made, shown until it no longer works, and what
  // revoking all of them came to, until a new one is made.
  const create = useCreateToken()
  const revokeAll = $api.useMutation('delete', '/api/v1/tokens', {
    onSuccess: async () => {
      create.reset()
      done()
      await asked()
    },
  })
  const revoke = $api.useMutation('delete', '/api/v1/tokens/{id}', {
    onSuccess: async (_, { params }) => {
      if (create.data?.id === params.path.id) {
        create.reset()
      }
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

  let body
  if (list.isPending) {
    body = <output className="block text-muted-foreground">{t.loading}</output>
  } else if (list.isError) {
    body = <LoadError error={list.error} onRetry={() => void list.refetch()} />
  } else if (list.data.tokens.length === 0) {
    body = <p className="text-sm text-muted-foreground">{t.settings.noTokens}</p>
  } else {
    body = (
      <ul className="divide-y rounded-lg border">
        {list.data.tokens.map((tok) => (
          <li key={tok.id} className="space-y-1 px-3 py-2 text-sm">
            <div className="flex items-center justify-between gap-2">
              <span className="font-medium">{tok.name}</span>
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
            <p className="text-muted-foreground">
              {tok.scopes.map((s) => t.settings.scope[s]).join(' · ')} ·{' '}
              {tok.last_used_at
                ? t.settings.lastUsed(
                    `${f.day(tok.last_used_at, timeZone)} ${f.time(tok.last_used_at, timeZone)}`,
                  )
                : t.settings.neverUsed}
            </p>
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
        ))}
      </ul>
    )
  }

  return (
    <section aria-labelledby="settings-tokens" className="space-y-3">
      <h2 id="settings-tokens" ref={heading} tabIndex={-1} className="font-medium outline-none">
        {t.settings.tokens}
      </h2>
      <p className="text-sm text-muted-foreground">{t.settings.tokensHint}</p>
      {body}
      {revoke.isError && statusOf(revoke.error) !== 404 && <SaveStatus change={revoke} />}
      <NewToken
        create={create}
        onCreated={() => {
          revokeAll.reset()
        }}
      />
      {list.isSuccess && list.data.tokens.length > 0 && (
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
      <span id={id}>{question}</span>
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

// NewToken makes a token of a name and scopes, and shows it, once. A
// browser that cannot copy it (no clipboard over plain http, or one
// refused) selects it for the user to copy.
function NewToken({
  create,
  onCreated,
}: {
  create: ReturnType<typeof useCreateToken>
  onCreated: () => void
}) {
  const t = useMessages()
  const id = useId()
  const [name, setName] = useState('')
  const [chosen, setChosen] = useState<ReadonlySet<Scope>>(new Set(['ingest']))
  const [copied, setCopied] = useState<'yes' | 'failed' | undefined>(undefined)
  const code = useRef<HTMLElement>(null)

  async function copy(token: string) {
    try {
      await navigator.clipboard.writeText(token)
      setCopied('yes')
    } catch {
      if (code.current) {
        window.getSelection()?.selectAllChildren(code.current)
      }
      setCopied('failed')
    }
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    setCopied(undefined)
    create.mutate(
      { body: { name: name.trim(), scopes: scopes.filter((s) => chosen.has(s)) } },
      {
        onSuccess: () => {
          setName('')
          onCreated()
        },
      },
    )
  }

  return (
    <form onSubmit={submit} className="space-y-2 rounded-lg border p-3">
      <label className="block space-y-1">
        <span className={cn(fieldLabel, 'block')}>{t.settings.tokenName}</span>
        <input
          value={name}
          maxLength={100}
          autoComplete="off"
          placeholder="iPhone"
          className={inputClass}
          onChange={(e) => {
            setName(e.currentTarget.value)
          }}
        />
      </label>
      <fieldset className="space-y-1">
        <legend className={fieldLabel}>{t.settings.scopes}</legend>
        {scopes.map((s) => (
          <label key={s} className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
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
      <SaveStatus change={create} />
      <Button type="submit" disabled={create.isPending || name.trim() === '' || chosen.size === 0}>
        {t.settings.create}
      </Button>
      {/* There before the token comes, so a screen reader says that it
          did, as the focus stays on the form; the token itself it does not
          read out. */}
      <output id={`${id}-made`} className="block text-sm">
        {create.isSuccess && t.settings.created}
      </output>
      {create.isSuccess && (
        <div className="space-y-1 rounded-lg bg-muted p-2 text-sm">
          <div className="flex items-center gap-2">
            <code
              ref={code}
              aria-describedby={`${id}-made`}
              className="min-w-0 flex-1 break-all select-all"
            >
              {create.data.token}
            </code>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void copy(create.data.token)
              }}
            >
              {copied === 'yes' ? t.settings.copied : t.settings.copy}
            </Button>
          </div>
          {copied === 'failed' && (
            <p role="alert" className="text-destructive">
              {t.settings.copyFailed}
            </p>
          )}
        </div>
      )}
    </form>
  )
}
