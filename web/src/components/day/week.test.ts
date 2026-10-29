import { expect, test } from 'vitest'

import type { components } from '@/api/schema'
import {
  budgetOf,
  floorOf,
  plannedTarget,
  weekAverage,
  weekDates,
  weekOf,
} from '@/components/day/week'
import { planBody } from '@/test/plan'

type HistoryDay = components['schemas']['HistoryDayBody']

// dayOf is a day of a week's history, of a target of target kcal.
function dayOf(
  date: string,
  kcal: number | undefined,
  { status = 'complete', target = 2079 }: { status?: HistoryDay['status']; target?: number } = {},
): HistoryDay {
  return {
    date,
    status,
    type: 'rest',
    meals: kcal === undefined ? 0 : 3,
    ...(kcal === undefined ? {} : { intake: { kcal, protein: 150, carbs: 200, fat: 70 } }),
    target: { kcal: target, protein: 160, carbs: 220, fat: 70 },
  }
}

// The week of Friday 23 October 2026, Monday to Sunday.
const week = [
  '2026-10-19',
  '2026-10-20',
  '2026-10-21',
  '2026-10-22',
  '2026-10-23',
  '2026-10-24',
  '2026-10-25',
]

test('a week is the calendar week, Monday to Sunday, whichever day of it is shown', () => {
  expect(weekDates('2026-10-19')).toEqual(week)
  expect(weekDates('2026-10-23')).toEqual(week)
  expect(weekDates('2026-10-25')).toEqual(week)
  expect(weekDates('2026-10-26')[0]).toBe('2026-10-26')
})

test("a week's history is up to today, from the first day there is one, and none to come", () => {
  expect(weekOf('2026-10-21', '2026-10-23')).toEqual({ from: '2026-10-19', to: '2026-10-23' })
  expect(weekOf('2026-10-14', '2026-10-23')).toEqual({ from: '2026-10-12', to: '2026-10-18' })
  // 1 January 1900 is a Monday, and the first day a history is of.
  expect(weekOf('1900-01-03', '2026-10-23')).toEqual({ from: '1900-01-01', to: '1900-01-07' })
  const toCome = weekOf('2026-11-04', '2026-10-23')
  expect(toCome.from > toCome.to).toBe(true)
})

// Saturday is today: the five days before it logged, 1,905 a day on
// average, 174 under their 2,079; today, open, not counted.
const logged = [
  dayOf('2026-10-19', 1850),
  dayOf('2026-10-20', 1950),
  dayOf('2026-10-21', 1915),
  dayOf('2026-10-22', 2000),
  dayOf('2026-10-23', 1810),
  dayOf('2026-10-24', 600, { status: 'open' }),
]

test('the week averages its days logged, today while open not among them', () => {
  expect(weekAverage(logged)).toEqual({ days: 5, kcal: 1905, target: 2079 })
})

test("a day without a meal, a day left out and today are not in the week's average", () => {
  expect(
    weekAverage([
      dayOf('2026-10-19', 2500, { status: 'excluded' }),
      dayOf('2026-10-20', undefined),
      dayOf('2026-10-21', 2000, { target: 2200 }),
      dayOf('2026-10-22', 900, { status: 'open' }),
    ]),
  ).toEqual({ days: 1, kcal: 2000, target: 2200 })
  expect(weekAverage([dayOf('2026-10-22', 900, { status: 'open' })])).toEqual({ days: 0 })
  // A day logged before the first target: the average has none.
  const { target: _, ...untargeted } = dayOf('2026-10-20', 1700)
  expect(weekAverage([untargeted, dayOf('2026-10-21', 2000)])).toEqual({ days: 2, kcal: 1850 })
})

// Sunday is a training day of 2,401, Saturday's target 2,079: the 870
// the week stayed under go to the two days left.
const plan = planBody({
  training_weekdays: ['sunday'],
  targets: [
    {
      effective_from: '2026-10-01',
      training: { kcal: 2401, protein: 180, carbs: 260, fat: 70 },
      rest: { kcal: 2079, protein: 160, carbs: 220, fat: 70 },
    },
  ],
})
const planned = (date: string) => plannedTarget(plan, date)

test('the days left can eat their targets and what the week stayed under, on average', () => {
  const byDate = new Map(logged.map((d) => [d.date, d]))
  expect(budgetOf(week, byDate, '2026-10-24', planned)).toEqual({
    days: 2,
    kcal: (2079 + 2401 + 870) / 2,
    excess: -870,
  })
  // On Sunday, Sunday alone.
  const sunday = new Map([...byDate, ['2026-10-25', dayOf('2026-10-25', 300, { status: 'open' })]])
  sunday.set('2026-10-24', dayOf('2026-10-24', 2079))
  expect(budgetOf(week, sunday, '2026-10-25', () => undefined)).toEqual({
    days: 1,
    kcal: 2079 + 870,
    excess: -870,
  })
})

test('a week over, a week to come, or a target not known has no budget', () => {
  const byDate = new Map(logged.map((d) => [d.date, d]))
  expect(budgetOf(week, byDate, '2026-10-27', planned)).toBeUndefined()
  expect(budgetOf(week, byDate, '2026-10-12', planned)).toBeUndefined()
  expect(budgetOf(week, byDate, '2026-10-24', () => undefined)).toBeUndefined()
})

test('a day to come has the target of its weekday, of the targets in force then', () => {
  const later = planBody({
    ...plan,
    targets: [
      ...plan.targets,
      {
        effective_from: '2026-10-25',
        training: { kcal: 2300, protein: 180, carbs: 240, fat: 70 },
        rest: { kcal: 1950, protein: 160, carbs: 200, fat: 70 },
      },
    ],
  })
  expect(plannedTarget(later, '2026-10-24')).toBe(2079)
  expect(plannedTarget(later, '2026-10-25')).toBe(2300)
  expect(plannedTarget(later, '2026-10-26')).toBe(1950)
  expect(plannedTarget(planBody(), '2026-10-26')).toBeUndefined()
})

test("the week's floor is the higher of the plan's kcal floors", () => {
  expect(floorOf(plan)).toBe(1500)
  expect(floorOf(undefined)).toBe(0)
})
