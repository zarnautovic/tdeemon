import { useQueryClient } from '@tanstack/react-query'
import { useId, useState, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import { sessionQuery } from '@/api/session'
import type { Session } from '@/api/sign-in'
import { SaveStatus } from '@/components/day/save-status'
import { TimeZoneField } from '@/components/settings/time-zone-field'
import { Button } from '@/components/ui/button'
import { locales, setLocale, type Locale } from '@/i18n/locale'
import { messagesOf, useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { fieldLabel, inputClass } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

type User = Session['user']

const hours = Array.from({ length: 24 }, (_, h) => h)

// Edits are the settings the user changed in the form, as typed.
type Edits = Partial<{ name: string; locale: Locale; timezone: string; day_start_hour: number }>

// YouForm changes the user's name, the app's language, their time zone
// and the hour their day starts. A field shows the user as the session
// has them until it is changed, and only the fields changed are sent, so
// what another tab changed meanwhile stays and shows. Saved, the session
// holds the user as changed and the user is asked again; a day, whose
// bounds may have moved, is asked whenever it is shown. labelledBy is the
// id of what names the form, the page's title.
export function YouForm({ user, labelledBy }: { user: User; labelledBy: string }) {
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
      sayChange({ title: messagesOf(answer.user.locale).toast.saved })
      await queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/me'] })
    },
  })
  const refused = save.isError && statusOf(save.error) === 422

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
    <form onSubmit={submit} className="space-y-4" aria-labelledby={labelledBy}>
      <div className="space-y-4 rounded-lg border bg-card p-4 lg:p-5">
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
        <p className="text-small text-muted-foreground">
          {t.settings.email}: {user.email}
        </p>
      </div>
      <div className="space-y-4 rounded-lg border bg-card p-4 lg:p-5">
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
        <TimeZoneField
          value={timeZone}
          stored={user.timezone}
          onChange={(zone) => {
            edit({ timezone: zone })
          }}
        />
        <div className="space-y-1">
          <label className="block space-y-1">
            <span className={cn(fieldLabel, 'block')}>{t.settings.dayStart}</span>
            <select
              value={dayStart}
              aria-describedby={`${id}-start`}
              className={cn(inputClass, 'tabular-nums')}
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
          <p id={`${id}-start`} className="text-caption text-muted-foreground">
            {t.settings.dayStartHint}
          </p>
        </div>
      </div>
      <Button type="submit" disabled={save.isPending}>
        {t.settings.save}
      </Button>
      {refused ? (
        <p role="alert" className="text-sm text-destructive">
          {t.settings.refused(detailOf(save.error))}
        </p>
      ) : (
        <SaveStatus change={save} />
      )}
    </form>
  )
}
