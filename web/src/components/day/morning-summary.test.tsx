import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { dateAt } from '@/lib/zoned'
import { fakeServer, problem, renderAt, session, signedIn, today } from '@/test/app'

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

test("today opens this morning's summary, of the calendar's date, and has the focus back", async () => {
  morningServer()
  const router = renderAt('/')
  const line = await screen.findByRole('button', { name: 'Morning summary' })
  fireEvent.click(line)
  const sheet = await screen.findByRole('dialog', { name: 'Morning summary' })
  // The calendar's date where the user is, not the day's (today here),
  // which a day that starts late keeps till noon.
  expect(router.state.location.search).toEqual({
    summary: dateAt(Date.now(), session().user.timezone),
  })
  fireEvent.keyDown(sheet, { key: 'Escape' })
  await waitFor(() => {
    expect(screen.queryByRole('dialog')).toBeNull()
  })
  await waitFor(() => {
    expect(document.activeElement).toBe(line)
  })
})

test('the summary of another date with a day shown keeps the day as the sheet closes', async () => {
  const asked = morningServer(() =>
    Response.json({
      date: '2026-10-21',
      title: 'Morning summary',
      text: 'Morning summary, Wednesday Oct 21\nSleep: 6 h\nSleep: 6 h',
      short: '',
    }),
  )
  const router = renderAt('/?date=2026-10-22&summary=2026-10-21')
  const sheet = within(await screen.findByRole('dialog', { name: 'Morning summary' }))
  // Two lines that read the same are both there.
  await waitFor(() => {
    expect(sheet.getAllByText('6 h')).toHaveLength(2)
  })
  expect(asked).toEqual(['2026-10-21'])
  fireEvent.click(sheet.getByRole('button', { name: 'Close' }))
  await waitFor(() => {
    expect(router.state.location.search).toEqual({ date: '2026-10-22' })
  })
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
