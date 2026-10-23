import { useCheckIn } from '@/api/day'
import type { Day } from '@/api/types'
import { DayNote } from '@/components/day/day-note'
import { DayTypePicker } from '@/components/day/day-type-picker'
import { Exclusion } from '@/components/day/exclusion'
import { SaveStatus } from '@/components/day/save-status'
import { Stepper } from '@/components/day/stepper'
import type { Scale } from '@/i18n/en'
import { useMessages } from '@/i18n/messages'

// CheckIn is what the user says of a day: their mood and hunger, a note,
// the day's type, and whether calibration counts it. Each is saved as it
// is given.
export function CheckIn({ day }: { day: Day }) {
  const t = useMessages()

  return (
    <section aria-labelledby="day-check-in" className="space-y-4 rounded-xl border p-4">
      <h2 id="day-check-in" className="font-medium">
        {t.day.checkIn}
      </h2>
      <ScaleField day={day} field="mood" label={t.day.mood} words={t.day.moods} tone="good" />
      <ScaleField
        day={day}
        field="hunger"
        label={t.day.hunger}
        words={t.day.hungers}
        tone="signal"
      />
      <DayNote day={day} />
      <DayTypePicker day={day} />
      <Exclusion day={day} />
    </section>
  )
}

// ScaleField is the mood or the hunger of the day, on its scale.
function ScaleField({
  day,
  field,
  label,
  words,
  tone,
}: {
  day: Day
  field: 'mood' | 'hunger'
  label: string
  words: Scale
  tone: 'good' | 'signal'
}) {
  const change = useCheckIn(day.date)
  const sent = change.variables?.body[field]
  // The step picked last shows while it is saved, and after it failed,
  // until it is sent again; then the day's.
  const value = (change.isPending || change.isError) && sent !== undefined ? sent : day[field]

  return (
    <Stepper
      label={label}
      words={words}
      value={value}
      tone={tone}
      status={<SaveStatus change={change} />}
      onPick={(n) => {
        change.mutate({
          params: { path: { date: day.date } },
          body: field === 'mood' ? { mood: n } : { hunger: n },
        })
      }}
    />
  )
}
