import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
  type BoundFunctions,
  type queries,
} from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { Day, Food, Item, Meal, RecentMeal } from '@/api/types'
import { day, fakeServer, problem, renderAt, signedIn, type Handler } from '@/test/app'

beforeEach(() => {
  // A clock of the tests' own, as what the sheet offers first is of the
  // hour: 15:00 on the 23rd in Los Angeles, a snack's.
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-10-23T22:00:00Z'))
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  localStorage.clear()
})

// recent is a meal of before as the server lists it: eaten at its time
// and at these others, each of its day (by default, the date of its time
// in UTC).
function recent(meal: Meal, ...others: [at: string, day?: string][]): RecentMeal {
  const times: [at: string, day?: string][] = [[meal.eaten_at], ...others]
  return { ...meal, eaten: times.map(([at, of]) => ({ at, day: of ?? at.slice(0, 10) })) }
}

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

// slices are two slices of the bread, an item.
const slices: Item = {
  id: '0193c3a5-0000-7000-8000-0000000000c1',
  food_id: bread.id,
  name: 'Rye bread',
  quantity: { servings: 2, serving: 'slice' },
  amount: 80,
  unit: 'g',
  nutrients: { kcal: 207, protein: 6.8, carbs: 38.4, fat: 2.6 },
  source: 'label',
}

// porridge is a meal of the day before.
const porridge: Meal = {
  id: '0193c3a5-0000-7000-8000-0000000000b1',
  eaten_at: '2026-10-22T15:00:00Z',
  description: 'Porridge',
  client: 'web',
  total: { kcal: 207, protein: 6.8, carbs: 38.4, fat: 2.6 },
  items: [slices],
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
    'GET /api/v1/meals/recent': () => Response.json({ meals: [porridge].map((m) => recent(m)) }),
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

// logMeal names the day's button that logs a meal, in the page (the
// navigation's Log a meal is the app's): today's names the meal of the
// hour, a day before's does not.
const logMeal =
  /^(Log a meal|Upiši obrok|Add (breakfast|lunch|a snack|dinner)|Dodaj (doručak|ručak|užinu|večeru))/

// page is what the queries find in the page, outside the navigation.
async function page() {
  return within(await screen.findByRole('main'))
}

// openSheet opens the meal to log, and gives what is in it.
async function openSheet(name = 'A new meal') {
  fireEvent.click(await (await page()).findByRole('button', { name: logMeal }))
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

// giveEstimate gives an estimate in its step, and adds it to the meal.
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
  fireEvent.click(sheet.getByRole('button', { name: 'Add to meal' }))
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
  // The day's page is the meal's: no way to it.
  expect(screen.queryByRole('link', { name: 'See it today' })).toBeNull()
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

test("the app's Log a meal logs today's over another page, which stays, its toast the way to today", async () => {
  const server = logServer()
  const router = renderAt('/progress?days=30')

  // The bar's, a phone's.
  const [, bar] = await within(
    await screen.findByRole('navigation', { name: 'Sections' }),
  ).findAllByRole('button', { name: 'Log a meal' })
  if (bar === undefined) {
    throw new Error('no Log a meal in the bar')
  }
  fireEvent.click(bar)
  const sheet = within(await screen.findByRole('dialog', { name: 'A new meal' }))
  await giveEstimate(sheet, 'Coffee', '50')
  // What today's target leaves, as its ring says it.
  expect(sheet.getByText('after: 2,450 kcal left')).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 50 kcal' }))

  const see = await screen.findByRole('link', { name: 'See it today' })
  expect(screen.getByText(/^Logged: Breakfast · 207 kcal/)).toBeDefined()
  expect(server.sent).toHaveLength(1)
  expect(router.state.location.pathname).toBe('/progress')
  expect(router.state.location.search).toEqual({ days: 30 })
  fireEvent.click(see)
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/')
  })
})

test('a meal without a name is named by what is in it, as its field and its toast say', async () => {
  const sent: unknown[] = []
  const coffee: Item = {
    id: '0193c3a5-0000-7000-8000-0000000000c2',
    name: 'Coffee',
    quantity: { portions: 1 },
    amount: 1,
    unit: 'portion',
    nutrients: { kcal: 50, protein: 1, carbs: 1, fat: 1 },
    source: 'estimate',
  }
  logServer('2026-10-23', {
    'POST /api/v1/meals': async (request) => {
      sent.push(await request.json())
      return Response.json(
        {
          meal: {
            ...porridge,
            id: '0193c3a5-0000-7000-8000-0000000000a1',
            description: '',
            items: [slices, coffee],
            total: { kcal: 257, protein: 7.8, carbs: 39.4, fat: 3.6 },
          },
          day: { date: '2026-10-23', status: 'open', type: 'rest', total: day().total },
          warnings: [],
        },
        { status: 201 },
      )
    },
  })
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(sheet.getByRole('button', { name: 'Change the name and time' }))
  const name = sheet.getByLabelText('What it was')
  await findRye(sheet)
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Rye bread' }))
  expect(name).toHaveProperty('placeholder', 'Rye bread')
  await giveEstimate(sheet, 'Coffee', '50')
  expect(name).toHaveProperty('placeholder', 'Rye bread and Coffee')
  await giveEstimate(sheet, 'Jam', '40')
  expect(name).toHaveProperty('placeholder', 'Rye bread + 2')
  fireEvent.click(sheet.getByRole('button', { name: 'Remove: Jam' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Log 2 items · 257 kcal' }))

  // Not by the hour's meal: what it was, and its kcal.
  expect(await screen.findByText(/^Logged: Rye bread and Coffee · 257 kcal/)).toBeDefined()
  // Its name is the app's, not sent.
  expect(sent).toHaveLength(1)
  expect(sent[0]).not.toHaveProperty('description')
})

test('a meal of before is logged again in two taps', async () => {
  const server = logServer()
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(await sheet.findByRole('button', { name: 'Repeat: Porridge' }))

  // Its toast says it is one of before, which and its kcal.
  expect(await screen.findByText(/^Repeated: Breakfast · 207 kcal/)).toBeDefined()
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

// The app's Log a meal (its key N, the bar's +) logs on today's page as
// the page's own button does: the meal's row takes the focus.
test("on today's page, N logs the meal there, and its row takes the focus", async () => {
  logServer()
  renderAt('/')
  await (await page()).findByRole('button', { name: logMeal })

  fireEvent.keyDown(window, { key: 'n' })
  const sheet = within(await screen.findByRole('dialog', { name: 'A new meal' }))
  fireEvent.click(await sheet.findByRole('button', { name: 'Repeat: Porridge' }))

  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'A new meal' })).toBeNull()
  })
  await waitFor(() => {
    expect(document.activeElement?.getAttribute('data-meal')).toBe(
      '0193c3a5-0000-7000-8000-0000000000a1',
    )
  })
})

test("another day's meal is at noon, and one past midnight is of the night after the day", async () => {
  const server = logServer('2026-10-21')
  renderAt('/?date=2026-10-21')

  const sheet = await openSheet()
  expect(sheet.getByText('Lunch · Oct 21 at 12:00 PM')).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'Change the name and time' }))
  expect(sheet.getByLabelText('Time')).toHaveProperty('value', '12:00 PM')
  await giveEstimate(sheet, 'Snack')
  // The day starts at 04:00: 01:30 is the night after the 21st.
  fireEvent.change(sheet.getByLabelText('Time'), { target: { value: '01:30' } })
  expect(sheet.getByText('1:30 AM is the night after this day.')).toBeDefined()
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
  // Disabled, and still in reach of the focus, it says what it waits for.
  const log = sheet.getByRole('button', {
    name: 'Log the meal',
    description: 'Pick a food, or repeat a meal.',
  })
  expect(log.getAttribute('aria-disabled')).toBe('true')
  expect(log.hasAttribute('disabled')).toBe(false)
  fireEvent.click(log)
  expect(sheet.queryByRole('alert')).toBeNull()

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
  // A step of its own, named by what was searched for, the focus in it.
  expect(sheet.getByLabelText('What it is')).toHaveProperty('value', 'kale')
  expect(document.activeElement).toBe(sheet.getByLabelText('What it is'))
  expect(sheet.queryByRole('searchbox', { name: 'Find a food or a meal' })).toBeNull()
  fireEvent.click(sheet.getByRole('button', { name: 'Back to search' }))
  expect(sheet.queryByLabelText('What it is')).toBeNull()
  // The search as it was, and the focus on what opened the step.
  expect(sheet.getByLabelText('Find a food or a meal')).toHaveProperty('value', 'kale')
  expect(document.activeElement).toBe(sheet.getByRole('button', { name: 'An estimate' }))

  fireEvent.click(sheet.getByRole('button', { name: 'Close' }))
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'A new meal' })).toBeNull()
  })
  // The focus is back on the button that opened it.
  expect(document.activeElement).toBe((await page()).getByRole('button', { name: logMeal }))
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
    sheet.getByRole('button', { name: 'Log 1 item · 207 kcal' }).getAttribute('aria-disabled'),
  ).toBe('true')
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

  // Without a description, the meal is named by what was in it, not by
  // the hour it was eaten.
  expect(await screen.findByText(/^Repeated: Rye bread · 207 kcal/)).toBeDefined()
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
  expect((await page()).queryByRole('button', { name: logMeal })).toBeNull()
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
  expect(sheet.getByText('2:30 AM is the night after this day.')).toBeDefined()
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
  fireEvent.click(sheet.getByRole('button', { name: 'Add to meal' }))
  expect(sheet.getByRole('alert').textContent).toContain('an estimate has a name')
  expect(sheet.getByLabelText('What it is').getAttribute('aria-invalid')).toBe('true')
  expect(sheet.queryByRole('list', { name: 'In the meal' })).toBeNull()

  fireEvent.click(sheet.getByRole('button', { name: 'Back to search' }))
  expect(sheet.queryByText(/an estimate has a name/)).toBeNull()
  // Given again, it is a new one.
  fireEvent.click(sheet.getByRole('button', { name: 'An estimate' }))
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
  fireEvent.click(sheet.getByRole('button', { name: 'slice (40 g)' }))
  expect(sheet.getByLabelText('Amount: Rye bread')).toHaveProperty('value', '1')
  expect(sheet.getByLabelText('Unit: Rye bread')).toHaveProperty('value', 'serving:slice')
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Rye bread' }))
  // One serving is its name alone.
  expect(basket(sheet)).toEqual([
    'Rye bread · 2 slice · 207 kcal',
    'Rye bread · 1 slice · 104 kcal',
  ])
  expect(sheet.getByRole('button', { name: 'Log 2 items · 311 kcal' })).toBeDefined()
})

test('a serving whose name holds a number is one of it, not one and the number, in Croatian too', async () => {
  const yoghurt: Food = {
    ...oats,
    id: '0193c3a5-0000-7000-8000-0000000000f6',
    name: 'Grčki jogurt',
    nutrients: { kcal: 73, protein: 10, carbs: 4, fat: 2 },
    servings: [
      { name: 'čaša', amount: 200 },
      { name: '½ čaše', amount: 100 },
    ],
  }
  logServer('2026-10-23', {
    ...signedIn('hr'),
    'GET /api/v1/foods': () => Response.json({ foods: [yoghurt], unfound: [] }),
  })
  renderAt('/')

  const sheet = await openSheet('Novi obrok')
  fireEvent.change(sheet.getByLabelText('Pronađi namirnicu ili obrok'), {
    target: { value: 'jog' },
  })
  fireEvent.click(await sheet.findByRole('button', { name: /^Grčki jogurt/ }))
  expect(sheet.getByRole('button', { name: 'čaša (200 g)' })).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: '½ čaše (100 g)' }))
  expect(sheet.getByLabelText('Količina: Grčki jogurt')).toHaveProperty('value', '1')
  fireEvent.click(sheet.getByRole('button', { name: 'Dodaj: Grčki jogurt' }))

  expect(
    within(sheet.getByRole('list', { name: 'U obroku' }))
      .getAllByRole('listitem')
      .map((li) => li.textContent.replace('×', '')),
  ).toEqual(['Grčki jogurt · ½ čaše · 73 kcal'])
})

test('the meal says what the day has after it, past its target too, and an item is taken out', async () => {
  logServer('2026-10-23', {
    'GET /api/v1/days/today': () =>
      Response.json(
        day({
          total: { kcal: 2400, protein: 150, carbs: 270, fat: 70 },
          remaining: { kcal: 100, protein: 10, carbs: 10, fat: 10 },
        }),
      ),
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

test("what the day has after a meal is what it has less the meal's kcal as shown", async () => {
  logServer()
  renderAt('/')

  const sheet = await openSheet()
  await giveEstimate(sheet, 'Soup', '154.5')
  // 2,500 less 155, not 2,345.5 rounded up.
  expect(sheet.getByRole('button', { name: 'Log 1 item · 155 kcal' })).toBeDefined()
  expect(sheet.getByText('after: 2,345 kcal left')).toBeDefined()
})

// newFoodServer is a day whose server makes a new food as it was sent,
// and keeps what it was sent.
function newFoodServer(over: Record<string, Handler> = {}) {
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
    ...over,
  })
  return { sent }
}

// primaries are the sheet's primary buttons, as the eye and a screen
// reader find them.
function primaries(sheet: Sheet) {
  return sheet
    .getAllByRole('button')
    .filter((b) => b.className.includes('bg-primary') && !b.className.includes('bg-primary-'))
    .map((b) => b.textContent)
}

test('a new food is a step of its own, in short, and goes into the meal in the amount eaten now', async () => {
  const server = newFoodServer()
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.change(sheet.getByLabelText('Find a food or a meal'), { target: { value: 'kefir' } })
  await sheet.findByText('Nothing of yours matches: give an estimate, or make a new food.')
  expect(primaries(sheet)).toEqual(['Log the meal'])
  fireEvent.click(sheet.getByRole('button', { name: 'A new food' }))

  // In the search's place, one button of its own: never two at once.
  expect(primaries(sheet)).toEqual(['Add to meal'])
  expect(sheet.queryByRole('button', { name: /^Log / })).toBeNull()
  expect(sheet.queryByRole('button', { name: 'Save the food' })).toBeNull()
  expect(sheet.getByRole('heading', { name: 'A new food' })).toBeDefined()
  // Named by what was searched for; the brand and the rest under More.
  expect(sheet.getByLabelText('Name')).toHaveProperty('value', 'kefir')
  expect(document.activeElement).toBe(sheet.getByLabelText('Name'))
  expect(sheet.getByText('More').closest('details')?.open).toBe(false)
  fireEvent.change(sheet.getByLabelText('Name'), { target: { value: 'Kefir' } })
  fireEvent.click(sheet.getByRole('radio', { name: 'per 100 ml' }))
  for (const [label, value] of [
    ['Energy (kcal)', '41'],
    ['Protein (g)', '3.4'],
    ['Carbs (g)', '4'],
    ['Fat (g)', '1.5'],
  ] as const) {
    fireEvent.change(sheet.getByLabelText(label), { target: { value } })
  }
  // 100 ml to start with.
  expect(sheet.getByLabelText('Eaten now (ml)')).toHaveProperty('value', '100')
  fireEvent.change(sheet.getByLabelText('Eaten now (ml)'), { target: { value: '250' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Add to meal' }))

  // In the meal, and the search empty, with the focus, for the next.
  await waitFor(() => {
    expect(basket(sheet)).toEqual(['Kefir · 250 ml · 103 kcal'])
  })
  expect(sheet.getByLabelText('Find a food or a meal')).toHaveProperty('value', '')
  expect(document.activeElement).toBe(sheet.getByLabelText('Find a food or a meal'))
  expect(primaries(sheet)).toEqual(['Log 1 item · 103 kcal'])
  expect(server.sent).toEqual([
    {
      name: 'Kefir',
      basis: '100ml',
      nutrients: { kcal: 41, protein: 3.4, carbs: 4, fat: 1.5 },
      source: 'label',
    },
  ])
})

test("a new food's step checks it as the food's form does, and its amount, and sends nothing wrong", async () => {
  const server = newFoodServer()
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(sheet.getByRole('button', { name: 'A new food' }))
  fireEvent.change(sheet.getByLabelText('Name'), { target: { value: 'Kefir' } })
  // The kcal the macros tell otherwise, which does not keep it from a save.
  for (const [label, value] of [
    ['Energy (kcal)', '4'],
    ['Protein (g)', '3.4'],
    ['Carbs (g)', '4'],
    ['Fat (g)', '1.5'],
  ] as const) {
    fireEvent.change(sheet.getByLabelText(label), { target: { value } })
  }
  expect(sheet.getByText('The macros make about 43 kcal, not 4: check the values.')).toBeDefined()
  // An amount past what an item holds, and another name twice, under More.
  fireEvent.change(sheet.getByLabelText('Eaten now (g)'), { target: { value: '20000' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Add another name' }))
  fireEvent.change(sheet.getByLabelText('Other name 1'), { target: { value: 'Milk kefir' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Add another name' }))
  fireEvent.change(sheet.getByLabelText('Other name 2'), { target: { value: 'milk kefir' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Add to meal' }))

  expect(
    await sheet.findByText('An amount is more than 0 and at most 10,000 g or ml, or 50 portions.'),
  ).toBeDefined()
  expect(sheet.getByLabelText('Eaten now (g)').getAttribute('aria-invalid')).toBe('true')
  // More opens on what is wrong in it.
  expect(sheet.getByText('More').closest('details')?.open).toBe(true)
  expect(sheet.getByText('This other name is given already.')).toBeDefined()
  expect(server.sent).toEqual([])
  expect(sheet.queryByRole('list', { name: 'In the meal' })).toBeNull()
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
  expect(await screen.findByText('Repeated: Breakfast · 207 kcal')).toBeDefined()
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

// skyrBowl is a breakfast of skyr, eaten on the 22nd and the 21st at
// 07:30 in Los Angeles; sandwich, a lunch of the bread on the 22nd.
const skyrBowl = recent(
  {
    ...porridge,
    id: '0193c3a5-0000-7000-8000-0000000000b7',
    eaten_at: '2026-10-22T14:30:00Z',
    description: 'Skyr bowl',
    items: [{ ...slices, food_id: skyr.id, name: skyr.name }],
  },
  ['2026-10-21T14:30:00Z'],
)
const sandwich = recent({
  ...porridge,
  id: '0193c3a5-0000-7000-8000-0000000000b8',
  eaten_at: '2026-10-22T19:30:00Z',
  description: 'Sandwich',
})

test.each([
  // 12:30 in Los Angeles: the bread of the lunch first.
  ['2026-10-23T19:30:00Z', ['Rye bread 2 slice', 'Skyr 150 g']],
  // 08:00: the skyr of the breakfasts, as the foods logged most.
  ['2026-10-23T15:00:00Z', ['Skyr 150 g', 'Rye bread 2 slice']],
])(
  "at %s today's chips offer the foods eaten at the meal of the hour first",
  async (now, chips) => {
    vi.setSystemTime(new Date(now))
    logServer('2026-10-23', {
      'GET /api/v1/meals/recent': () => Response.json({ meals: [sandwich, skyrBowl] }),
    })
    renderAt('/')

    const quick = within(await screen.findByRole('region', { name: 'Quick add' }))
    await quick.findByRole('button', { name: 'Skyr 150 g' })
    await waitFor(() => {
      expect(quick.getAllByRole('button').map((b) => b.textContent)).toEqual(chips)
    })
  },
)

test('the foods eaten at the meal of the day lately come first: skyr in the morning', async () => {
  // 08:00 in Los Angeles.
  vi.setSystemTime(new Date('2026-10-23T15:00:00Z'))
  logServer('2026-10-23', {
    // The bread used last.
    'GET /api/v1/foods/list': () => Response.json({ foods: [bread, skyr], total: 2 }),
    'GET /api/v1/meals/recent': () => Response.json({ meals: [sandwich, skyrBowl] }),
  })
  renderAt('/')

  const sheet = await openSheet()
  await sheet.findByRole('region', { name: 'For breakfast' })
  expect(await offered(sheet)).toEqual(['Repeat: Sandwich', 'Add: Skyr', 'Add: Rye bread'])
})

// plate is a meal of before of one estimate, named, eaten at these times
// (the latest first), each of its day on the user's clock.
function plate(name: string, ...times: [at: string, day: string][]): RecentMeal {
  const nutrients = { kcal: 500, protein: 30, carbs: 50, fat: 20 }
  return {
    id: name,
    eaten_at: times[0]?.[0] ?? '',
    description: name,
    client: 'web',
    total: nutrients,
    items: [
      {
        id: `${name} item`,
        name,
        quantity: { portions: 1 },
        amount: 1,
        unit: 'portion',
        nutrients,
        source: 'estimate',
      },
    ],
    eaten: times.map(([at, of]) => ({ at, day: of })),
  }
}

// The meals of before at 20:00 on the 23rd in Los Angeles, the latest
// first: today's lunch and breakfast; a stew eaten at dinner twice, a
// pasta and a soup once; a salad at lunch; and a goulash at dinner, three
// weeks before.
const ofDinner = [
  plate('Tuna', ['2026-10-23T20:32:00Z', '2026-10-23']),
  plate('Yoghurt', ['2026-10-23T14:32:00Z', '2026-10-23']),
  plate('Stew', ['2026-10-23T02:00:00Z', '2026-10-22'], ['2026-10-21T02:30:00Z', '2026-10-20']),
  plate('Salad', ['2026-10-22T19:00:00Z', '2026-10-22']),
  plate('Pasta', ['2026-10-22T03:00:00Z', '2026-10-21']),
  plate('Soup', ['2026-10-20T03:00:00Z', '2026-10-19']),
  plate('Goulash', ['2026-10-04T03:00:00Z', '2026-10-03']),
]

// suggested are the meals the sheet suggests as cards, by their buttons.
async function suggested(sheet: Sheet, name: string) {
  return within(await sheet.findByRole('region', { name }))
    .getAllByRole('button')
    .map((b) => b.getAttribute('aria-label'))
}

test("at dinner the dinners of lately come first, the most eaten first, and today's meals last", async () => {
  vi.setSystemTime(new Date('2026-10-24T03:00:00Z'))
  const server = logServer('2026-10-23', {
    'GET /api/v1/meals/recent': () => Response.json({ meals: ofDinner }),
  })
  renderAt('/')

  const sheet = await openSheet()
  // Above the search, two on a phone.
  expect(await suggested(sheet, 'For dinner')).toEqual(['Repeat: Stew', 'Repeat: Pasta'])
  expect(
    within(sheet.getByRole('region', { name: 'For dinner' }))
      .getAllByRole('listitem')
      .map((li) => li.textContent),
  ).toEqual(['Stew500 kcalRepeat', 'Pasta500 kcalRepeat'])
  // The lists leave them out: the next dinner, the others, and the meals
  // of today last, the one just logged no first Repeat.
  expect(await offered(sheet)).toEqual([
    'Repeat: Soup',
    'Repeat: Salad',
    'Repeat: Goulash',
    'Add: Rye bread',
  ])
  fireEvent.click(sheet.getByRole('tab', { name: 'Meals' }))
  expect(await offered(sheet)).toEqual([
    'Repeat: Soup',
    'Repeat: Salad',
    'Repeat: Goulash',
    'Repeat: Tuna',
    'Repeat: Yoghurt',
  ])
  // A search is of them all, without the cards.
  const search = sheet.getByLabelText('Find a food or a meal')
  fireEvent.change(search, { target: { value: 'stew' } })
  expect(await offered(sheet)).toEqual(['Repeat: Stew'])
  expect(sheet.queryByRole('region', { name: 'For dinner' })).toBeNull()

  // A card's Repeat logs the meal again.
  fireEvent.change(search, { target: { value: '' } })
  fireEvent.click(
    within(await sheet.findByRole('region', { name: 'For dinner' })).getByRole('button', {
      name: 'Repeat: Stew',
    }),
  )
  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  expect(server.sent[0]).toMatchObject({
    description: 'Stew',
    items: [{ name: 'Stew', nutrients: { kcal: 500, protein: 30, carbs: 50, fat: 20 } }],
  })
})

// media makes matchMedia match one query alone: a desktop's width, or a
// finger's screen.
function media(matched: string) {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: query === matched,
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }))
}

test("a desktop's sheet suggests three", async () => {
  media('(min-width: 64rem)')
  vi.setSystemTime(new Date('2026-10-24T03:00:00Z'))
  logServer('2026-10-23', {
    'GET /api/v1/meals/recent': () => Response.json({ meals: ofDinner }),
  })
  renderAt('/')

  const sheet = await openSheet()
  expect(await suggested(sheet, 'For dinner')).toEqual([
    'Repeat: Stew',
    'Repeat: Pasta',
    'Repeat: Soup',
  ])
  expect((await offered(sheet)).slice(0, 2)).toEqual(['Repeat: Salad', 'Repeat: Goulash'])
})

test('on a touch screen the keyboard waits for a tap on the search, and does not come with an item', async () => {
  media('(pointer: coarse)')
  logServer()
  renderAt('/')

  const sheet = await openSheet()
  // The sheet has the focus, not the search.
  const dialog = screen.getByRole('dialog', { name: 'A new meal' })
  await waitFor(() => {
    expect(document.activeElement).toBe(dialog)
  })
  fireEvent.click(await sheet.findByRole('button', { name: 'Add: Rye bread' }))
  expect(basket(sheet)).toEqual(['Rye bread · 2 slice · 207 kcal'])
  // Added, the meal's button has it.
  expect(document.activeElement).toBe(sheet.getByRole('button', { name: 'Log 1 item · 207 kcal' }))
  await giveEstimate(sheet, 'Tea')
  expect(document.activeElement).toBe(sheet.getByRole('button', { name: 'Log 2 items · 208 kcal' }))
})

test("a time is typed on the language's clock, or a 24-hour one, and one that is none is said", async () => {
  const server = logServer('2026-10-21')
  renderAt('/?date=2026-10-21')

  const sheet = await openSheet()
  await giveEstimate(sheet, 'Tea')
  fireEvent.click(sheet.getByRole('button', { name: 'Change the name and time' }))
  fireEvent.change(sheet.getByLabelText('Time'), { target: { value: '19 pm' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 1 kcal' }))
  expect(sheet.getByRole('alert').textContent).toBe('Type the time as 7:30 PM.')
  expect(sheet.getByLabelText('Time').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])

  // 7 pm is 7:00 PM once the field is left, and 1930 is 7:30 PM.
  fireEvent.change(sheet.getByLabelText('Time'), { target: { value: '7 pm' } })
  fireEvent.blur(sheet.getByLabelText('Time'))
  expect(sheet.getByLabelText('Time')).toHaveProperty('value', '7:00 PM')
  expect(sheet.getByText('Dinner · Oct 21 at 7:00 PM')).toBeDefined()
  fireEvent.change(sheet.getByLabelText('Time'), { target: { value: '1930' } })
  fireEvent.blur(sheet.getByLabelText('Time'))
  expect(sheet.getByLabelText('Time')).toHaveProperty('value', '7:30 PM')
  expect(sheet.getByText('Dinner · Oct 21 at 7:30 PM')).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 1 kcal' }))
  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  expect(server.sent[0]).toMatchObject({ eaten_at: '2026-10-22T02:30:00.000Z' })
})

// offered is what a list of the sheet offers, by its buttons: a meal's
// Repeat, a food's +.
async function offered(sheet: Sheet) {
  return within(await sheet.findByRole('list', { name: 'What there is to add' }))
    .getAllByRole('button')
    .map((b) => b.getAttribute('aria-label'))
    .filter((label) => label !== null)
}

test.each(['grozde', 'grozdje', 'grozd'])(
  '%s finds the foods eaten and the meals of before with a đ, as the server does',
  async (query) => {
    const grapes: Food = {
      ...skyr,
      id: '0193c3a5-0000-7000-8000-0000000000f7',
      name: 'Bijelo grožđe',
      nutrients: { kcal: 69, protein: 0.7, carbs: 18, fat: 0.2 },
    }
    const snack: Meal = {
      ...porridge,
      id: '0193c3a5-0000-7000-8000-0000000000b2',
      description: 'Grožđe za užinu',
      items: [{ ...slices, food_id: grapes.id, name: grapes.name }],
    }
    logServer('2026-10-23', {
      'GET /api/v1/foods/list': () => Response.json({ foods: [grapes], total: 1 }),
      'GET /api/v1/meals/recent': () =>
        Response.json({ meals: [snack, porridge].map((m) => recent(m)) }),
    })
    renderAt('/')

    const sheet = await openSheet()
    await sheet.findByRole('button', { name: 'Add: Bijelo grožđe' })
    fireEvent.change(sheet.getByLabelText('Find a food or a meal'), { target: { value: query } })
    expect(await offered(sheet)).toEqual(['Repeat: Grožđe za užinu'])
    fireEvent.click(sheet.getByRole('tab', { name: 'Recent' }))
    expect(await offered(sheet)).toEqual(['Add: Bijelo grožđe'])
    fireEvent.click(sheet.getByRole('tab', { name: 'Meals' }))
    expect(await offered(sheet)).toEqual(['Repeat: Grožđe za užinu'])
  },
)

// marks are what is marked in the row of a button.
function marks(button: HTMLElement) {
  return Array.from(button.closest('li')?.querySelectorAll('mark') ?? []).map((m) => m.textContent)
}

test('what the search matched is marked in the names of the foods and the meals', async () => {
  const toast: Meal = {
    ...porridge,
    id: '0193c3a5-0000-7000-8000-0000000000b3',
    description: 'Rye toast with what was on it, as a model lists it',
  }
  logServer('2026-10-23', {
    'GET /api/v1/foods': () =>
      Response.json({ foods: [{ ...bread, brand: 'Ryeland' }], unfound: [] }),
    'GET /api/v1/meals/recent': () => Response.json({ meals: [toast].map((m) => recent(m)) }),
  })
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.change(sheet.getByLabelText('Find a food or a meal'), { target: { value: 'rye' } })
  // In the food's name, and its brand.
  expect(marks(await sheet.findByRole('button', { name: 'Add: Rye bread · Ryeland' }))).toEqual([
    'Rye',
    'Rye',
  ])
  // A long description goes under the meal's name, what was in it, which
  // the search matches too.
  expect(marks(sheet.getByRole('button', { name: 'Repeat: Rye bread' }))).toEqual(['Rye', 'Rye'])
})

test("a meal of before is found by its items' brands, which do not name it", async () => {
  const branded: Meal = {
    ...porridge,
    description: '',
    items: [{ ...slices, name: 'Sourdough', brand: 'Ryeland' }],
  }
  logServer('2026-10-23', {
    'GET /api/v1/meals/recent': () => Response.json({ meals: [branded].map((m) => recent(m)) }),
  })
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.change(sheet.getByLabelText('Find a food or a meal'), {
    target: { value: 'ryeland' },
  })
  expect(await sheet.findByRole('button', { name: 'Repeat: Sourdough' })).toBeDefined()
})

test('a meal of before is offered once, the latest of the same foods the search matches', async () => {
  // The porridge eaten before in another amount, and named otherwise.
  const more: Meal = {
    ...porridge,
    id: '0193c3a5-0000-7000-8000-0000000000b4',
    eaten_at: '2026-10-21T15:00:00Z',
    total: { kcal: 311, protein: 10, carbs: 58, fat: 4 },
    items: [{ ...slices, quantity: { servings: 3, serving: 'slice' }, amount: 120 }],
  }
  const toast: Meal = { ...more, id: '0193c3a5-0000-7000-8000-0000000000b5', description: 'Toast' }
  const lunch: Meal = {
    ...porridge,
    id: '0193c3a5-0000-7000-8000-0000000000b6',
    description: 'Lunch',
    items: [{ ...slices, food_id: oats.id, name: oats.name }],
  }
  logServer('2026-10-23', {
    'GET /api/v1/meals/recent': () =>
      Response.json({ meals: [porridge, more, lunch, toast].map((m) => recent(m)) }),
  })
  renderAt('/')

  const sheet = await openSheet()
  fireEvent.click(sheet.getByRole('tab', { name: 'Meals' }))
  expect(await offered(sheet)).toEqual(['Repeat: Porridge', 'Repeat: Lunch'])
  fireEvent.change(sheet.getByLabelText('Find a food or a meal'), { target: { value: 'toa' } })
  expect(await offered(sheet)).toEqual(['Repeat: Toast'])
})
