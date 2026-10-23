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
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

type User = Session['user']

// zones are the time zones to choose from: the browser's own list, with
// the user's in it whatever it holds.
function zones(current: string): string[] {
  const all = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : []
  return all.includes(current) ? all : [current, ...all]
}

const hours = Array.from({ length: 24 }, (_, h) => h)

// YouForm changes the user's name, the app's language, their time zone
// and the hour their day starts. Saved, the session holds the user as
// changed, and the days are asked again, as their bounds may have moved.
export function YouForm({ user }: { user: User }) {
  const t = useMessages()
  const id = useId()
  const queryClient = useQueryClient()
  const [name, setName] = useState(user.name)
  const [locale, setChosen] = useState<Locale>(user.locale)
  const [timeZone, setTimeZone] = useState(user.timezone)
  const [dayStart, setDayStart] = useState(user.day_start_hour)
  const save = $api.useMutation('patch', '/api/v1/me', {
    onSuccess: async (answer) => {
      queryClient.setQueryData(sessionQuery.queryKey, (s: Session | undefined) =>
        s ? { ...s, user: answer.user } : s,
      )
      setLocale(answer.user.locale)
      await queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/me'] })
      await queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/days/{date}'] })
    },
  })
  const refused = save.isError && statusOf(save.error) === 422

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    save.mutate({
      body: { name: name.trim(), locale, timezone: timeZone, day_start_hour: dayStart },
    })
  }

  return (
    <form onSubmit={submit} className="space-y-3" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className="font-medium">
        {t.settings.you}
      </h2>
      <label className="block space-y-1">
        <span className={cn(fieldLabel, 'block')}>{t.settings.name}</span>
        <input
          value={name}
          maxLength={100}
          autoComplete="name"
          className={inputClass}
          onChange={(e) => {
            setName(e.currentTarget.value)
          }}
        />
      </label>
      <label className="block space-y-1">
        <span className={cn(fieldLabel, 'block')}>{t.settings.language}</span>
        <select
          value={locale}
          className={inputClass}
          onChange={(e) => {
            const chosen = locales.find((l) => l.locale === e.currentTarget.value)
            if (chosen) {
              setChosen(chosen.locale)
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
        <select
          value={timeZone}
          className={inputClass}
          onChange={(e) => {
            setTimeZone(e.currentTarget.value)
          }}
        >
          {zones(user.timezone).map((z) => (
            <option key={z} value={z}>
              {z}
            </option>
          ))}
        </select>
      </label>
      <div className="space-y-1">
        <label className="block space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.settings.dayStart}</span>
          <select
            value={dayStart}
            aria-describedby={`${id}-start`}
            className={inputClass}
            onChange={(e) => {
              setDayStart(Number(e.currentTarget.value))
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
