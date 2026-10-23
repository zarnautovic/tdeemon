import { cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Meal } from '@/api/types'
import { setLocale } from '@/i18n/locale'
import { day, fakeServer, problem, renderAt, session, signedIn, signedOut } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

const breakfast: Meal = {
  id: '0193c3a5-0000-7000-8000-0000000000a1',
  // 08:45 in Zagreb, the session user's time zone.
  eaten_at: '2026-10-23T06:45:00Z',
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
          ],
        }),
      ),
  })

  renderAt('/')

  expect(await screen.findByRole('heading', { name: 'Today' })).toBeDefined()
  expect(screen.getByText('Friday, October 23 · Rest day')).toBeDefined()
  const eaten = screen.getByRole('region', { name: 'Eaten' })
  expect(within(eaten).getByText('of 2,500 kcal')).toBeDefined()
  expect(within(eaten).getByText('1,040 kcal left')).toBeDefined()
  expect(within(eaten).getByText('88 / 160 g')).toBeDefined()

  const meal = within(screen.getByRole('region', { name: 'Meals' })).getByRole('article')
  expect(within(meal).getByRole('heading').textContent).toBe('08:45 AM Oats with a banana')
  expect(within(meal).getByText('520 kcal')).toBeDefined()
  const items = within(meal).getAllByRole('listitem')
  expect(items[0]?.textContent).toBe('Oats 80 g300 kcal · P 11 · C 54 · F 6')
  expect(items[1]?.textContent).toContain('Banana bread 1.5 portions')

  const measurements = screen.getByRole('region', { name: 'Measurements' })
  expect(within(measurements).getByText('72.5 kg')).toBeDefined()
  expect(within(measurements).getByText('8,042')).toBeDefined()
})

test('a day past its target says by how much', async () => {
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

  expect(await screen.findByText('200 kcal over')).toBeDefined()
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

  expect(
    await screen.findByText('No target yet: the plan sets one. P 50 · C 100 · F 30'),
  ).toBeDefined()
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
  expect(screen.getByText('Training day')).toBeDefined()
  expect(screen.getByText('Out of calibration: restaurant')).toBeDefined()
  // Today is /, not /?date=… of today.
  expect(screen.getByRole('link', { name: 'The day after' }).getAttribute('href')).toBe('/')
  expect(screen.getByRole('link', { name: 'Back to today' }).getAttribute('href')).toBe('/')
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
  expect(screen.getByText('petak, 23. listopada · Dan odmora')).toBeDefined()
  expect(screen.getByText('još 1.040 kcal')).toBeDefined()
})
