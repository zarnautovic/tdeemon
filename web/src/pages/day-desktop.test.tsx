import { cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Meal } from '@/api/types'
import { day, fakeServer, renderAt, signedIn, today } from '@/test/app'
import { planBody } from '@/test/plan'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  localStorage.clear()
})

// desktop makes the window a desktop's width, as matchMedia says.
function desktop() {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: query === '(min-width: 64rem)',
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }))
}

const breakfast: Meal = {
  id: '0193c3a5-0000-7000-8000-0000000000a1',
  // 08:45 in Los Angeles, the session user's time zone.
  eaten_at: '2026-10-23T15:45:00Z',
  description: 'Oats with a banana',
  client: 'mcp',
  total: { kcal: 520, protein: 18, carbs: 92, fat: 9 },
  items: [
    {
      id: '0193c3a5-0000-7000-8000-0000000000b1',
      name: 'Oats',
      quantity: { amount: 80, unit: 'g' },
      amount: 80,
      unit: 'g',
      nutrients: { kcal: 520, protein: 18, carbs: 92, fat: 9 },
      source: 'label',
    },
  ],
}

// A day of a week whose days logged ate 1,905 a day, of a target of
// 2,079 (the week's most); in a cut, from 1 September to 23 November.
function serveDay() {
  const target = { kcal: 2079, protein: 160, carbs: 220, fat: 70 }
  const logged = (date: string, kcal: number) => ({
    date,
    status: 'complete',
    type: 'rest',
    meals: 3,
    intake: { kcal, protein: 150, carbs: 200, fat: 70 },
    target,
  })
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(
        day({
          target,
          total: { kcal: 1460, protein: 88, carbs: 146, fat: 41 },
          meals: [breakfast],
        }),
      ),
    'GET /api/v1/history': () =>
      Response.json({
        from: '2026-10-19',
        to: today,
        days: [logged('2026-10-19', 1850), logged('2026-10-20', 1960)],
        weeks: [],
      }),
    'GET /api/v1/plan': () =>
      Response.json(
        planBody({
          phases: [{ type: 'cut', start: '2026-09-01', end: '2026-11-23', rate_pct_week: -0.75 }],
        }),
      ),
  })
}

test("a desktop's day: the meals a table with their total, the week, the body, the check-in and the phase beside it", async () => {
  desktop()
  serveDay()
  renderAt('/')

  // The meals in columns: each row its time, name and marks, kcal and
  // each macro, said with its name; the heads for the eye; the total last.
  const meals = await screen.findByRole('region', { name: 'Meals' })
  const meal = within(meals).getByRole('article', { name: 'Oats with a banana' })
  const row = within(meal).getByRole('button', { expanded: false })
  expect(row.textContent).toBe('8:45 AMOats with a bananaAI520520 kcalP 18C 92F 9')
  const heads = meals.querySelector('[aria-hidden="true"]')
  expect(Array.from(heads?.children ?? [], (c) => c.textContent)).toEqual([
    'Time',
    'Meal',
    'kcal',
    'P',
    'C',
    'F',
  ])
  expect(within(meals).getByText('Total').parentElement?.textContent).toBe(
    'Total1,4601,460 kcalP 88C 146F 41',
  )
  // The row still opens to its items, and the ways to change it.
  fireEvent.click(row)
  expect(
    within(meal).getByRole('button', { name: 'Change the meal: Oats with a banana' }),
  ).toBeDefined()

  // Beside the day: the week first, its days and its average in a line.
  const week = screen.getByRole('region', { name: 'This week' })
  expect(within(week).getByRole('navigation', { name: 'The week' })).toBeDefined()
  expect(
    await within(week).findByRole('button', {
      name: 'This week: average 1,905 of 2,079 · 174 under a day · 2 days logged',
    }),
  ).toBeDefined()
  // The average a line across the bars, against the week's most.
  const average = week.querySelector<HTMLElement>('[data-average]')
  expect(Number.parseFloat(average?.style.bottom ?? '')).toBeCloseTo((1905 / 2079) * 100, 5)
  // Then the body, the check-in and the phase, as a card of its own.
  const order = screen
    .getAllByRole('region')
    .map((r) => r.getAttribute('aria-labelledby'))
    .filter((id) => ['day-week', 'day-body', 'day-check-in', 'day-phase'].includes(id ?? ''))
  expect(order).toEqual(['day-week', 'day-body', 'day-check-in', 'day-phase'])
  const phase = screen.getByRole('region', { name: 'Phase' })
  expect(await within(phase).findByRole('link', { name: 'Cut · week 8 of 12' })).toBeDefined()
  expect(within(phase).getByText('31 days left')).toBeDefined()
  // Not in the day's hero too.
  expect(screen.getAllByRole('link', { name: /^Cut/ })).toHaveLength(1)
})

test('the phone has its week above the day, and the phase in the hero', async () => {
  serveDay()
  renderAt('/')

  const meals = await screen.findByRole('region', { name: 'Meals' })
  expect(within(meals).queryByText('Total')).toBeNull()
  expect(screen.queryByRole('region', { name: 'This week' })).toBeNull()
  expect(screen.queryByRole('region', { name: 'Phase' })).toBeNull()
  const summary = screen.getByRole('region', { name: 'Energy and macros' })
  expect(await within(summary).findByRole('link', { name: 'Cut · week 8 of 12' })).toBeDefined()
})

// On a desktop another day's header is today's, as wide as the page: the
// arrows, the date, then its type and the way back to today.
test("another day's header on a desktop is laid out as today's", async () => {
  desktop()
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/2026-10-22': () => Response.json(day({ date: '2026-10-22' })),
  })
  renderAt('/?date=2026-10-22')

  const date = await screen.findByRole('heading', { level: 1, name: 'Thursday, October 22' })
  const header = date.closest('header')
  const parts = Array.from(header?.querySelectorAll('a, h1, button') ?? []).map(
    (e) => e.getAttribute('aria-label') ?? e.textContent,
  )
  expect(parts).toEqual([
    'The day before',
    'The day after',
    'Thursday, October 22',
    'Type of day: Rest day',
    'Back to today',
  ])
  // The week beside the day: that week's.
  expect(screen.getByRole('region', { name: 'This week' })).toBeDefined()
})
