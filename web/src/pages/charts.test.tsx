import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { fakeServer, problem, renderAt, signedIn, today } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  localStorage.clear()
})

type HistoryDay = components['schemas']['HistoryDayBody']

// historyOf is a history of n days up to today, made up: a day of 2000
// kcal, a weight falling 0.1 kg a day, 7.5 hours of sleep and 8000 steps.
function historyOf(from: string): HistoryDay[] {
  const first = Date.parse(`${from}T00:00:00Z`)
  const last = Date.parse(`${today}T00:00:00Z`)
  const days: HistoryDay[] = []
  for (let at = first, i = 0; at <= last; at += 86_400_000, i++) {
    days.push({
      date: new Date(at).toISOString().slice(0, 10),
      status: 'complete',
      type: 'rest',
      meals: 1,
      intake: { kcal: 2000, protein: 150, carbs: 200, fat: 67 },
      target: { kcal: 2100, protein: 150, carbs: 220, fat: 60 },
      weight_kg: 85 - 0.1 * i,
      trend_kg: 85 - 0.1 * i,
      body_fat_pct: 20,
      sleep_h: 7.5,
      steps: 8000,
      resting_hr: 58,
      hrv: 50,
      tdee: { kcal: 2400, half_width: 60, trusted: true, before_phase: false, to: today },
    })
  }
  return days
}

function historyServer(over: Partial<Record<string, (r: Request) => Response>> = {}) {
  return fakeServer({
    ...signedIn(),
    'GET /api/v1/history': (request) => {
      const from = new URL(request.url).searchParams.get('from') ?? today
      return Response.json({ from, to: today, days: historyOf(from), weeks: [] })
    },
    ...over,
  })
}

test('the charts say in words what they show, over 90 days by default', async () => {
  const fetch = historyServer()
  renderAt('/charts')

  // The charts' module, ECharts with it, loads when they are first shown.
  const weight = await screen.findByRole(
    'region',
    { name: 'Weight and trend' },
    { timeout: 10_000 },
  )
  expect(within(weight).getByText('Trend weight from 85 to 76.1 kg.')).toBeDefined()
  expect(
    within(screen.getByRole('region', { name: 'What was eaten' })).getByText(
      '2,000 kcal a day on average, over 90 logged days.',
    ),
  ).toBeDefined()
  expect(screen.getByText('TDEE 2,400 ± 60 kcal a day, as of the last day.')).toBeDefined()
  expect(screen.getByText('7.5 hours a night on average.')).toBeDefined()
  expect(screen.getByText('Resting heart rate 58, HRV 50, on average.')).toBeDefined()
  expect(screen.getByText('Body fat 20 %, as of the last day with it.')).toBeDefined()
  expect(screen.getByText('8,000 steps a day on average.')).toBeDefined()
  const asked = fetch.mock.calls
    .map(([r]) => (r instanceof Request ? new URL(r.url) : undefined))
    .find((u) => u?.pathname === '/api/v1/history')
  expect(asked?.searchParams.get('from')).toBe('2026-07-26')
  // Each chart is drawn.
  await waitFor(() => {
    expect(weight.querySelector('svg')).not.toBeNull()
  })
})

test('a span of days is chosen, and kept in the address', async () => {
  const fetch = historyServer()
  const router = renderAt('/charts')

  fireEvent.click(await screen.findByRole('button', { name: '30 days' }))

  expect(await screen.findByText('2,000 kcal a day on average, over 30 logged days.')).toBeDefined()
  expect(router.state.location.search).toEqual({ days: 30 })
  expect(screen.getByRole('button', { name: '30 days' }).getAttribute('aria-pressed')).toBe('true')
  const froms = fetch.mock.calls
    .map(([r]) => (r instanceof Request ? new URL(r.url) : undefined))
    .filter((u) => u?.pathname === '/api/v1/history')
    .map((u) => u?.searchParams.get('from'))
  expect(froms.at(-1)).toBe('2026-09-24')
})

test('days with nothing logged say so', async () => {
  historyServer({
    'GET /api/v1/history': () =>
      Response.json({
        from: today,
        to: today,
        days: [{ date: today, status: 'open', type: 'rest', meals: 0 }],
        weeks: [],
      }),
  })
  renderAt('/charts')

  expect(await screen.findByText('Nothing logged in these days yet.')).toBeDefined()
})

test('a history that did not come says why, and asks again', async () => {
  let up = false
  historyServer({
    'GET /api/v1/history': (request) => {
      if (!up) {
        return problem(500)
      }
      const from = new URL(request.url).searchParams.get('from') ?? today
      return Response.json({ from, to: today, days: historyOf(from), weeks: [] })
    },
  })
  renderAt('/charts')

  expect((await screen.findByRole('alert')).textContent).toBe('The server failed to answer.')
  up = true
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

  expect(await screen.findByRole('region', { name: 'Weight and trend' })).toBeDefined()
})

test('the charts speak Croatian', async () => {
  fakeServer({
    ...signedIn('hr'),
    'GET /api/v1/history': (request) => {
      const from = new URL(request.url).searchParams.get('from') ?? today
      return Response.json({ from, to: today, days: historyOf(from), weeks: [] })
    },
  })
  renderAt('/charts')

  expect(
    await screen.findByText('Prosječno 2.000 kcal na dan, kroz 90 upisanih dana.'),
  ).toBeDefined()
  expect(screen.getByRole('button', { name: '30 dana' })).toBeDefined()
})
