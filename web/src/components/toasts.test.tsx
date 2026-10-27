import { cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { fakeServer, problem, renderAt, signedIn, today, type Handler } from '@/test/app'
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
  expect(await screen.findByText('The check-in is saved')).toBeDefined()
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

  expect(await screen.findByText('Taken back: a check-in.')).toBeDefined()
  // Of the day shown: no link to it.
  expect(screen.queryByRole('link', { name: /^See/ })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull()
  expect(undos).toEqual([{ id: expect.any(String) as unknown, last: checkIn }])
})

test.each([
  [412, 'Another change came since, so nothing was taken back.'],
  [409, 'It cannot be taken back: what came since clashes with it.'],
  [404, 'There is no change to take back.'],
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
  expect(await screen.findByText(/^Not taken back: /)).toBeDefined()
  answer = true
  fireEvent.click(screen.getByRole('button', { name: 'Undo' }))

  expect(await screen.findByText('Taken back: a check-in.')).toBeDefined()
  expect(undos).toHaveLength(2)
  expect(undos[1]).toEqual(undos[0])
})

test('the toast of a change takes the place of the one before: one Undo, of the latest', async () => {
  const { mood } = await tapMood(() => problem(500))

  fireEvent.click(mood.getByRole('button', { name: '2 low' }))
  await vi.waitFor(() => {
    expect(mood.getByRole('button', { name: '2 low' }).getAttribute('aria-pressed')).toBe('true')
  })

  expect(screen.getAllByText('The check-in is saved')).toHaveLength(1)
  expect(screen.getAllByRole('button', { name: 'Undo' })).toHaveLength(1)
})

test('a toast is closed by its button', async () => {
  await tapMood(() => problem(500))

  // Base UI hides it from a screen reader, which hears the toast in the
  // viewport's live region; a pointer or the keyboard in the toast uses
  // it.
  fireEvent.click(screen.getByLabelText('Close'))

  await vi.waitFor(() => {
    expect(screen.queryByText('The check-in is saved')).toBeNull()
  })
})
