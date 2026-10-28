import { Copy } from 'lucide-react'
import { useEffect, useId, useRef, useState, type RefObject, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

const scopes = ['ingest', 'read', 'tools'] as const
type Scope = (typeof scopes)[number]

// NewToken makes a token of a name and scopes, in the sheet it opens in,
// and shows it there, once: closed, the sheet forgets it. The token is
// copied with a tap; a browser that cannot copy it (no clipboard over
// plain http, or one refused) selects it for the user to copy. first is
// the name's field, which the sheet focuses as it opens.
export function NewToken({
  first,
  onCreated,
  onDone,
}: {
  first: RefObject<HTMLInputElement | null>
  onCreated: () => void
  onDone: () => void
}) {
  const t = useMessages()
  const id = useId()
  const [name, setName] = useState('')
  const [chosen, setChosen] = useState<ReadonlySet<Scope>>(new Set(['ingest']))
  const create = $api.useMutation('post', '/api/v1/tokens', { onSuccess: onCreated })

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    create.mutate({ body: { name: name.trim(), scopes: scopes.filter((s) => chosen.has(s)) } })
  }

  return (
    <div className="space-y-4">
      {/* There before the token comes, so a screen reader says that it
          did; the token itself it does not read out. */}
      <output id={`${id}-made`} className="block text-small">
        {create.isSuccess && t.settings.created}
      </output>
      {create.isSuccess ? (
        <Made token={create.data.token} describedBy={`${id}-made`} onDone={onDone} />
      ) : (
        <form onSubmit={submit} className="space-y-4">
          <label className="block space-y-1">
            <span className={cn(fieldLabel, 'block')}>{t.settings.tokenName}</span>
            <input
              ref={first}
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
          <Button
            type="submit"
            className="w-full"
            disabled={create.isPending || name.trim() === '' || chosen.size === 0}
          >
            {t.settings.create}
          </Button>
          <SaveStatus change={create} />
        </form>
      )}
    </div>
  )
}

// Made is a token just made, shown this once, with a button to copy it,
// which takes the focus, and one to close the sheet.
function Made({
  token,
  describedBy,
  onDone,
}: {
  token: string
  describedBy: string
  onDone: () => void
}) {
  const t = useMessages()
  const [copied, setCopied] = useState<'yes' | 'failed' | undefined>(undefined)
  const code = useRef<HTMLElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    button.current?.focus()
  }, [])

  async function copy() {
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

  return (
    <div className="space-y-3">
      <code
        ref={code}
        aria-describedby={describedBy}
        className="block rounded-md bg-surface-2 p-3 font-mono text-small break-all select-all"
      >
        {token}
      </code>
      {copied === 'failed' && (
        <p role="alert" className="text-small text-destructive">
          {t.settings.copyFailed}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button ref={button} onClick={() => void copy()}>
          <Copy aria-hidden />
          {copied === 'yes' ? t.settings.copied : t.settings.copy}
        </Button>
        <Button variant="outline" onClick={onDone}>
          {t.settings.done}
        </Button>
      </div>
    </div>
  )
}
