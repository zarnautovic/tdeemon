import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { day, fakeServer, problem, renderAt, signedIn, today } from '@/test/app'
import { dayServer } from '@/test/day-server'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

const path = `/api/v1/days/${today}`

function checkIn() {
  return screen.getByRole('region', { name: 'Check-in' })
}

test('a step of the mood is saved in a tap, and a tap on it again clears it', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  const mood = within(await screen.findByRole('group', { name: 'Mood' }))
  fireEvent.click(mood.getByRole('button', { name: '4 good' }))

  await waitFor(() => {
    expect(mood.getByRole('button', { name: '4 good' }).getAttribute('aria-pressed')).toBe('true')
  })
  expect(await within(checkIn()).findByText('Saved')).toBeDefined()
  expect(server.sent).toEqual([{ method: 'PATCH', path, body: { mood: 4 } }])
  expect(server.day().mood).toBe(4)

  fireEvent.click(mood.getByRole('button', { name: '4 good' }))

  await waitFor(() => {
    expect(server.day().mood).toBeUndefined()
  })
  expect(server.sent.at(-1)?.body).toEqual({ mood: 0 })
  await waitFor(() => {
    expect(mood.getByRole('button', { name: '4 good' }).getAttribute('aria-pressed')).toBe('false')
  })
})

test('the hunger has its words, and the day the server keeps shows', async () => {
  const server = dayServer(day({ mood: 2, hunger: 5 }))
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  const hunger = within(await screen.findByRole('group', { name: 'Hunger' }))
  expect(hunger.getAllByRole('button').map((b) => b.textContent)).toEqual([
    '1 full',
    '2 calm',
    '3 moderate',
    '4 hungry',
    '5 very hungry',
  ])
  expect(hunger.getByRole('button', { name: '5 very hungry' }).getAttribute('aria-pressed')).toBe(
    'true',
  )
  const mood = within(screen.getByRole('group', { name: 'Mood' }))
  expect(mood.getByRole('button', { name: '2 low' }).getAttribute('aria-pressed')).toBe('true')
})

test('a pick that was not saved stays, says why, and is sent again', async () => {
  const server = dayServer()
  let up = false
  fakeServer({
    ...signedIn(),
    ...server.routes,
    [`PATCH ${path}`]: (request) =>
      up
        ? (server.routes[`PATCH ${path}`]?.(request) ?? Response.error())
        : Promise.reject(new TypeError('Failed to fetch')),
  })
  renderAt('/')

  const hunger = within(await screen.findByRole('group', { name: 'Hunger' }))
  fireEvent.click(hunger.getByRole('button', { name: '3 moderate' }))

  const alert = await within(checkIn()).findByRole('alert')
  expect(alert.textContent).toContain('Not saved. The server cannot be reached.')
  expect(hunger.getByRole('button', { name: '3 moderate' }).getAttribute('aria-pressed')).toBe(
    'true',
  )

  up = true
  fireEvent.click(within(alert).getByRole('button', { name: 'Try again' }))

  expect(await within(checkIn()).findByText('Saved')).toBeDefined()
  expect(server.day().hunger).toBe(3)
})

test('the changes of a day go one at a time, each after the one before answered', async () => {
  const server = dayServer()
  const patch = server.routes[`PATCH ${path}`]
  // Each change waits here until the test lets it through.
  const answers: (() => void)[] = []
  fakeServer({
    ...signedIn(),
    ...server.routes,
    [`PATCH ${path}`]: async (request) => {
      await new Promise<void>((resolve) => {
        answers.push(resolve)
      })
      return patch ? patch(request) : Response.error()
    },
  })
  renderAt('/')

  fireEvent.click(
    within(await screen.findByRole('group', { name: 'Mood' })).getByRole('button', {
      name: '5 great',
    }),
  )
  fireEvent.click(
    within(screen.getByRole('group', { name: 'Hunger' })).getByRole('button', { name: '1 full' }),
  )

  await waitFor(() => {
    expect(answers).toHaveLength(1)
  })
  answers[0]?.()
  await waitFor(() => {
    expect(answers).toHaveLength(2)
  })
  answers[1]?.()

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([{ mood: 5 }, { hunger: 1 }])
  })
  // The page shows the day the last change answered with: both.
  const mood = within(screen.getByRole('group', { name: 'Mood' }))
  const hunger = within(screen.getByRole('group', { name: 'Hunger' }))
  await waitFor(() => {
    expect(mood.getByRole('button', { name: '5 great' }).getAttribute('aria-pressed')).toBe('true')
    expect(hunger.getByRole('button', { name: '1 full' }).getAttribute('aria-pressed')).toBe('true')
  })
})

test('the note is saved when the field is left, and only when it changed', async () => {
  const server = dayServer(day({ note: 'slept badly' }))
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  const note = await screen.findByLabelText('Note')
  expect(note).toHaveProperty('value', 'slept badly')
  fireEvent.blur(note)
  expect(server.sent).toEqual([])

  fireEvent.change(note, { target: { value: 'slept badly\nlong walk' } })
  fireEvent.blur(note)

  expect(await within(checkIn()).findByText('Saved')).toBeDefined()
  expect(server.sent).toEqual([{ method: 'PATCH', path, body: { note: 'slept badly\nlong walk' } }])
})

test("the day's type is the user's choice, or the plan's", async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  const type = within(await screen.findByRole('group', { name: 'Type of day' }))
  expect(type.getByRole('button', { name: 'By plan' }).getAttribute('aria-pressed')).toBe('true')
  fireEvent.click(type.getByRole('button', { name: 'By plan' }))
  fireEvent.click(type.getByRole('button', { name: 'Training' }))

  expect(await screen.findByText('Friday, October 23 · Training day')).toBeDefined()
  expect(type.getByRole('button', { name: 'Training' }).getAttribute('aria-pressed')).toBe('true')
  // The choice already made is not sent again.
  expect(server.sent).toEqual([{ method: 'PATCH', path, body: { training_day: 'training' } }])
})

test('a day is left out of calibration with a reason, and counted again', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  expect(await screen.findByText('Counted in calibration.')).toBeDefined()
  fireEvent.click(screen.getByRole('button', { name: 'Leave out of calibration' }))
  const leaveOut = screen.getByRole('button', { name: 'Leave out of calibration' })
  // A reason is needed: the server refuses a day left out without one.
  expect(leaveOut.hasAttribute('disabled')).toBe(true)
  fireEvent.click(screen.getByRole('button', { name: 'restaurant' }))
  expect(screen.getByLabelText('Reason')).toHaveProperty('value', 'restaurant')
  fireEvent.change(screen.getByLabelText('Reason'), { target: { value: ' restaurant, wine ' } })
  fireEvent.click(leaveOut)

  expect(await screen.findByText('Out of calibration: restaurant, wine')).toBeDefined()
  expect(server.sent.at(-1)?.body).toEqual({ excluded_reason: 'restaurant, wine' })

  fireEvent.click(screen.getByRole('button', { name: 'Count it again' }))

  expect(await screen.findByText('Counted in calibration.')).toBeDefined()
  expect(server.sent.at(-1)?.body).toEqual({ excluded_reason: '' })
})

test('leaving a day out can be given up', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Leave out of calibration' }))
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

  expect(screen.getByText('Counted in calibration.')).toBeDefined()
  expect(server.sent).toEqual([])
})

test('a change of another day goes to that day', async () => {
  const server = dayServer(day({ date: '2026-10-21' }))
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/?date=2026-10-21')

  fireEvent.click(
    within(await screen.findByRole('group', { name: 'Mood' })).getByRole('button', {
      name: '3 okay',
    }),
  )

  await waitFor(() => {
    expect(server.sent).toEqual([
      { method: 'PATCH', path: '/api/v1/days/2026-10-21', body: { mood: 3 } },
    ])
  })
})

test('the check-in speaks Croatian', async () => {
  fakeServer({ ...signedIn('hr'), ...dayServer().routes })
  renderAt('/')

  const mood = within(await screen.findByRole('group', { name: 'Osjećaj' }))
  expect(mood.getAllByRole('button').map((b) => b.textContent)).toEqual([
    '1 loše',
    '2 slabije',
    '3 solidno',
    '4 dobro',
    '5 odlično',
  ])
  const hunger = within(screen.getByRole('group', { name: 'Glad' }))
  expect(hunger.getByRole('button', { name: '5 stalno gladan' })).toBeDefined()
})

test('the note is saved without the spaces at its ends, which the server refuses', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  const note = await screen.findByLabelText('Note')
  fireEvent.change(note, { target: { value: ' long walk \n' } })
  fireEvent.blur(note)

  expect(await within(checkIn()).findByText('Saved')).toBeDefined()
  expect(server.sent.map((s) => s.body)).toEqual([{ note: 'long walk' }])
  // Only spaces are no change of an empty note.
  fireEvent.change(screen.getByLabelText('Note'), { target: { value: 'long walk  ' } })
  fireEvent.blur(screen.getByLabelText('Note'))
  expect(server.sent).toHaveLength(1)
})

test('a check-in the server refuses says so, with nothing to try again', async () => {
  fakeServer({ ...signedIn(), ...dayServer().routes, [`PATCH ${path}`]: () => problem(422) })
  renderAt('/')

  fireEvent.click(
    within(await screen.findByRole('group', { name: 'Mood' })).getByRole('button', {
      name: '4 good',
    }),
  )

  expect((await within(checkIn()).findByRole('alert')).textContent).toBe(
    'Not saved. The server did not take it.',
  )
  expect(within(checkIn()).queryByRole('button', { name: 'Try again' })).toBeNull()
})

test('a day to come takes its type and exclusion only', async () => {
  fakeServer({ ...signedIn(), ...dayServer(day({ date: '2026-10-25' })).routes })
  renderAt('/?date=2026-10-25')

  expect(await screen.findByRole('group', { name: 'Type of day' })).toBeDefined()
  expect(screen.getByRole('button', { name: 'Leave out of calibration' })).toBeDefined()
  expect(screen.queryByRole('group', { name: 'Mood' })).toBeNull()
  expect(screen.queryByRole('group', { name: 'Hunger' })).toBeNull()
  expect(screen.queryByLabelText('Note')).toBeNull()
  expect(screen.queryByLabelText('Weight')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Add a measurement' })).toBeNull()
})
