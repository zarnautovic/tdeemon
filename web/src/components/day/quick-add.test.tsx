import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
  type BoundFunctions,
  type queries,
} from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Day, Food, Meal } from '@/api/types'
import { day, fakeServer, problem, renderAt, signedIn, type Handler } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.useRealTimers()
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
    day: '2026-10-21',
    meal_id: '0193c3a5-0000-7000-8000-0000000000a9',
    quantity: { serving: 'slice', servings: 2 },
    amount: 80,
    unit: 'g',
  },
}

const oats: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000f4',
  name: 'Rolled oats',
  basis: '100g',
  source: 'label',
  archived: false,
  nutrients: { kcal: 370, protein: 13, carbs: 60, fat: 7 },
}

const skyr: Food = {
  ...oats,
  id: '0193c3a5-0000-7000-8000-0000000000f5',
  name: 'Skyr',
  nutrients: { kcal: 63, protein: 11, carbs: 4, fat: 0.2 },
  last_use: {
    at: '2026-10-22T07:00:00Z',
    day: '2026-10-22',
    meal_id: '0193c3a5-0000-7000-8000-0000000000a8',
    quantity: { amount: 150, unit: 'g' },
    amount: 150,
    unit: 'g',
  },
}

// porridge is a meal of the day before.
const porridge: Meal = {
  id: '0193c3a5-0000-7000-8000-0000000000b1',
  eaten_at: '2026-10-22T15:00:00Z',
  description: 'Porridge',
  client: 'web',
  total: { kcal: 207, protein: 6.8, carbs: 38.4, fat: 2.6 },
  items: [
    {
      id: '0193c3a5-0000-7000-8000-0000000000c1',
      food_id: bread.id,
      name: 'Rye bread',
      quantity: { servings: 2, serving: 'slice' },
      amount: 80,
      unit: 'g',
      nutrients: { kcal: 207, protein: 6.8, carbs: 38.4, fat: 2.6 },
      source: 'label',
    },
  ],
}

// logServer is a day that takes a meal: it answers as the API does and
// keeps the bodies it was sent. Its user's foods are found by "rye",
// listed by their use (bread, and oats never eaten) and by how often
// (skyr, then bread); the porridge is the meal of before.
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
    'GET /api/v1/foods/list': (request) =>
      Response.json({
        foods:
          new URL(request.url).searchParams.get('order') === 'frequent'
            ? [skyr, bread]
            : [bread, oats],
        total: 2,
      }),
    'GET /api/v1/meals/recent': () => Response.json({ meals: [porridge] }),
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

// Sheet is what the queries find in the sheet.
type Sheet = BoundFunctions<typeof queries>

// logMeal names the button that logs a meal: today's names the meal of
// the hour, a day before's does not.
const logMeal =
  /^(Log a meal|Upiši obrok|Add (breakfast|lunch|a snack|dinner)|Dodaj (doručak|ručak|užinu|večeru))/

// openSheet opens the meal to log, and gives what is in it.
async function openSheet(name = 'A new meal') {
  fireEvent.click(await screen.findByRole('button', { name: logMeal }))
  return within(await screen.findByRole('dialog', { name }))
}

// basket is what the meal holds, each item as it says it.
function basket(sheet: Sheet) {
  return within(sheet.getByRole('list', { name: 'In the meal' }))
    .getAllByRole('listitem')
    .map((li) => li.textContent.replace('×', ''))
}

async function findRye(sheet: Sheet, search = 'Find a food or a meal') {
  fireEvent.change(sheet.getByLabelText(search), { target: { value: 'rye' } })
  fireEvent.click(await sheet.findByRole('button', { name: /^Rye bread/ }))
}

async function giveEstimate(sheet: Sheet, name: string, kcal = '1') {
  fireEvent.click(sheet.getByRole('button', { name: 'An estimate' }))
  fireEvent.change(sheet.getByLabelText('What it is'), { target: { value: name } })
  for (const [label, value] of [
    ['Energy (kcal)', kcal],
    ['Protein (g)', '1'],
    ['Carbs (g)', '1'],
    ['Fat (g)', '1'],
  ] as const) {
    fireEvent.change(sheet.getByLabelText(label), { target: { value } })
  }
  fireEvent.click(sheet.getByRole('button', { name: `Add: ${name}` }))
  await waitFor(() => {
    expect(basket(sheet).some((item) => item.startsWith(name))).toBe(true)
  })
}

test("a meal is logged of a food of the user's and an estimate, each with what it brings", async () => {
  const server = logServer()
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(sheet.getByRole('button', { name: 'Change the name and time' }))
  fireEvent.change(sheet.getByLabelText('What it was'), { target: { value: 'Breakfast' } })
  fireEvent.change(sheet.getByLabelText('Time'), { target: { value: '08:30' } })
  expect(sheet.getByText('Breakfast · today at 8:30 AM')).toBeDefined()
  await findRye(sheet)
  // As it was eaten last: two slices.
  expect(sheet.getByLabelText('Amount: Rye bread')).toHaveProperty('value', '2')
  expect(sheet.getByLabelText('Unit: Rye bread')).toHaveProperty('value', 'serving:slice')
  expect(sheet.getByText(/^= 207 kcal · P 6\.8 · C 38 · F 2\.6$/)).toBeDefined()
  fireEvent.change(sheet.getByLabelText('Unit: Rye bread'), { target: { value: 'g' } })
  expect(sheet.getByLabelText('Amount: Rye bread')).toHaveProperty('value', '80')
  fireEvent.change(sheet.getByLabelText('Amount: Rye bread'), { target: { value: '60' } })
  expect(sheet.getByText(/^= 155 kcal/)).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Rye bread' }))
  await giveEstimate(sheet, 'Coffee', '50')

  expect(basket(sheet)).toEqual(['Rye bread · 60 g · 155 kcal', 'Coffee · 50 kcal'])
  expect(sheet.getByText('after: 2,295 kcal left')).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'Log 2 items · 205 kcal' }))

  expect(await screen.findByText(/^Logged: Breakfast · 207 kcal/)).toBeDefined()
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'A new meal' })).toBeNull()
  })
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
          nutrients: { kcal: 50, protein: 1, carbs: 1, fat: 1 },
        },
      ],
    },
  ])
})

test('a meal of before is logged again in two taps', async () => {
  const server = logServer()
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(await sheet.findByRole('button', { name: 'Repeat: Porridge' }))

  expect(await screen.findByText(/^Logged: Breakfast · 207 kcal/)).toBeDefined()
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'A new meal' })).toBeNull()
  })
  // Today's, now: the server takes the time from its id.
  expect(server.sent).toEqual([
    {
      id: expect.stringMatching(/^[0-9a-f]{8}-[0-9a-f]{4}-7/) as unknown,
      description: 'Porridge',
      items: [{ food_id: bread.id, serving: 'slice', servings: 2 }],
    },
  ])
})

test("another day's meal is at noon, and one past midnight is of the night after the day", async () => {
  const server = logServer('2026-10-21')
  renderAt('/?date=2026-10-21')

  const sheet = await openSheet()
  expect(sheet.getByText('Lunch · Oct 21 at 12:00 PM')).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'Change the name and time' }))
  expect(sheet.getByLabelText('Time')).toHaveProperty('value', '12:00')
  await giveEstimate(sheet, 'Snack')
  // The day starts at 04:00: 01:30 is the night after the 21st.
  fireEvent.change(sheet.getByLabelText('Time'), { target: { value: '01:30' } })
  expect(sheet.getByText('01:30 is the night after this day.')).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 1 kcal' }))

  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  expect(server.sent[0]).toMatchObject({ eaten_at: '2026-10-22T08:30:00.000Z' })
})

test('a meal without items is not sent, nor a food without a number for its amount added', async () => {
  const server = logServer()
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(sheet.getByRole('button', { name: 'Log the meal' }))
  expect(sheet.getByRole('alert').textContent).toBe('Add a food or an estimate first.')

  await findRye(sheet)
  fireEvent.change(sheet.getByLabelText('Amount: Rye bread'), { target: { value: 'a few' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Rye bread' }))

  expect(
    sheet.getByText(
      'An amount is more than 0 and at most 10,000 g or ml, 100 servings or 50 portions; an estimate has a name and its values, such as 72.5, and at most 10,000 kcal.',
    ),
  ).toBeDefined()
  expect(sheet.getByLabelText('Amount: Rye bread').getAttribute('aria-invalid')).toBe('true')
  expect(sheet.queryByRole('list', { name: 'In the meal' })).toBeNull()
  expect(server.sent).toEqual([])
})

test('an amount past what the server takes is said with its bounds, in Croatian, and not added', async () => {
  const server = logServer('2026-10-23', { ...signedIn('hr') })
  renderAt('/')

  const sheet = await openSheet('Novi obrok')
  await findRye(sheet, 'Pronađi namirnicu ili obrok')
  fireEvent.change(sheet.getByLabelText('Količina: Rye bread'), { target: { value: '99999' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Dodaj: Rye bread' }))

  expect(sheet.getByRole('alert').textContent).toBe(
    'Količina je veća od 0 i najviše 10.000 g ili ml, 100 servinga ili 50 porcija; procjena ima naziv i vrijednosti, npr. 72,5, i najviše 10.000 kcal.',
  )
  expect(sheet.getByLabelText('Količina: Rye bread').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])
})

test('a search that finds nothing says so, an estimate is given up, and the sheet closed', async () => {
  logServer()
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.change(sheet.getByLabelText('Find a food or a meal'), { target: { value: 'kale' } })
  expect(
    await sheet.findByText('Nothing of yours matches: give an estimate, or make a new food.'),
  ).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'An estimate' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Remove: an estimate' }))
  expect(sheet.queryByLabelText('What it is')).toBeNull()

  fireEvent.click(sheet.getByRole('button', { name: 'Close' }))
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'A new meal' })).toBeNull()
  })
  // The focus is back on the button that opened it.
  expect(document.activeElement).toBe(screen.getByRole('button', { name: logMeal }))
})

test('a meal sent without an answer holds still, and goes again as it was', async () => {
  let up = false
  const bodies: unknown[] = []
  let gets = 0
  logServer('2026-10-23', {
    'GET /api/v1/days/today': () => {
      gets++
      return Response.json(day())
    },
    'POST /api/v1/meals': async (request) => {
      bodies.push(await request.json())
      return up ? problem(503) : Promise.reject(new TypeError('Failed to fetch'))
    },
  })
  renderAt('/')

  const sheet = await openSheet()
  await findRye(sheet)
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Rye bread' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 207 kcal' }))
  await sheet.findByRole('button', { name: 'Try again' })

  // It may have been logged: the day is asked again, and the meal holds.
  await waitFor(() => {
    expect(gets).toBe(2)
  })
  expect(sheet.getByLabelText('Find a food or a meal').matches(':disabled')).toBe(true)
  expect(sheet.getByRole('button', { name: 'Remove: Rye bread' }).hasAttribute('disabled')).toBe(
    true,
  )
  expect(
    sheet.getByRole('button', { name: 'Log 1 item · 207 kcal' }).hasAttribute('disabled'),
  ).toBe(true)
  up = true
  fireEvent.click(sheet.getByRole('button', { name: 'Try again' }))

  await waitFor(() => {
    expect(bodies).toHaveLength(2)
  })
  expect(bodies[1]).toEqual(bodies[0])
})

test("a meal the server refuses says why, in the server's words", async () => {
  logServer('2026-10-23', { 'POST /api/v1/meals': () => problem(422) })
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(await sheet.findByRole('button', { name: 'Repeat: Porridge' }))

  expect((await sheet.findByRole('alert')).textContent).toBe(
    'The server did not take this meal: what went wrong',
  )
})

test("the server's warnings of a logged meal are said", async () => {
  logServer('2026-10-23', {
    'POST /api/v1/meals': () =>
      Response.json(
        {
          meal: { ...porridge, id: 'x', description: '' },
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

  const sheet = await openSheet()
  fireEvent.click(await sheet.findByRole('button', { name: 'Repeat: Porridge' }))

  // Without a description, the meal is named by the hour it was eaten.
  expect(await screen.findByText(/^Logged: Breakfast · 207 kcal/)).toBeDefined()
  expect(
    await screen.findByText(
      'A meal with the same items was logged just before: if this one was sent twice, delete it.',
    ),
  ).toBeDefined()
})

test('a day to come has no meal to log', async () => {
  logServer('2026-10-25')
  renderAt('/?date=2026-10-25')

  expect(await screen.findByRole('button', { name: /^Type of day: / })).toBeDefined()
  expect(screen.queryByRole('button', { name: logMeal })).toBeNull()
})

test('Enter in the search opens the first food for its amount, and sends nothing', async () => {
  const server = logServer()
  renderAt('/')

  const sheet = await openSheet()
  const search = sheet.getByLabelText('Find a food or a meal')
  fireEvent.change(search, { target: { value: 'rye' } })
  await sheet.findByRole('button', { name: /^Rye bread/ })
  fireEvent.keyDown(search, { key: 'Enter' })

  expect(sheet.getByLabelText('Amount: Rye bread')).toBeDefined()
  // And Enter in the amount adds it.
  fireEvent.keyDown(sheet.getByLabelText('Amount: Rye bread'), { key: 'Enter' })
  expect(basket(sheet)).toEqual(['Rye bread · 2 slice · 207 kcal'])
  expect(server.sent).toEqual([])
})

test("today's meal is logged now when its time is left as it was, the time of its id", async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  // 08:20 in Los Angeles.
  vi.setSystemTime(new Date('2026-10-23T15:20:00Z'))
  const server = logServer()
  renderAt('/')

  const sheet = await openSheet()
  expect(sheet.getByText('Breakfast · today at 8:20 AM')).toBeDefined()
  await giveEstimate(sheet, 'Tea')
  // Some minutes later.
  vi.setSystemTime(new Date('2026-10-23T15:27:00Z'))
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 1 kcal' }))

  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  const [body] = server.sent
  expect(body).not.toHaveProperty('eaten_at')
  const id =
    typeof body === 'object' && body !== null && 'id' in body && typeof body.id === 'string'
      ? body.id
      : ''
  expect(Number.parseInt(id.slice(0, 8) + id.slice(9, 13), 16)).toBe(
    Date.parse('2026-10-23T15:27:00Z'),
  )
})

test('a time still to come is not sent, and one of the night after says so', async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  // 11:00 on the 23rd in Los Angeles, whose day starts at 4.
  vi.setSystemTime(new Date('2026-10-23T18:00:00Z'))
  const server = logServer()
  renderAt('/')

  const sheet = await openSheet()
  await giveEstimate(sheet, 'Tea')
  fireEvent.click(sheet.getByRole('button', { name: 'Change the name and time' }))
  fireEvent.change(sheet.getByLabelText('Time'), { target: { value: '02:30' } })
  expect(sheet.getByText('02:30 is the night after this day.')).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 1 kcal' }))

  expect((await sheet.findByRole('alert')).textContent).toBe(
    'That time is still to come: a meal is logged once eaten.',
  )
  expect(sheet.getByLabelText('Time').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])
  // A time already gone is sent, and the alert goes once it is typed.
  fireEvent.change(sheet.getByLabelText('Time'), { target: { value: '10:15' } })
  expect(sheet.queryByRole('alert')).toBeNull()
  expect(sheet.queryByText(/the night after this day/)).toBeNull()
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 1 kcal' }))
  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  expect(server.sent[0]).toMatchObject({ eaten_at: '2026-10-23T17:15:00.000Z' })
})

test('the focus goes to the search, to a food picked, and back to the search once it is added', async () => {
  logServer()
  renderAt('/')

  const sheet = await openSheet()
  await waitFor(() => {
    expect(document.activeElement).toBe(sheet.getByLabelText('Find a food or a meal'))
  })
  await findRye(sheet)
  await waitFor(() => {
    expect(document.activeElement).toBe(sheet.getByLabelText('Amount: Rye bread'))
  })
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Rye bread' }))
  expect(document.activeElement).toBe(sheet.getByLabelText('Find a food or a meal'))
  expect(sheet.getByLabelText('Find a food or a meal')).toHaveProperty('value', '')
})

test("a food's brand tells it apart", async () => {
  logServer('2026-10-23', {
    'GET /api/v1/foods': () => Response.json({ foods: [{ ...bread, brand: 'Mill' }], unfound: [] }),
  })
  renderAt('/')

  const sheet = await openSheet()
  await findRye(sheet)

  expect(sheet.getByLabelText('Amount: Rye bread · Mill')).toBeDefined()
})

test('what is wrong with an estimate is no longer said once it is given up', async () => {
  logServer()
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(sheet.getByRole('button', { name: 'An estimate' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Add: an estimate' }))
  expect(sheet.getByRole('alert').textContent).toContain('an estimate has a name')

  fireEvent.click(sheet.getByRole('button', { name: 'Remove: an estimate' }))

  expect(sheet.queryByText(/an estimate has a name/)).toBeNull()
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

  const sheet = await openSheet()
  fireEvent.change(sheet.getByLabelText('Find a food or a meal'), { target: { value: 'stew' } })
  fireEvent.click(await sheet.findByRole('button', { name: /^Stew/ }))
  const unit = () => sheet.getByRole('option', { name: /^portion/ }).textContent
  expect(sheet.getByLabelText('Amount: Stew')).toHaveProperty('value', '1')
  expect(unit()).toBe('portion')
  fireEvent.change(sheet.getByLabelText('Amount: Stew'), { target: { value: '2' } })
  expect(unit()).toBe('portions')
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Stew' }))
  expect(basket(sheet)).toEqual(['Stew · 2 portions · 900 kcal'])
})

test('a unit chosen works the amount out anew, both ways, and what is sent is as worked out', async () => {
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

  const sheet = await openSheet()
  fireEvent.change(sheet.getByLabelText('Find a food or a meal'), { target: { value: 'wal' } })
  fireEvent.click(await sheet.findByRole('button', { name: /^Walnuts/ }))
  const amount = () => sheet.getByLabelText('Amount: Walnuts')
  const unit = sheet.getByLabelText('Unit: Walnuts')
  expect(amount()).toHaveProperty('value', '100')
  fireEvent.change(unit, { target: { value: 'serving:handful' } })
  expect(amount()).toHaveProperty('value', '3.33')
  fireEvent.change(unit, { target: { value: 'g' } })
  expect(amount()).toHaveProperty('value', '100')
  fireEvent.change(unit, { target: { value: 'serving:handful' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Walnuts' }))
  // The mass it was, whatever the unit.
  expect(basket(sheet)).toEqual(['Walnuts · 3.33 handful · 654 kcal'])
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 654 kcal' }))

  expect(await screen.findByText(/^Logged: /)).toBeDefined()
  expect(server.sent).toEqual([
    {
      id: expect.any(String) as unknown,
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

test('the lists show the foods eaten lately and most, and the meals, each as the search has it', async () => {
  logServer()
  renderAt('/')

  const sheet = await openSheet()
  // What a list offers, by its buttons: a meal's Repeat, a food's +.
  const found = () =>
    within(sheet.getByRole('list', { name: 'What there is to add' }))
      .getAllByRole('button')
      .map((b) => b.getAttribute('aria-label'))
      .filter((label) => label !== null)
  // All: the meals of before first, then the foods eaten, the oats never.
  await sheet.findByRole('button', { name: 'Repeat: Porridge' })
  expect(found()).toEqual(['Repeat: Porridge', 'Add: Rye bread'])

  fireEvent.click(sheet.getByRole('tab', { name: 'Recent' }))
  expect(found()).toEqual(['Add: Rye bread'])
  fireEvent.click(sheet.getByRole('tab', { name: 'Frequent' }))
  await sheet.findByRole('button', { name: /^Skyr/ })
  expect(found()).toEqual(['Add: Skyr', 'Add: Rye bread'])
  fireEvent.change(sheet.getByLabelText('Find a food or a meal'), { target: { value: 'sky' } })
  expect(found()).toEqual(['Add: Skyr'])
  fireEvent.click(sheet.getByRole('tab', { name: 'Meals' }))
  expect(
    sheet.getByText('Nothing of yours matches: give an estimate, or make a new food.'),
  ).toBeDefined()
  // A meal is found by its items too.
  fireEvent.change(sheet.getByLabelText('Find a food or a meal'), { target: { value: 'rye' } })
  expect(found()).toEqual(['Repeat: Porridge'])
})

test("a food's + adds it as it was eaten last, and a serving's chip sets one of it", async () => {
  logServer()
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(await sheet.findByRole('button', { name: 'Add: Rye bread' }))
  expect(basket(sheet)).toEqual(['Rye bread · 2 slice · 207 kcal'])

  fireEvent.click(sheet.getByRole('button', { name: /^Rye bread/ }))
  fireEvent.click(sheet.getByRole('button', { name: '1 slice (40 g)' }))
  expect(sheet.getByLabelText('Amount: Rye bread')).toHaveProperty('value', '1')
  expect(sheet.getByLabelText('Unit: Rye bread')).toHaveProperty('value', 'serving:slice')
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Rye bread' }))
  expect(basket(sheet)).toEqual([
    'Rye bread · 2 slice · 207 kcal',
    'Rye bread · 1 slice · 104 kcal',
  ])
  expect(sheet.getByRole('button', { name: 'Log 2 items · 311 kcal' })).toBeDefined()
})

test('the meal says what the day has after it, past its target too, and an item is taken out', async () => {
  logServer('2026-10-23', {
    'GET /api/v1/days/today': () =>
      Response.json(day({ remaining: { kcal: 100, protein: 10, carbs: 10, fat: 10 } })),
  })
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(await sheet.findByRole('button', { name: 'Add: Rye bread' }))
  expect(sheet.getByText('after: 107 kcal over')).toBeDefined()

  fireEvent.click(sheet.getByRole('button', { name: 'Remove: Rye bread' }))
  expect(sheet.queryByRole('list', { name: 'In the meal' })).toBeNull()
  expect(sheet.getByText('Pick a food, or repeat a meal.')).toBeDefined()
  expect(sheet.queryByText(/^after: /)).toBeNull()
})

test('a new food is made in the sheet, and opens for its amount', async () => {
  const sent: unknown[] = []
  logServer('2026-10-23', {
    'POST /api/v1/foods/new': async (request) => {
      const body: unknown = await request.json()
      sent.push(body)
      const fields = typeof body === 'object' && body !== null ? body : {}
      return Response.json(
        { ...fields, id: '0193c3a5-0000-7000-8000-0000000000f9', archived: false },
        { status: 201 },
      )
    },
  })
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(sheet.getByRole('button', { name: 'A new food' }))
  fireEvent.change(sheet.getByLabelText('Name'), { target: { value: 'Kefir' } })
  fireEvent.change(sheet.getByLabelText('The values are'), { target: { value: '100ml' } })
  for (const [label, value] of [
    ['Energy (kcal)', '41'],
    ['Protein (g)', '3.4'],
    ['Carbs (g)', '4'],
    ['Fat (g)', '1.5'],
  ] as const) {
    fireEvent.change(sheet.getByLabelText(label), { target: { value } })
  }
  fireEvent.click(sheet.getByRole('button', { name: 'Save the food' }))

  const amount = await sheet.findByLabelText('Amount: Kefir')
  expect(amount).toHaveProperty('value', '100')
  expect(sent).toHaveLength(1)
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Kefir' }))
  expect(basket(sheet)).toEqual(['Kefir · 100 ml · 41 kcal'])
})

test("today's chips log yesterday's meal of the hour again, and open the meal with a food", async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  // 12:30 in Los Angeles: lunch.
  vi.setSystemTime(new Date('2026-10-23T19:30:00Z'))
  const bowl: Meal = { ...porridge, eaten_at: '2026-10-22T19:00:00Z', description: 'Lunch bowl' }
  const server = logServer('2026-10-23', {
    'GET /api/v1/days/2026-10-22': () =>
      Response.json(day({ date: '2026-10-22', meals: [porridge, bowl] })),
  })
  renderAt('/')

  const chips = within(await screen.findByRole('region', { name: 'Quick add' }))
  fireEvent.click(await chips.findByRole('button', { name: "Yesterday's lunch" }))
  expect(await screen.findByText('Logged again, today at this time.')).toBeDefined()
  expect(server.sent).toEqual([
    {
      id: expect.any(String) as unknown,
      description: 'Lunch bowl',
      items: [{ food_id: bread.id, serving: 'slice', servings: 2 }],
    },
  ])

  fireEvent.click(chips.getByRole('button', { name: 'Rye bread 2 slice' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'A new meal' }))
  expect(basket(sheet)).toEqual(['Rye bread · 2 slice · 207 kcal'])
  expect(sheet.getByRole('button', { name: 'Log 1 item · 207 kcal' })).toBeDefined()
})

test('a time is typed on a 24-hour clock, and one that is none is said', async () => {
  const server = logServer('2026-10-21')
  renderAt('/?date=2026-10-21')

  const sheet = await openSheet()
  await giveEstimate(sheet, 'Tea')
  fireEvent.click(sheet.getByRole('button', { name: 'Change the name and time' }))
  fireEvent.change(sheet.getByLabelText('Time'), { target: { value: '7 pm' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 1 kcal' }))
  expect(sheet.getByRole('alert').textContent).toBe('Type the time as 19:30.')
  expect(sheet.getByLabelText('Time').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])

  // 1930 is 19:30, so it reads once the field is left.
  fireEvent.change(sheet.getByLabelText('Time'), { target: { value: '1930' } })
  fireEvent.blur(sheet.getByLabelText('Time'))
  expect(sheet.getByLabelText('Time')).toHaveProperty('value', '19:30')
  expect(sheet.getByText('Dinner · Oct 21 at 7:30 PM')).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 1 kcal' }))
  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  expect(server.sent[0]).toMatchObject({ eaten_at: '2026-10-22T02:30:00.000Z' })
})
