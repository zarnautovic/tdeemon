import { useQueryClient } from '@tanstack/react-query'
import { useId, useState, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import { sessionQuery } from '@/api/session'
import type { Session } from '@/api/sign-in'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { locales, setLocale, type Locale } from '@/i18n/locale'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { fieldLabel, inputClass } from '@/lib/styles'
import { nameNow } from '@/lib/time-zones'
import { cn } from '@/lib/utils'

type User = Session['user']

// zones are the time zones to choose from: the browser's own list under
// the names IANA gives them now, with UTC, the browser's zone and the
// user's in it whatever it holds, an old name too (V8's has no UTC);
// undefined for a browser without a list, where the zone is typed, which
// the server checks.
function zones(current: string): string[] | undefined {
  if (typeof Intl.supportedValuesOf !== 'function') {
    return undefined
  }
  const own = Intl.DateTimeFormat().resolvedOptions().timeZone
  return [
    ...new Set([...Intl.supportedValuesOf('timeZone').map(nameNow), 'UTC', nameNow(own), current]),
  ].toSorted()
}

const hours = Array.from({ length: 24 }, (_, h) => h)

// Edits are the settings the user changed in the form, as typed.
type Edits = Partial<{ name: string; locale: Locale; timezone: string; day_start_hour: number }>

// YouForm changes the user's name, the app's language, their time zone
// and the hour their day starts. A field shows the user as the session
// has them until it is changed, and only the fields changed are sent, so
// what another tab changed meanwhile stays and shows. Saved, the session holds the user as changed
// and the user is asked again; a day, whose bounds may have moved, is
// asked whenever it is shown.
export function YouForm({ user }: { user: User }) {
  const t = useMessages()
  const id = useId()
  const queryClient = useQueryClient()
  const [edits, setEdits] = useState<Edits>({})
  // A name cleared, which the server refuses.
  const [noName, setNoName] = useState(false)
  const name = edits.name ?? user.name
  const locale = edits.locale ?? user.locale
  const timeZone = edits.timezone ?? user.timezone
  const dayStart = edits.day_start_hour ?? user.day_start_hour
  const edit = (e: Edits) => {
    setEdits((es) => ({ ...es, ...e }))
  }
  const save = $api.useMutation('patch', '/api/v1/me', {
    onSuccess: async (answer) => {
      // An edit the user now has is no longer one; one made since stays.
      const now = answer.user
      setEdits((es) => ({
        ...(es.name === undefined || es.name.trim() === now.name ? {} : { name: es.name }),
        ...(es.locale === undefined || es.locale === now.locale ? {} : { locale: es.locale }),
        ...(es.timezone === undefined || es.timezone === now.timezone
          ? {}
          : { timezone: es.timezone }),
        ...(es.day_start_hour === undefined || es.day_start_hour === now.day_start_hour
          ? {}
          : { day_start_hour: es.day_start_hour }),
      }))
      queryClient.setQueryData(sessionQuery.queryKey, (s: Session | undefined) =>
        s ? { ...s, user: answer.user } : s,
      )
      setLocale(answer.user.locale)
      await queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/me'] })
    },
  })
  const refused = save.isError && statusOf(save.error) === 422
  const choices = zones(user.timezone)

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const typed = edits.name?.trim()
    setNoName(typed === '')
    if (typed === '') {
      return
    }
    save.mutate({
      body: {
        ...(typed === undefined || typed === user.name ? {} : { name: typed }),
        ...(edits.locale === undefined || edits.locale === user.locale
          ? {}
          : { locale: edits.locale }),
        ...(edits.timezone === undefined || edits.timezone === user.timezone
          ? {}
          : { timezone: edits.timezone }),
        ...(edits.day_start_hour === undefined || edits.day_start_hour === user.day_start_hour
          ? {}
          : { day_start_hour: edits.day_start_hour }),
      },
    })
  }

  return (
    <form onSubmit={submit} className="space-y-3" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className="font-medium">
        {t.settings.you}
      </h2>
      <div className="space-y-1">
        <label className="block space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.settings.name}</span>
          <input
            value={name}
            maxLength={limits.user.maxNameLen}
            autoComplete="name"
            aria-invalid={noName || undefined}
            aria-describedby={noName ? `${id}-name` : undefined}
            className={inputClass}
            onChange={(e) => {
              edit({ name: e.currentTarget.value })
              setNoName(false)
            }}
          />
        </label>
        {noName && (
          <p id={`${id}-name`} role="alert" className="text-sm text-destructive">
            {t.settings.nameNeeded}
          </p>
        )}
      </div>
      <label className="block space-y-1">
        <span className={cn(fieldLabel, 'block')}>{t.settings.language}</span>
        <select
          value={locale}
          className={inputClass}
          onChange={(e) => {
            const chosen = locales.find((l) => l.locale === e.currentTarget.value)
            if (chosen) {
              edit({ locale: chosen.locale })
            }
          }}
        >
          {locales.map((l) => (
            <option key={l.locale} value={l.locale} lang={l.locale}>
              {l.name}
            </option>
          ))}
        </select>
      </label>
      <label className="block space-y-1">
        <span className={cn(fieldLabel, 'block')}>{t.settings.timeZone}</span>
        {choices ? (
          <select
            value={timeZone}
            className={inputClass}
            onChange={(e) => {
              edit({ timezone: e.currentTarget.value })
            }}
          >
            {choices.map((z) => (
              <option key={z} value={z}>
                {z}
              </option>
            ))}
          </select>
        ) : (
          <input
            value={timeZone}
            autoComplete="off"
            spellCheck={false}
            placeholder="Europe/Zagreb"
            className={inputClass}
            onChange={(e) => {
              edit({ timezone: e.currentTarget.value.trim() })
            }}
          />
        )}
      </label>
      <div className="space-y-1">
        <label className="block space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.settings.dayStart}</span>
          <select
            value={dayStart}
            aria-describedby={`${id}-start`}
            className={inputClass}
            onChange={(e) => {
              edit({ day_start_hour: Number(e.currentTarget.value) })
            }}
          >
            {hours.map((h) => (
              <option key={h} value={h}>
                {`${String(h).padStart(2, '0')}:00`}
              </option>
            ))}
          </select>
        </label>
        <p id={`${id}-start`} className="text-xs text-muted-foreground">
          {t.settings.dayStartHint}
        </p>
      </div>
      {refused ? (
        <p role="alert" className="text-sm text-destructive">
          {t.settings.refused(detailOf(save.error))}
        </p>
      ) : (
        <SaveStatus change={save} />
      )}
      <Button type="submit" disabled={save.isPending}>
        {t.settings.save}
      </Button>
    </form>
  )
}
