import { focusManager } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { fakeServer, me, renderAt, signedIn } from '@/test/app'
import { limitsBody, planBody } from '@/test/plan'

// The clock is the tests' day, so a date of another year says its year.
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-10-23T16:00:00Z'))
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  setLocale('en')
})

type Plan = components['schemas']['PlanBody']
type Proposal = components['schemas']['ProposalBody']

const step: Proposal = {
  id: '0193c3a5-0000-7000-8000-0000000000a1',
  status: 'pending',
  rationale: "The loss is under the cut's rate.",
  by: { client: 'mcp', model: 'claude-opus-5-5' },
  window: { from: '2026-09-25', to: '2026-10-22' },
  changes: {
    phases: [],
    targets: [
      {
        from: '2026-10-24',
        after: {
          training: { kcal: 1900, protein: 160, carbs: 220, fat: 60 },
          rest: { kcal: 1900, protein: 160, carbs: 220, fat: 60 },
        },
      },
    ],
    limits: [],
  },
  created_at: '2026-10-23T08:00:00Z',
  expires_at: '2026-10-30T08:00:00Z',
}

const preview: components['schemas']['PreviewBody'] = {
  trend_kg: 82.5,
  tdee: 2385,
  checks: [
    {
      of: 'target',
      day: '2026-10-24',
      weekly_kcal: 1900,
      expected_pct_week: -0.53,
      expected_kg_week: -0.44,
      warnings: [],
      violations: [],
    },
  ],
}

// sundays is a revision applied, of the training weekdays.
const sundays: Proposal = {
  id: '0193c3a5-0000-7000-8000-0000000000b1',
  status: 'applied',
  rationale: 'Train on Sundays too.',
  by: { client: 'mcp', model: 'claude-opus-5-5' },
  changes: {
    phases: [],
    targets: [],
    training_weekdays: { before: ['monday'], after: ['sunday', 'monday'] },
    limits: [],
  },
  created_at: '2026-10-20T08:00:00Z',
  expires_at: '2026-10-27T08:00:00Z',
  decided_at: '2026-10-20T09:00:00Z',
  decided_by: { client: 'mcp', model: 'claude-opus-5-5' },
}

// refusal is the API's refusal of a proposal past the limits, with its
// violation as an error detail's value.
function refusal(): Response {
  return Response.json(
    {
      status: 422,
      title: 'Unprocessable Entity',
      detail: 'the proposal breaks the limits: kcal_floor (target 2026-10-24)',
      errors: [
        {
          location: 'target 2026-10-24',
          message: 'kcal_floor: 1800 proposed, the limit 1850 (user)',
          value: { rule: 'kcal_floor', limit: 1850, proposed: 1800, level: 'user' },
        },
      ],
    },
    { status: 422, headers: { 'Content-Type': 'application/problem+json' } },
  )
}

function closed(status: number): Response {
  return Response.json(
    { status, title: 'An error', detail: 'the proposal is superseded: it waits no more' },
    { status, headers: { 'Content-Type': 'application/problem+json' } },
  )
}

// coachServer keeps a plan as the server does, and what it was sent, by
// "METHOD /path"; over answers a route its own way.
function coachServer(
  start: Partial<Plan>,
  over: { apply?: () => Response; revert?: () => Response } = {},
) {
  let plan = planBody(start)
  const sent: string[] = []
  const done = (r: Request, status: Proposal['status']) => {
    sent.push(`${r.method} ${new URL(r.url).pathname}`)
    const pending = plan.pending
    if (pending === undefined) {
      return closed(409)
    }
    const decided = {
      ...pending,
      status,
      decided_at: '2026-10-23T09:00:00Z',
      decided_by: { client: 'web' },
    }
    plan = {
      ...planBody({
        revisions: status === 'applied' ? [decided, ...plan.revisions] : plan.revisions,
      }),
    }
    return Response.json({ proposal: decided })
  }
  fakeServer({
    ...signedIn(),
    'GET /api/v1/profile': () => Response.json({ missing: [] }),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => Response.json(plan),
    [`POST /api/v1/proposals/${step.id}/apply`]: (r) =>
      over.apply ? over.apply() : done(r, 'applied'),
    [`POST /api/v1/proposals/${step.id}/reject`]: (r) => done(r, 'rejected'),
    [`POST /api/v1/proposals/${sundays.id}/revert`]: (r) => {
      sent.push(`${r.method} ${new URL(r.url).pathname}`)
      if (over.revert) {
        return over.revert()
      }
      plan = { ...plan, pending: { ...step, reverts: sundays.id } }
      return Response.json({ proposal: plan.pending, preview }, { status: 201 })
    },
  })
  return { sent }
}

test('a proposal that waits says why, what it changes and what it comes to, and is accepted', async () => {
  const { sent } = coachServer({ pending: step, pending_preview: preview })
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  card.getByText('Proposed by claude-opus-5-5')
  card.getByText("The loss is under the cut's rate.")
  card.getByText('Drawn from the days Sep 25 to Oct 22.')
  card.getByText('New target from Oct 24')
  card.getByText('1,900 kcal · P 160 g · C 220 g · F 60 g')
  card.getByText('From Oct 24: 1,900 kcal a day on average, -0.53% a week (-0.44 kg)')
  card.getByText('It waits for your answer until Oct 30.')

  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  await waitFor(() => {
    expect(screen.queryByRole('region', { name: 'A proposed change' })).toBeNull()
  })
  expect(sent).toEqual([`POST /api/v1/proposals/${step.id}/apply`])
  // Now a revision of the plan.
  const history = within(screen.getByRole('region', { name: 'History of the plan' }))
  history.getByText("The loss is under the cut's rate.")
})

test('a proposal that waits is rejected', async () => {
  const { sent } = coachServer({ pending: step })
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Reject' }))
  await waitFor(() => {
    expect(screen.queryByRole('region', { name: 'A proposed change' })).toBeNull()
  })
  expect(sent).toEqual([`POST /api/v1/proposals/${step.id}/reject`])
  // No plan yet, and no history of one.
  screen.getByRole('region', { name: 'No plan yet' })
  expect(screen.queryByRole('region', { name: 'History of the plan' })).toBeNull()
})

test('a proposal the limits refuse now says why, in words', async () => {
  coachServer({ pending: step }, { apply: refusal })
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  const alert = await card.findByRole('alert')
  expect(alert.textContent).toBe(
    'The limits refuse it now:the kcal of a day: 1,800 proposed, the limit 1,850: your limit, which you may loosen below',
  )
})

test('a proposal that waits no more says so', async () => {
  coachServer({ pending: step }, { apply: () => closed(409) })
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  await card.findByText('The proposal waits no more: the plan changed, or it expired.')
  expect(card.getByRole('button', { name: 'Accept' })).toHaveProperty('disabled', true)
})

test('the history shows a revision, which is taken back in a step', async () => {
  const { sent } = coachServer({ revisions: [sundays] })
  renderAt('/plan')
  const history = within(await screen.findByRole('region', { name: 'History of the plan' }))
  history.getByText('Train on Sundays too.')
  history.getByText('Training days')
  history.getByText('Sun, Mon')
  history.getByText('was Mon')

  fireEvent.click(history.getByRole('button', { name: 'Take back' }))
  history.getByText('Take this change back, from now on?')
  fireEvent.click(history.getByRole('button', { name: 'Take back' }))
  await waitFor(() => {
    expect(sent).toEqual([
      `POST /api/v1/proposals/${sundays.id}/revert`,
      `POST /api/v1/proposals/${step.id}/apply`,
    ])
  })
})

test('a change that took effect cannot be taken back, and says so', async () => {
  coachServer(
    { revisions: [sundays] },
    { revert: () => Response.json({ status: 422, detail: 'invalid proposal' }, { status: 422 }) },
  )
  renderAt('/plan')
  const history = within(await screen.findByRole('region', { name: 'History of the plan' }))
  fireEvent.click(history.getByRole('button', { name: 'Take back' }))
  fireEvent.click(history.getByRole('button', { name: 'Take back' }))
  expect((await history.findByRole('alert')).textContent).toBe(
    'This change cannot be taken back any more: it took effect, or the plan changed since. Ask for a new change instead.',
  )
})

// A proposal that came in place of one that waits no more starts afresh:
// its buttons work.
test('a new proposal, in place of one that waits no more, can be answered', async () => {
  let plan = planBody({ pending: step })
  const smaller: Proposal = {
    ...step,
    id: '0193c3a5-0000-7000-8000-0000000000a2',
    rationale: 'A smaller step.',
  }
  fakeServer({
    ...signedIn(),
    'GET /api/v1/profile': () => Response.json({ missing: [] }),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => Response.json(plan),
    [`POST /api/v1/proposals/${step.id}/apply`]: () => {
      plan = planBody({ pending: smaller })
      return closed(409)
    },
  })
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  await card.findByText('The proposal waits no more: the plan changed, or it expired.')
  // The plan is read again, as when the app comes back to the front.
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
  await screen.findByText('A smaller step.')
  const fresh = within(screen.getByRole('region', { name: 'A proposed change' }))
  expect(fresh.queryByRole('alert')).toBeNull()
  expect(fresh.getByRole('button', { name: 'Accept' })).toHaveProperty('disabled', false)
  focusManager.setFocused(undefined)
})

// A proposal a day of which went by is refused for good: it says so, and
// may still be rejected.
test('a proposal that cannot be applied any more says so, and may be rejected', async () => {
  coachServer(
    { pending: step },
    { apply: () => Response.json({ status: 422, detail: 'invalid proposal' }, { status: 422 }) },
  )
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  expect((await card.findByRole('alert')).textContent).toBe(
    'It cannot be applied any more: a change of it would now start before today. Ask for a new one.',
  )
  expect(card.getByRole('button', { name: 'Accept' })).toHaveProperty('disabled', true)
  expect(card.getByRole('button', { name: 'Reject' })).toHaveProperty('disabled', false)
})

// The latest answer is the one shown: a reject after a refused accept
// says what became of the reject.
test("an action's answer is its own, not the one before's", async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/profile': () => Response.json({ missing: [] }),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => Response.json(planBody({ pending: step })),
    [`POST /api/v1/proposals/${step.id}/apply`]: refusal,
    [`POST /api/v1/proposals/${step.id}/reject`]: () => closed(409),
  })
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  await card.findByText('The limits refuse it now:')
  fireEvent.click(card.getByRole('button', { name: 'Reject' }))
  await card.findByText('The proposal waits no more: the plan changed, or it expired.')
  expect(card.queryByText('The limits refuse it now:')).toBeNull()
  expect(card.getByRole('button', { name: 'Reject' })).toHaveProperty('disabled', true)
})

// A date of another year says its year: a phase made a year longer does
// not read as it was.
test("a change's date of another year says its year", async () => {
  const longer: Proposal = {
    ...step,
    changes: {
      phases: [
        {
          start: '2026-09-07',
          before: { type: 'cut', start: '2026-09-07', end: '2026-12-31', rate_pct_week: -0.5 },
          after: { type: 'cut', start: '2026-09-07', end: '2027-12-31', rate_pct_week: -0.5 },
        },
      ],
      targets: [],
      limits: [],
    },
  }
  coachServer({ pending: longer })
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  card.getByText('Cut, -0.5% a week, until Dec 31, 2027')
  expect(card.getByText('was Cut, -0.5% a week, until Dec 31')).toBeTruthy()
})

type Summary = components['schemas']['SummaryBody']

// summaryOf is calibration's summary of nothing measured yet; over is
// laid over it.
function summaryOf(over: Partial<Summary> = {}): Summary {
  return {
    date: '2026-10-23',
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
    ...over,
  }
}

// planPage serves a plan, calibration's summary, and the user's profile.
function planPage(plan: Partial<Plan>, summary: Summary = summaryOf(), profile = me().profile) {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/me': () => Response.json({ ...me(), profile }),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => Response.json(planBody(plan)),
    'GET /api/v1/summary': () => Response.json(summary),
  })
}

// cut is a cut of 14 weeks, a phase of maintenance before it, and one
// after; today, October 23, is in its seventh week.
const cut: Partial<Plan> = {
  phases: [
    { type: 'maintain', start: '2026-06-01', end: '2026-09-06', rate_pct_week: 0 },
    { type: 'cut', start: '2026-09-07', end: '2026-12-13', rate_pct_week: -0.5, goal_weight: 80 },
    { type: 'maintain', start: '2026-12-14', rate_pct_week: 0 },
  ],
  targets: [
    {
      effective_from: '2026-09-07',
      to: '2026-10-25',
      training: { kcal: 2300, protein: 160, carbs: 230, fat: 70 },
      rest: { kcal: 2000, protein: 160, carbs: 170, fat: 60 },
    },
    {
      effective_from: '2026-10-26',
      training: { kcal: 2200, protein: 160, carbs: 205, fat: 70 },
      rest: { kcal: 2200, protein: 160, carbs: 205, fat: 70 },
    },
  ],
  training_weekdays: ['monday', 'wednesday'],
}

test('a phase with a milestone and no goal shows the milestone', async () => {
  planPage(
    {
      ...cut,
      phases: [{ type: 'cut', start: '2026-09-07', rate_pct_week: -0.5, milestone_weight: 82 }],
    },
    summaryOf({ trend: { date: '2026-10-22', kg: 82.5 } }),
  )
  renderAt('/plan')
  const hero = within(await screen.findByRole('region', { name: 'Cut' }))
  expect(hero.getByText(/^Milestone/).textContent).toBe('Milestone 82 kg')
  expect(hero.queryByText(/^Goal/)).toBeNull()
})

// The phase of today is the plan's hero: its type, its pace and range,
// the rate measured, its week, how far it has come, and its goal; and
// the phases on a timeline.
test('the phase of today shows its pace, its week and how far it came, among the phases', async () => {
  planPage(
    cut,
    summaryOf({
      trend: { date: '2026-10-22', kg: 82.5 },
      verdict: {
        ...summaryOf().verdict,
        rate: 'on_target',
        measured_pct_week: -0.48,
        target_pct_week: -0.5,
        tolerance_pct_week: 0.1,
      },
    }),
  )
  renderAt('/plan')
  const hero = within(await screen.findByRole('region', { name: 'Cut' }))
  hero.getByText('Current phase · since Sep 7')
  hero.getByText('Week 7 of 14')
  hero.getByText('-0.5% a week')
  expect(await hero.findByText('≈ -0.41 kg a week')).toBeTruthy()
  hero.getByText('in range from -0.6% to -0.4%')
  hero.getByText('-0.48% a week')
  hero.getByText('in range')
  // 47 days gone of 98, today's included.
  const days = hero.getByRole('progressbar', { name: 'Days of the phase' })
  expect([days.getAttribute('value'), days.getAttribute('max')]).toEqual(['47', '98'])
  expect(days.getAttribute('aria-valuetext')).toBe('51 days left')
  // Under the bar, and on the timeline.
  expect(hero.getAllByText('Sep 7 – Dec 13')).toHaveLength(2)
  expect(hero.getByText(/^Goal 80 kg/).textContent).toBe('Goal 80 kg · trend 82.5 kg, 2.5 kg to go')

  const phases = within(hero.getByRole('list', { name: 'Phases' }))
    .getAllByRole('listitem')
    .map((li) => li.textContent)
  expect(phases).toEqual([
    'Maintenance · 0% a weekJun 1 – Sep 6Past',
    'Cut · -0.5% a weekSep 7 – Dec 13Now',
    'Maintenance · 0% a weekfrom Dec 14Planned',
  ])
})

test('a phase not measured yet says so, and one with no end its week alone', async () => {
  planPage({ phases: [{ type: 'bulk', start: '2026-10-12', rate_pct_week: 0.25 }] })
  renderAt('/plan')
  const hero = within(await screen.findByRole('region', { name: 'Bulk' }))
  hero.getByText('Week 2')
  hero.getByText('0.25% a week')
  expect(await hero.findByText('Not measured yet')).toBeTruthy()
  expect(hero.queryByRole('progressbar')).toBeNull()
  // A phase alone needs no timeline.
  expect(hero.queryByRole('list', { name: 'Phases' })).toBeNull()
})

// The targets: each type of day's, its days of the week, and the weekly
// average they come to against the TDEE; and what is to come.
test('the targets by type of day come to a weekly average, against the TDEE', async () => {
  planPage(
    cut,
    summaryOf({
      tdee: { ...summaryOf().tdee, measured: true, trusted: true, kcal: 2400.4 },
    }),
  )
  renderAt('/plan')
  const targets = within(await screen.findByRole('region', { name: 'Targets by type of day' }))
  targets.getByText('since Sep 7')
  const tile = (name: string) => targets.getByRole('heading', { name }).parentElement?.textContent
  expect(tile('Training')).toBe('Training2,300 kcalP 160 · C 230 · F 70 g2 days a week · Mon, Wed')
  expect(tile('Rest')).toBe(
    'Rest2,000 kcalP 160 · C 170 · F 60 g5 days a week · Tue, Thu, Fri, Sat, Sun',
  )
  // (2 × 2,300 + 5 × 2,000) / 7.
  const term = (name: string) => targets.getByText(name).nextElementSibling?.textContent
  expect(term('Weekly average')).toBe('2,086 kcal a day')
  await waitFor(() => {
    expect(term('TDEE')).toBe('2,400 kcal a day')
  })
  expect(term('Deficit')).toBe('-315 kcal a day')
  expect(targets.getByText('From Oct 26:').parentElement?.textContent).toBe(
    'From Oct 26: 2,200 kcal · P 160 g · C 205 g · F 70 g',
  )
})

test('a TDEE not trusted yet is about so much, and one not measured says so', async () => {
  planPage(
    cut,
    summaryOf({ tdee: { ...summaryOf().tdee, measured: true, trusted: false, kcal: 1900 } }),
  )
  renderAt('/plan')
  const targets = within(await screen.findByRole('region', { name: 'Targets by type of day' }))
  const term = (name: string) => targets.getByText(name).nextElementSibling?.textContent
  await waitFor(() => {
    expect(term('TDEE')).toBe('≈1,900 kcal a day')
  })
  expect(term('Surplus')).toBe('+186 kcal a day')
  cleanup()

  planPage(cut)
  renderAt('/plan')
  const without = within(await screen.findByRole('region', { name: 'Targets by type of day' }))
  await waitFor(() => {
    expect(without.getByText('TDEE').nextElementSibling?.textContent).toBe('not measured yet')
  })
  expect(without.queryByText('Deficit')).toBeNull()
})

// A plan that starts tomorrow is all to come.
test('a plan that starts tomorrow is one to come', async () => {
  planPage({
    phases: [{ type: 'cut', start: '2026-10-24', rate_pct_week: -0.4 }],
    targets: [
      {
        effective_from: '2026-10-24',
        training: { kcal: 2400, protein: 200, carbs: 231, fat: 75 },
        rest: { kcal: 2400, protein: 200, carbs: 231, fat: 75 },
      },
    ],
  })
  renderAt('/plan')
  const hero = within(await screen.findByRole('region', { name: 'No phase now' }))
  hero.getByText('Next: Cut, from Oct 24')
  expect(within(hero.getByRole('list', { name: 'Phases' })).getByRole('listitem').textContent).toBe(
    'Cut · -0.4% a weekfrom Oct 24Planned',
  )
  const targets = within(screen.getByRole('region', { name: 'Targets by type of day' }))
  targets.getByText('No target yet.')
  expect(targets.getByText('From Oct 24:').parentElement?.textContent).toBe(
    'From Oct 24: 2,400 kcal · P 200 g · C 231 g · F 75 g',
  )
})

test('no plan yet says how one comes, and the first step to it', async () => {
  planPage({}, summaryOf(), { missing: ['height_cm'] })
  const router = renderAt('/plan')
  const empty = within(await screen.findByRole('region', { name: 'No plan yet' }))
  empty.getByText(
    'Your AI coach proposes one: ask it to "look at the last month and propose a plan". Its proposal waits here for your answer.',
  )
  // A plan needs the profile first.
  const first = await empty.findByRole('link', { name: 'Complete your profile' })
  // The coach's link asks for a token it can use: MCP takes no other.
  expect(empty.getAllByRole('link').map((a) => a.getAttribute('href'))).toEqual([
    '/plan/profile',
    '/settings/devices?connect=coach',
  ])
  expect(screen.queryByRole('region', { name: 'Targets by type of day' })).toBeNull()
  fireEvent.click(first)
  await screen.findByRole('heading', { name: 'Profile', level: 1 })
  expect(router.state.location.pathname).toBe('/plan/profile')
  cleanup()

  // With the profile whole, the coach is the first step.
  planPage({})
  renderAt('/plan')
  const whole = within(await screen.findByRole('region', { name: 'No plan yet' }))
  expect(whole.getAllByRole('link').map((a) => a.textContent)).toEqual(['Connect an AI coach'])
})

// Training days alone are a plan's part too: they give each day its type.
test('a plan of training days alone shows them', async () => {
  planPage({ training_weekdays: ['monday', 'wednesday'] })
  renderAt('/plan')
  const targets = within(await screen.findByRole('region', { name: 'Targets by type of day' }))
  expect(targets.getByText('Training days:').parentElement?.textContent).toBe(
    'Training days: Mon, Wed',
  )
  targets.getByText('No target yet.')
})

test('a plan loading is a skeleton of its shape, and one that did not come says why', async () => {
  let answer: ((r: Response) => void) | undefined
  let fails = true
  fakeServer({
    ...signedIn(),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () =>
      fails
        ? new Promise<Response>((resolve) => {
            answer = resolve
          })
        : Response.json(planBody(cut)),
  })
  renderAt('/plan')
  await screen.findByRole('heading', { name: 'Plan', level: 1 })
  expect(screen.getByText('Loading…')).toBeTruthy()
  await waitFor(() => {
    expect(answer).toBeDefined()
  })
  answer?.(
    Response.json(
      { status: 500, title: 'An error', detail: 'what went wrong' },
      { status: 500, headers: { 'Content-Type': 'application/problem+json' } },
    ),
  )
  expect((await screen.findByRole('alert')).textContent).toBe('The server failed to answer.')
  fails = false
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
  expect(await screen.findByRole('region', { name: 'Cut' })).toBeTruthy()
})

// faster is a proposal of a faster cut's limit, loosened, and of more
// protein, tightened.
const faster: Proposal = {
  ...step,
  rationale: 'A faster cut, with more protein.',
  changes: {
    phases: [],
    targets: [],
    limits: [
      { name: 'cut_rate', before: -0.7, after: -1, loosens: true },
      { name: 'protein_floor', before: 1.6, after: 1.8, loosens: false },
    ],
  },
}

// limitsServer serves a plan with pending, and keeps the body of each
// apply sent.
function limitsServer(pending: Proposal) {
  const applied: string[] = []
  fakeServer({
    ...signedIn(),
    'GET /api/v1/profile': () => Response.json({ missing: [] }),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => Response.json(planBody({ pending })),
    [`POST /api/v1/proposals/${pending.id}/apply`]: async (r) => {
      applied.push(await r.text())
      return Response.json({ proposal: { ...pending, status: 'applied' } })
    },
  })
  return { applied }
}

test('a proposal of the limits shows each, loosened or tightened, and asks before it loosens one', async () => {
  const { applied } = limitsServer(faster)
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  card.getByText('Limit: Fastest loss in a cut, BMI under 30')
  card.getByText('1% a week, loosened')
  card.getByText('was 0.7% a week')
  card.getByText('Limit: Least protein')
  card.getByText('1.8 g/kg, tightened')
  card.getByText('was 1.6 g/kg')
  card.getByText(
    'A change of your limits is accepted only here, in the app, never by your AI coach.',
  )

  // Asked first, the user may go back: nothing is sent.
  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  card.getByText(
    'Loosening Fastest loss in a cut, BMI under 30 allows a plan the limit refused. Loosen it?',
  )
  fireEvent.click(card.getByRole('button', { name: 'Back' }))
  expect(card.queryByText(/^Loosening/)).toBeNull()
  expect(applied).toEqual([])

  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  fireEvent.click(card.getByRole('button', { name: 'Loosen' }))
  await waitFor(() => {
    expect(applied).toEqual(['{"confirm":true}'])
  })
})

// Rejected while the question is open, it is rejected alone: the
// question goes, so no apply follows the reject.
test('a proposal rejected while the loosening is asked closes the question', async () => {
  const { applied } = limitsServer(faster)
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  card.getByRole('button', { name: 'Loosen' })
  fireEvent.click(card.getByRole('button', { name: 'Reject' }))
  expect(card.queryByRole('button', { name: 'Loosen' })).toBeNull()
  expect(applied).toEqual([])
})

test('a proposal that only tightens the limits is accepted without a question', async () => {
  const tighter: Proposal = {
    ...faster,
    changes: { ...faster.changes, limits: faster.changes.limits.slice(1) },
  }
  const { applied } = limitsServer(tighter)
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  await waitFor(() => {
    expect(applied).toEqual([''])
  })
  expect(card.queryByText(/^Loosening/)).toBeNull()
})

// A limit the limits check refuses is named as the page names it.
test("a limit past the instance's is said in words", async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/profile': () => Response.json({ missing: [] }),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () =>
      Response.json(
        planBody({
          pending: faster,
          pending_preview: {
            checks: [
              {
                of: 'limits',
                day: '2026-10-23',
                warnings: [],
                violations: [
                  { rule: 'kcal_floor_male', limit: 1200, proposed: 1100, level: 'instance' },
                ],
              },
            ],
          },
        }),
      ),
  })
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  expect(
    card.getByText(
      "Fewest kcal of a day, for a man: 1,100 proposed, the limit 1,200: the instance's limit",
    ),
  ).toBeTruthy()
})

// takeBackServer serves the revision tightened, whose take-back is back,
// and keeps the body of each apply sent.
function takeBackServer(tightened: Proposal, back: Proposal) {
  const applied: string[] = []
  fakeServer({
    ...signedIn(),
    'GET /api/v1/profile': () => Response.json({ missing: [] }),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => Response.json(planBody({ revisions: [tightened] })),
    [`POST /api/v1/proposals/${tightened.id}/revert`]: () =>
      Response.json({ proposal: back, preview: { checks: [] } }, { status: 201 }),
    [`POST /api/v1/proposals/${back.id}/apply`]: async (r) => {
      applied.push(await r.text())
      return Response.json({ proposal: { ...back, status: 'applied' } })
    },
  })
  return { applied }
}

const tightened: Proposal = {
  ...sundays,
  rationale: 'More protein.',
  changes: {
    phases: [],
    targets: [],
    limits: [{ name: 'protein_floor', before: 1.6, after: 1.8, loosens: false }],
  },
}

// Taking back a revision that tightened a limit loosens it: the question
// says so, and the confirmation goes with it.
test('taking back a tightened limit says it loosens it, and confirms it', async () => {
  const back: Proposal = {
    ...step,
    reverts: tightened.id,
    changes: {
      phases: [],
      targets: [],
      limits: [{ name: 'protein_floor', before: 1.8, after: 1.6, loosens: true }],
    },
  }
  const { applied } = takeBackServer(tightened, back)
  renderAt('/plan')
  const history = within(await screen.findByRole('region', { name: 'History of the plan' }))
  history.getByText('1.8 g/kg, tightened')
  fireEvent.click(history.getByRole('button', { name: 'Take back' }))
  history.getByText('Taking it back loosens Least protein, which allows a plan the limit refused.')
  fireEvent.click(history.getByRole('button', { name: 'Take back' }))
  await waitFor(() => {
    expect(applied).toEqual(['{"confirm":true}'])
  })
})

// A take-back that loosens more than the question said is not confirmed:
// it waits, to be asked about as any proposal.
test('a take-back that loosens what the question did not say is not confirmed', async () => {
  const back: Proposal = {
    ...step,
    reverts: tightened.id,
    changes: {
      phases: [],
      targets: [],
      limits: [
        { name: 'cut_rate', before: -0.7, after: -1, loosens: true },
        { name: 'protein_floor', before: 1.8, after: 1.6, loosens: true },
      ],
    },
  }
  const { applied } = takeBackServer(tightened, back)
  renderAt('/plan')
  const history = within(await screen.findByRole('region', { name: 'History of the plan' }))
  fireEvent.click(history.getByRole('button', { name: 'Take back' }))
  fireEvent.click(history.getByRole('button', { name: 'Take back' }))
  await waitFor(() => {
    expect(applied).toEqual([''])
  })
})
