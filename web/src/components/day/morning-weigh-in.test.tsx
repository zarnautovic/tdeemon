import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { day, fakeServer, renderAt, signedIn, today } from '@/test/app'
import { dayServer } from '@/test/day-server'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.useRealTimers()
  setLocale('en')
  localStorage.clear()
})

// at sets the clock to a time of today in Los Angeles, the user's.
function at(hours: number) {
  vi.useFakeTimers({ toFake: ['Date'] })
  // Daylight time there: UTC less 7 hours.
  vi.setSystemTime(new Date(`${today}T${String(hours + 7).padStart(2, '0')}:00:00Z`))
}

// yesterday is the day before today, weighed as given.
function yesterday(source: 'manual' | 'import' = 'manual') {
  return {
    'GET /api/v1/days/2026-10-22': () =>
      Response.json(
        day({
          date: '2026-10-22',
          metrics: [{ kind: 'weight', value: 82.5, unit: 'kg', source }],
        }),
      ),
  }
}

function hero() {
  return within(screen.getByRole('region', { name: 'Energy and macros' }))
}

test('in the morning, the hero asks for the weigh-in, hinting with yesterday’s, and saves it', async () => {
  at(8)
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes, ...yesterday() })
  renderAt('/')

  const field = await within(
    await screen.findByRole('region', { name: 'Energy and macros' }),
  ).findByLabelText('Morning weigh-in?')
  expect(field.getAttribute('placeholder')).toBe('82.5')
  expect(field.getAttribute('inputmode')).toBe('decimal')
  fireEvent.change(field, { target: { value: '82.3' } })
  fireEvent.click(hero().getByRole('button', { name: 'Save the weigh-in' }))

  expect(await screen.findByText('Saved: Weight')).toBeDefined()
  expect(server.sent.map((s) => s.body)).toEqual([{ value: 82.3 }])
  // Weighed, the line goes, and the body has it.
  await waitFor(() => {
    expect(hero().queryByLabelText('Morning weigh-in?')).toBeNull()
  })
  expect(
    within(screen.getByRole('region', { name: 'Body today' })).getByRole('button', {
      name: /^Weight82\.3 kg/,
    }),
  ).toBeDefined()
})

test('the line says its hint in the language', async () => {
  at(7)
  fakeServer({ ...signedIn('hr'), ...dayServer().routes, ...yesterday() })
  renderAt('/')

  const field = await within(
    await screen.findByRole('region', { name: 'Energija i makroi' }),
  ).findByLabelText('Jutarnje vaganje?')
  expect(field.getAttribute('placeholder')).toBe('82,5')
})

test.each([
  ['in the afternoon', 14, []],
  ['before the day starts', 3, []],
  [
    'once today has a weigh-in, from a device too',
    8,
    [{ kind: 'weight' as const, value: 82, unit: 'kg', source: 'import' as const }],
  ],
])('the hero asks for no weigh-in %s', async (_, hour, metrics) => {
  at(hour)
  const fetch = fakeServer({
    ...signedIn(),
    ...dayServer(day({ metrics })).routes,
    ...yesterday(),
  })
  renderAt('/')

  await screen.findByRole('region', { name: 'Energy and macros' })
  // The day before has come, with a weigh-in by hand (the day's chips ask
  // for it too).
  await waitFor(() => {
    expect(asked(fetch, '/api/v1/days/2026-10-22')).toBe(1)
  })
  await waitFor(() => {
    expect(screen.queryByLabelText('Morning weigh-in?')).toBeNull()
  })
})

test('the hero asks no weigh-in of a user whose weight a device sends', async () => {
  at(8)
  const fetch = fakeServer({ ...signedIn(), ...dayServer().routes, ...yesterday('import') })
  renderAt('/')

  await screen.findByRole('region', { name: 'Energy and macros' })
  // The day before, which says so, has come.
  await waitFor(() => {
    expect(asked(fetch, '/api/v1/days/2026-10-22')).toBe(1)
  })
  await waitFor(() => {
    expect(screen.queryByLabelText('Morning weigh-in?')).toBeNull()
  })
})

test("before the app knows the day before, it asks nothing: a device's weight may be on its way", async () => {
  at(8)
  // The day before does not come.
  const fetch = fakeServer({ ...signedIn(), ...dayServer().routes })
  renderAt('/')

  await screen.findByRole('region', { name: 'Energy and macros' })
  await waitFor(() => {
    expect(asked(fetch, '/api/v1/days/2026-10-22')).toBe(1)
  })
  expect(screen.queryByLabelText('Morning weigh-in?')).toBeNull()
})

// asked counts the requests a server was sent of a path.
function asked(fetch: ReturnType<typeof fakeServer>, path: string): number {
  return fetch.mock.calls.filter(([input, init]) => {
    const request = input instanceof Request ? input : new Request(input, init)
    return new URL(request.url).pathname === path
  }).length
}
