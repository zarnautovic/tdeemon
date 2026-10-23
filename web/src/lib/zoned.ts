import { addDays } from '@/i18n/format'

// The parts of an instant on a time zone's clock.
function partsAt(instant: number, timeZone: string) {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(new Date(instant))
  const get = (type: Intl.DateTimeFormatPartTypes) =>
    Number(parts.find((p) => p.type === type)?.value ?? 0)
  return {
    year: get('year'),
    month: get('month'),
    day: get('day'),
    hour: get('hour'),
    minute: get('minute'),
    second: get('second'),
  }
}

// offsetOf is how far timeZone's clock is ahead of UTC at instant, in
// milliseconds.
function offsetOf(instant: number, timeZone: string): number {
  const p = partsAt(instant, timeZone)
  const shown = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second)
  return shown - Math.floor(instant / 1000) * 1000
}

// instantAt is the instant at which timeZone's clock shows date
// (YYYY-MM-DD) at time (HH:MM), as an ISO 8601 instant in UTC. A time the
// clock skips (summer time begins) is the instant an hour later; one it
// shows twice (summer time ends), the second.
export function instantAt(date: string, time: string, timeZone: string): string {
  const d = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date)
  const t = /^(\d{2}):(\d{2})$/.exec(time)
  if (!d || !t) {
    throw new RangeError(`not a date and a time: ${date} ${time}`)
  }
  const wall = Date.UTC(Number(d[1]), Number(d[2]) - 1, Number(d[3]), Number(t[1]), Number(t[2]))
  // The offset at the wall time read as UTC is near; the offset at the
  // instant it gives is the one.
  const first = wall - offsetOf(wall, timeZone)
  return new Date(wall - offsetOf(first, timeZone)).toISOString()
}

// clockAt is the time (HH:MM) timeZone's clock shows at instant.
export function clockAt(instant: number, timeZone: string): string {
  const p = partsAt(instant, timeZone)
  return `${String(p.hour).padStart(2, '0')}:${String(p.minute).padStart(2, '0')}`
}

// mealInstant is when a meal of the logical day day was eaten at time on
// the user's clock: a time before the day's start hour is of the night
// after it, the next calendar date.
export function mealInstant(day: string, time: string, timeZone: string, startHour: number) {
  const hour = Number(time.slice(0, 2))
  return instantAt(hour < startHour ? addDays(day, 1) : day, time, timeZone)
}
