import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { fakeServer, renderAt, signedIn } from '@/test/app'
import { limitsBody, planBody } from '@/test/plan'

// The clock is the tests' day, October 23.
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

// A cut since September 7 to December 13, its target split.
const plan: Partial<Plan> = {
  phases: [
    { type: 'cut', start: '2026-09-07', end: '2026-12-13', rate_pct_week: -0.5, goal_weight: 80 },
  ],
  targets: [
    {
      effective_from: '2026-09-07',
      training: { kcal: 2250, protein: 165, carbs: 251.25, fat: 65 },
      rest: { kcal: 1950, protein: 160, carbs: 197, fat: 58 },
    },
  ],
  training_weekdays: ['monday', 'wednesday'],
}

// applied is the revision the server answers a change with.
const applied: Proposal = {
  id: '0193c3a5-0000-7000-8000-0000000000c1',
  status: 'applied',
  rationale: 'changed in the app',
  by: { client: 'web' },
  changes: { phases: [], targets: [], limits: [] },
  created_at: '2026-10-23T16:00:00Z',
  expires_at: '2026-10-30T16:00:00Z',
  decided_at: '2026-10-23T16:00:00Z',
  decided_by: { client: 'web' },
}

// refusal is the server's refusal of a change past the limits, its
// violation as an error detail's value.
function refusal(value: Record<string, unknown>): Response {
  return Response.json(
    {
      status: 422,
      title: 'Unprocessable Entity',
      detail: 'the proposal breaks the limits',
      errors: [{ location: 'target 2026-10-23', message: 'past the limit', value }],
    },
    { status: 422, headers: { 'Content-Type': 'application/problem+json' } },
  )
}

// editServer serves the plan, and keeps each change sent; answer answers
// it, the revision by default.
function editServer(
  answer: () => Response = () => Response.json({ proposal: applied, preview: { checks: [] } }),
) {
  const sent: unknown[] = []
  fakeServer({
    ...signedIn(),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => Response.json(planBody(plan)),
    'POST /api/v1/plan/changes': async (r) => {
      sent.push(await r.json())
      return answer()
    },
  })
  return { sent }
}

// openTargets opens the targets' edit, and returns the sheet.
async function openTargets() {
  renderAt('/plan')
  const card = within(await screen.findByRole('region', { name: 'Targets by type of day' }))
  fireEvent.click(card.getByRole('button', { name: 'Edit targets' }))
  return within(await screen.findByRole('dialog', { name: 'Targets by type of day' }))
}

// field is a field of a type of day's target in the targets' sheet.
function field(day: 'Training' | 'Rest', name: string) {
  const sheet = within(screen.getByRole('dialog', { name: 'Targets by type of day' }))
  return within(sheet.getByRole('group', { name: day })).getByRole('textbox', { name })
}

test('the targets are edited from today on, the carbs following the kcal', async () => {
  const { sent } = editServer()
  const sheet = await openTargets()
  sheet.getByText('From today on, until a change.')
  expect(field('Rest', 'kcal')).toHaveProperty('value', '1950')
  within(sheet.getByRole('group', { name: 'Rest' })).getByText('Carbs follow: 197 g')
  // A decimal no one typed is not shown as typed.
  within(sheet.getByRole('group', { name: 'Training' })).getByText('Carbs follow: 251 g')

  fireEvent.change(field('Rest', 'kcal'), { target: { value: '1850' } })
  within(sheet.getByRole('group', { name: 'Rest' })).getByText('Carbs follow: 172 g')
  fireEvent.click(sheet.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(sent).toEqual([
      {
        targets: [
          {
            from: '2026-10-23',
            training: { kcal: 2250, protein: 165, fat: 65 },
            rest: { kcal: 1850, protein: 160, fat: 58 },
          },
        ],
      },
    ])
  })
  await screen.findByText('Targets changed')
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'Targets by type of day' })).toBeNull()
  })
})

test('a target that is no number, or more fat and protein than its kcal, is not sent', async () => {
  const { sent } = editServer()
  const sheet = await openTargets()
  fireEvent.change(field('Training', 'kcal'), { target: { value: '' } })
  fireEvent.change(field('Rest', 'Fat, g'), { target: { value: '200' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Save' }))
  expect(field('Training', 'kcal').getAttribute('aria-invalid')).toBe('true')
  within(sheet.getByRole('group', { name: 'Training' })).getByText('Not a number.')
  within(sheet.getByRole('group', { name: 'Rest' })).getByText(
    'Protein and fat give more kcal than the day has.',
  )
  expect(sent).toEqual([])
})

test("a target past the user's limit says so at its field, and where to loosen it", async () => {
  editServer(() =>
    refusal({ rule: 'kcal_floor', day_type: 'rest', limit: 1500, proposed: 1300, level: 'user' }),
  )
  const sheet = await openTargets()
  fireEvent.change(field('Rest', 'kcal'), { target: { value: '1300' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Save' }))
  expect((await sheet.findByRole('alert')).textContent).toBe('The limits do not allow it.')
  const rest = within(sheet.getByRole('group', { name: 'Rest' }))
  rest.getByText('At least 1500 kcal: your limit.')
  expect(field('Rest', 'kcal').getAttribute('aria-invalid')).toBe('true')
  expect(field('Training', 'kcal').getAttribute('aria-invalid')).toBe('false')
  expect(sheet.getByRole('link', { name: 'Loosen it in your limits' }).getAttribute('href')).toBe(
    '/plan/limits',
  )
  // A change of the field forgets the refusal.
  fireEvent.change(field('Rest', 'kcal'), { target: { value: '1600' } })
  expect(sheet.queryByText('At least 1500 kcal: your limit.')).toBeNull()
})

test('a refusal of the target as a whole is said under the form', async () => {
  editServer(() =>
    refusal({ rule: 'implied_loss_rate', limit: -0.7, proposed: -0.9, level: 'instance' }),
  )
  const sheet = await openTargets()
  fireEvent.click(sheet.getByRole('button', { name: 'Save' }))
  const alert = await sheet.findByRole('alert')
  expect(alert.textContent).toBe(
    "The limits do not allow it.the loss the target makes: -0.9 proposed, the limit -0.7: the instance's limit",
  )
  expect(sheet.queryByRole('link', { name: 'Loosen it in your limits' })).toBeNull()
})

test('the phase is edited: its rate as how fast, its end and its goal', async () => {
  const { sent } = editServer()
  renderAt('/plan')
  const hero = within(await screen.findByRole('region', { name: 'Cut' }))
  fireEvent.click(hero.getByRole('button', { name: 'Edit phase' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Edit phase' }))
  sheet.getByText('Cut, since Sep 7')
  const rate = sheet.getByRole('textbox', { name: 'Loss (% a week)' })
  expect(rate).toHaveProperty('value', '0.5')
  fireEvent.change(rate, { target: { value: '0.6' } })
  fireEvent.change(sheet.getByLabelText('Ends on'), { target: { value: '2026-12-20' } })
  fireEvent.change(sheet.getByRole('textbox', { name: 'Goal weight, kg' }), {
    target: { value: '78' },
  })
  fireEvent.click(sheet.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(sent).toEqual([
      {
        phases: [
          {
            start: '2026-09-07',
            type: 'cut',
            rate_pct_week: -0.6,
            end: '2026-12-20',
            goal_weight: 78,
          },
        ],
      },
    ])
  })
  await screen.findByText('Phase changed')
})

test("a rate past the user's limit says so at the rate", async () => {
  editServer(() => refusal({ rule: 'cut_rate', limit: -0.7, proposed: -0.9, level: 'user' }))
  renderAt('/plan')
  const hero = within(await screen.findByRole('region', { name: 'Cut' }))
  fireEvent.click(hero.getByRole('button', { name: 'Edit phase' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Edit phase' }))
  const rate = sheet.getByRole('textbox', { name: 'Loss (% a week)' })
  fireEvent.change(rate, { target: { value: '0.9' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Save' }))
  await sheet.findByText('At most 0.7% a week: your limit.')
  expect(rate.getAttribute('aria-invalid')).toBe('true')
})

test('the next phase is planned after the one now, which ends the day before it', async () => {
  const { sent } = editServer()
  renderAt('/plan')
  const hero = within(await screen.findByRole('region', { name: 'Cut' }))
  fireEvent.click(hero.getByRole('button', { name: 'Next phase' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Next phase' }))
  const start = sheet.getByLabelText('Starts on')
  // By default, the day after the last phase's end.
  expect(start).toHaveProperty('value', '2026-12-14')
  // Maintenance has no rate to type.
  expect(sheet.queryByRole('textbox', { name: /% a week/ })).toBeNull()
  fireEvent.change(start, { target: { value: '2026-12-01' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(sent).toEqual([
      {
        phases: [
          {
            type: 'cut',
            start: '2026-09-07',
            end: '2026-11-30',
            rate_pct_week: -0.5,
            goal_weight: 80,
          },
          { start: '2026-12-01', type: 'maintain', rate_pct_week: 0 },
        ],
      },
    ])
  })
  await screen.findByText('Next phase planned')
})

test('a next phase before today, or a recomp without its rate, is not sent', async () => {
  const { sent } = editServer()
  renderAt('/plan')
  const hero = within(await screen.findByRole('region', { name: 'Cut' }))
  fireEvent.click(hero.getByRole('button', { name: 'Next phase' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Next phase' }))
  fireEvent.change(sheet.getByRole('combobox', { name: 'Type' }), { target: { value: 'recomp' } })
  fireEvent.change(sheet.getByLabelText('Starts on'), { target: { value: '2026-10-01' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Save' }))
  sheet.getByText('Today or later.')
  sheet.getByText('Not a number.')
  // A recomp's rate is typed with its sign.
  fireEvent.change(sheet.getByRole('textbox', { name: 'Rate (% a week, a minus is a loss)' }), {
    target: { value: '-0.2' },
  })
  fireEvent.change(sheet.getByLabelText('Starts on'), { target: { value: '2026-12-14' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(sent).toEqual([
      { phases: [{ start: '2026-12-14', type: 'recomp', rate_pct_week: -0.2 }] },
    ])
  })
})

test('in Croatian the edits speak Croatian, and take a decimal comma', async () => {
  const sent: unknown[] = []
  fakeServer({
    ...signedIn('hr'),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => Response.json(planBody(plan)),
    'POST /api/v1/plan/changes': async (r) => {
      sent.push(await r.json())
      return Response.json({ proposal: applied, preview: { checks: [] } })
    },
  })
  renderAt('/plan')
  const hero = within(await screen.findByRole('region', { name: 'Rez' }))
  fireEvent.click(hero.getByRole('button', { name: 'Uredi fazu' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Uredi fazu' }))
  fireEvent.change(sheet.getByRole('textbox', { name: 'Gubitak (% tjedno)' }), {
    target: { value: '0,6' },
  })
  fireEvent.click(sheet.getByRole('button', { name: 'Spremi' }))
  await waitFor(() => {
    expect(sent).toEqual([
      {
        phases: [
          {
            start: '2026-09-07',
            type: 'cut',
            rate_pct_week: -0.6,
            end: '2026-12-13',
            goal_weight: 80,
          },
        ],
      },
    ])
  })
  await screen.findByText('Faza promijenjena')
})
