import { cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { day, fakeServer, problem, renderAt, signedIn, today, type Handler } from '@/test/app'
import { dayServer } from '@/test/day-server'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

const checkIn = { action: 'day.checkin', entity: today }

// tapMood logs a mood on today's page, through a server whose undo is
// undo, and returns the bodies the undo was sent.
async function tapMood(undo: Handler) {
  const server = dayServer()
  const undos: unknown[] = []
  fakeServer({
    ...signedIn(),
    ...server.routes,
    'POST /api/v1/undo': async (request) => {
      undos.push(await request.json())
      return undo(request)
    },
  })
  renderAt('/')
  const mood = within(await screen.findByRole('group', { name: 'Mood' }))
  fireEvent.click(mood.getByRole('button', { name: '4 good' }))
  expect(await screen.findByText('Check-in saved')).toBeDefined()
  return { undos, mood }
}

test('a change is taken back from its toast, and the toast says so', async () => {
  const { undos } = await tapMood(() =>
    Response.json({
      undone: {
        action: 'day.checkin',
        entity: today,
        at: `${today}T16:00:00Z`,
        client: 'web',
        days: [today],
      },
    }),
  )

  fireEvent.click(screen.getByRole('button', { name: 'Undo' }))

  expect(await screen.findByText('Undone: a check-in.')).toBeDefined()
  // Of the day shown: no link to it.
  expect(screen.queryByRole('link', { name: /^See/ })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull()
  expect(undos).toEqual([{ id: expect.any(String) as unknown, last: checkIn }])
})

test.each([
  [412, 'Another change came first, so nothing was undone.'],
  [409, "A later change is in the way, so this can't be undone."],
  [404, 'Nothing to undo.'],
])('an undo refused (%i) says why, and offers no other', async (status, said) => {
  await tapMood(() => problem(status))

  fireEvent.click(screen.getByRole('button', { name: 'Undo' }))

  expect(await screen.findByText(said)).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull()
})

test('an undo without an answer is sent again under its id', async () => {
  let answer = false
  const { undos } = await tapMood(() =>
    answer
      ? Response.json({
          undone: {
            action: 'day.checkin',
            entity: today,
            at: `${today}T16:00:00Z`,
            client: 'web',
            days: [today],
          },
          repeated: true,
        })
      : Promise.reject(new TypeError('Failed to fetch')),
  )

  fireEvent.click(screen.getByRole('button', { name: 'Undo' }))
  expect(await screen.findByText(/^Not undone. /)).toBeDefined()
  answer = true
  fireEvent.click(screen.getByRole('button', { name: 'Undo' }))

  expect(await screen.findByText('Undone: a check-in.')).toBeDefined()
  expect(undos).toHaveLength(2)
  expect(undos[1]).toEqual(undos[0])
})

test('the toast of a change takes the place of the one before: one Undo, of the latest', async () => {
  const { mood } = await tapMood(() => problem(500))

  fireEvent.click(mood.getByRole('button', { name: '2 low' }))
  await vi.waitFor(() => {
    expect(mood.getByRole('button', { name: '2 low' }).getAttribute('aria-pressed')).toBe('true')
  })

  expect(screen.getAllByText('Check-in saved')).toHaveLength(1)
  expect(screen.getAllByRole('button', { name: 'Undo' })).toHaveLength(1)
})

test('from a sheet, F6 takes the focus to the toasts, named in the language, and Undo there leaves the sheet open', async () => {
  const weighed = day({ metrics: [{ kind: 'weight', value: 72.5, unit: 'kg', source: 'manual' }] })
  const server = dayServer(weighed)
  let undone = false
  const now = () => Response.json(undone ? weighed : server.day())
  fakeServer({
    ...signedIn('hr'),
    ...server.routes,
    'GET /api/v1/days/today': now,
    [`GET /api/v1/days/${today}`]: now,
    'POST /api/v1/undo': () => {
      undone = true
      return Response.json({
        undone: {
          action: 'metric.delete',
          entity: `${today}/weight`,
          at: `${today}T16:00:00Z`,
          client: 'web',
          days: [today],
        },
      })
    },
  })
  renderAt('/')
  fireEvent.click(await screen.findByRole('button', { name: 'Sva mjerenja' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Mjerenja' }))
  fireEvent.click(await sheet.findByRole('button', { name: 'Promijeni: Kilaža' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Obriši' }))
  expect(await screen.findByText('Obrisano: Kilaža')).toBeDefined()

  // A sheet keeps Tab within it; F6 is Base UI's way out to the toasts.
  fireEvent.keyDown(window, { key: 'F6' })
  const region = screen.getByRole('region', { name: 'Obavijesti' })
  expect(document.activeElement).toBe(region)
  fireEvent.click(within(region).getByRole('button', { name: 'Poništi' }))

  expect(await screen.findByText('Poništeno: obrisano mjerenje.')).toBeDefined()
  expect(await sheet.findByText('72,5 kg')).toBeDefined()
  expect(screen.getByRole('dialog', { name: 'Mjerenja' })).toBeDefined()
})

test('a toast is closed by its button', async () => {
  await tapMood(() => problem(500))

  // Base UI hides it from a screen reader, which hears the toast in the
  // viewport's live region; a pointer or the keyboard in the toast uses
  // it.
  fireEvent.click(screen.getByLabelText('Close'))

  await vi.waitFor(() => {
    expect(screen.queryByText('Check-in saved')).toBeNull()
  })
})
