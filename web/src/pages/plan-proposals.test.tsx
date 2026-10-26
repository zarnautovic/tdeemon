import { focusManager } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { fakeServer, renderAt, signedIn } from '@/test/app'
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
  renderAt('/settings/plan')
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
  renderAt('/settings/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Reject' }))
  await waitFor(() => {
    expect(screen.queryByRole('region', { name: 'A proposed change' })).toBeNull()
  })
  expect(sent).toEqual([`POST /api/v1/proposals/${step.id}/reject`])
  screen.getByText('No change of the plan yet.')
})

test('a proposal the limits refuse now says why, in words', async () => {
  coachServer({ pending: step }, { apply: refusal })
  renderAt('/settings/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  const alert = await card.findByRole('alert')
  expect(alert.textContent).toBe(
    'The limits refuse it now:the kcal of a day: 1,800 proposed, the limit 1,850: your limit, which you may loosen below',
  )
})

test('a proposal that waits no more says so', async () => {
  coachServer({ pending: step }, { apply: () => closed(409) })
  renderAt('/settings/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  fireEvent.click(card.getByRole('button', { name: 'Accept' }))
  await card.findByText('The proposal waits no more: the plan changed, or it expired.')
  expect(card.getByRole('button', { name: 'Accept' })).toHaveProperty('disabled', true)
})

test('the history shows a revision, which is taken back in a step', async () => {
  const { sent } = coachServer({ revisions: [sundays] })
  renderAt('/settings/plan')
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
  renderAt('/settings/plan')
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
  renderAt('/settings/plan')
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

// A saved profile or limits change what the limits hold the proposal to:
// the plan, and its preview, are read again.
test('a saved profile reads the plan again, for the preview', async () => {
  let reads = 0
  fakeServer({
    ...signedIn(),
    'GET /api/v1/profile': () => Response.json({ missing: ['sex', 'birth_date', 'height_cm'] }),
    'PUT /api/v1/profile': () =>
      Response.json({ sex: 'male', missing: ['birth_date', 'height_cm'] }),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => {
      reads++
      return Response.json(planBody({ pending: step, pending_preview: preview }))
    },
  })
  renderAt('/settings/plan')
  const form = await screen.findByRole('form', { name: 'Profile' })
  await waitFor(() => {
    expect(reads).toBe(1)
  })
  fireEvent.click(within(form).getByRole('radio', { name: 'Male' }))
  fireEvent.click(within(form).getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(reads).toBe(2)
  })
})

// A proposal a day of which went by is refused for good: it says so, and
// may still be rejected.
test('a proposal that cannot be applied any more says so, and may be rejected', async () => {
  coachServer(
    { pending: step },
    { apply: () => Response.json({ status: 422, detail: 'invalid proposal' }, { status: 422 }) },
  )
  renderAt('/settings/plan')
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
  renderAt('/settings/plan')
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
    },
  }
  coachServer({ pending: longer })
  renderAt('/settings/plan')
  const card = within(await screen.findByRole('region', { name: 'A proposed change' }))
  card.getByText('Cut, -0.5% a week, until Dec 31, 2027')
  expect(card.getByText('was Cut, -0.5% a week, until Dec 31')).toBeTruthy()
})
