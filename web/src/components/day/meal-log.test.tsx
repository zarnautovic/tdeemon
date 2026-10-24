import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Day, Food } from '@/api/types'
import { day, fakeServer, problem, renderAt, signedIn, type Handler } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  localStorage.clear()
})

const bread: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000f1',
  name: 'Rye bread',
  basis: '100g',
  source: 'label',
  archived: false,
  nutrients: { kcal: 259, protein: 8.5, carbs: 48, fat: 3.3 },
  servings: [{ name: 'slice', amount: 40 }],
  last_use: {
    at: '2026-10-21T07:00:00Z',
    meal_id: '0193c3a5-0000-7000-8000-0000000000a9',
    quantity: { serving: 'slice', servings: 2 },
    amount: 80,
    unit: 'g',
  },
}

// logServer is a day that takes a meal: it answers as the API does and
// keeps the bodies it was sent.
function logServer(date = '2026-10-23', over: Record<string, Handler> = {}) {
  let current: Day = day({ date })
  const sent: unknown[] = []
  const routes: Record<string, Handler> = {
    ...signedIn(),
    'GET /api/v1/days/today': () => Response.json(current),
    [`GET /api/v1/days/${date}`]: () => Response.json(current),
    'GET /api/v1/foods': (request) =>
      Response.json({
        foods: new URL(request.url).searchParams.get('query') === 'rye' ? [bread] : [],
        unfound: [],
      }),
    'POST /api/v1/meals': async (request) => {
      const body: unknown = await request.json()
      sent.push(body)
      current = {
        ...current,
        meals: [
          {
            id: '0193c3a5-0000-7000-8000-0000000000a1',
            eaten_at: '2026-10-23T15:30:00Z',
            description: 'Breakfast',
            client: 'web',
            total: { kcal: 207, protein: 7, carbs: 38, fat: 3 },
            items: [],
          },
        ],
      }
      const { meals: _meals, metrics: _metrics, ...summary } = current
      return Response.json({ meal: current.meals[0], day: summary, warnings: [] }, { status: 201 })
    },
    ...over,
  }
  fakeServer(routes)
  return { sent }
}

function composer() {
  return within(screen.getByRole('form', { name: 'A new meal' }))
}

test("a meal is logged of a food of the user's and an estimate, at a time of the day", async () => {
  const server = logServer()
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.change(composer().getByLabelText('What it was'), { target: { value: 'Breakfast' } })
  fireEvent.change(composer().getByLabelText('Time'), { target: { value: '08:30' } })
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await composer().findByRole('button', { name: /Rye bread/ }))
  // As it was eaten last: two slices.
  expect(composer().getByLabelText('Amount: Rye bread')).toHaveProperty('value', '2')
  expect(composer().getByLabelText('Unit: Rye bread')).toHaveProperty('value', 'serving:slice')
  fireEvent.change(composer().getByLabelText('Unit: Rye bread'), { target: { value: 'g' } })
  fireEvent.change(composer().getByLabelText('Amount: Rye bread'), { target: { value: '60' } })
  fireEvent.click(composer().getByRole('button', { name: 'Add an estimate' }))
  fireEvent.change(composer().getByLabelText('What it is'), { target: { value: 'Coffee' } })
  for (const [label, value] of [
    ['Energy (kcal)', '50'],
    ['Protein (g)', '2'],
    ['Carbs (g)', '6'],
    ['Fat (g)', '2.5'],
  ] as const) {
    fireEvent.change(composer().getByLabelText(label), { target: { value } })
  }
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))

  expect(await screen.findByText('The meal is logged.')).toBeDefined()
  expect(await screen.findByText('Breakfast')).toBeDefined()
  expect(screen.queryByRole('form', { name: 'A new meal' })).toBeNull()
  expect(server.sent).toEqual([
    {
      id: expect.stringMatching(/^[0-9a-f]{8}-[0-9a-f]{4}-7/) as unknown,
      description: 'Breakfast',
      // 08:30 in Los Angeles, the user's time zone.
      eaten_at: '2026-10-23T15:30:00.000Z',
      items: [
        { id: expect.any(String) as unknown, food_id: bread.id, amount: 60, unit: 'g' },
        {
          id: expect.any(String) as unknown,
          name: 'Coffee',
          nutrients: { kcal: 50, protein: 2, carbs: 6, fat: 2.5 },
        },
      ],
    },
  ])
})

test("another day's meal is at noon, and one past midnight is of the night after the day", async () => {
  const server = logServer('2026-10-21')
  renderAt('/?date=2026-10-21')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  expect(composer().getByLabelText('Time')).toHaveProperty('value', '12:00')
  fireEvent.click(composer().getByRole('button', { name: 'Add an estimate' }))
  fireEvent.change(composer().getByLabelText('What it is'), { target: { value: 'Snack' } })
  for (const label of ['Energy (kcal)', 'Protein (g)', 'Carbs (g)', 'Fat (g)']) {
    fireEvent.change(composer().getByLabelText(label), { target: { value: '1' } })
  }
  // The day starts at 04:00: 01:30 is the night after the 21st.
  fireEvent.change(composer().getByLabelText('Time'), { target: { value: '01:30' } })
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))

  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  expect(server.sent[0]).toMatchObject({ eaten_at: '2026-10-22T08:30:00.000Z' })
})

test('a meal without items, or with an amount that is no number, is not sent', async () => {
  const server = logServer()
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))
  expect(composer().getByRole('alert').textContent).toBe('Add a food or an estimate first.')

  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await composer().findByRole('button', { name: /Rye bread/ }))
  fireEvent.change(composer().getByLabelText('Amount: Rye bread'), { target: { value: 'a few' } })
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))

  expect(composer().getByRole('alert').textContent).toBe(
    'An amount is more than 0, and an estimate has a name and its values, such as 72.5.',
  )
  expect(composer().getByLabelText('Amount: Rye bread').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])
})

test('a search that finds nothing says to give an estimate, and an item is removed', async () => {
  logServer()
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'kale' } })
  expect(
    await composer().findByText('No food of yours matches: give it as an estimate.'),
  ).toBeDefined()
  fireEvent.click(composer().getByRole('button', { name: 'Add an estimate' }))
  fireEvent.click(composer().getByRole('button', { name: 'Remove: an estimate' }))
  expect(composer().queryByLabelText('What it is')).toBeNull()

  fireEvent.click(composer().getByRole('button', { name: 'Cancel' }))
  expect(screen.queryByRole('form', { name: 'A new meal' })).toBeNull()
})

test('a meal that got no answer is sent again under its id', async () => {
  let up = false
  const bodies: unknown[] = []
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods': () => Response.json({ foods: [bread], unfound: [] }),
    'POST /api/v1/meals': async (request) => {
      bodies.push(await request.json())
      return up ? problem(503) : Promise.reject(new TypeError('Failed to fetch'))
    },
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await composer().findByRole('button', { name: /Rye bread/ }))
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))
  up = true
  fireEvent.click(await composer().findByRole('button', { name: 'Try again' }))

  await waitFor(() => {
    expect(bodies).toHaveLength(2)
  })
  expect(bodies[1]).toEqual(bodies[0])
})

test("a meal the server refuses says why, in the server's words", async () => {
  logServer('2026-10-23', { 'POST /api/v1/meals': () => problem(422) })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await composer().findByRole('button', { name: /Rye bread/ }))
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))

  expect((await composer().findByRole('alert')).textContent).toBe(
    'The server did not take this meal: what went wrong',
  )
})

test("the server's warnings of a logged meal are said", async () => {
  logServer('2026-10-23', {
    'POST /api/v1/meals': () =>
      Response.json(
        {
          meal: {
            id: 'x',
            eaten_at: '2026-10-23T15:30:00Z',
            description: '',
            client: 'web',
            total: { kcal: 0, protein: 0, carbs: 0, fat: 0 },
            items: [],
          },
          day: {
            date: '2026-10-23',
            status: 'open',
            type: 'rest',
            total: { kcal: 0, protein: 0, carbs: 0, fat: 0 },
          },
          warnings: [{ code: 'duplicate_meal', message: 'the same meal at 08:25' }],
        },
        { status: 201 },
      ),
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await composer().findByRole('button', { name: /Rye bread/ }))
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))

  expect(
    await screen.findByText(
      'A meal with the same items was logged just before: if this one was sent twice, delete it.',
    ),
  ).toBeDefined()
})

test('a day to come has no meal to log', async () => {
  logServer('2026-10-25')
  renderAt('/?date=2026-10-25')

  expect(await screen.findByRole('group', { name: 'Type of day' })).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Log a meal' })).toBeNull()
})

test('Enter in the search picks the first food, and sends nothing', async () => {
  const server = logServer()
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  const search = composer().getByLabelText('Find a food of yours')
  fireEvent.change(search, { target: { value: 'rye' } })
  await composer().findByRole('button', { name: /Rye bread/ })
  fireEvent.keyDown(search, { key: 'Enter' })

  expect(composer().getByLabelText('Amount: Rye bread')).toBeDefined()
  expect(server.sent).toEqual([])
})

test("today's meal is logged now when its time is left as it was", async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  // 08:20 in Los Angeles.
  vi.setSystemTime(new Date('2026-10-23T15:20:00Z'))
  try {
    const server = logServer()
    renderAt('/')

    fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
    expect(composer().getByLabelText('Time')).toHaveProperty('value', '08:20')
    fireEvent.click(composer().getByRole('button', { name: 'Add an estimate' }))
    fireEvent.change(composer().getByLabelText('What it is'), { target: { value: 'Tea' } })
    for (const label of ['Energy (kcal)', 'Protein (g)', 'Carbs (g)', 'Fat (g)']) {
      fireEvent.change(composer().getByLabelText(label), { target: { value: '1' } })
    }
    // Some minutes later.
    vi.setSystemTime(new Date('2026-10-23T15:27:00Z'))
    fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))

    await waitFor(() => {
      expect(server.sent).toHaveLength(1)
    })
    expect(server.sent[0]).toMatchObject({ eaten_at: '2026-10-23T15:27:00.000Z' })
  } finally {
    vi.useRealTimers()
  }
})

test('a meal sent without an answer holds still, and goes again as it was', async () => {
  let up = false
  const bodies: unknown[] = []
  let gets = 0
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () => {
      gets++
      return Response.json(day())
    },
    'GET /api/v1/foods': () => Response.json({ foods: [bread], unfound: [] }),
    'POST /api/v1/meals': async (request) => {
      bodies.push(await request.json())
      return up ? problem(503) : Promise.reject(new TypeError('Failed to fetch'))
    },
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await composer().findByRole('button', { name: /Rye bread/ }))
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))
  await composer().findByRole('button', { name: 'Try again' })

  // It may have been logged: the day is asked again, and the form holds.
  await waitFor(() => {
    expect(gets).toBe(2)
  })
  expect(composer().getByLabelText('Amount: Rye bread').matches(':disabled')).toBe(true)
  expect(composer().getByRole('button', { name: 'Log the meal' }).hasAttribute('disabled')).toBe(
    true,
  )
  up = true
  fireEvent.click(composer().getByRole('button', { name: 'Try again' }))

  await waitFor(() => {
    expect(bodies).toHaveLength(2)
  })
  expect(bodies[1]).toEqual(bodies[0])
})

test('the focus goes into the form, to a food picked, and back to the button', async () => {
  logServer()
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  await waitFor(() => {
    expect(document.activeElement).toBe(composer().getByLabelText('What it was'))
  })
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await composer().findByRole('button', { name: /Rye bread/ }))
  await waitFor(() => {
    expect(document.activeElement).toBe(composer().getByLabelText('Amount: Rye bread'))
  })
  fireEvent.click(composer().getByRole('button', { name: 'Cancel' }))
  await waitFor(() => {
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Log a meal' }))
  })
})

test("a food's brand tells it apart in the meal", async () => {
  logServer('2026-10-23', {
    'GET /api/v1/foods': () => Response.json({ foods: [{ ...bread, brand: 'Mill' }], unfound: [] }),
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await composer().findByRole('button', { name: /Rye bread/ }))

  expect(composer().getByLabelText('Amount: Rye bread · Mill')).toBeDefined()
})

test('what is wrong with an entry is no longer said once it is removed', async () => {
  logServer()
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.click(composer().getByRole('button', { name: 'Add an estimate' }))
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))
  expect(composer().getByRole('alert').textContent).toContain('an estimate has a name')

  fireEvent.click(composer().getByRole('button', { name: 'Remove: an estimate' }))

  expect(composer().queryByText(/an estimate has a name/)).toBeNull()
})

test('a meal being sent cannot be given up', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods': () => Response.json({ foods: [bread], unfound: [] }),
    'POST /api/v1/meals': () => new Promise<Response>(() => undefined),
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await composer().findByRole('button', { name: /Rye bread/ }))
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))

  await waitFor(() => {
    expect(composer().getByRole('button', { name: 'Cancel' }).hasAttribute('disabled')).toBe(true)
  })
})

test('the word for portions agrees with the amount typed', async () => {
  const stew: Food = {
    id: '0193c3a5-0000-7000-8000-0000000000f2',
    name: 'Stew',
    basis: 'portion',
    source: 'dish',
    archived: false,
    nutrients: { kcal: 450, protein: 30, carbs: 40, fat: 15 },
  }
  logServer('2026-10-23', {
    'GET /api/v1/foods': () => Response.json({ foods: [stew], unfound: [] }),
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'stew' } })
  fireEvent.click(await composer().findByRole('button', { name: /Stew/ }))
  const unit = () => composer().getByRole('option', { name: /^portion/ }).textContent
  expect(composer().getByLabelText('Amount: Stew')).toHaveProperty('value', '1')
  expect(unit()).toBe('portion')
  fireEvent.change(composer().getByLabelText('Amount: Stew'), { target: { value: '2' } })
  expect(unit()).toBe('portions')
})

test('a unit chosen in a new meal works the amount out anew, both ways', async () => {
  const nuts: Food = {
    id: '0193c3a5-0000-7000-8000-0000000000f3',
    name: 'Walnuts',
    basis: '100g',
    source: 'label',
    archived: false,
    nutrients: { kcal: 654, protein: 15, carbs: 14, fat: 65 },
    servings: [{ name: 'handful', amount: 30 }],
  }
  const server = logServer('2026-10-23', {
    'GET /api/v1/foods': () => Response.json({ foods: [nuts], unfound: [] }),
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  fireEvent.change(composer().getByLabelText('Find a food of yours'), { target: { value: 'wal' } })
  fireEvent.click(await composer().findByRole('button', { name: /Walnuts/ }))
  const amount = () => composer().getByLabelText('Amount: Walnuts')
  const unit = composer().getByLabelText('Unit: Walnuts')
  expect(amount()).toHaveProperty('value', '100')
  fireEvent.change(unit, { target: { value: 'serving:handful' } })
  expect(amount()).toHaveProperty('value', '3.33')
  fireEvent.change(unit, { target: { value: 'g' } })
  expect(amount()).toHaveProperty('value', '100')
  fireEvent.change(unit, { target: { value: 'serving:handful' } })
  fireEvent.click(composer().getByRole('button', { name: 'Log the meal' }))

  expect(await screen.findByText('The meal is logged.')).toBeDefined()
  expect(server.sent).toEqual([
    {
      id: expect.any(String) as unknown,
      eaten_at: expect.any(String) as unknown,
      items: [
        {
          id: expect.any(String) as unknown,
          food_id: nuts.id,
          serving: 'handful',
          // As worked out, not as shown.
          servings: 100 / 30,
        },
      ],
    },
  ])
})
