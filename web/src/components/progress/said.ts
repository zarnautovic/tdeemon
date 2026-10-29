import type { components } from '@/api/schema'
import { daysBetween, type useFormat } from '@/i18n/format'
import type { useMessages } from '@/i18n/messages'

type Tdee = components['schemas']['TdeeBody']
type Recovery = components['schemas']['RecoveryBody']
type Phase = components['schemas']['PhaseBody']
type Messages = ReturnType<typeof useMessages>
type Format = ReturnType<typeof useFormat>

// The words the progress's hero says of the summary, worked out of what
// the server gives, never of a threshold of its own.

// doubtsSaid is what a TDEE measured but not trusted needs to be, a
// sentence for each of its doubts, by the summary's own values: a
// narrower ±, more weigh-ins or logged days a week.
export function doubtsSaid(tdee: Tdee, t: Messages, f: Format): string[] {
  const d = t.calibration.doubts
  return tdee.doubts.flatMap((doubt) => {
    switch (doubt) {
      case 'wide_interval':
        return [d.wide_interval(f.number(tdee.trust.half_width), f.number(tdee.trust.weigh_ins))]
      case 'few_weigh_ins':
        return [d.few_weigh_ins(f.number(tdee.trust.weigh_ins), tdee.trust.weigh_ins)]
      case 'few_logged_days':
        return [d.few_logged_days(f.number(tdee.trust.logged_days), tdee.trust.logged_days)]
      default:
        return []
    }
  })
}

// The flags of recovery, in the order a sentence names them.
const flags = ['short_sleep', 'high_resting_hr', 'low_hrv'] as const

// recoverySaid is what recovery flags, in a sentence: "Sleep and HRV were
// worse than usual last week."; nothing when it flags nothing.
export function recoverySaid(recovery: Recovery, t: Messages, f: Format): string | undefined {
  const worse = flags.filter((flag) => recovery.flags.includes(flag))
  return worse.length === 0
    ? undefined
    : t.charts.worse(f.list(worse.map((flag) => t.charts.flags[flag])), worse.length)
}

// phaseSaid is the phase and its week today: "Cut · week 8 of 12".
export function phaseSaid(phase: Phase, today: string, t: Messages): string {
  const week = Math.floor(daysBetween(phase.start, today) / 7) + 1
  const weeks =
    phase.end === undefined ? undefined : Math.ceil((daysBetween(phase.start, phase.end) + 1) / 7)
  return `${t.plan.change.types[phase.type]} · ${
    weeks === undefined ? t.day.summary.week(week) : t.day.summary.weekOf(week, weeks)
  }`
}
