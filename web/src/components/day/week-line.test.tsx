import { cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { day, fakeServer, renderAt, signedIn, today } from '@/test/app'
import { planBody } from '@/test/plan'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

type HistoryDay = components['schemas']['HistoryDayBody']

// dayOf is a day of the week's history, of a target of 2,079 kcal.
function dayOf(date: string, kcal: number | undefined, status: HistoryDay['status'] = 'complete') {
  return {
    date,
    status,
    type: 'rest',
    meals: kcal === undefined ? 0 : 3,
    ...(kcal === undefined ? {} : { intake: { kcal, protein: 150, carbs: 200, fat: 70 } }),
    target: { kcal: 2079, protein: 160, carbs: 220, fat: 70 },
  }
}

// serveWeek serves today's week, Friday 23 October: Monday to Thursday
// as days, and today, open, with 600 eaten; the plan's rest days 2,079.
function serveWeek(days: ReturnType<typeof dayOf>[], locale: 'en' | 'hr' = 'en') {
  fakeServer({
    ...signedIn(locale),
    'GET /api/v1/days/today': () =>
      Response.json(day({ target: { kcal: 2079, protein: 160, carbs: 220, fat: 70 } })),
    'GET /api/v1/history': () =>
      Response.json({
        from: '2026-10-19',
        to: today,
        days: [...days, dayOf(today, 600, 'open')],
        weeks: [],
      }),
    'GET /api/v1/plan': () =>
      Response.json(
        planBody({
          targets: [
            {
              effective_from: '2026-10-01',
              training: { kcal: 2400, protein: 180, carbs: 260, fat: 70 },
              rest: { kcal: 2079, protein: 160, carbs: 220, fat: 70 },
            },
          ],
        }),
      ),
  })
}

// Three days logged, 1,905 a day; Tuesday without a meal.
const under = [
  dayOf('2026-10-19', 1850),
  dayOf('2026-10-20', undefined),
  dayOf('2026-10-21', 1950),
  dayOf('2026-10-22', 1915),
]

test('this week: its average against the target, the days it counts, and what is left of it', async () => {
  serveWeek(under)
  renderAt('/')

  // Under the week, on a phone.
  const line = await screen.findByRole('button', {
    name: 'This week: average 1,905 of 2,079 · 174 under a day · 3 days logged',
  })
  expect(line.getAttribute('aria-haspopup')).toBe('dialog')
  fireEvent.click(line)

  const sheet = await screen.findByRole('dialog', { name: 'This week' })
  expect(within(sheet).getByText('Oct 19 – Oct 25')).toBeDefined()
  const rows = within(within(sheet).getByRole('table', { name: "The week's days" })).getAllByRole(
    'row',
  )
  expect(rows.map((r) => Array.from(r.children, (c) => c.textContent))).toEqual([
    ['Day', 'Eaten', 'Target'],
    ['Mon 19', '1,850', '2,079'],
    ['Tue 20nothing logged', '—', '2,079'],
    ['Wed 21', '1,950', '2,079'],
    ['Thu 22', '1,915', '2,079'],
    ['Fri 23today, counted once over', '600', '2,079'],
    // The plan's, of the days to come.
    ['Sat 24to come', '—', '2,079'],
    ['Sun 25to come', '—', '2,079'],
  ])
  // Today and the two days to come: 3 × 2,079, and the 522 the week
  // stayed under, 2,253 a day.
  expect(
    within(sheet).getByText(
      'Until Sunday you can average 2,253 a day and stay on your weekly target.',
    ),
  ).toBeDefined()
})

// Far over its target, the days left would have to eat less than the
// plan's floor: they keep to their target, which the line says.
test("a week far over its target says to keep to the day's target, not to eat less", async () => {
  serveWeek([
    dayOf('2026-10-19', 3500),
    dayOf('2026-10-20', 3500),
    dayOf('2026-10-21', 3500),
    dayOf('2026-10-22', 3500),
  ])
  renderAt('/')

  fireEvent.click(
    await screen.findByRole('button', {
      name: 'This week: average 3,500 of 2,079 · 1,421 over a day · 4 days logged',
    }),
  )
  const sheet = await screen.findByRole('dialog', { name: 'This week' })
  expect(
    within(sheet).getByText(
      "The week is 5,684 over its target: keep to your day's target until Sunday.",
    ),
  ).toBeDefined()
  expect(within(sheet).queryByText(/you can average/)).toBeNull()
})

test('the week speaks Croatian', async () => {
  serveWeek(under, 'hr')
  renderAt('/')

  fireEvent.click(
    await screen.findByRole('button', {
      name: 'Ovaj tjedan: prosjek 1.905 od 2.079 · 174 ispod na dan · 3 upisana dana',
    }),
  )
  const sheet = await screen.findByRole('dialog', { name: 'Ovaj tjedan' })
  expect(
    within(sheet).getByText('Do nedjelje možeš prosječno 2.253 na dan i ostati na tjednom cilju.'),
  ).toBeDefined()
})

test('a week of nothing over yet says so, and what the days left can eat', async () => {
  serveWeek([])
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'This week: nothing to average yet' }))
  const sheet = await screen.findByRole('dialog', { name: 'This week' })
  expect(
    within(sheet).getByText(
      'Until Sunday you can average 2,079 a day and stay on your weekly target.',
    ),
  ).toBeDefined()
})

test('a week before this one says its average, and no budget', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/2026-10-14': () => Response.json(day({ date: '2026-10-14' })),
    'GET /api/v1/history': () =>
      Response.json({
        from: '2026-10-12',
        to: '2026-10-18',
        days: [dayOf('2026-10-12', 2000), dayOf('2026-10-13', 2158)],
        weeks: [],
      }),
  })
  renderAt('/?date=2026-10-14')

  fireEvent.click(
    await screen.findByRole('button', {
      name: 'That week: average 2,079 of 2,079 · on target · 2 days logged',
    }),
  )
  const sheet = await screen.findByRole('dialog', { name: 'That week' })
  expect(within(sheet).queryByText(/Until Sunday|Today you can/)).toBeNull()
})
