import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { palette } from '@/components/charts/palette'
import { addDays } from '@/i18n/format'
import { setLocale } from '@/i18n/locale'
import { setTheme } from '@/lib/theme'
import { fakeServer, me, problem, renderAt, signedIn, today, type Handler } from '@/test/app'
import { themeStyle, themeTokens } from '@/test/charts'

// The first test of the file loads the charts' module, ECharts with it,
// which a busy machine takes seconds to.
vi.setConfig({ testTimeout: 20_000 })

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  act(() => {
    setLocale('en')
    setTheme('system')
  })
  localStorage.clear()
  for (const style of document.head.querySelectorAll('style')) {
    style.remove()
  }
})

type HistoryDay = components['schemas']['HistoryDayBody']
type Week = components['schemas']['WeekBody']
type Summary = components['schemas']['SummaryBody']
type Verdict = components['schemas']['VerdictBody']

// historyOf is a history of the days from from up to today, made up: a day
// of 2000 kcal, a weight falling 0.1 kg a day, 7.5 hours of sleep and
// 8000 steps. Today is open.
function historyOf(from: string): HistoryDay[] {
  const days: HistoryDay[] = []
  for (let date = from, i = 0; date <= today; date = addDays(date, 1), i++) {
    days.push({
      date,
      status: date === today ? 'open' : 'complete',
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

// weeksOf are the weeks of 7 days ending on to, back to from, made up: the
// trend weight 0.5 kg less each, 2000 kcal and 150 g of protein a day
// over 6 days logged.
function weeksOf(from: string, to: string): Week[] {
  const weeks: Week[] = []
  for (let end = to, i = 0; end >= from; end = addDays(end, -7), i++) {
    weeks.unshift({
      from: addDays(end, -6),
      to: end,
      kcal: 2000,
      protein: 150,
      logged_days: 6,
      weigh_ins: 5,
      training_days: 2,
      trend_kg: 80 + 0.5 * i,
    })
  }
  return weeks
}

const onTrack: Verdict = {
  action: 'none',
  flags: [],
  gap_kcal: 0,
  guards: [],
  rate: 'on_target',
  reasons: [],
  step_kcal: 0,
  measured_pct_week: -0.68,
  target_pct_week: -0.75,
  tolerance_pct_week: 0.1,
  intake_kcal: 2050,
  target_kcal: 2100,
}

// none is a verdict of no rate, for its reasons.
function none(...reasons: string[]): Verdict {
  return { action: 'none', flags: [], gap_kcal: 0, guards: [], rate: 'none', reasons, step_kcal: 0 }
}

const trusted: Summary['tdee'] = {
  before_phase: false,
  doubts: [],
  from: '2026-09-11',
  to: '2026-10-22',
  logged_days: 40,
  weigh_ins: 38,
  measured: true,
  trusted: true,
  kcal: 2400,
  half_width: 60,
  kg_per_week: -0.6,
}

// summaryOf is a summary of a verdict, a cut on track by default, and a
// TDEE, trusted by default.
function summaryOf(verdict: Verdict = onTrack, tdee: Summary['tdee'] = trusted): Summary {
  return {
    date: today,
    incomplete_days: [],
    recovery: { flags: [] },
    tdee,
    trend: { date: today, kg: 84.2 },
    verdict,
    weeks: [],
  }
}

// A cut since September, as /me has it.
const cut: components['schemas']['PhaseBody'] = {
  type: 'cut',
  start: '2026-09-01',
  end: '2026-11-23',
  rate_pct_week: -0.75,
  goal_weight: 78,
}

// historyAnswer answers a history as the server does: its days, or by
// week its weeks.
function historyAnswer(request: Request, days: (from: string) => HistoryDay[] = historyOf) {
  const query = new URL(request.url).searchParams
  const from = query.get('from') ?? today
  const to = query.get('to') ?? today
  return query.get('granularity') === 'week'
    ? Response.json({ from, to, days: [], weeks: weeksOf(from, to) })
    : Response.json({ from, to, days: days(from), weeks: [] })
}

function progressServer(over: Record<string, Handler> = {}) {
  return fakeServer({
    ...signedIn(),
    'GET /api/v1/me': () => Response.json({ ...me(), phase: cut }),
    'GET /api/v1/summary': () => Response.json(summaryOf()),
    'GET /api/v1/history': (r) => historyAnswer(r),
    ...over,
  })
}

// asked are the histories asked for, each as its query.
function asked(fetch: ReturnType<typeof fakeServer>): URLSearchParams[] {
  return fetch.mock.calls.flatMap(([r]) => {
    const url = r instanceof Request ? new URL(r.url) : undefined
    return url?.pathname === '/api/v1/history' ? [url.searchParams] : []
  })
}

// hero is the verdict on the phase, once it has come.
async function hero() {
  // The page's module loads with its route, the first time a while.
  const region = await screen.findByRole(
    'region',
    { name: 'The phase so far' },
    { timeout: 10_000 },
  )
  await waitFor(() => {
    expect(region.getAttribute('aria-busy')).toBeNull()
  })
  return region
}

test('the progress says how the phase goes, then the charts and the weeks, over 90 days', async () => {
  const fetch = progressServer()
  renderAt('/progress')

  const verdict = await hero()
  expect(within(verdict).getByText('Cut · week 8 of 12')).toBeDefined()
  expect(within(verdict).getByText('On track.')).toBeDefined()
  expect(
    within(verdict).getByText(
      "You're losing 0.68 % a week, within the phase's range (0.65–0.85 %).",
    ),
  ).toBeDefined()
  expect(within(verdict).getByText('TDEE 2,400 ± 60 kcal')).toBeDefined()
  expect(within(verdict).getByText('Reliable')).toBeDefined()
  expect(within(verdict).getByText('Trend weight 84.2 kg')).toBeDefined()
  expect(within(verdict).getByText('Eaten 2,050 of 2,100 kcal a day')).toBeDefined()

  // The charts' module, ECharts with it, loads when they are first shown.
  const weight = await screen.findByRole('region', { name: 'Weight' }, { timeout: 10_000 })
  expect(
    within(weight).getByRole('img', {
      name: 'Trend weight from 85 to 76.1 kg, 90 weigh-ins.',
    }),
  ).toBeDefined()
  expect(within(weight).getByText('76.1 kg')).toBeDefined()
  expect(within(weight).getByText('-8.9 kg in these days')).toBeDefined()
  const tdee = screen.getByRole('region', { name: 'TDEE and intake' })
  expect(
    within(tdee).getByRole('img', {
      name: 'TDEE 2,400 ± 60 kcal a day, as of the last day. 2,000 kcal a day eaten on average, over 89 logged days.',
    }),
  ).toBeDefined()
  // Each chart is drawn.
  await waitFor(() => {
    expect(weight.querySelector('svg')).not.toBeNull()
  })

  const histories = asked(fetch)
  expect(histories.find((q) => q.get('granularity') === null)?.get('from')).toBe('2026-07-26')
  // The weeks up to yesterday, 13 of them and the one before.
  const weekly = histories.find((q) => q.get('granularity') === 'week')
  expect([weekly?.get('from'), weekly?.get('to')]).toEqual(['2026-07-23', '2026-10-22'])
})

test.each([
  {
    state: 'too fast',
    summary: summaryOf({ ...onTrack, rate: 'below', measured_pct_week: -1.2 }),
    headline: 'Too fast.',
    said: "You're losing 1.2 % a week, outside the phase's range (0.65–0.85 %).",
  },
  {
    state: 'too slow',
    summary: summaryOf({ ...onTrack, rate: 'above', measured_pct_week: -0.3 }),
    headline: 'Too slow.',
    said: "You're losing 0.3 % a week, outside the phase's range (0.65–0.85 %).",
  },
  {
    state: 'too slow, the weight going the other way',
    summary: summaryOf({ ...onTrack, rate: 'above', measured_pct_week: 0.2 }),
    headline: 'Too slow.',
    said: "You're gaining 0.2 % a week, outside the phase's range (-0.85 to -0.65 %).",
  },
  {
    state: 'gaining too fast on a bulk',
    summary: summaryOf({
      ...onTrack,
      rate: 'above',
      measured_pct_week: 0.6,
      target_pct_week: 0.25,
    }),
    headline: 'Too fast.',
    said: "You're gaining 0.6 % a week, outside the phase's range (0.15–0.35 %).",
  },
  {
    state: 'drifting in maintenance',
    summary: summaryOf({ ...onTrack, rate: 'below', measured_pct_week: -0.3, target_pct_week: 0 }),
    headline: 'Too fast.',
    said: "You're losing 0.3 % a week, outside the phase's range (± 0.1 %).",
  },
])('the verdict says a phase going $state', async ({ summary, headline, said }) => {
  progressServer({ 'GET /api/v1/summary': () => Response.json(summary) })
  renderAt('/progress')

  const verdict = await hero()
  expect(within(verdict).getByText(headline)).toBeDefined()
  expect(within(verdict).getByText(said)).toBeDefined()
})

test('without enough data, the verdict says why, what there is, and how far the TDEE holds', async () => {
  progressServer({
    'GET /api/v1/summary': () =>
      Response.json(
        summaryOf(none('not_trusted'), {
          ...trusted,
          trusted: false,
          kcal: 3034,
          half_width: 927,
          doubts: ['wide_interval'],
        }),
      ),
  })
  renderAt('/progress')

  const verdict = await hero()
  expect(within(verdict).getByText('Not enough data yet.')).toBeDefined()
  expect(
    within(verdict).getByText(
      'The TDEE is measured, but not reliable enough yet for a verdict. So far 40 logged days and 38 weigh-ins.',
    ),
  ).toBeDefined()
  expect(within(verdict).getByText('TDEE 3,034 ± 927 kcal')).toBeDefined()
  expect(within(verdict).getByText('Not reliable yet')).toBeDefined()
})

test('without a phase, the verdict leads to the plan, and the period has no phase', async () => {
  progressServer({
    'GET /api/v1/me': () => Response.json(me()),
    'GET /api/v1/summary': () =>
      Response.json(
        summaryOf(none('no_phase', 'no_target'), {
          before_phase: false,
          doubts: ['few_days'],
          from: '2026-10-23',
          to: '2026-10-22',
          logged_days: 0,
          weigh_ins: 0,
          measured: false,
          trusted: false,
        }),
      ),
  })
  const router = renderAt('/progress?days=phase')

  const verdict = await hero()
  expect(screen.queryByRole('radio', { name: 'The phase' })).toBeNull()
  expect(screen.getByRole<HTMLInputElement>('radio', { name: '90 days' }).checked).toBe(true)
  expect(within(verdict).queryByRole('list')).toBeNull()
  expect(within(verdict).getByText('No phase yet.')).toBeDefined()
  expect(
    within(verdict).getByText('A phase of the plan sets the rate the weight is judged by.'),
  ).toBeDefined()
  fireEvent.click(within(verdict).getByRole('link', { name: 'Set up a plan' }))
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/plan')
  })
})

test('a phase with no target says so, and opens the plan', async () => {
  progressServer({
    'GET /api/v1/summary': () => Response.json(summaryOf(none('no_target'))),
  })
  renderAt('/progress')

  const verdict = await hero()
  expect(within(verdict).getByText('No target yet.')).toBeDefined()
  expect(within(verdict).getByRole('link', { name: 'Open the plan' }).getAttribute('href')).toBe(
    '/plan',
  )
})

test('a period is chosen, and kept in the address; the phase is one', async () => {
  const fetch = progressServer()
  const router = renderAt('/progress')

  fireEvent.click(await screen.findByRole('radio', { name: '30 days' }))

  expect(
    await screen.findByText('Eaten 2,000 kcal a day, over 29 logged days', {}, { timeout: 10_000 }),
  ).toBeDefined()
  expect(router.state.location.search).toEqual({ days: 30 })
  expect(screen.getByRole<HTMLInputElement>('radio', { name: '30 days' }).checked).toBe(true)
  expect(
    asked(fetch)
      .findLast((q) => q.get('granularity') === null)
      ?.get('from'),
  ).toBe('2026-09-24')

  fireEvent.click(screen.getByRole('radio', { name: 'The phase' }))
  await waitFor(() => {
    expect(router.state.location.search).toEqual({ days: 'phase' })
  })
  await waitFor(() => {
    expect(asked(fetch).some((q) => q.get('from') === cut.start)).toBe(true)
  })

  fireEvent.click(screen.getByRole('radio', { name: '90 days' }))
  await waitFor(() => {
    expect(router.state.location.search).toEqual({})
  })
})

test("a year's period asks for a year, its weeks a year's, the year said where it changes", async () => {
  const fetch = progressServer()
  renderAt('/progress?days=365')

  await screen.findByRole('region', { name: 'Weight' }, { timeout: 10_000 })
  const histories = asked(fetch)
  expect(histories.find((q) => q.get('granularity') === null)?.get('from')).toBe('2025-10-24')
  const weekly = histories.find((q) => q.get('granularity') === 'week')
  expect([weekly?.get('from'), weekly?.get('to')]).toEqual(['2025-10-23', '2026-10-22'])

  // A week of last year says its year.
  const weeks = await screen.findByRole('table', { name: 'By week' })
  fireEvent.click(screen.getByRole('button', { name: 'Show all weeks (52)' }))
  const rows = within(weeks).getAllByRole('row')
  expect(rows[1]?.querySelector('th')?.textContent).toBe('Oct 16')
  expect(rows.at(-1)?.querySelector('th')?.textContent).toBe('Oct 24, 2025')
  // A year's nights in small are its weeks'.
  const sleep = screen.getByRole('img', { name: '7.5 hours a night on average.' })
  expect(sleep.querySelectorAll('rect')).toHaveLength(53)
})

test('the weeks are a table, the latest first, the rest a tap away', async () => {
  progressServer()
  renderAt('/progress')

  const weeks = await screen.findByRole('table', { name: 'By week' }, { timeout: 10_000 })
  const rows = within(weeks).getAllByRole('row')
  expect(rows[0]?.textContent).toBe(
    'Week ofkcalkcal a dayPProtein, g a dayΔ kgChange of the trend weight, kgDaysDays logged',
  )
  // 8 weeks, the latest first: 2026-10-16 to 22.
  expect(rows).toHaveLength(9)
  expect(rows[1]?.textContent).toBe('Oct 162,000150-0.56/7')
  fireEvent.click(screen.getByRole('button', { name: 'Show all weeks (13)' }))
  expect(within(weeks).getAllByRole('row')).toHaveLength(14)
  expect(screen.getByRole('button', { name: 'Show fewer' }).getAttribute('aria-expanded')).toBe(
    'true',
  )
})

test('the measurements are small charts, each a whole one a tap away', async () => {
  progressServer()
  renderAt('/progress')

  const body = await screen.findByRole('region', { name: 'Body and recovery' }, { timeout: 10_000 })
  expect(within(body).getByRole('img', { name: '7.5 hours a night on average.' })).toBeDefined()
  expect(within(body).getByText('7.5 h')).toBeDefined()
  expect(within(body).getByText('58 bpm')).toBeDefined()
  expect(within(body).getByText('50 ms')).toBeDefined()
  expect(within(body).getByText('8,000')).toBeDefined()
  expect(within(body).getByText('20%')).toBeDefined()
  // What the colours of the nights mean.
  expect(within(body).getByText('7 h or more')).toBeDefined()
  expect(within(body).getByText('Under 6 h')).toBeDefined()

  fireEvent.click(within(body).getByRole('button', { name: 'Resting heart rate' }))
  const sheet = await screen.findByRole('dialog', { name: 'Resting heart rate' })
  expect(
    within(sheet).getByRole('img', { name: 'Resting heart rate 58 bpm on average.' }),
  ).toBeDefined()
})

test('days with nothing logged say so, and how to start', async () => {
  progressServer({
    'GET /api/v1/history': (r) =>
      historyAnswer(r, () => [{ date: today, status: 'open', type: 'rest', meals: 0 }]),
  })
  const router = renderAt('/progress')

  expect(
    await screen.findByText('Nothing logged in these days yet.', {}, { timeout: 10_000 }),
  ).toBeDefined()
  fireEvent.click(within(screen.getByRole('main')).getByRole('link', { name: 'Log a meal' }))
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/')
  })
})

test('a history that did not come says why, and asks again', async () => {
  let up = false
  progressServer({
    'GET /api/v1/history': (r) => (up ? historyAnswer(r) : problem(500)),
  })
  renderAt('/progress')

  expect((await screen.findByRole('alert')).textContent).toBe('The server failed to answer.')
  up = true
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

  expect(await screen.findByRole('region', { name: 'Weight' }, { timeout: 10_000 })).toBeDefined()
})

test('a summary that did not come says why in the hero, and asks again', async () => {
  let up = false
  progressServer({
    'GET /api/v1/summary': () => (up ? Response.json(summaryOf()) : problem(503)),
  })
  renderAt('/progress')

  const verdict = await screen.findByRole('region', { name: 'The phase so far' })
  expect((await within(verdict).findByRole('alert')).textContent).toBeTruthy()
  up = true
  fireEvent.click(within(verdict).getByRole('button', { name: 'Try again' }))

  expect(await within(verdict).findByText('On track.')).toBeDefined()
})

test('the progress speaks Croatian', async () => {
  fakeServer({
    ...signedIn('hr'),
    'GET /api/v1/me': () => Response.json({ ...me(), locale: 'hr', phase: cut }),
    'GET /api/v1/summary': () => Response.json(summaryOf()),
    'GET /api/v1/history': (r) => historyAnswer(r),
  })
  renderAt('/progress')

  const verdict = await screen.findByRole('region', { name: 'Faza dosad' })
  expect(await within(verdict).findByText('Na putu.')).toBeDefined()
  expect(
    within(verdict).getByText('Gubiš 0,68 % tjedno, u rasponu faze (0,65–0,85 %).'),
  ).toBeDefined()
  expect(within(verdict).getByText('TDEE 2.400 ± 60 kcal')).toBeDefined()
  expect(
    await screen.findByText(
      'Pojedeno 2.000 kcal na dan, kroz 89 upisanih dana',
      {},
      { timeout: 10_000 },
    ),
  ).toBeDefined()
  expect(screen.getByRole('radio', { name: '30 dana' })).toBeDefined()
  expect(screen.getByRole('table', { name: 'Po tjednima' })).toBeDefined()
})

test("today's open day and a day left out count in no average, as calibration counts", async () => {
  progressServer({
    'GET /api/v1/history': (r) =>
      historyAnswer(r, (from) => {
        const days = historyOf(from)
        // Today open, the first day left out.
        const [first] = days
        const now = days.at(-1)
        if (first && now) {
          Object.assign(now, {
            status: 'open',
            intake: { kcal: 600, protein: 30, carbs: 60, fat: 20 },
            steps: 1000,
          })
          Object.assign(first, {
            status: 'excluded',
            intake: { kcal: 5000, protein: 1, carbs: 1, fat: 1 },
            steps: 30000,
          })
        }
        return days
      }),
  })
  renderAt('/progress')

  expect(
    await screen.findByText('Eaten 2,000 kcal a day, over 88 logged days', {}, { timeout: 10_000 }),
  ).toBeDefined()
  expect(screen.getByRole('img', { name: '8,000 steps a day on average.' })).toBeDefined()
})

test('a measurement without a value in the days has no chart, and 0 steps are none', async () => {
  progressServer({
    'GET /api/v1/history': (r) =>
      historyAnswer(r, () => [
        { date: today, status: 'complete', type: 'rest', meals: 0, steps: 0, hrv: 45 },
      ]),
  })
  renderAt('/progress')

  const body = await screen.findByRole('region', { name: 'Body and recovery' }, { timeout: 10_000 })
  expect(within(body).getByRole('button', { name: 'HRV' })).toBeDefined()
  expect(within(body).queryByRole('button', { name: 'Steps' })).toBeNull()
  expect(within(body).queryByRole('button', { name: 'Sleep' })).toBeNull()
  // Neither main chart has a value to sum up.
  expect(screen.getAllByRole('img', { name: 'No values in these days.' })).toHaveLength(2)
})

test("the charts take the dark theme's tokens when it is chosen, and draw again", async () => {
  document.head.append(themeStyle())
  const { light, dark } = themeTokens()
  progressServer()
  renderAt('/progress')

  const weight = await screen.findByRole('region', { name: 'Weight' }, { timeout: 10_000 })
  const svg = () => weight.querySelector('svg')?.outerHTML ?? ''
  await waitFor(() => {
    expect(svg()).toContain(light['--primary'])
  })
  expect(svg()).not.toContain(palette.primary)

  act(() => {
    setTheme('dark')
  })
  await waitFor(() => {
    expect(svg()).toContain(dark['--primary'])
  })
  expect(dark['--primary']).toBe(palette.primary)
  expect(svg()).not.toContain(light['--primary'])
})
