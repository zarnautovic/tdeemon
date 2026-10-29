import { focusManager } from '@tanstack/react-query'
import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { day, fakeServer, me, problem, renderAt, signedIn, today } from '@/test/app'
import { dayServer } from '@/test/day-server'
import { dayWithoutTarget, newUser, summaryOfNothing } from '@/test/new-user'

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

// noteField is the day's note, its field asked for when the day has none.
async function noteField() {
  await screen.findByRole('region', { name: 'Check-in' })
  const add = screen.queryByRole('button', { name: '+ Add a note' })
  if (add) {
    fireEvent.click(add)
  }
  return screen.getByLabelText('Note')
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
  expect(await screen.findByText('Check-in saved')).toBeDefined()
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
    '2 a bit hungry',
    '3 hungry',
    '4 very hungry',
    '5 starving',
  ])
  expect(hunger.getByRole('button', { name: '5 starving' }).getAttribute('aria-pressed')).toBe(
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
  fireEvent.click(hunger.getByRole('button', { name: '3 hungry' }))

  const alert = await within(checkIn()).findByRole('alert')
  expect(alert.textContent).toContain('Not saved. The server cannot be reached.')
  expect(hunger.getByRole('button', { name: '3 hungry' }).getAttribute('aria-pressed')).toBe('true')

  up = true
  fireEvent.click(within(alert).getByRole('button', { name: 'Try again' }))

  expect(await screen.findByText('Check-in saved')).toBeDefined()
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

test('a day without a note has a link for one, whose field takes the focus', async () => {
  fakeServer({ ...signedIn(), ...dayServer().routes })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: '+ Add a note' }))

  await waitFor(() => {
    expect(document.activeElement).toBe(screen.getByLabelText('Note'))
  })
})

test('the note is saved when the field is left, and only when it changed', async () => {
  const server = dayServer(day({ note: 'slept badly' }))
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  const note = await noteField()
  expect(note).toHaveProperty('value', 'slept badly')
  fireEvent.blur(note)
  expect(server.sent).toEqual([])

  fireEvent.change(note, { target: { value: 'slept badly\nlong walk' } })
  fireEvent.blur(note)

  expect(await screen.findByText('Note saved')).toBeDefined()
  expect(server.sent).toEqual([{ method: 'PATCH', path, body: { note: 'slept badly\nlong walk' } }])
})

// typeMenu opens the day type's menu from the chip in the day's header.
async function typeMenu(chip: string) {
  fireEvent.click(await screen.findByRole('button', { name: chip }))
  return within(await screen.findByRole('menu'))
}

test("the day's type is the user's choice, or the plan's", async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  let menu = await typeMenu('Type of day: Rest day')
  expect(
    menu.getByRole('menuitemradio', { name: 'By plan · rest' }).getAttribute('aria-checked'),
  ).toBe('true')
  // The choice already made is not sent again.
  fireEvent.click(menu.getByRole('menuitemradio', { name: 'By plan · rest' }))
  menu = await typeMenu('Type of day: Rest day')
  fireEvent.click(menu.getByRole('menuitemradio', { name: 'Training' }))

  expect(await screen.findByRole('button', { name: 'Type of day: Training day' })).toBeDefined()
  expect(await screen.findByText('Day type saved')).toBeDefined()
  expect(server.sent).toEqual([{ method: 'PATCH', path, body: { training_day: 'training' } }])
})

test('by plan, the day says what the plan decided', async () => {
  // A training weekday of the plan's, or a workout the day had.
  fakeServer({ ...signedIn(), ...dayServer(day({ type: 'training' })).routes })
  renderAt('/')

  const menu = await typeMenu('Type of day: Training day')
  expect(
    menu.getByRole('menuitemradio', { name: 'By plan · training' }).getAttribute('aria-checked'),
  ).toBe('true')
})

test("the plan's decision is shown once the server has the plan, not before", async () => {
  const server = dayServer(day({ type: 'training', training_toggle: true }))
  const patch = server.routes[`PATCH ${path}`]
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

  // Chosen by hand: the plan decides nothing.
  let menu = await typeMenu('Type of day: Training day')
  fireEvent.click(menu.getByRole('menuitemradio', { name: 'By plan' }))
  await waitFor(() => {
    expect(answers).toHaveLength(1)
  })
  // On its way, the day's type is still the one the day has.
  expect(screen.getByRole('button', { name: 'Type of day: Training day' })).toBeDefined()
  answers[0]?.()

  expect(await screen.findByRole('button', { name: 'Type of day: Rest day' })).toBeDefined()
  menu = await typeMenu('Type of day: Rest day')
  expect(
    menu.getByRole('menuitemradio', { name: 'By plan · rest' }).getAttribute('aria-checked'),
  ).toBe('true')
})

test("in Croatian the plan's decision is in Croatian", async () => {
  fakeServer({ ...signedIn('hr'), ...dayServer().routes })
  renderAt('/')

  const menu = await typeMenu('Tip dana: Dan odmora')
  expect(menu.getByRole('menuitemradio', { name: 'Po planu · odmor' })).toBeDefined()
})

test('a day is left out of calibration with a reason, and counted again', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  const counted = await screen.findByRole('switch', { name: 'Counted in calibration' })
  expect(counted.getAttribute('aria-checked')).toBe('true')
  fireEvent.click(counted)
  const leaveOut = screen.getByRole('button', { name: 'Leave out of calibration' })
  // A reason is needed: the server refuses a day left out without one.
  expect(leaveOut.hasAttribute('disabled')).toBe(true)
  fireEvent.click(screen.getByRole('button', { name: 'restaurant' }))
  expect(screen.getByLabelText('Reason')).toHaveProperty('value', 'restaurant')
  fireEvent.change(screen.getByLabelText('Reason'), { target: { value: ' restaurant, wine ' } })
  fireEvent.click(leaveOut)

  expect(await screen.findByText('Out of calibration: restaurant, wine')).toBeDefined()
  expect(server.sent.at(-1)?.body).toEqual({ excluded_reason: 'restaurant, wine' })

  expect(counted.getAttribute('aria-checked')).toBe('false')
  fireEvent.click(counted)

  await waitFor(() => {
    expect(counted.getAttribute('aria-checked')).toBe('true')
  })
  expect(server.sent.at(-1)?.body).toEqual({ excluded_reason: '' })
})

test('a reason a tap gives is stored under its code, and said in the language', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn('hr'), ...server.routes })
  renderAt('/')

  fireEvent.click(await screen.findByRole('switch', { name: 'Ulazi u kalibraciju' }))
  fireEvent.click(screen.getByRole('button', { name: 'bolest' }))
  fireEvent.click(screen.getByRole('button', { name: 'Izostavi iz kalibracije' }))

  expect(await screen.findByText('Izvan kalibracije: bolest')).toBeDefined()
  expect(server.sent.at(-1)?.body).toEqual({ excluded_reason: 'sick' })
})

test.each([
  ['sick', 'sick'],
  // Stored in Croatian before the taps gave codes.
  ['bolest', 'sick'],
  ['nije sve upisano', 'not logged'],
  ['a wedding', 'a wedding'],
])('a day left out as %j says %j in English', async (stored, said) => {
  fakeServer({
    ...signedIn(),
    ...dayServer(day({ status: 'excluded', excluded_reason: stored })).routes,
  })
  renderAt('/')

  expect(await screen.findByText(`Out of calibration: ${said}`)).toBeDefined()
})

test('leaving a day out can be given up', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  fireEvent.click(await screen.findByRole('switch', { name: 'Counted in calibration' }))
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

  expect(
    screen.getByRole('switch', { name: 'Counted in calibration' }).getAttribute('aria-checked'),
  ).toBe('true')
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

  const note = await noteField()
  fireEvent.change(note, { target: { value: ' long walk \n' } })
  fireEvent.blur(note)

  expect(await screen.findByText('Note saved')).toBeDefined()
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

  expect(await screen.findByRole('button', { name: /^Type of day: / })).toBeDefined()
  expect(screen.getByRole('switch', { name: 'Counted in calibration' })).toBeDefined()
  expect(screen.queryByRole('group', { name: 'Mood' })).toBeNull()
  expect(screen.queryByRole('group', { name: 'Hunger' })).toBeNull()
  expect(screen.queryByLabelText('Note')).toBeNull()
  expect(screen.queryByLabelText('Weight')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Add a measurement' })).toBeNull()
})

test('until the app knows which day today is, a day an address names takes no mood', async () => {
  let answer: ((r: Response) => void) | undefined
  fakeServer({
    ...signedIn(),
    ...dayServer(day({ date: '2026-10-21' })).routes,
    'GET /api/v1/me': () =>
      new Promise<Response>((resolve) => {
        answer = resolve
      }),
  })
  renderAt('/?date=2026-10-21')

  // The day waits for it, in its skeleton: nothing of it to change yet.
  expect(await screen.findByRole('button', { name: /^Type of day: / })).toBeDefined()
  expect(screen.getByText('Loading…')).toBeDefined()
  expect(screen.queryByRole('group', { name: 'Mood' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'All measurements' })).toBeNull()

  answer?.(Response.json(me()))

  // A day before today takes it all.
  expect(await screen.findByRole('group', { name: 'Mood' })).toBeDefined()
  fireEvent.click(screen.getByRole('button', { name: 'All measurements' }))
  expect(await screen.findByLabelText('Weight')).toBeDefined()
})

test('what is typed in the note while an earlier note is saved stays', async () => {
  const server = dayServer()
  const patch = server.routes[`PATCH ${path}`]
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

  const note = await noteField()
  fireEvent.change(note, { target: { value: 'long walk' } })
  fireEvent.blur(note)
  await waitFor(() => {
    expect(answers).toHaveLength(1)
  })
  fireEvent.focus(note)
  fireEvent.change(note, { target: { value: 'long walk, then pizza' } })
  answers[0]?.()

  expect(await screen.findByText('Note saved')).toBeDefined()
  expect(screen.getByLabelText('Note')).toHaveProperty('value', 'long walk, then pizza')
  fireEvent.blur(screen.getByLabelText('Note'))
  await waitFor(() => {
    expect(answers).toHaveLength(2)
  })
  answers[1]?.()
  await waitFor(() => {
    expect(server.day().note).toBe('long walk, then pizza')
  })
})

test('a note sent again after it failed leaves the field to follow the day', async () => {
  const server = dayServer()
  let up = false
  // The note as another device left it, once one has.
  const elsewhere: { note?: string } = {}
  const patch = server.routes[`PATCH ${path}`]
  fakeServer({
    ...signedIn(),
    ...server.routes,
    'GET /api/v1/days/today': () => Response.json({ ...server.day(), ...elsewhere }),
    [`PATCH ${path}`]: (request) =>
      up && patch ? patch(request) : Promise.reject(new TypeError('Failed to fetch')),
  })
  renderAt('/')

  const note = await noteField()
  fireEvent.change(note, { target: { value: 'long walk' } })
  fireEvent.blur(note)
  const alert = await within(checkIn()).findByRole('alert')
  fireEvent.change(screen.getByLabelText('Note'), { target: { value: 'long walk, then pizza' } })
  up = true
  fireEvent.click(within(alert).getByRole('button', { name: 'Try again' }))

  // Sent again with what is typed now.
  expect(await screen.findByText('Note saved')).toBeDefined()
  expect(server.day().note).toBe('long walk, then pizza')
  // The note changed elsewhere, the day comes again: the field shows it.
  elsewhere.note = 'from the phone'
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  await waitFor(() => {
    expect(screen.getByLabelText('Note')).toHaveProperty('value', 'from the phone')
  })
})

test("a new user's check-in is a line that asks how the day was, and opens", async () => {
  const server = dayServer(dayWithoutTarget())
  fakeServer({ ...newUser(), ...server.routes })
  renderAt('/')

  const region = within(await screen.findByRole('region', { name: 'Check-in' }))
  const add = await region.findByRole('button', { name: 'Add a check-in' })
  expect(add.textContent).toBe('Add')
  expect(within(checkIn()).getByText('How was today?')).toBeDefined()
  expect(within(checkIn()).queryByRole('group', { name: 'Mood' })).toBeNull()

  fireEvent.click(add)
  // Open, its heading has the focus, and the steps follow it.
  expect(document.activeElement).toBe(within(checkIn()).getByRole('heading', { name: 'Check-in' }))
  fireEvent.click(
    within(within(checkIn()).getByRole('group', { name: 'Mood' })).getByRole('button', {
      name: '4 good',
    }),
  )
  expect(await screen.findByText('Check-in saved')).toBeDefined()
  expect(server.day().mood).toBe(4)
})

test('a user who measured before, or said something of the day, has the whole check-in', async () => {
  fakeServer({
    ...newUser(),
    'GET /api/v1/summary': () =>
      Response.json(summaryOfNothing({ recovery: { flags: [], sleep_h: 7.2 } })),
  })
  renderAt('/')
  expect(await screen.findByRole('group', { name: 'Mood' })).toBeDefined()
  cleanup()

  fakeServer({
    ...newUser(),
    'GET /api/v1/days/today': () => Response.json(dayWithoutTarget({ hunger: 2 })),
  })
  renderAt('/')
  expect(await screen.findByRole('group', { name: 'Hunger' })).toBeDefined()
  expect(screen.queryByText('How was today?')).toBeNull()
})
