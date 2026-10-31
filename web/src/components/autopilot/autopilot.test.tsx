import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { fakeServer, problem, renderAt, signedIn, today } from '@/test/app'
import { autopilotBody, limitsBody, planBody, review } from '@/test/plan'

type Plan = components['schemas']['PlanBody']
type Proposal = components['schemas']['ProposalBody']
type Autopilot = components['schemas']['AutopilotBody']
type Review = components['schemas']['ReviewBody']

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

const before = {
  training: { kcal: 2350, protein: 170, carbs: 260, fat: 70 },
  rest: { kcal: 2050, protein: 170, carbs: 196, fat: 65 },
}
const after = {
  training: { kcal: 2250, protein: 170, carbs: 235, fat: 70 },
  rest: { kcal: 1950, protein: 170, carbs: 171, fat: 65 },
}

// cut is a plan of a cut since September, its target in force the one
// the autopilot stepped to today.
const cut: Partial<Plan> = {
  phases: [{ type: 'cut', start: '2026-09-28', rate_pct_week: -0.5 }],
  training_weekdays: ['monday', 'wednesday', 'friday'],
  targets: [{ effective_from: today, ...after }],
  autopilot: { mode: 'autopilot', next: '2026-10-30', drift_kcal: -200, drift_limit_kcal: 390 },
}

// step is the autopilot's step of today, applied, with its review.
const step: Proposal = {
  id: '0193c3a5-0000-7000-8000-0000000000c1',
  status: 'applied',
  rationale: 'autopilot',
  by: { client: 'autopilot' },
  changes: { phases: [], targets: [{ from: today, before, after }], limits: [] },
  created_at: '2026-10-23T16:00:00Z',
  expires_at: '2026-10-30T16:00:00Z',
  decided_at: '2026-10-23T16:00:00Z',
  decided_by: { client: 'autopilot' },
  check_in: review(),
}

const onAutopilot: Autopilot = autopilotBody({
  mode: 'autopilot',
  weekday: 'friday',
  next: '2026-10-30',
  drift: {
    used_kcal: -200,
    limit_pct: 15,
    limit_kcal: 390,
    base_kcal: 2600,
    base_from: '2026-09-28',
    agreed_on: '2026-09-28',
    agreed_kcal: 2279,
  },
  last: review(),
})

// serve answers the plan and the autopilot as given.
function serve(plan: Partial<Plan>, autopilot: Autopilot | undefined, locale: 'en' | 'hr' = 'en') {
  fakeServer({
    ...signedIn(locale),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => Response.json(planBody(plan)),
    'GET /api/v1/autopilot': () =>
      autopilot === undefined ? problem(503) : Response.json(autopilot),
  })
}

test("the plan's card says the last review, why, the next and the drift, and leads to its settings", async () => {
  serve({ ...cut, revisions: [step] }, onAutopilot)
  const router = renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'Autopilot' }))
  card.getByText('On')
  card.getByText('Last review · Oct 23')
  card.getByText('Lowered your target by 100 kcal a day')
  card.getByText('You lost 0.31% a week; the plan is 0.5% (0.4% to 0.6%).')
  // The rest of the numbers, a tap away.
  const numbers = card.getByText('The numbers').closest('details')
  expect(numbers?.open).toBe(false)
  within(numbers ?? document.body).getByText(
    'Your TDEE is 2,600 ± 120 kcal; you ate 2,140 kcal a day against a target of 2,179.',
  )
  within(numbers ?? document.body).getByText(
    'Each type of day moves -100 kcal, all carbs (-25 g): 2,179 → 2,079 kcal a day on average.',
  )
  expect(card.getByRole('link', { name: "Take it back in the plan's history." })).toBeTruthy()
  card.getByText('Fri, Oct 30')
  card.getByText('Fridays')
  card.getByText('-200 of 390 kcal')
  expect(card.getByRole('progressbar', { name: 'Drift' })).toBeTruthy()

  // The step is a revision of the plan, by the autopilot, taken back there.
  const history = within(screen.getByRole('region', { name: 'Plan history' }))
  history.getByText('Oct 23, by Autopilot')
  history.getByText('Lowered your target by 100 kcal a day')
  // Brief: the changes beneath say the rest.
  expect(history.queryByText(/^Your TDEE is/)).toBeNull()
  history.getByRole('button', { name: 'Take back' })

  // The row of "You and limits" says it in short.
  expect(
    screen.getByRole('link', {
      name: 'Autopilot',
      description: 'On · Fridays · drift -200 of 390 kcal',
    }),
  ).toBeTruthy()

  fireEvent.click(card.getByRole('link', { name: 'Autopilot settings' }))
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/plan/autopilot')
  })
})

test('asking first, its proposal is the card above, by Autopilot, without the data beside it', async () => {
  const proposed: Review = review({
    outcome: 'proposed',
    proposal_id: '0193c3a5-0000-7000-8000-0000000000c2',
    past_drift: true,
    drift_kcal: -400,
    drift_limit_kcal: 390,
  })
  const pending: Proposal = {
    ...step,
    id: '0193c3a5-0000-7000-8000-0000000000c2',
    status: 'pending',
    check_in: proposed,
  }
  fakeServer({
    ...signedIn(),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () =>
      Response.json(
        planBody({
          ...cut,
          pending,
          autopilot: { mode: 'propose', next: '2026-10-30', drift_kcal: 0 },
        }),
      ),
    'GET /api/v1/autopilot': () =>
      Response.json(autopilotBody({ next: '2026-10-30', weekday: 'friday', last: proposed })),
    'GET /api/v1/summary': () => problem(404),
  })
  renderAt('/plan')
  const proposal = within(await screen.findByRole('region', { name: 'A proposed change' }))
  proposal.getByText(/^Proposed by Autopilot · /)
  proposal.getByText('Suggests lowering your target by 100 kcal a day')
  proposal.getByText('You lost 0.31% a week; the plan is 0.5% (0.4% to 0.6%).')
  proposal.getByText('Past your drift limit')
  proposal.getByText(
    'On autopilot this step would apply by itself, but it takes the drift to -400 kcal. That is past your limit of 390 kcal. Accepting it makes it a target you agreed to, and the drift starts again at 0.',
  )
  expect(proposal.queryByRole('region', { name: 'What the data say' })).toBeNull()

  // The card says only that it waits, and the step it would take.
  const card = within(screen.getByRole('region', { name: 'Autopilot' }))
  card.getByText('Asks first')
  card.getByText('It waits for your answer, above.')
  expect(card.queryByText('The numbers')).toBeNull()
  card.getByText('up to 100 kcal')
  card.getByText('a day, if you accept')
})

test('before a drift base, the card says the limit comes with the first step, and when that is', async () => {
  serve(
    { ...cut, autopilot: { mode: 'autopilot', next: '2026-10-30', drift_kcal: 0 } },
    autopilotBody({
      mode: 'autopilot',
      weekday: 'friday',
      next: '2026-10-30',
      first_step: '2026-11-13',
    }),
  )
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'Autopilot' }))
  card.getByText('0 kcal')
  card.getByText('Its limit in kcal is set from the first step.')
  card.getByText('The first step can come from Nov 13.')
  expect(card.queryByText(/^Last review/)).toBeNull()
})

test('an autopilot that did not come is said as the plan has it', async () => {
  serve(cut, undefined)
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'Autopilot' }))
  card.getByText('On')
  card.getByText('Fri, Oct 30')
  expect(card.getByText('-200 of 390 kcal')).toBeTruthy()
})

test('in Croatian the card says the review in its words', async () => {
  serve(cut, onAutopilot, 'hr')
  setLocale('hr')
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'Autopilot' }))
  card.getByText('Uključen')
  card.getByText('Spustio je cilj za 100 kcal dnevno')
  card.getByText('Gubiš 0,31 % tjedno; plan je 0,5 % (0,4 % do 0,6 %).')
  card.getByText('petkom')
  expect(card.getByText('−200 od 390 kcal')).toBeTruthy()
})

test("today says this morning's step in a line to the plan, and nothing on another day", async () => {
  serve(cut, onAutopilot)
  const router = renderAt('/')
  const line = await screen.findByRole('link', { name: /^Autopilot: / })
  expect(line.textContent).toBe('Autopilot: target -100 kcal a day, from todayWhy')
  fireEvent.click(line)
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/plan')
  })
  cleanup()

  serve(cut, { ...onAutopilot, last: review({ date: '2026-10-16' }) })
  renderAt('/')
  await screen.findByRole('heading', { name: 'Meals' })
  expect(screen.queryByRole('link', { name: /^Autopilot: / })).toBeNull()
})

test('today says a hold plainly, and a proposal of its own as the autopilot suggests it', async () => {
  serve(cut, {
    ...onAutopilot,
    last: review({ outcome: 'hold', reason: 'on_target', step_kcal: 0 }),
  })
  renderAt('/')
  const line = await screen.findByRole('link', { name: /^Autopilot: / })
  expect(line.textContent).toBe('Autopilot: holding, pace on planWhy')
  cleanup()

  const pending: Proposal = {
    ...step,
    status: 'pending',
    check_in: review({ outcome: 'proposed' }),
  }
  serve(
    { ...cut, targets: [{ effective_from: '2026-09-28', ...before }], pending },
    { ...onAutopilot, last: review({ outcome: 'proposed' }) },
  )
  renderAt('/')
  const nudge = await screen.findByRole('link', { name: /^The autopilot suggests: / })
  expect(nudge.textContent).toBe(
    'The autopilot suggests: training days 2,350 → 2,250 kcal, rest days 2,050 → 1,950 kcalReview',
  )
  expect(screen.queryByRole('link', { name: /^Autopilot: / })).toBeNull()
})

test('the recent changes list a step as by Autopilot, without Undo', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/changes': () =>
      Response.json({
        changes: [
          {
            id: crypto.randomUUID(),
            action: 'proposal.status',
            status: 'applied',
            entity: step.id,
            at: `${today}T16:00:00Z`,
            client: 'autopilot',
            days: [],
            items: [],
            next: false,
          },
          {
            id: crypto.randomUUID(),
            action: 'autopilot.set',
            entity: 'autopilot',
            at: `${today}T15:00:00Z`,
            client: 'web',
            days: [],
            items: [],
          },
        ],
      }),
  })
  renderAt('/settings')
  fireEvent.click(await screen.findByRole('button', { name: 'Recent changes' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Recent changes' }))
  const rows = await sheet.findAllByRole('listitem')
  expect(rows.map((r) => r.textContent)).toEqual([
    'A proposal applied9:00 AM · Autopilot',
    'The autopilot changed8:00 AM · you',
  ])
  expect(sheet.queryByRole('button', { name: /^Undo/ })).toBeNull()
})
