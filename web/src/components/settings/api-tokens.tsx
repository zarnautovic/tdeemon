import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
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

// ApiTokens are the user's tokens, with when each was used last: one is
// made, shown once, or revoked, and all at once, as after a password
// reset.
export function ApiTokens() {
  const t = useMessages()
  const f = useFormat()
  const queryClient = useQueryClient()
  const session = useQuery(sessionQuery)
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const list = $api.useQuery('get', '/api/v1/tokens')
  const asked = () => queryClient.invalidateQueries({ queryKey: tokens })
  const revoke = $api.useMutation('delete', '/api/v1/tokens/{id}', { onSuccess: asked })
  const revokeAll = $api.useMutation('delete', '/api/v1/tokens', { onSuccess: asked })
  // The token asked to be revoked, until it is or is kept; all of them.
  const [asking, setAsking] = useState<string | undefined>(undefined)
  const [askingAll, setAskingAll] = useState(false)

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
                  variant="ghost"
                  size="sm"
                  aria-label={t.settings.revoke(tok.name)}
                  onClick={() => {
                    setAsking(tok.id)
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
              <div className="flex flex-wrap items-center gap-2">
                <span>{t.settings.confirmRevoke(tok.name)}</span>
                <Button
                  variant="destructive"
                  size="sm"
                  disabled={revoke.isPending}
                  onClick={() => {
                    revoke.mutate(
                      { params: { path: { id: tok.id } } },
                      {
                        onSuccess: () => {
                          setAsking(undefined)
                        },
                      },
                    )
                  }}
                >
                  {t.settings.yesRevoke}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setAsking(undefined)
                  }}
                >
                  {t.settings.keep}
                </Button>
              </div>
            )}
          </li>
        ))}
      </ul>
    )
  }

  return (
    <section aria-labelledby="settings-tokens" className="space-y-3">
      <h2 id="settings-tokens" className="font-medium">
        {t.settings.tokens}
      </h2>
      <p className="text-sm text-muted-foreground">{t.settings.tokensHint}</p>
      {body}
      {revoke.isError && <SaveStatus change={revoke} />}
      <NewToken />
      {list.isSuccess && list.data.tokens.length > 0 && (
        <div className="space-y-2 border-t pt-3">
          {askingAll ? (
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span>{t.settings.confirmRevokeAll}</span>
              <Button
                variant="destructive"
                disabled={revokeAll.isPending}
                onClick={() => {
                  revokeAll.mutate(
                    {},
                    {
                      onSuccess: () => {
                        setAskingAll(false)
                      },
                    },
                  )
                }}
              >
                {t.settings.yesRevoke}
              </Button>
              <Button
                variant="ghost"
                onClick={() => {
                  setAskingAll(false)
                }}
              >
                {t.settings.keep}
              </Button>
            </div>
          ) : (
            <Button
              variant="destructive"
              onClick={() => {
                setAskingAll(true)
              }}
            >
              {t.settings.revokeAll}
            </Button>
          )}
        </div>
      )}
      {revokeAll.isSuccess && (
        <output className="block text-sm">{t.settings.revoked(revokeAll.data.revoked)}</output>
      )}
      {revokeAll.isError && <SaveStatus change={revokeAll} />}
    </section>
  )
}

// NewToken makes a token of a name and scopes, and shows it, once.
function NewToken() {
  const t = useMessages()
  const id = useId()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [chosen, setChosen] = useState<ReadonlySet<Scope>>(new Set(['ingest']))
  const [copied, setCopied] = useState(false)
  const create = $api.useMutation('post', '/api/v1/tokens', {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: tokens }),
  })

  async function copy(token: string) {
    await navigator.clipboard.writeText(token)
    setCopied(true)
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    setCopied(false)
    create.mutate(
      { body: { name: name.trim(), scopes: scopes.filter((s) => chosen.has(s)) } },
      {
        onSuccess: () => {
          setName('')
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
      {create.isSuccess && (
        <div className="space-y-1 rounded-lg bg-muted p-2 text-sm">
          <p id={`${id}-made`}>{t.settings.created}</p>
          <div className="flex items-center gap-2">
            <code aria-describedby={`${id}-made`} className="min-w-0 flex-1 break-all">
              {create.data.token}
            </code>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void copy(create.data.token)
              }}
            >
              {copied ? t.settings.copied : t.settings.copy}
            </Button>
          </div>
        </div>
      )}
    </form>
  )
}
