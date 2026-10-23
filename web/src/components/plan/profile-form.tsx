import { useQueryClient } from '@tanstack/react-query'
import { useId, useState, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import type { components } from '@/api/schema'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

type Profile = components['schemas']['ProfileBody']
type Sex = NonNullable<Profile['sex']>

const sexes: readonly Sex[] = ['male', 'female']

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
    profile.height_cm === undefined ? '' : f.field(profile.height_cm, 1),
  )
  const [wrongHeight, setWrongHeight] = useState(false)
  const save = $api.useMutation('put', '/api/v1/profile', {
    onSuccess: async (answer) => {
      queryClient.setQueryData($api.queryOptions('get', '/api/v1/profile').queryKey, answer)
      // The age and what is missing, which get_me gives a model.
      await queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/me'] })
    },
  })

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const typed = height.trim()
    const cm = typed === '' ? undefined : f.parse(typed)
    setWrongHeight(typed !== '' && cm === undefined)
    if (typed !== '' && cm === undefined) {
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
      <label className="block space-y-1">
        <span className={cn(fieldLabel, 'block')}>{t.plan.birthDate}</span>
        <input
          type="date"
          value={birthDate}
          className={inputClass}
          onChange={(e) => {
            setBirthDate(e.currentTarget.value)
          }}
        />
      </label>
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
            {t.plan.wrongHeight}
          </p>
        )}
      </div>
      <p className="text-sm">
        {profile.missing.length === 0
          ? t.plan.complete
          : t.plan.missing(profile.missing.map((m) => t.plan.missingField[m]).join(', '))}
      </p>
      {save.isError && statusOf(save.error) === 422 ? (
        <p role="alert" className="text-sm text-destructive">
          {t.plan.refused(detailOf(save.error))}
        </p>
      ) : (
        <SaveStatus change={save} />
      )}
      <Button type="submit" disabled={save.isPending}>
        {t.plan.save}
      </Button>
    </form>
  )
}
