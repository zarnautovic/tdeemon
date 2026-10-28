import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Meal } from '@/api/types'
import { setLocale } from '@/i18n/locale'
import {
  day,
  fakeServer,
  me,
  problem,
  renderAt,
  session,
  signedIn,
  signedOut,
  today,
} from '@/test/app'
import { dayServer } from '@/test/day-server'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

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
      nutrients: { kcal: 300, protein: 11, carbs: 54, fat: 6 },
      source: 'label',
    },
    {
      id: '0193c3a5-0000-7000-8000-0000000000b2',
      name: 'Banana bread',
      quantity: { portions: 1.5 },
      amount: 1.5,
      unit: 'portion',
      nutrients: { kcal: 220, protein: 7, carbs: 38, fat: 3 },
      source: 'dish',
    },
  ],
}

// currentIn are the links of a navigation marked as the current page.
function currentIn(name: string) {
  return within(screen.getByRole('navigation', { name }))
    .getAllByRole('link')
    .filter((a) => a.getAttribute('aria-current') === 'page')
}

// energy is the day's summary, its energy and macros.
function energy() {
  return screen.getByRole('region', { name: 'Energy and macros' })
}

// macroOf is what a macro's bar says, its name first.
function macroOf(summary: HTMLElement, name: string): string | null {
  return within(summary).getByText(name).closest('p')?.textContent ?? null
}

// termOf is the value of a term of the summary's list.
function termOf(summary: HTMLElement, term: string): string | null {
  return within(summary).getByText(term).nextElementSibling?.textContent ?? null
}

// summaryOf is calibration's summary of nothing measured yet.
function summaryOf() {
  return {
    date: today,
    incomplete_days: [],
    recovery: { flags: [] },
    tdee: {
      before_phase: false,
      doubts: ['few_days'],
      from: '2026-09-25',
      to: '2026-10-22',
      logged_days: 0,
      measured: false,
      trusted: false,
      weigh_ins: 0,
    },
    verdict: {
      action: 'none',
      flags: [],
      gap_kcal: 0,
      guards: [],
      rate: 'none',
      reasons: ['no_estimate'],
      step_kcal: 0,
    },
    weeks: [],
  }
}

// planOf is a plan of no phase nor target yet.
function planOf() {
  return { limits: {}, phases: [], revisions: [], targets: [], today, training_weekdays: [] }
}

test('today shows what was eaten against the target, the meals and the measurements', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(
        day({
          total: { kcal: 1460, protein: 88, carbs: 146, fat: 41 },
          remaining: { kcal: 1040, protein: 72, carbs: 134, fat: 39 },
          meals: [breakfast],
          metrics: [
            { kind: 'weight', value: 72.46, unit: 'kg', source: 'manual' },
            { kind: 'steps', value: 8042, unit: 'steps', source: 'import' },
            { kind: 'body_fat', value: 18.24, unit: '%', source: 'import' },
          ],
        }),
      ),
  })

  renderAt('/')

  expect(await screen.findByRole('heading', { name: 'Today' })).toBeDefined()
  expect(screen.getByText('Friday, October 23')).toBeDefined()
  expect(screen.getByRole('button', { name: 'Type of day: Rest day' })).toBeDefined()
  const summary = screen.getByRole('region', { name: 'Energy and macros' })
  expect(within(summary).getByText('Eaten 1,460 of 2,500 kcal, 1,040 left')).toBeDefined()
  expect(macroOf(summary, 'Protein')).toBe('Protein88 / 160 g · 72 to go')

  const meals = screen.getByRole('region', { name: 'Meals' })
  expect(within(meals).getByText('1 · 1,460 kcal')).toBeDefined()
  const meal = within(meals).getByRole('article', { name: 'Oats with a banana' })
  // Its row: when, what, its kcal and macros, that a model logged it,
  // and how many items; a tap shows them.
  const row = within(meal).getByRole('button', { expanded: false })
  expect(row.textContent).toBe('8:45 AMOats with a banana520 kcalP 18 · C 92 · F 9AI2 items')
  fireEvent.click(row)
  const items = within(meal).getAllByRole('listitem')
  expect(items[0]?.textContent).toBe('Oats 80 g300 kcal · P 11 · C 54 · F 6')
  expect(items[1]?.textContent).toContain('Banana bread 1.5 portions')

  // The body at a glance, and all its measurements a sheet away.
  const body = screen.getByRole('region', { name: 'Body today' })
  expect(within(body).getByText('72.5 kg')).toBeDefined()
  expect(within(body).getByText('8,042')).toBeDefined()
  fireEvent.click(within(body).getByRole('button', { name: 'All measurements' }))
  const measurements = await screen.findByRole('dialog', { name: 'Measurements' })
  expect(within(measurements).getByText('18.2%')).toBeDefined()
})

test("the body's tiles: the weight with its trend, the night's sleep, a dash for what is missing", async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(
        day({
          metrics: [
            { kind: 'weight', value: 86.42, unit: 'kg', source: 'manual' },
            { kind: 'sleep_h', value: 7.999, unit: 'h', source: 'import' },
          ],
        }),
      ),
    'GET /api/v1/summary': () =>
      Response.json({ ...summaryOf(), trend: { date: '2026-10-22', kg: 86.14 } }),
  })

  renderAt('/')

  const body = await screen.findByRole('region', { name: 'Body today' })
  const tile = (label: string) => within(body).getByText(label).closest('div')?.textContent ?? null
  await waitFor(() => {
    expect(tile('Weight')).toBe('Weight86.4 kgtrend 86.1 kg')
  })
  // 7.999 hours are 8 h, not 7 h 60.
  expect(tile('Sleep')).toBe('Sleep8 h 0 min')
  expect(tile('Steps')).toBe('Steps—')
  expect(tile('Resting heart rate')).toBe('Resting heart rate—')
})

test("a day before's weight has the trend of that day, which a weigh-in changed moves", async () => {
  const date = '2026-08-25'
  const server = dayServer(
    day({
      date,
      status: 'complete',
      metrics: [{ kind: 'weight', value: 85.9, unit: 'kg', source: 'manual' }],
    }),
  )
  // The history of the day's week, its trend weight as the weigh-in makes
  // it; today's, the summary's, is another.
  let trend = 86.41
  const asked: string[] = []
  fakeServer({
    ...signedIn(),
    ...server.routes,
    'GET /api/v1/summary': () =>
      Response.json({ ...summaryOf(), trend: { date: '2026-10-22', kg: 82.9 } }),
    'GET /api/v1/history': (request) => {
      const query = new URL(request.url).searchParams
      asked.push(`${query.get('from') ?? ''} to ${query.get('to') ?? ''}`)
      return Response.json({
        from: '2026-08-19',
        to: date,
        days: [{ date, status: 'complete', type: 'rest', meals: 0, trend_kg: trend }],
        weeks: [],
      })
    },
  })

  renderAt(`/?date=${date}`)

  const body = await screen.findByRole('region', { name: 'Body today' })
  const tile = (label: string) => within(body).getByText(label).closest('div')?.textContent ?? null
  await waitFor(() => {
    expect(tile('Weight')).toBe('Weight85.9 kgtrend 86.4 kg')
  })
  // Asked once, with the week strip.
  expect(new Set(asked)).toEqual(new Set(['2026-08-19 to 2026-08-25']))

  trend = 86.2
  fireEvent.click(within(body).getByRole('button', { name: 'All measurements' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Change: Weight' }))
  const weight = screen.getByLabelText('Weight')
  fireEvent.change(weight, { target: { value: '84' } })
  fireEvent.click(
    within(weight.closest('form') ?? document.body).getByRole('button', { name: 'Save' }),
  )
  await waitFor(() => {
    expect(tile('Weight')).toBe('Weight84 kgtrend 86.2 kg')
  })
})

test("a model's long list goes under the meal of its hour, which names the row", async () => {
  const list = 'Rye bread 60 g, two eggs, a tomato and a slice of cheese from the fridge'
  const [oats] = breakfast.items
  if (!oats) {
    throw new Error('the fixture has no items')
  }
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(
        day({
          meals: [{ ...breakfast, description: list, items: [{ ...oats, source: 'estimate' }] }],
        }),
      ),
  })

  renderAt('/')

  // 08:45 on the user's clock: breakfast.
  const meal = await screen.findByRole('article', { name: 'Breakfast' })
  const row = within(meal).getByRole('button', { expanded: false })
  expect(within(row).getByText(list)).toBeDefined()
  expect(within(row).getByText('AI')).toBeDefined()
  expect(within(row).getByText('estimate')).toBeDefined()
  expect(within(row).getByText('1 item')).toBeDefined()
})

test('a meal is deleted from its row at once, and its toast takes it back', async () => {
  let meals = [breakfast]
  const undos: unknown[] = []
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () => Response.json(day({ meals })),
    [`DELETE /api/v1/meals/${breakfast.id}`]: () => {
      meals = []
      return Response.json({
        day: { date: today, status: 'open', type: 'rest', total: day().total },
      })
    },
    'POST /api/v1/undo': async (request) => {
      undos.push(await request.json())
      meals = [breakfast]
      return Response.json({
        undone: {
          action: 'meal.delete',
          entity: breakfast.id,
          at: breakfast.eaten_at,
          client: 'web',
          days: [today],
        },
      })
    },
  })

  renderAt('/')

  const meal = await screen.findByRole('article', { name: 'Oats with a banana' })
  fireEvent.click(within(meal).getByRole('button', { expanded: false }))
  fireEvent.click(within(meal).getByRole('button', { name: 'Delete: Oats with a banana' }))

  expect(await screen.findByText('Nothing logged yet.')).toBeDefined()
  fireEvent.click(await screen.findByRole('button', { name: 'Undo' }))
  expect(await screen.findByRole('article', { name: 'Oats with a banana' })).toBeDefined()
  expect(undos).toEqual([
    { id: expect.any(String) as unknown, last: { action: 'meal.delete', entity: breakfast.id } },
  ])
})

test("today's way to log a meal names the meal of the hour and what is left", async () => {
  fakeServer(signedIn())

  renderAt('/')

  expect(
    await screen.findByRole('button', {
      name: /^Add (breakfast|lunch|a snack|dinner) · 2,500 kcal left$/,
    }),
  ).toBeDefined()
})

test('an item of a named serving says it as it was given, with its weight', async () => {
  const [oats] = breakfast.items
  if (!oats) {
    throw new Error('the fixture has no items')
  }
  const handful = {
    ...oats,
    quantity: { servings: 2, serving: 'šaka' },
    amount: 60,
    unit: 'g' as const,
  }
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(day({ meals: [{ ...breakfast, items: [handful] }] })),
  })

  renderAt('/')

  fireEvent.click(
    within(await screen.findByRole('article')).getByRole('button', { expanded: false }),
  )
  expect(await screen.findByText('2 šaka (60 g)')).toBeDefined()
  cleanup()

  // A gram of a bag of 500 g is not none of it.
  const gram = { ...handful, quantity: { servings: 0.002, serving: 'bag' }, amount: 1 }
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(day({ meals: [{ ...breakfast, items: [gram] }] })),
  })
  renderAt('/')

  fireEvent.click(
    within(await screen.findByRole('article')).getByRole('button', { expanded: false }),
  )
  expect(await screen.findByText('0.002 bag (1 g)')).toBeDefined()
})

test("an amount of portions takes the word's form of the number shown", async () => {
  const [oats] = breakfast.items
  if (!oats) {
    throw new Error('the fixture has no items')
  }
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(
        day({ meals: [{ ...breakfast, items: [{ ...oats, amount: 0.999, unit: 'portion' }] }] }),
      ),
  })

  renderAt('/')

  fireEvent.click(
    within(await screen.findByRole('article')).getByRole('button', { expanded: false }),
  )
  expect(await screen.findByText('1 portion')).toBeDefined()
})

test('a day past its target says by how much, in words and not in colour alone', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(
        day({
          total: { kcal: 2700, protein: 170, carbs: 300, fat: 90 },
          remaining: { kcal: -200, protein: -10, carbs: -20, fat: -10 },
        }),
      ),
  })

  renderAt('/')

  const summary = await screen.findByRole('region', { name: 'Energy and macros' })
  expect(within(summary).getByText('Eaten 2,700 of 2,500 kcal, 200 over')).toBeDefined()
  expect(within(summary).getByText('+200')).toBeDefined()
  expect(within(summary).getByText('kcal over')).toBeDefined()
  expect(macroOf(summary, 'Protein')).toBe('Protein170 / 160 g · 10 over')
})

test('a day on its target has nothing left, and nothing over', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(
        day({
          total: { kcal: 2500, protein: 160, carbs: 280, fat: 80 },
          remaining: { kcal: 0, protein: 0, carbs: 0, fat: 0 },
        }),
      ),
  })

  renderAt('/')

  const summary = await screen.findByRole('region', { name: 'Energy and macros' })
  expect(within(summary).getByText('Eaten 2,500 of 2,500 kcal, 0 left')).toBeDefined()
  expect(within(summary).getByText('kcal left')).toBeDefined()
  expect(macroOf(summary, 'Fat')).toBe('Fat80 / 80 g · 0 to go')
})

// The server rounds what remains once, of the values unrounded (2400 less
// 1079.5 is 1320.5, so 1321), and the total on its own (1080): what is
// left is the target shown less what was eaten shown, in every place.
test('what is left is the target less what was eaten, as both are shown, in every place', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(
        day({
          target: { kcal: 2400, protein: 160, carbs: 280, fat: 60 },
          total: { kcal: 1080, protein: 112.4, carbs: 145.6, fat: 58.7 },
          remaining: { kcal: 1321, protein: 47.6, carbs: 134.4, fat: 1.3 },
        }),
      ),
  })

  renderAt('/')

  const summary = await screen.findByRole('region', { name: 'Energy and macros' })
  expect(within(summary).getByText('Eaten 1,080 of 2,400 kcal, 1,320 left')).toBeDefined()
  expect(within(summary).getByText('1,320')).toBeDefined()
  expect(
    screen.getByRole('button', {
      name: /^Add (breakfast|lunch|a snack|dinner) · 1,320 kcal left$/,
    }),
  ).toBeDefined()
  // 59 g eaten of 60 leave 1, not 1.3; 112 of 160 leave 48.
  expect(macroOf(summary, 'Fat')).toBe('Fat59 / 60 g · 1 to go')
  expect(macroOf(summary, 'Protein')).toBe('Protein112 / 160 g · 48 to go')
})

test('a few grams keep their decimal, many are whole', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      Response.json(
        day({
          total: { kcal: 120, protein: 12.4, carbs: 9.6, fat: 1.5 },
          remaining: { kcal: 2380, protein: 147.6, carbs: 270.4, fat: 78.5 },
        }),
      ),
  })

  renderAt('/')

  const summary = await screen.findByRole('region', { name: 'Energy and macros' })
  expect(macroOf(summary, 'Protein')).toBe('Protein12 / 160 g · 148 to go')
  expect(macroOf(summary, 'Carbs')).toBe('Carbs9.6 / 280 g · 270 to go')
  expect(macroOf(summary, 'Fat')).toBe('Fat1.5 / 80 g · 79 to go')
})

test('the TDEE calibration measures, and the deficit the target makes of it', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/summary': () =>
      Response.json({
        ...summaryOf(),
        tdee: { ...summaryOf().tdee, measured: true, trusted: true, kcal: 2950.4 },
      }),
  })

  renderAt('/')

  const summary = await screen.findByRole('region', { name: 'Energy and macros' })
  expect(await within(summary).findByText('TDEE')).toBeDefined()
  expect(termOf(summary, 'TDEE')).toBe('2,950')
  expect(termOf(summary, 'Deficit')).toBe('-450')
})

test('a TDEE not trusted yet is about so much, and a target over it a surplus', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/summary': () =>
      Response.json({
        ...summaryOf(),
        tdee: { ...summaryOf().tdee, measured: true, trusted: false, kcal: 2300 },
      }),
  })

  renderAt('/')

  const summary = await screen.findByRole('region', { name: 'Energy and macros' })
  expect(await within(summary).findByText('Surplus')).toBeDefined()
  expect(termOf(summary, 'TDEE')).toBe('≈2,300')
  expect(termOf(summary, 'Surplus')).toBe('+200')
})

test('the phase of the day, its week, and the rate against the phase', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/summary': () =>
      Response.json({
        ...summaryOf(),
        verdict: { ...summaryOf().verdict, rate: 'on_target', measured_pct_week: -0.62 },
      }),
    'GET /api/v1/plan': () =>
      Response.json({
        ...planOf(),
        phases: [{ type: 'cut', start: '2026-09-01', end: '2026-11-23', rate_pct_week: -0.75 }],
      }),
  })

  renderAt('/')

  const phase = await screen.findByRole('link', { name: /^Cut/ })
  expect(phase.textContent).toBe('Cut · week 8 of 12 · -0.6%/wk, in range')
  expect(phase.getAttribute('href')).toBe('/progress')
})

test('the week up to today, each day what it ate of its target, linked', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/history': (request) => {
      const query = new URL(request.url).searchParams
      expect([query.get('from'), query.get('to')]).toEqual(['2026-10-17', today])
      return Response.json({
        from: '2026-10-17',
        to: today,
        days: [
          {
            date: '2026-10-22',
            status: 'complete',
            type: 'rest',
            meals: 3,
            intake: { kcal: 2300, protein: 150, carbs: 250, fat: 70 },
            target: { kcal: 2500, protein: 160, carbs: 280, fat: 80 },
          },
        ],
        weeks: [],
      })
    },
  })

  renderAt('/')

  const week = await screen.findByRole('navigation', { name: 'The week' })
  const days = within(week).getAllByRole('link')
  expect(days).toHaveLength(7)
  expect(
    await within(week).findByRole('link', { name: 'Thu 22: 2,300 of 2,500 kcal' }),
  ).toBeDefined()
  // Named as it reads, so a voice that says what it sees reaches it.
  expect(days[0]?.getAttribute('aria-label')).toBe('Sat 17: nothing logged')
  expect(days[0]?.textContent).toBe('Sat 17')
  // Today, the day shown, is the current one.
  expect(days[6]?.getAttribute('aria-current')).toBe('page')
  expect(days[6]?.getAttribute('href')).toBe('/')
  expect(days[5]?.getAttribute('href')).toBe('/?date=2026-10-22')
})

test('a day before the first target says it has none, and still adds up', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () => {
      const {
        target: _target,
        remaining: _remaining,
        ...rest
      } = day({
        total: { kcal: 900, protein: 50, carbs: 100, fat: 30 },
      })
      return Response.json(rest)
    },
  })

  renderAt('/')

  const eaten = await screen.findByRole('region', { name: 'Eaten' })
  expect(within(eaten).getByText('900 kcal')).toBeDefined()
  expect(within(eaten).getByText('P 50 · C 100 · F 30')).toBeDefined()
  expect(within(eaten).getByText('A plan gives the day its target.')).toBeDefined()
  expect(within(eaten).getByRole('link', { name: 'Set up a plan' }).getAttribute('href')).toBe(
    '/plan',
  )
})

test('the days move back and ahead, up to today', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/2026-10-22': () =>
      Response.json(
        day({
          date: '2026-10-22',
          type: 'training',
          status: 'excluded',
          excluded_reason: 'restaurant',
        }),
      ),
  })
  const router = renderAt('/')

  const before = await screen.findByRole('link', { name: 'The day before' })
  expect(before.getAttribute('href')).toBe('/?date=2026-10-22')
  expect(screen.queryByRole('link', { name: 'The day after' })).toBeNull()
  fireEvent.click(before)

  expect(await screen.findByRole('heading', { name: 'Thursday, October 22' })).toBeDefined()
  expect(router.state.location.href).toBe('/?date=2026-10-22')
  expect(await screen.findByText('Training day')).toBeDefined()
  expect(screen.getByText('Out of calibration: restaurant')).toBeDefined()
  // Today is /, not /?date=… of today.
  expect(screen.getByRole('link', { name: 'The day after' }).getAttribute('href')).toBe('/')
  expect(screen.getByRole('link', { name: 'Back to today' }).getAttribute('href')).toBe('/')
})

test('a day of another year than today says its year, in each language', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/2025-10-24': () => Response.json(day({ date: '2025-10-24' })),
  })
  renderAt('/?date=2025-10-24')

  expect(await screen.findByRole('heading', { name: 'Friday, October 24, 2025' })).toBeDefined()
  cleanup()

  fakeServer({
    ...signedIn('hr'),
    'GET /api/v1/days/1900-01-01': () => Response.json(day({ date: '1900-01-01' })),
  })
  renderAt('/?date=1900-01-01')

  expect(
    await screen.findByRole('heading', { name: 'ponedjeljak, 1. siječnja 1900.' }),
  ).toBeDefined()
})

test('a day that did not come says why, and asks again', async () => {
  let up = false
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () =>
      up ? Response.json(day()) : Promise.reject(new TypeError('Failed to fetch')),
  })
  renderAt('/')

  expect((await screen.findByRole('alert')).textContent).toBe('The server cannot be reached.')

  up = true
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

  expect(await screen.findByRole('heading', { name: 'Today' })).toBeDefined()
})

test('a session that ends while the app is open sends the browser to sign in', async () => {
  let live = true
  fakeServer({
    ...signedIn(),
    ...signedOut,
    'GET /api/v1/auth/session': () => (live ? Response.json(session()) : problem(401)),
    'GET /api/v1/days/today': () => problem(401),
  })
  const router = renderAt('/')
  live = false

  expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeDefined()
  expect(router.state.location.pathname).toBe('/login')
})

test('the day speaks Croatian, its numbers too', async () => {
  fakeServer({
    ...signedIn('hr'),
    'GET /api/v1/days/today': () =>
      Response.json(
        day({
          total: { kcal: 1460, protein: 88, carbs: 146, fat: 41 },
          remaining: { kcal: 1040, protein: 72, carbs: 134, fat: 39 },
        }),
      ),
  })

  renderAt('/')

  expect(await screen.findByRole('heading', { name: 'Danas' })).toBeDefined()
  expect(screen.getByText('petak, 23. listopada')).toBeDefined()
  expect(screen.getByRole('button', { name: 'Tip dana: Dan odmora' })).toBeDefined()
  const summary = screen.getByRole('region', { name: 'Energija i makroi' })
  expect(within(summary).getByText('Pojedeno 1.460 od 2.500 kcal, preostalo 1.040')).toBeDefined()
  expect(macroOf(summary, 'Proteini')).toBe('Proteini88 / 160 g · još 72')
})

test('on another day, the section is still the current one, and no link to a day is', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/2026-10-22': () => Response.json(day({ date: '2026-10-22' })),
  })

  renderAt('/?date=2026-10-22')

  expect(await screen.findByRole('link', { name: 'Back to today' })).toBeDefined()
  expect(currentIn('Sections').map((a) => a.textContent)).toEqual(['Today'])
  // And in the week, the day shown.
  expect(currentIn('The week').map((a) => a.getAttribute('href'))).toEqual(['/?date=2026-10-22'])
  const others = screen
    .getAllByRole('link')
    .filter((a) => a.getAttribute('aria-current') === 'page' && a.closest('nav') === null)
  expect(others).toEqual([])
})

test('a date that is no date is today', async () => {
  const fetch = fakeServer(signedIn())

  const router = renderAt('/?date=2026-02-31')

  expect(await screen.findByRole('heading', { name: 'Today' })).toBeDefined()
  expect(router.state.location.href).toBe('/')
  const paths = fetch.mock.calls.map(([r]) => (r instanceof Request ? new URL(r.url).pathname : r))
  expect(paths).not.toContain('/api/v1/days/2026-02-31')
})

test('/ is today though what the app knew of today is older', async () => {
  // The day's start hour passed while the app was open: the day came
  // again, /api/v1/me not yet.
  fakeServer({
    ...signedIn(),
    'GET /api/v1/me': () => Response.json({ ...me(), today: '2026-10-22' }),
  })

  renderAt('/')

  expect(await screen.findByRole('heading', { name: 'Today' })).toBeDefined()
  expect(screen.queryByRole('link', { name: 'Back to today' })).toBeNull()
  expect(screen.getByRole('link', { name: 'The day before' }).getAttribute('href')).toBe(
    '/?date=2026-10-22',
  )
})

test('another day keeps its way back to today when the server cannot say which day it is', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/me': () => problem(500),
    'GET /api/v1/days/2026-10-20': () => Response.json(day({ date: '2026-10-20' })),
  })

  renderAt('/?date=2026-10-20')

  expect(await screen.findByRole('link', { name: 'Back to today' })).toBeDefined()
})

test('a day the server refuses is not blamed on the server', async () => {
  fakeServer({ ...signedIn(), 'GET /api/v1/days/today': () => problem(422) })

  renderAt('/')

  expect((await screen.findByRole('alert')).textContent).toBe('Something went wrong.')
})

test('while another day comes, the page says which, and the one before waits dimmed', async () => {
  let answer: ((r: Response) => void) | undefined
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/2026-10-22': () =>
      new Promise<Response>((resolve) => {
        answer = resolve
      }),
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('link', { name: 'The day before' }))

  expect(await screen.findByRole('heading', { name: 'Thursday, October 22' })).toBeDefined()
  expect(screen.getByRole('link', { name: 'The day before' }).getAttribute('href')).toBe(
    '/?date=2026-10-21',
  )
  expect(screen.getByRole('link', { name: 'The day after' }).getAttribute('href')).toBe('/')
  expect(energy().closest('[aria-busy="true"]')).not.toBeNull()

  answer?.(Response.json(day({ date: '2026-10-22' })))

  await waitFor(() => {
    expect(energy().closest('[aria-busy="true"]')).toBeNull()
  })
})
