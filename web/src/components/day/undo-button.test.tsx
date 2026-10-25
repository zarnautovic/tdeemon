import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { day, fakeServer, problem, renderAt, signedIn, today } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  localStorage.clear()
})

const undone = {
  undone: {
    action: 'meal.delete',
    entity: '0193c3a5-0000-7000-8000-0000000000a1',
    at: '2026-10-23T16:00:00Z',
    client: 'web',
    days: [today],
  },
}

// idOf is the id an undo was sent under.
async function idOf(request: Request): Promise<unknown> {
  const body: unknown = await request.json()
  return typeof body === 'object' && body !== null && 'id' in body ? body.id : undefined
}

test('the latest change is taken back, said, and the day asked again', async () => {
  let taken = false
  const ids: unknown[] = []
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () => Response.json(day(taken ? { note: 'as it was before' } : {})),
    'POST /api/v1/undo': async (request) => {
      ids.push(await idOf(request))
      taken = true
      return Response.json(undone)
    },
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))

  expect(await screen.findByText('Taken back: a meal deleted.')).toBeDefined()
  await waitFor(() => {
    expect(screen.getByLabelText('Note')).toHaveProperty('value', 'as it was before')
  })
  fireEvent.click(screen.getByRole('button', { name: 'Undo the latest change' }))
  await waitFor(() => {
    expect(ids).toHaveLength(2)
  })
  // Each tap is an undo of its own, under a UUIDv7 of its own.
  expect(ids[0]).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-7/)
  expect(ids[1]).not.toBe(ids[0])
})

test('a change of another day is said with its day, a link to it', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/2026-10-20': () => Response.json(day({ date: '2026-10-20' })),
    'POST /api/v1/undo': () =>
      Response.json({ undone: { ...undone.undone, action: 'meal.create' } }),
  })
  const router = renderAt('/?date=2026-10-20')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))

  const said = await screen.findByText(/^Taken back: a meal logged, on another day:/)
  expect(said.textContent).toBe('Taken back: a meal logged, on another day: Friday, October 23')
  // Today's link is / itself.
  fireEvent.click(screen.getByRole('link', { name: 'Friday, October 23' }))
  await waitFor(() => {
    expect(router.state.location.href).toBe('/')
  })
})

test('a meal moved between two other days is said with both', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/2026-10-20': () => Response.json(day({ date: '2026-10-20' })),
    'POST /api/v1/undo': () =>
      Response.json({
        undone: { ...undone.undone, action: 'meal.replace', days: ['2026-10-18', today] },
      }),
  })
  const router = renderAt('/?date=2026-10-20')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))

  const said = await screen.findByText(/^Taken back: a meal changed, on other days:/)
  expect(said.textContent).toBe(
    'Taken back: a meal changed, on other days: Sunday, October 18 · Friday, October 23',
  )
  fireEvent.click(screen.getByRole('link', { name: 'Sunday, October 18' }))
  await waitFor(() => {
    expect(router.state.location.href).toBe('/?date=2026-10-18')
  })
})

test('a change of a day of another year says its year', async () => {
  fakeServer({
    ...signedIn(),
    'POST /api/v1/undo': () =>
      Response.json({ undone: { ...undone.undone, action: 'metric.set', days: ['2025-12-31'] } }),
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))

  expect(await screen.findByRole('link', { name: 'Wednesday, December 31, 2025' })).toBeDefined()
})

test('a change of the day shown, or of no day, is said without a day', async () => {
  // A meal moved from the day shown, then a food.
  const answers = [
    { action: 'meal.replace', days: ['2026-10-18', '2026-10-20'] },
    { action: 'food.save', days: [] },
  ]
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/2026-10-20': () => Response.json(day({ date: '2026-10-20' })),
    'POST /api/v1/undo': () => Response.json({ undone: { ...undone.undone, ...answers.shift() } }),
  })
  renderAt('/?date=2026-10-20')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))
  expect(await screen.findByText('Taken back: a meal changed.')).toBeDefined()
  fireEvent.click(screen.getByRole('button', { name: 'Undo the latest change' }))
  expect(await screen.findByText('Taken back: a food saved.')).toBeDefined()
  expect(screen.queryByRole('link', { name: /October 18/ })).toBeNull()
})

test('an undo that got no answer is sent again under the same id', async () => {
  let up = false
  const ids: unknown[] = []
  fakeServer({
    ...signedIn(),
    'POST /api/v1/undo': async (request) => {
      ids.push(await idOf(request))
      return up ? Response.json(undone) : Promise.reject(new TypeError('Failed to fetch'))
    },
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))
  expect((await screen.findByRole('alert')).textContent).toContain(
    'Not saved. The server cannot be reached.',
  )
  up = true
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

  expect(await screen.findByText('Taken back: a meal deleted.')).toBeDefined()
  expect(ids[1]).toBe(ids[0])
})

test('an undo says when there is nothing to take back, and when it clashes', async () => {
  let status = 404
  fakeServer({ ...signedIn(), 'POST /api/v1/undo': () => problem(status) })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))
  expect(await screen.findByText('There is no change to take back.')).toBeDefined()

  status = 409
  fireEvent.click(screen.getByRole('button', { name: 'Undo the latest change' }))
  expect((await screen.findByRole('alert')).textContent).toBe(
    'The latest change cannot be taken back: what came since clashes with it. The next undo takes back the one before.',
  )
})

test('a change of a kind the app does not know is still said', async () => {
  fakeServer({
    ...signedIn(),
    'POST /api/v1/undo': () =>
      Response.json({ undone: { ...undone.undone, action: 'something.new' } }),
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))

  expect(await screen.findByText('Taken back: a change.')).toBeDefined()
})

test('an undo whose id the server refuses, of a clock far from its own, goes without one', async () => {
  const bodies: unknown[] = []
  fakeServer({
    ...signedIn(),
    'POST /api/v1/undo': async (request) => {
      const body: unknown = await request.json()
      bodies.push(body)
      return typeof body === 'object' && body !== null && 'id' in body
        ? problem(422)
        : Response.json(undone)
    },
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))

  expect(await screen.findByText('Taken back: a meal deleted.')).toBeDefined()
  expect(bodies).toHaveLength(2)
  expect(bodies[1]).toEqual({})
})

test('an undo without an id that got no answer is not offered again', async () => {
  const bodies: unknown[] = []
  fakeServer({
    ...signedIn(),
    'POST /api/v1/undo': async (request) => {
      const body: unknown = await request.json()
      bodies.push(body)
      return typeof body === 'object' && body !== null && 'id' in body
        ? problem(422)
        : Promise.reject(new TypeError('Failed to fetch'))
    },
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))

  expect((await screen.findByRole('alert')).textContent).toBe(
    'The undo got no answer. Look at the day before you undo again.',
  )
  expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()
  expect(bodies).toHaveLength(2)
})

test('after an undo that got no answer, the day is asked again', async () => {
  let gets = 0
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () => {
      gets++
      return Response.json(day())
    },
    'POST /api/v1/undo': () => Promise.reject(new TypeError('Failed to fetch')),
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Undo the latest change' }))
  await screen.findByRole('alert')

  await waitFor(() => {
    expect(gets).toBe(2)
  })
})
