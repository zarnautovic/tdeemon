import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { fakeServer, problem, renderAt, signedIn } from '@/test/app'
import { autopilotBody, limitsBody, planBody } from '@/test/plan'

type Autopilot = components['schemas']['AutopilotBody']

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

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null
}

// autopilotServer keeps the autopilot as the server does, and the bodies
// it was sent.
function autopilotServer(start: Autopilot = autopilotBody(), locale: 'en' | 'hr' = 'en') {
  let current = start
  const sent: unknown[] = []
  let fails = false
  fakeServer({
    ...signedIn(locale),
    'GET /api/v1/limits': () => Response.json(limitsBody()),
    'GET /api/v1/plan': () => Response.json(planBody()),
    'GET /api/v1/autopilot': () => (fails ? problem(503) : Response.json(current)),
    'PUT /api/v1/autopilot': async (r) => {
      const body: unknown = await r.json()
      sent.push(body)
      const b = isObject(body) ? body : {}
      if (b.mode === 'autopilot' || b.mode === 'propose') {
        current = { ...current, mode: b.mode }
      }
      if (b.weekday === 'saturday') {
        current = { ...current, weekday: 'saturday', next: '2026-10-24' }
      }
      return Response.json(current)
    },
  })
  return {
    sent,
    fail: (f: boolean) => {
      fails = f
    },
  }
}

test('turning the autopilot on asks first, with its bounds in numbers, and says it in a toast', async () => {
  const { sent } = autopilotServer()
  renderAt('/plan/autopilot')
  await screen.findByRole('heading', { name: 'Autopilot', level: 1 })
  const mode = within(await screen.findByRole('group', { name: 'When the review finds a step' }))
  expect(mode.getByRole('radio', { name: 'Ask me first' })).toHaveProperty('checked', true)
  // Asking first, it says no drift.
  expect(screen.queryByRole('heading', { name: 'Drift' })).toBeNull()

  fireEvent.click(mode.getByRole('radio', { name: 'Autopilot' }))
  const ask = within(await screen.findByRole('region', { name: 'Turn on the autopilot?' }))
  // No drift limit in kcal before the first step: never a number that moves.
  ask.getByText(
    "It will change your kcal target by itself, and tell you each time. At most 100 kcal a day in a week, and 15% of your TDEE in all, from the first step. You can take any step back in the plan's history.",
  )
  expect(document.activeElement).toBe(ask.getByRole('button', { name: 'Not now' }))
  expect(sent).toEqual([])

  fireEvent.click(ask.getByRole('button', { name: 'Turn on' }))
  expect(await screen.findByText('Autopilot on')).toBeTruthy()
  expect(sent).toEqual([{ mode: 'autopilot' }])
  expect(screen.queryByRole('region', { name: 'Turn on the autopilot?' })).toBeNull()
  // The focus goes back to the mode, not to the page.
  await waitFor(() => {
    expect(document.activeElement).toBe(screen.getByRole('radio', { name: 'Autopilot' }))
  })
  // Its drift, now it counts.
  await screen.findByRole('heading', { name: 'Drift' })
  screen.getByText('0 kcal')
  screen.getByText('Its limit in kcal is set from the first step.')
  // No Undo: undo leaves it.
  expect(screen.queryByRole('button', { name: /^Undo/ })).toBeNull()
})

test('turning it on with a drift base says its limit in kcal, and Not now keeps it asking', async () => {
  const { sent } = autopilotServer(
    autopilotBody({
      // The step it would take now, the API's, under the limits'.
      step_kcal: 80,
      drift: {
        used_kcal: 0,
        limit_pct: 15,
        limit_kcal: 390,
        base_kcal: 2600,
        base_from: '2026-09-28',
        agreed_on: '2026-09-28',
        agreed_kcal: 2179,
      },
    }),
  )
  renderAt('/plan/autopilot')
  fireEvent.click(await screen.findByRole('radio', { name: 'Autopilot' }))
  const ask = within(await screen.findByRole('region', { name: 'Turn on the autopilot?' }))
  ask.getByText(/At most 80 kcal a day in a week, and 390 kcal in all\./)
  screen.getByText(/^Each week the autopilot reviews your pace and takes a small step/)

  fireEvent.click(ask.getByRole('button', { name: 'Not now' }))
  expect(screen.queryByRole('region', { name: 'Turn on the autopilot?' })).toBeNull()
  expect(screen.getByRole('radio', { name: 'Ask me first' })).toHaveProperty('checked', true)
  expect(document.activeElement).toBe(screen.getByRole('radio', { name: 'Ask me first' }))
  expect(sent).toEqual([])
  screen.getByText('In all, at most 390 kcal from the target you last agreed to (15% of your TDEE)')
})

test('turning it off applies at once, and the drift goes', async () => {
  const { sent } = autopilotServer(
    autopilotBody({
      mode: 'autopilot',
      step_kcal: 100,
      drift: {
        used_kcal: -300,
        limit_pct: 15,
        limit_kcal: 390,
        base_kcal: 2600,
        base_from: '2026-09-28',
        agreed_on: '2026-09-28',
        agreed_kcal: 2279,
      },
    }),
  )
  renderAt('/plan/autopilot')
  const drift = within(await screen.findByRole('region', { name: 'Drift' }))
  drift.getByText('-300 of 390 kcal')
  drift.getByText('90 kcal left')
  // A step of 100 more would pass it.
  drift.getByText('At the limit: a next step the same way would be proposed')
  drift.getByText(
    'What the autopilot moved your target since you agreed to 2,279 kcal a day on Sep 28. Accepting or setting a target starts it again at 0.',
  )

  fireEvent.click(screen.getByRole('radio', { name: 'Ask me first' }))
  expect(await screen.findByText('The autopilot asks first')).toBeTruthy()
  expect(sent).toEqual([{ mode: 'propose' }])
  expect(screen.queryByRole('region', { name: 'Turn on the autopilot?' })).toBeNull()
  await waitFor(() => {
    expect(screen.queryByRole('region', { name: 'Drift' })).toBeNull()
  })
})

test('the review day is a chip of the week, with the next review', async () => {
  const { sent } = autopilotServer()
  renderAt('/plan/autopilot')
  const day = within(await screen.findByRole('region', { name: 'Review day' }))
  expect(day.getAllByRole('radio').map((r) => r.closest('label')?.textContent)).toEqual([
    'Mon',
    'Tue',
    'Wed',
    'Thu',
    'Fri',
    'Sat',
    'Sun',
  ])
  expect(day.getByRole('radio', { name: 'Mon' })).toHaveProperty('checked', true)
  day.getByText('In the morning, before your morning summary. Next review: Mon, Oct 26.')

  fireEvent.click(day.getByRole('radio', { name: 'Sat' }))
  expect(await screen.findByText('Review day changed')).toBeTruthy()
  expect(sent).toEqual([{ weekday: 'saturday' }])
  await day.findByText('In the morning, before your morning summary. Next review: Sat, Oct 24.')
})

test('what it may change, never changes and waits for come from the limits', async () => {
  autopilotServer()
  renderAt('/plan/autopilot')
  const may = within(await screen.findByRole('region', { name: 'What it may change' }))
  may.getByText('Your kcal target, by at most 100 kcal a day in a week (5% of it at most)')
  may.getByText(
    'In all, at most 15% of your TDEE from the target you last agreed to, from the first step',
  )
  within(screen.getByRole('region', { name: 'What it never changes' })).getByText(
    'Your phase, its pace or its goal',
  )
  const waits = within(screen.getByRole('region', { name: 'When it waits' }))
  waits.getByText('4 logged days and 3 weigh-ins in the last 7 days')
  waits.getByText('14 days into a phase, or from a target you set; 7 days since its last step')
  screen.getByText(
    "Undo (⌘Z) doesn't take back a step. Take it back in the plan's history, as any change of the plan.",
  )
  const limits = screen.getByRole('link', { name: 'Step and drift limits' })
  expect(limits.getAttribute('href')).toBe('/plan/limits')
})

test('in Croatian the weekdays are in lower case, and the modes in its words', async () => {
  autopilotServer(autopilotBody(), 'hr')
  setLocale('hr')
  renderAt('/plan/autopilot')
  const day = within(await screen.findByRole('region', { name: 'Dan pregleda' }))
  expect(day.getAllByRole('radio').map((r) => r.closest('label')?.textContent)).toEqual([
    'pon',
    'uto',
    'sri',
    'čet',
    'pet',
    'sub',
    'ned',
  ])
  screen.getByRole('radio', { name: 'Pitaj me prvo' })
  screen.getByText('4 upisana dana i 3 vaganja u zadnjih 7 dana')
})

test('an autopilot that did not load says so, with a way to ask again', async () => {
  const server = autopilotServer()
  server.fail(true)
  renderAt('/plan/autopilot')
  const again = await screen.findByRole('button', { name: 'Try again' })
  screen.getByText("Couldn't load the autopilot.")
  server.fail(false)
  fireEvent.click(again)
  expect(await screen.findByRole('radio', { name: 'Ask me first' })).toBeTruthy()
})

test('the question cancelled by asking first again sends nothing', async () => {
  const { sent } = autopilotServer()
  renderAt('/plan/autopilot')
  fireEvent.click(await screen.findByRole('radio', { name: 'Autopilot' }))
  await screen.findByRole('region', { name: 'Turn on the autopilot?' })
  fireEvent.click(screen.getByRole('radio', { name: 'Ask me first' }))
  expect(screen.queryByRole('region', { name: 'Turn on the autopilot?' })).toBeNull()
  expect(screen.getByRole('radio', { name: 'Ask me first' })).toHaveProperty('checked', true)
  expect(screen.queryByText('The autopilot asks first')).toBeNull()
  expect(sent).toEqual([])
})
