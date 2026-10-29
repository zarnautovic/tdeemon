import type { components } from '@/api/schema'
import { targetNow, weekdays } from '@/components/plan/phases'
import { addDays, rounded } from '@/i18n/format'
import limits from '@/lib/limits.json'

type HistoryDay = components['schemas']['HistoryDayBody']
type Plan = components['schemas']['PlanBody']

// weekdayOf is a date's day of the week, 0 for Monday to 6 for Sunday.
function weekdayOf(date: string): number {
  return (new Date(`${date}T00:00:00Z`).getUTCDay() + 6) % 7
}

// weekDates are the seven days of the calendar week a date is in, Monday
// to Sunday: the week the day shows, and the progress's weeks are.
export function weekDates(date: string): string[] {
  const monday = addDays(date, -weekdayOf(date))
  return Array.from({ length: 7 }, (_, i) => addDays(monday, i))
}

// weekOf is the history a day's week asks for (WeekStrip): its calendar
// week up to today, and from the first day a history is of (limits.json),
// as no history goes further back. A week to come asks for none: its to
// is before its from. The day's history is asked of it, once for every
// part of the day that reads it.
export function weekOf(date: string, today: string): { from: string; to: string } {
  const dates = weekDates(date)
  const monday = dates[0] ?? date
  const sunday = dates[6] ?? date
  const first = limits.history.since
  return { from: monday < first ? first : monday, to: sunday < today ? sunday : today }
}

// counts tells a day whose intake the week's average counts, as
// calibration and the progress's weeks count it: one that is over
// (complete: neither today, still open, nor left out), with a meal.
export function counts(day: HistoryDay): boolean {
  return day.status === 'complete' && day.intake !== undefined
}

// A WeekAverage is what the days a week counts ate a day, against their
// targets, each whole as shown.
export interface WeekAverage {
  days: number
  kcal?: number
  // Of the days counted, when each has a target.
  target?: number
}

// weekAverage is the average of the days of a week's history that count.
export function weekAverage(days: readonly HistoryDay[]): WeekAverage {
  const counted = days.filter(counts)
  if (counted.length === 0) {
    return { days: 0 }
  }
  const sum = (kcal: (day: HistoryDay) => number) =>
    counted.reduce((total, day) => total + kcal(day), 0)
  const kcal = rounded(sum((day) => day.intake?.kcal ?? 0) / counted.length)
  if (!counted.every((day) => day.target !== undefined)) {
    return { days: counted.length, kcal }
  }
  const target = rounded(sum((day) => day.target?.kcal ?? 0) / counted.length)
  return { days: counted.length, kcal, target }
}

// plannedTarget is the kcal the plan sets a day to come: of the target in
// force on it, by the type its weekday has. A workout or the day's own
// type may change it on the day.
export function plannedTarget(plan: Plan, date: string): number | undefined {
  const target = targetNow(plan.targets, date)
  if (target === undefined) {
    return undefined
  }
  const weekday = weekdays[weekdayOf(date)]
  return weekday !== undefined && plan.training_weekdays.includes(weekday)
    ? target.training.kcal
    : target.rest.kcal
}

// A Budget is what the days a week has left, today's among them, can eat
// on average and keep the week on its target.
export interface Budget {
  days: number
  kcal: number
  // What the days counted ate past their targets; below 0, how far they
  // stayed under them.
  excess: number
}

// budgetOf is the budget of the week of dates (weekDates) today is in:
// the targets of the days left (today's of its history, those to come
// the plan's), less what the days counted ate past theirs. The days that
// do not count are left out, as from the average. None for another week,
// or when a target it needs is not known.
export function budgetOf(
  dates: readonly string[],
  byDate: ReadonlyMap<string, HistoryDay>,
  today: string,
  planned: (date: string) => number | undefined,
): Budget | undefined {
  if (!dates.includes(today)) {
    return undefined
  }
  let left = 0
  let excess = 0
  for (const date of dates) {
    const day = byDate.get(date)
    if (date >= today) {
      const target = date === today ? day?.target?.kcal : planned(date)
      if (target === undefined) {
        return undefined
      }
      left += target
    } else if (day !== undefined && counts(day)) {
      if (day.target === undefined) {
        return undefined
      }
      excess += (day.intake?.kcal ?? 0) - day.target.kcal
    }
  }
  const days = dates.filter((date) => date >= today).length
  return { days, kcal: (left - excess) / days, excess }
}

// floorOf is the least a day may eat that the week's budget says: the
// higher of the plan's kcal floors, as the week does not know whose body
// it is (docs/limits.md). Below it, the day's target is said instead.
export function floorOf(plan: Plan | undefined): number {
  const floors = [plan?.limits.kcal_floor_male, plan?.limits.kcal_floor_female]
  return Math.max(0, ...floors.filter((floor) => floor !== undefined))
}
