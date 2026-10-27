import { useQueryClient } from '@tanstack/react-query'
import { useId, useRef, useState, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import type { components } from '@/api/schema'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { fieldLabel, inputClass } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

type Profile = components['schemas']['ProfileBody']
type Sex = NonNullable<Profile['sex']>

const sexes: readonly Sex[] = ['male', 'female']

// The heights the server takes, in cm, and the oldest a user may be.
const { minHeightCm: minHeight, maxHeightCm: maxHeight, maxAge } = limits.profile

// ageOn is the age in whole years, on today, of one born on birth (both
// YYYY-MM-DD), as the server counts it (profile.AgeOn).
function ageOn(birth: string, today: string): number {
  const years = Number(today.slice(0, 4)) - Number(birth.slice(0, 4))
  return today.slice(5) < birth.slice(5) ? years - 1 : years
}

// ProfileForm sets what a plan needs of the user: the sex the energy
// equations take, the birth date and the height. A field left empty is
// cleared, as the server takes a field left out; what a plan still needs
// is the server's word, from its answer.
export function ProfileForm({ profile }: { profile: Profile }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const queryClient = useQueryClient()
  const [sex, setSex] = useState<Sex | ''>(profile.sex ?? '')
  const [birthDate, setBirthDate] = useState(profile.birth_date ?? '')
  const [height, setHeight] = useState(
    profile.height_cm === undefined ? '' : f.field(profile.height_cm),
  )
  const [wrongHeight, setWrongHeight] = useState(false)
  // A date part typed and part not reads as none, which would clear the
  // one stored: the field says it is no whole date. A whole one after the
  // user's today, or of an age past the oldest, the server refuses.
  const date = useRef<HTMLInputElement>(null)
  const [wrongDate, setWrongDate] = useState<'part' | 'toCome' | 'old' | undefined>(undefined)
  const today = $api.useQuery('get', '/api/v1/me').data?.today
  const save = $api.useMutation('put', '/api/v1/profile', {
    onSuccess: (answer) => {
      queryClient.setQueryData($api.queryOptions('get', '/api/v1/profile').queryKey, answer)
      sayChange({ title: t.toast.saved })
      // The limits hold a proposal to the profile: its preview changes.
      void queryClient.invalidateQueries({
        queryKey: $api.queryOptions('get', '/api/v1/plan').queryKey,
      })
    },
  })

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const typed = height.trim()
    const cm = typed === '' ? undefined : f.parse(typed)
    const wrong = typed !== '' && (cm === undefined || cm < minHeight || cm > maxHeight)
    let dateWrong: typeof wrongDate
    if (date.current?.validity.badInput === true) {
      dateWrong = 'part'
    } else if (birthDate && today !== undefined && birthDate > today) {
      dateWrong = 'toCome'
    } else if (birthDate && today !== undefined && ageOn(birthDate, today) > maxAge) {
      dateWrong = 'old'
    }
    setWrongHeight(wrong)
    setWrongDate(dateWrong)
    if (wrong || dateWrong !== undefined) {
      return
    }
    save.mutate({
      body: {
        ...(sex ? { sex } : {}),
        ...(birthDate ? { birth_date: birthDate } : {}),
        ...(cm === undefined ? {} : { height_cm: cm }),
      },
    })
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-3" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className="font-medium">
        {t.plan.profile}
      </h2>
      <p className="text-sm text-muted-foreground">{t.plan.profileHint}</p>
      <fieldset className="space-y-1" aria-describedby={`${id}-sex`}>
        <legend className={fieldLabel}>{t.plan.sex}</legend>
        <div className="flex flex-wrap gap-x-4 gap-y-1">
          {sexes.map((s) => (
            <label key={s} className="flex items-center gap-2">
              <input
                type="radio"
                name={`${id}-sex`}
                checked={sex === s}
                onChange={() => {
                  setSex(s)
                }}
              />
              {t.plan.sexes[s]}
            </label>
          ))}
          <label className="flex items-center gap-2">
            <input
              type="radio"
              name={`${id}-sex`}
              checked={sex === ''}
              onChange={() => {
                setSex('')
              }}
            />
            {t.plan.noSex}
          </label>
        </div>
        <p id={`${id}-sex`} className="text-xs text-muted-foreground">
          {t.plan.sexHint}
        </p>
      </fieldset>
      <div className="space-y-1">
        <label className="block space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.plan.birthDate}</span>
          <input
            ref={date}
            type="date"
            value={birthDate}
            aria-invalid={wrongDate !== undefined || undefined}
            aria-describedby={wrongDate ? `${id}-date` : undefined}
            className={inputClass}
            onChange={(e) => {
              setBirthDate(e.currentTarget.value)
            }}
          />
        </label>
        {wrongDate && (
          <p id={`${id}-date`} role="alert" className="text-sm text-destructive">
            {wrongDate === 'part' && t.plan.wrongDate}
            {wrongDate === 'toCome' && t.plan.dateToCome}
            {wrongDate === 'old' && t.plan.tooOld(f.number(maxAge))}
          </p>
        )}
      </div>
      <div className="space-y-1">
        <label className="block space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.plan.height}</span>
          <span className="flex items-center gap-2">
            <input
              value={height}
              inputMode="decimal"
              autoComplete="off"
              aria-invalid={wrongHeight || undefined}
              aria-describedby={wrongHeight ? `${id}-height` : undefined}
              className={cn(inputClass, 'w-28 tabular-nums')}
              onChange={(e) => {
                setHeight(e.currentTarget.value)
              }}
            />
            <span className="text-sm text-muted-foreground">cm</span>
          </span>
        </label>
        {wrongHeight && (
          <p id={`${id}-height`} role="alert" className="text-sm text-destructive">
            {t.plan.wrongHeight(f.number(minHeight), f.number(maxHeight))}
          </p>
        )}
      </div>
      <p className="text-sm">
        {profile.missing.length === 0
          ? t.plan.complete
          : t.plan.missing(profile.missing.map((m) => t.plan.missingField[m]).join(', '))}
      </p>
      <Button type="submit" disabled={save.isPending}>
        {t.plan.save}
      </Button>
      {save.isError && statusOf(save.error) === 422 ? (
        <p role="alert" className="text-sm text-destructive">
          {t.plan.refused(detailOf(save.error))}
        </p>
      ) : (
        <SaveStatus change={save} />
      )}
    </form>
  )
}
