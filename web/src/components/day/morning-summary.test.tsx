import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { fakeServer, problem, renderAt, signedIn, today } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
})

const text = `Morning summary, Friday Oct 23
Sleep: 7 h 12 min
Weight: 81.2 kg · trend 81.6 kg
Yesterday: 2140/2300 kcal · P 150/160 g · C 230/280 g · F 70/80 g`

// morningServer answers the morning summary of each date asked, and
// keeps the dates.
function morningServer(answer?: () => Response, locale?: 'hr') {
  const asked: (string | null)[] = []
  fakeServer({
    ...signedIn(locale),
    'GET /api/v1/morning': (r) => {
      const date = new URL(r.url).searchParams.get('date')
      asked.push(date)
      return answer
        ? answer()
        : Response.json({ date, title: 'Morning summary', text, short: 'Yesterday 2140/2300 kcal' })
    },
  })
  return asked
}

test("the morning's notification opens its summary over the day, a part a line", async () => {
  const asked = morningServer()
  const router = renderAt(`/?summary=${today}`)

  const sheet = within(await screen.findByRole('dialog', { name: 'Morning summary' }))
  expect(await sheet.findByText('Morning summary, Friday Oct 23')).toBeTruthy()
  const sleep = sheet.getByText('Sleep')
  expect(sleep.tagName).toBe('DT')
  expect(sleep.nextElementSibling?.textContent).toBe('7 h 12 min')
  expect(sheet.getByText('2140/2300 kcal · P 150/160 g · C 230/280 g · F 70/80 g')).toBeTruthy()
  expect(asked).toEqual([today])

  fireEvent.click(sheet.getByRole('button', { name: 'Close' }))
  await waitFor(() => {
    expect(router.state.location.search).toEqual({})
  })
  expect(screen.queryByRole('dialog')).toBeNull()
})

test("today's header opens this morning's summary", async () => {
  morningServer()
  const router = renderAt('/')
  fireEvent.click(await screen.findByRole('button', { name: 'Morning summary' }))
  expect(await screen.findByRole('dialog', { name: 'Morning summary' })).toBeTruthy()
  expect(router.state.location.search).toEqual({ summary: today })
})

test('a summary that did not load says so, with a way to ask again, in Croatian', async () => {
  let fail = true
  morningServer(
    () =>
      fail
        ? problem(503)
        : Response.json({
            date: today,
            title: 'Jutarnji sažetak',
            text: 'Jutarnji sažetak, petak 23. 10.\nSan: 7 h 12 min',
            short: '',
          }),
    'hr',
  )
  renderAt(`/?summary=${today}`)
  const sheet = within(await screen.findByRole('dialog', { name: 'Jutarnji sažetak' }))
  expect(await sheet.findByText('Jutarnji sažetak se nije učitao')).toBeTruthy()
  fail = false
  fireEvent.click(sheet.getByRole('button', { name: 'Pokušaj ponovno' }))
  expect(await sheet.findByText('7 h 12 min')).toBeTruthy()
})

test('an address whose summary is no date opens nothing', async () => {
  const asked = morningServer()
  renderAt('/?summary=tomorrow')
  expect(await screen.findByRole('heading', { name: 'Today' })).toBeTruthy()
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(asked).toEqual([])
})
