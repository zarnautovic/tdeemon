import { onlineManager } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import {
  askAgain,
  day,
  fakeServer,
  problem,
  renderAt,
  signedIn,
  unreachable,
  type Handler,
} from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.useRealTimers()
  onlineManager.setOnline(true)
  setLocale('en')
})

// at sets the clock to an instant.
function at(instant: string) {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(instant))
}

// routes are a server's of a user and their day.
function routes(locale: 'en' | 'hr' = 'en'): Record<string, Handler> {
  return {
    ...signedIn(locale),
    'GET /api/v1/days/today': () => Response.json(day()),
  }
}

// meals is the day's meals, once the day shows.
function meals() {
  return screen.findByRole('region', { name: /^(Meals|Obroci)$/ })
}

test('a day asked for again without an answer stays, with a bar that says since when, until it comes', async () => {
  // 9:30 in Los Angeles, the user's.
  at('2026-10-23T16:30:00Z')
  fakeServer(routes())
  const router = renderAt('/')
  await meals()
  expect(screen.queryByText(/^Offline/)).toBeNull()

  unreachable()
  await askAgain(router)

  expect(await screen.findByText('Offline · data from 9:30 AM')).toBeDefined()
  expect(screen.getByRole('region', { name: 'Meals' })).toBeDefined()
  expect(screen.queryByRole('alert')).toBeNull()

  fakeServer(routes())
  await askAgain(router)

  await waitFor(() => {
    expect(screen.queryByText(/^Offline/)).toBeNull()
  })
  expect(screen.getByRole('region', { name: 'Meals' })).toBeDefined()
})

test('a server that fails to answer leaves the data not updated, not offline', async () => {
  at('2026-10-23T16:30:00Z')
  fakeServer(routes())
  const router = renderAt('/')
  await meals()

  fakeServer({ 'GET /api/v1/days/today': () => problem(500) })
  await askAgain(router)

  expect(await screen.findByText('Not updated · data from 9:30 AM')).toBeDefined()
})

test('a refusal is no bad network: no bar', async () => {
  at('2026-10-23T16:30:00Z')
  fakeServer(routes())
  const router = renderAt('/')
  await meals()

  fakeServer({ ...routes(), 'GET /api/v1/days/today': () => problem(422) })
  await askAgain(router)

  expect(screen.queryByText(/data from/)).toBeNull()
})

test('data of a day before say their date too, in the language', async () => {
  // 21:14 the evening before in Los Angeles; then the next morning.
  at('2026-10-23T04:14:00Z')
  fakeServer(routes('hr'))
  const router = renderAt('/')
  await meals()

  vi.setSystemTime(new Date('2026-10-23T16:30:00Z'))
  unreachable()
  await askAgain(router)

  expect(await screen.findByText('Nema veze · podaci od 22. lis u 21:14')).toBeDefined()
})

test('the network gone, what waits for it is offline, and the bar goes once it is back', async () => {
  at('2026-10-23T16:30:00Z')
  fakeServer(routes())
  const router = renderAt('/')
  await meals()

  act(() => {
    onlineManager.setOnline(false)
    void router.options.context.queryClient.invalidateQueries()
  })

  expect(await screen.findByText('Offline · data from 9:30 AM')).toBeDefined()
  act(() => {
    onlineManager.setOnline(true)
  })
  await waitFor(() => {
    expect(screen.queryByText(/^Offline/)).toBeNull()
  })
})
