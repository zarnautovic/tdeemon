import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { palette } from '@/components/charts/palette'
import { weekStart } from '@/components/progress/span'
import { addDays, daysBetween } from '@/i18n/format'
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

// weeksOf are the calendar weeks, Monday to Sunday, the last up to to, back
// to from, made up: the trend weight 0.5 kg less each, 2000 kcal and 150 g
// of protein a day over 6 days logged, or all of a shorter week's.
function weeksOf(from: string, to: string): Week[] {
  const weeks: Week[] = []
  for (let end = to, i = 0; end >= from; end = addDays(weekStart(end), -1), i++) {
    weeks.unshift({
      from: weekStart(end),
      to: end,
      kcal: 2000,
      protein: 150,
      logged_days: Math.min(6, daysBetween(weekStart(end), end) + 1),
      weigh_ins: 5,
      training_days: 2,
      trend_kg: 80 + 0.5 * i,
    })
  }
  return weeks
}

// needs are what calibration counts toward a TDEE, of a window that has
// all it needs.
const needs: Summary['tdee']['needs'] = {
  days: { has: 42, need: 14 },
  logged_days: { has: 40, need: 7 },
  weigh_ins: { has: 38, need: 6 },
  weigh_in_days: { has: 41, need: 7 },
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
  needs,
  trust: { half_width: 200, weigh_ins: 3, logged_days: 4 },
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
    within(verdict).getByText("You're losing 0.68% a week, within the phase's range (0.65–0.85%)."),
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
  // What was eaten against the TDEE comes to a sentence: 2,000 a day
  // against 2,400, 400 kcal a day, 2,800 a week, about 0.4 kg.
  expect(
    within(tdee).getByText('You eat about 400 kcal under what you burn: about -0.4 kg a week.'),
  ).toBeDefined()
  // Each chart is drawn.
  await waitFor(() => {
    expect(weight.querySelector('svg')).not.toBeNull()
  })

  const histories = asked(fetch)
  expect(histories.find((q) => q.get('granularity') === null)?.get('from')).toBe('2026-07-26')
  // The calendar weeks up to yesterday, Thursday: 14 of them, from the
  // Monday of Sunday the 26th, and the one before.
  const weekly = histories.find((q) => q.get('granularity') === 'week')
  expect([weekly?.get('from'), weekly?.get('to')]).toEqual(['2026-07-13', '2026-10-22'])
})

test.each([
  // Within a step of the weights, the goal is drawn.
  { goal: 78, drawn: true, note: '-8.9 kg in these days' },
  // Far below them, the axis would squeeze them: the card says it.
  { goal: 65, drawn: false, note: '-8.9 kg in these days · Goal 65 kg' },
])('a goal of $goal kg near the weights is drawn, else said', async ({ goal, drawn, note }) => {
  progressServer({
    'GET /api/v1/me': () => Response.json({ ...me(), phase: { ...cut, goal_weight: goal } }),
  })
  renderAt('/progress')

  const weight = await screen.findByRole('region', { name: 'Weight' }, { timeout: 10_000 })
  expect(within(weight).getByText(note)).toBeDefined()
  expect(within(weight).queryByText('Goal') !== null).toBe(drawn)
  await waitFor(() => {
    expect(weight.querySelector('svg')).not.toBeNull()
  })
  // The axis holds the weights, from 76 to 85 kg, by 2 kg, not the goal
  // too, by 5.
  const labels = [...weight.querySelectorAll('svg text')].map((t) => t.textContent)
  expect(labels).toContain('84')
  expect(labels).not.toContain('70')
})

test("the space between the TDEE and the 7 days' intake is shaded, and said, either way", async () => {
  progressServer({
    'GET /api/v1/history': (r) =>
      historyAnswer(r, (from) =>
        historyOf(from).map((d) =>
          Object.assign(d, { intake: { kcal: 2700, protein: 150, carbs: 300, fat: 90 } }),
        ),
      ),
  })
  renderAt('/progress')

  const tdee = await screen.findByRole('region', { name: 'TDEE and intake' }, { timeout: 10_000 })
  expect(
    within(tdee).getByText('You eat about 300 kcal over what you burn: about +0.3 kg a week.'),
  ).toBeDefined()
  expect(within(tdee).getByText('Eaten to TDEE, 7-day average')).toBeDefined()
})

test.each([
  {
    state: 'too fast',
    summary: summaryOf({ ...onTrack, rate: 'below', measured_pct_week: -1.2 }),
    headline: 'Too fast.',
    said: "You're losing 1.2% a week, outside the phase's range (0.65–0.85%).",
  },
  {
    state: 'too slow',
    summary: summaryOf({ ...onTrack, rate: 'above', measured_pct_week: -0.3 }),
    headline: 'Too slow.',
    said: "You're losing 0.3% a week, outside the phase's range (0.65–0.85%).",
  },
  {
    state: 'too slow, the weight going the other way',
    summary: summaryOf({ ...onTrack, rate: 'above', measured_pct_week: 0.2 }),
    headline: 'Too slow.',
    said: "You're gaining 0.2% a week, outside the phase's range (-0.85 to -0.65%).",
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
    said: "You're gaining 0.6% a week, outside the phase's range (0.15–0.35%).",
  },
  {
    state: 'drifting in maintenance',
    summary: summaryOf({ ...onTrack, rate: 'below', measured_pct_week: -0.3, target_pct_week: 0 }),
    headline: 'Too fast.',
    said: "You're losing 0.3% a week, outside the phase's range (± 0.1%).",
  },
])('the verdict says a phase going $state', async ({ summary, headline, said }) => {
  progressServer({ 'GET /api/v1/summary': () => Response.json(summary) })
  renderAt('/progress')

  const verdict = await hero()
  expect(within(verdict).getByText(headline)).toBeDefined()
  expect(within(verdict).getByText(said)).toBeDefined()
})

// calibration is calibration's card in the verdict's place, once it has
// come.
async function calibration() {
  return screen.findByRole('region', { name: 'Calibration' }, { timeout: 10_000 })
}

// A TDEE measured but not reliable yet: what it needs to be, a sentence
// for each doubt, by the summary's own thresholds.
test.each([
  ['wide_interval', 'It is once ± is 200 or less; weighing in 3× a week helps most.'],
  ['few_weigh_ins', 'It needs 3 weigh-ins a week, in the last week too.'],
  ['few_logged_days', 'It needs 4 logged days a week, in the last week too.'],
])('a TDEE not reliable yet for %s says what it needs', async (doubt, said) => {
  progressServer({
    'GET /api/v1/summary': () =>
      Response.json(
        summaryOf(none('not_trusted'), {
          ...trusted,
          trusted: false,
          kcal: 2350,
          half_width: 310,
          doubts: [doubt],
        }),
      ),
  })
  renderAt('/progress')

  const card = await calibration()
  expect(within(card).getByText('Cut · week 8 of 12')).toBeDefined()
  expect(within(card).getByText('TDEE ≈ 2,350 ± 310 · not reliable yet.')).toBeDefined()
  expect(within(card).getByText(said)).toBeDefined()
  expect(screen.queryByRole('region', { name: 'The phase so far' })).toBeNull()
})

test('a threshold of the user’s own is said as the summary gives it', async () => {
  progressServer({
    'GET /api/v1/summary': () =>
      Response.json(
        summaryOf(none('not_trusted'), {
          ...trusted,
          trusted: false,
          kcal: 2350,
          half_width: 160,
          doubts: ['few_weigh_ins', 'wide_interval'],
          trust: { half_width: 150, weigh_ins: 1, logged_days: 5 },
        }),
      ),
  })
  renderAt('/progress')

  const card = await calibration()
  expect(
    within(card).getByText(
      'It needs 1 weigh-in a week, in the last week too. It is once ± is 150 or less; weighing in 1× a week helps most.',
    ),
  ).toBeDefined()
})

// tdeeOf is a summary's TDEE of nothing measured yet, with what it has of
// what it needs.
function tdeeOf(has: { days: number; logged: number; weighIns: number; apart: number }) {
  return {
    before_phase: false,
    doubts: ['few_days'],
    from: '2026-10-14',
    to: '2026-10-22',
    logged_days: has.logged,
    weigh_ins: has.weighIns,
    measured: false,
    trusted: false,
    needs: {
      days: { has: has.days, need: 14 },
      logged_days: { has: has.logged, need: 7 },
      weigh_ins: { has: has.weighIns, need: 6 },
      weigh_in_days: { has: has.apart, need: 7 },
    },
    trust: { half_width: 200, weigh_ins: 3, logged_days: 4 },
  }
}

// rowsOf are the counts of calibration's card, each as it reads.
function rowsOf(card: HTMLElement): (string | null)[] {
  return within(within(card).getAllByRole('list')[0] ?? card)
    .getAllByRole('listitem')
    .map((li) => li.textContent)
}

test('without a phase or a TDEE, calibration counts toward one, says when, and leads to the plan', async () => {
  progressServer({
    'GET /api/v1/me': () => Response.json(me()),
    'GET /api/v1/summary': () =>
      Response.json(
        summaryOf(none('no_phase', 'no_target', 'no_estimate'), {
          ...tdeeOf({ days: 9, logged: 9, weighIns: 9, apart: 8 }),
          expected: '2026-11-02',
        }),
      ),
  })
  const router = renderAt('/progress?days=phase')

  const card = await calibration()
  expect(screen.queryByRole('radio', { name: 'The phase' })).toBeNull()
  expect(screen.getByRole<HTMLInputElement>('radio', { name: '90 days' }).checked).toBe(true)
  // Days too few; logged days and weigh-ins enough, ticked.
  expect(rowsOf(card)).toEqual(['Days9 of 14', 'Logged days9 of 7enough', 'Weigh-ins9 of 6enough'])
  expect(within(card).getByText('First TDEE around Nov 2 if you keep this up.')).toBeDefined()
  // No phase is a second row, beside the way to the plan.
  expect(within(card).getByText('No phase yet.')).toBeDefined()
  fireEvent.click(within(card).getByRole('link', { name: 'Set up a plan' }))
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/plan')
  })
})

test('weigh-ins enough but too close together are not yet, and say how far apart', async () => {
  progressServer({
    'GET /api/v1/summary': () =>
      Response.json(
        summaryOf(none('no_estimate'), tdeeOf({ days: 14, logged: 6, weighIns: 6, apart: 5 })),
      ),
  })
  renderAt('/progress')

  const card = await calibration()
  expect(rowsOf(card)).toEqual([
    'Days14 of 14enough',
    'Logged days6 of 7',
    'Weigh-ins6 of 65 of 7 days from the first to the last',
  ])
  // No day foreseen: what starts it.
  expect(
    within(card).getByText('Log your meals and weigh in, and the counts grow day by day.'),
  ).toBeDefined()
})

test('calibration speaks Croatian', async () => {
  fakeServer({
    ...signedIn('hr'),
    'GET /api/v1/me': () => Response.json({ ...me(), locale: 'hr' }),
    'GET /api/v1/summary': () =>
      Response.json(
        summaryOf(none('no_phase', 'no_estimate'), {
          ...tdeeOf({ days: 9, logged: 9, weighIns: 4, apart: 8 }),
          expected: '2026-11-02',
        }),
      ),
    'GET /api/v1/history': (r) => historyAnswer(r),
  })
  renderAt('/progress')

  const card = await screen.findByRole('region', { name: 'Kalibracija' }, { timeout: 10_000 })
  expect(rowsOf(card)).toEqual(['Dani9 od 14', 'Upisani dani9 od 7dovoljno', 'Vaganja4 od 6'])
  expect(within(card).getByText('Prvi TDEE oko 2. stu, ako nastaviš ovako.')).toBeDefined()
  expect(within(card).getByText('Još nema faze.')).toBeDefined()
})

test('a new phase says until when the TDEE is from the days before it', async () => {
  progressServer({
    'GET /api/v1/summary': () =>
      Response.json(
        summaryOf(none('before_phase'), {
          ...trusted,
          before_phase: true,
          before_phase_until: '2026-11-21',
        }),
      ),
  })
  renderAt('/progress')

  const verdict = await hero()
  expect(within(verdict).getByText('Not enough data yet.')).toBeDefined()
  expect(
    within(verdict).getByText('New phase: until Nov 21 the TDEE is from the days before it.'),
  ).toBeDefined()
  expect(within(verdict).getByText('From before the phase')).toBeDefined()
})

test('a TDEE without a phase leads, the way to the plan follows', async () => {
  progressServer({
    'GET /api/v1/me': () => Response.json(me()),
    'GET /api/v1/summary': () => Response.json(summaryOf(none('no_phase', 'no_target'))),
  })
  renderAt('/progress')

  const verdict = await hero()
  expect(within(verdict).getByText('TDEE 2,400 ± 60 kcal')).toBeDefined()
  expect(within(verdict).getByText('Reliable')).toBeDefined()
  expect(within(verdict).getByText('Trend weight 84.2 kg')).toBeDefined()
  expect(within(verdict).getByText('No phase yet.')).toBeDefined()
  expect(within(verdict).getByRole('link', { name: 'Set up a plan' })).toBeDefined()
})

test('a phase with no target says so, and opens the plan', async () => {
  progressServer({
    'GET /api/v1/summary': () => Response.json(summaryOf(none('no_target'))),
  })
  renderAt('/progress')

  const verdict = await hero()
  expect(within(verdict).getByText('No target yet.')).toBeDefined()
  expect(within(verdict).getByText('TDEE 2,400 ± 60 kcal')).toBeDefined()
  expect(within(verdict).getByRole('link', { name: 'Open the plan' }).getAttribute('href')).toBe(
    '/plan',
  )
})

test("what recovery flags is a sentence of the verdict's", async () => {
  progressServer({
    'GET /api/v1/summary': () =>
      Response.json({
        ...summaryOf(),
        recovery: { flags: ['short_sleep', 'low_hrv'], sleep_h: 6.2, hrv: 44, hrv_base: 52 },
      }),
  })
  renderAt('/progress')

  const verdict = await hero()
  expect(within(verdict).getByText('Sleep and HRV were worse than usual last week.')).toBeDefined()
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
  // 52 calendar weeks up to yesterday, and the one before.
  const weekly = histories.find((q) => q.get('granularity') === 'week')
  expect([weekly?.get('from'), weekly?.get('to')]).toEqual(['2025-10-20', '2026-10-22'])

  // A week of last year says its year.
  const weeks = await screen.findByRole('table', { name: 'By week' })
  fireEvent.click(screen.getByRole('button', { name: 'Show all weeks (52)' }))
  const rows = within(weeks).getAllByRole('row')
  expect(rows[1]?.querySelector('th')?.textContent).toBe('Oct 19 – 22')
  expect(rows.at(-1)?.querySelector('th')?.textContent).toBe('Oct 27 – Nov 2, 2025')
  // A year's nights in small are its weeks' 7 days' averages.
  const sleep = within(screen.getByRole('button', { name: /^Sleep/ })).getByTestId('sparkline')
  expect(sleep.querySelector('path')?.getAttribute('d')?.split('L')).toHaveLength(53)
})

test('the weeks are calendar weeks in a table, the latest first, the rest a tap away', async () => {
  progressServer()
  renderAt('/progress')

  const weeks = await screen.findByRole('table', { name: 'By week' }, { timeout: 10_000 })
  const rows = within(weeks).getAllByRole('row')
  expect(rows[0]?.textContent).toBe(
    'Weekkcalkcal a dayPProtein, g a dayΔ kgChange of the trend weight, kgDaysDays logged',
  )
  // 8 weeks, the latest first: Monday the 19th to yesterday, Thursday,
  // 4 days logged of the 4 over; the week before it, 6 of 7.
  expect(rows).toHaveLength(9)
  expect(rows[1]?.textContent).toBe('Oct 19 – 222,000150-0.54/4')
  expect(rows[2]?.textContent).toBe('Oct 12 – 182,000150-0.56/7')
  fireEvent.click(screen.getByRole('button', { name: 'Show all weeks (14)' }))
  expect(within(weeks).getAllByRole('row')).toHaveLength(15)
  expect(screen.getByRole('button', { name: 'Show fewer' }).getAttribute('aria-expanded')).toBe(
    'true',
  )
})

test('the measurements are small charts, each a whole one a tap away', async () => {
  progressServer()
  renderAt('/progress')

  const body = await screen.findByRole('region', { name: 'Body and recovery' }, { timeout: 10_000 })
  // Without recovery's last 7 days, the days' averages; each tile one
  // button, which its chart's summary describes.
  const tile = (name: string) => within(body).getByRole('button', { name: new RegExp(`^${name}`) })
  expect(tile('Sleep').textContent).toBe('Sleep7 h 30average')
  expect(tile('Sleep').getAttribute('aria-describedby')).toBeTruthy()
  expect(tile('Resting heart rate').textContent).toBe('Resting heart rate58 bpmaverage')
  expect(tile('HRV').textContent).toBe('HRV50 msaverage')
  expect(tile('Steps').textContent).toBe('Steps8,000average')
  expect(tile('Body fat').textContent).toBe('Body fat20%latest')
  // A line in the one neutral colour, no bars of the nights' colours, and
  // no usual range without a base.
  const lines = within(body).getAllByTestId('sparkline')
  expect(lines.map((l) => l.querySelector('path')?.getAttribute('class'))).toEqual(
    Array(5).fill('stroke-chart-5'),
  )
  expect(within(body).queryByTestId('usual')).toBeNull()
  expect(body.querySelector('rect.fill-chart-2, .stroke-chart-1')).toBeNull()

  fireEvent.click(tile('Sleep'))
  const nights = await screen.findByRole('dialog', { name: 'Sleep' })
  expect(within(nights).getByRole('img', { name: '7 h 30 a night on average.' })).toBeDefined()
  // What the colours of the nights mean.
  expect(within(nights).getByText('7 h or more')).toBeDefined()
  expect(within(nights).getByText('Under 6 h')).toBeDefined()
  fireEvent.keyDown(nights, { key: 'Escape' })
  await waitFor(() => {
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  fireEvent.click(tile('Resting heart rate'))
  const sheet = await screen.findByRole('dialog', { name: 'Resting heart rate' })
  expect(
    within(sheet).getByRole('img', { name: 'Resting heart rate 58 bpm on average.' }),
  ).toBeDefined()
})

test("sleep, the heart rate and the HRV are the last 7 days', against the usual", async () => {
  progressServer({
    'GET /api/v1/summary': () =>
      Response.json({
        ...summaryOf(),
        recovery: {
          flags: ['low_hrv'],
          sleep_h: 7.15,
          resting_hr: 59,
          resting_hr_base: 57.1,
          resting_hr_usual: { low: 54.2, high: 60 },
          hrv: 48.7,
          hrv_base: 52.2,
          hrv_usual: { low: 44.4, high: 60 },
        },
      }),
  })
  renderAt('/progress')

  const body = await screen.findByRole('region', { name: 'Body and recovery' }, { timeout: 10_000 })
  const tile = (name: string) => within(body).getByRole('button', { name: new RegExp(`^${name}`) })
  await waitFor(() => {
    expect(tile('Sleep').textContent).toBe('Sleep7 h 09last 7 days')
  })
  // 59 against 57 as shown; 48.7 against 52.2, 6.7% under.
  expect(tile('Resting heart rate').textContent).toBe(
    'Resting heart rate59 bpmlast 7 days+2 vs usual',
  )
  expect(tile('HRV').textContent).toBe('HRV49 mslast 7 days-7% vs usual')
  // Worse than usual, as recovery flags it, in the warning's colour.
  expect(within(tile('HRV')).getByText('-7% vs usual').className).toContain('text-warning')
  expect(within(tile('Resting heart rate')).getByText('+2 vs usual').className).not.toContain(
    'text-warning',
  )
  // The usual range shaded, of what has a base.
  expect(within(tile('HRV')).getByTestId('usual')).toBeDefined()
  expect(within(tile('Sleep')).queryByTestId('usual')).toBeNull()
  expect(within(await hero()).getByText('HRV was worse than usual last week.')).toBeDefined()
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
  expect(screen.getByRole('button', { name: /^Steps/ }).textContent).toBe('Steps8,000average')
})

// A first weigh-in, today's: the trend takes it once the day is over, and
// the chart says so, not that it has nothing.
test("today's weigh-in alone is said as one, before the trend has it", async () => {
  progressServer({
    'GET /api/v1/history': (r) =>
      historyAnswer(r, (from) =>
        historyOf(from).map((d, i, days) =>
          i === days.length - 1
            ? { date: d.date, status: 'open', type: 'rest', meals: 0, weight_kg: 80.5 }
            : { date: d.date, status: 'complete', type: 'rest', meals: 0 },
        ),
      ),
  })
  renderAt('/progress')

  expect(
    await screen.findByRole(
      'img',
      { name: '1 weigh-in today; the trend weight takes it once the day is over.' },
      { timeout: 10_000 },
    ),
  ).toBeDefined()
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
  expect(within(body).getByRole('button', { name: /^HRV/ })).toBeDefined()
  expect(within(body).queryByRole('button', { name: /^Steps/ })).toBeNull()
  expect(within(body).queryByRole('button', { name: /^Sleep/ })).toBeNull()
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

// The verdict, the TDEE and its ±, its trust and the trend weight: each an
// ⓘ away from what it is, in words.
test.each([
  ['How the phase is judged', 'The verdict', 'measured from the phase'],
  ['What TDEE is', 'TDEE', 'with 80% probability'],
  ['What reliable means', 'Reliable', '200 kcal or less'],
  ['What trend weight is', 'Trend weight', 'swings of water and salt'],
])('the hero\'s "%s" ⓘ says what it is', async (tip, name, said) => {
  progressServer()
  renderAt('/progress')

  const verdict = await hero()
  fireEvent.click(within(verdict).getByRole('button', { name: tip }))
  expect((await screen.findByRole('dialog', { name })).textContent).toContain(said)
})
