import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Meal } from '@/api/types'
import { setLocale } from '@/i18n/locale'
import { fakeServer, me, renderAt } from '@/test/app'
import { dayServer } from '@/test/day-server'
import { dayWithoutTarget, newUser, summaryOfNothing } from '@/test/new-user'
import { planBody } from '@/test/plan'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

async function card() {
  return within(await screen.findByRole('region', { name: 'First steps' }))
}

// steps are what each first step says, a screen reader's words before
// it.
function steps(): (string | null)[] {
  return within(screen.getByRole('region', { name: 'First steps' }))
    .getAllByRole('listitem')
    .map((li) => li.textContent)
}

const used = {
  id: '0193c3a5-0000-7000-8000-0000000000c2',
  name: 'Claude',
  scopes: ['tools' as const],
  created_at: '2026-10-22T08:00:00Z',
  last_used_at: '2026-10-22T09:00:00Z',
}

const meal: Meal = {
  id: '0193c3a5-0000-7000-8000-0000000000a1',
  eaten_at: '2026-10-22T15:45:00Z',
  description: 'Oats',
  client: 'mcp',
  total: { kcal: 300, protein: 11, carbs: 54, fat: 6 },
  items: [],
}

test("a new user's today is the first steps, none done, each the way to its own", async () => {
  fakeServer(newUser())
  renderAt('/')

  const steps4 = await card()
  expect(steps4.getByText('0 of 4')).toBeDefined()
  expect(steps()).toEqual([
    'To do:Complete your profile',
    'To do:First weigh-inkg',
    'To do:Connect your AI coach or a phone',
    'To do:Log your first meal',
  ])
  expect(steps4.getByRole('link', { name: 'Complete your profile' }).getAttribute('href')).toBe(
    '/plan/profile',
  )
  expect(
    steps4.getByRole('link', { name: 'Connect your AI coach or a phone' }).getAttribute('href'),
  ).toBe('/settings/devices?connect=choose')
  expect(steps4.getByRole('link', { name: 'Log your first meal' }).getAttribute('href')).toBe(
    '/?log=true',
  )
  // In place of the hero of a day without a target.
  expect(screen.queryByText('A plan gives the day its target.')).toBeNull()
  expect(steps4.queryByText('Ask your AI coach for a plan')).toBeNull()
})

test('the first weigh-in is given in the card, which ticks it', async () => {
  const server = dayServer(dayWithoutTarget())
  fakeServer({ ...newUser(), ...server.routes })
  renderAt('/')

  const steps4 = await card()
  fireEvent.change(steps4.getByLabelText('First weigh-in'), { target: { value: '72.5' } })
  fireEvent.click(steps4.getByRole('button', { name: 'Save the weigh-in' }))

  expect(await steps4.findByText('1 of 4')).toBeDefined()
  expect(steps()[1]).toBe('Done:First weigh-in · 72.5 kg')
  expect(await screen.findByText('Saved: Weight')).toBeDefined()
})

test('with the profile, a weigh-in and a call of the AI coach, it says what to ask it for', async () => {
  const writeText = vi.fn<(s: string) => Promise<void>>(() => Promise.resolve())
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  fakeServer({
    ...newUser(),
    'GET /api/v1/me': () => Response.json(me()),
    'GET /api/v1/summary': () =>
      Response.json(summaryOfNothing({ trend: { date: '2026-10-22', kg: 80.2 } })),
    'GET /api/v1/tokens': () => Response.json({ tokens: [used] }),
  })
  renderAt('/')

  const steps4 = await card()
  expect(await steps4.findByText('3 of 4')).toBeDefined()
  expect(steps()).toEqual([
    'Done:Your profile',
    'Done:First weigh-in',
    'Done:Your AI coach or a phone',
    'To do:Log your first meal',
  ])
  expect(steps4.getByText('Ask your AI coach for a plan')).toBeDefined()
  fireEvent.click(steps4.getByRole('button', { name: 'Copy: Write this to your AI coach' }))
  await waitFor(() => {
    expect(writeText).toHaveBeenCalledWith('Look at my data and propose a plan.')
  })
  Reflect.deleteProperty(navigator, 'clipboard')
})

test('a meal logged before counts, and a key never used does not', async () => {
  fakeServer({
    ...newUser(),
    'GET /api/v1/meals/recent': () =>
      Response.json({ meals: [{ ...meal, eaten: [{ at: meal.eaten_at, day: '2026-10-22' }] }] }),
    'GET /api/v1/tokens': () => Response.json({ tokens: [{ ...used, last_used_at: undefined }] }),
  })
  renderAt('/')

  const steps4 = await card()
  expect(await steps4.findByText('1 of 4')).toBeDefined()
  expect(steps()[2]).toBe('To do:Connect your AI coach or a phone')
  expect(steps()[3]).toBe('Done:Your first meal')
})

test('put off for later, the hero is back, until the plan shows the steps again', async () => {
  fakeServer(newUser())
  const router = renderAt('/')

  fireEvent.click((await card()).getByRole('button', { name: 'Later' }))
  expect(await screen.findByText('A plan gives the day its target.')).toBeDefined()
  expect(screen.queryByRole('region', { name: 'First steps' })).toBeNull()

  fireEvent.click(
    within(screen.getByRole('navigation', { name: 'Sections' })).getByRole('link', {
      name: 'Plan',
    }),
  )
  fireEvent.click(await screen.findByRole('link', { name: 'Show the first steps' }))
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/')
  })
  expect(await card()).toBeDefined()
  expect(screen.queryByText('A plan gives the day its target.')).toBeNull()
})

test.each([
  ['a plan', { phases: [{ start: '2026-10-30', type: 'cut' as const, rate_pct_week: -0.5 }] }],
  [
    'a proposal that waits',
    {
      pending: {
        id: '0193c3a5-0000-7000-8000-0000000000d1',
        status: 'pending' as const,
        rationale: 'A first plan.',
        by: { client: 'mcp' as const },
        created_at: '2026-10-23T08:00:00Z',
        expires_at: '2026-10-30T08:00:00Z',
        changes: { phases: [], targets: [], limits: [] },
      },
    },
  ],
])('there are no first steps with %s', async (_, over) => {
  fakeServer({ ...newUser(), 'GET /api/v1/plan': () => Response.json(planBody(over)) })
  renderAt('/')

  expect(await screen.findByRole('region', { name: 'Eaten' })).toBeDefined()
  expect(screen.queryByRole('region', { name: 'First steps' })).toBeNull()
})

test('the first steps speak Croatian', async () => {
  fakeServer(newUser('hr'))
  renderAt('/')

  const steps4 = within(await screen.findByRole('region', { name: 'Prvi koraci' }))
  expect(steps4.getByText('0 od 4')).toBeDefined()
  expect(steps4.getByRole('link', { name: 'Dopuni profil' })).toBeDefined()
  expect(steps4.getByRole('button', { name: 'Kasnije' })).toBeDefined()
})
