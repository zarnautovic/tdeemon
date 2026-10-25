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
// clock skips (summer time begins) comes an hour off, after it in a zone
// east of UTC, before it west of it; one it shows twice (summer time
// ends) is the second east of UTC, the first west of it.
export function instantAt(date: string, time: string, timeZone: string): string {
  const wall = wallOf(date, time)
  // The offset at the wall time read as UTC is near; the offset at the
  // instant it gives is the one.
  const first = wall - offsetOf(wall, timeZone)
  return new Date(wall - offsetOf(first, timeZone)).toISOString()
}

// wallOf is date (YYYY-MM-DD) at time (HH:MM) read as UTC.
function wallOf(date: string, time: string): number {
  const d = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date)
  const t = /^(\d{2}):(\d{2})$/.exec(time)
  if (!d || !t) {
    throw new RangeError(`not a date and a time: ${date} ${time}`)
  }
  return Date.UTC(Number(d[1]), Number(d[2]) - 1, Number(d[3]), Number(t[1]), Number(t[2]))
}

// otherInstantAt is the other instant than taken at which timeZone's
// clock shows date at time, when it shows it twice (summer time ends):
// one of the offsets half a day before and after taken, which is within
// an hour of the change.
function otherInstantAt(
  date: string,
  time: string,
  timeZone: string,
  taken: string,
): string | undefined {
  const wall = wallOf(date, time)
  const halfDay = 12 * 60 * 60 * 1000
  const other = [Date.parse(taken) - halfDay, Date.parse(taken) + halfDay]
    .map((near) => wall - offsetOf(near, timeZone))
    .find((at) => at !== Date.parse(taken) && wall - offsetOf(at, timeZone) === at)
  return other === undefined ? undefined : new Date(other).toISOString()
}

// clockAt is the time (HH:MM) timeZone's clock shows at instant.
export function clockAt(instant: number, timeZone: string): string {
  const p = partsAt(instant, timeZone)
  return `${String(p.hour).padStart(2, '0')}:${String(p.minute).padStart(2, '0')}`
}

// nightAfter tells a time (HH:MM) on the user's clock before the day's
// start hour, of the night after the day.
export function nightAfter(time: string, startHour: number): boolean {
  return Number(time.slice(0, 2)) < startHour
}

// mealInstant is when a meal of the logical day day was eaten at time on
// the user's clock: a time before the day's start hour is of the night
// after it, the next calendar date. Of a time the clock shows twice
// (summer time ends), the one instantAt takes, unless it is still to
// come at now and the other is not: a meal is eaten by now.
export function mealInstant(
  day: string,
  time: string,
  timeZone: string,
  startHour: number,
  now: number = Date.now(),
): string {
  const date = nightAfter(time, startHour) ? addDays(day, 1) : day
  const taken = instantAt(date, time, timeZone)
  if (!toCome(taken, now)) {
    return taken
  }
  const other = otherInstantAt(date, time, timeZone, taken)
  return other !== undefined && !toCome(other, now) ? other : taken
}

// clockAhead is how far after now a meal may be eaten: a clock a little
// ahead of the server's (meal.MaxEatenAhead).
const clockAhead = 5 * 60 * 1000

// toCome tells an instant still to come at now, past a clock's margin: a
// meal is logged once eaten, and the server refuses one eaten later.
export function toCome(instant: string, now: number = Date.now()): boolean {
  return Date.parse(instant) > now + clockAhead
}
