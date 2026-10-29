import type { components } from '@/api/schema'
import { CalibrationCounts } from '@/components/progress/calibration-counts'
import { InfoTip } from '@/components/ui/info-tip'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

type Tdee = components['schemas']['TdeeBody']

// CalibrationRow is, in the day's summary, calibration on its way to a
// first TDEE, in the TDEE's place while there is none: its counts, short,
// and the day a first may come. A term and its descriptions, for a list
// of them (<dl>) or a group of one (<div>).
export function CalibrationRow({ tdee }: { tdee: Tdee }) {
  const t = useMessages()
  const f = useFormat()
  const c = t.calibration

  return (
    <>
      <dt className="flex items-center gap-1 text-muted-foreground">
        {c.title}
        <InfoTip label={t.info.tdee.label} title={t.info.tdee.title}>
          {t.info.tdee.text}
        </InfoTip>
      </dt>
      <dd>
        <CalibrationCounts needs={tdee.needs} compact />
      </dd>
      <dd className="text-caption text-muted-foreground">
        {tdee.expected === undefined ? c.start : c.expected(f.short(tdee.expected))}
      </dd>
    </>
  )
}
