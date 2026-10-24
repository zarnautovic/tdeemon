import { focusManager } from '@tanstack/react-query'
import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Day, Food, Item, Meal } from '@/api/types'
import { setLocale } from '@/i18n/locale'
import { day, fakeServer, problem, renderAt, signedIn, today, type Handler } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

const mealId = '0193c3a5-0000-7000-8000-0000000000a1'
const oatsFood = '0193c3a5-0000-7000-8000-0000000000f1'

const oats: Item = {
  id: '0193c3a5-0000-7000-8000-0000000000b1',
  food_id: oatsFood,
  name: 'Oats',
  quantity: { amount: 80, unit: 'g' },
  amount: 80,
  unit: 'g',
  nutrients: { kcal: 300, protein: 11, carbs: 54, fat: 6 },
  source: 'label',
}
const bread: Item = {
  id: '0193c3a5-0000-7000-8000-0000000000b2',
  food_id: '0193c3a5-0000-7000-8000-0000000000f2',
  name: 'Banana bread',
  quantity: { portions: 1.5 },
  amount: 1.5,
  unit: 'portion',
  nutrients: { kcal: 220, protein: 7, carbs: 38, fat: 3 },
  source: 'dish',
  note: 'home-made',
}
const egg: Item = {
  id: '0193c3a5-0000-7000-8000-0000000000b3',
  food_id: '0193c3a5-0000-7000-8000-0000000000f3',
  name: 'Egg',
  quantity: { serving: 'egg', servings: 2 },
  amount: 100,
  unit: 'g',
  nutrients: { kcal: 140, protein: 12, carbs: 1, fat: 10 },
  source: 'label',
}
const salad: Item = {
  id: '0193c3a5-0000-7000-8000-0000000000b4',
  name: 'Salad from the canteen',
  quantity: { portions: 1 },
  amount: 1,
  unit: 'portion',
  nutrients: { kcal: 250, protein: 8, carbs: 12, fat: 18 },
  source: 'estimate',
}

const breakfast: Meal = {
  id: mealId,
  eaten_at: '2026-10-23T15:45:00Z',
  description: 'Breakfast',
  client: 'mcp',
  total: { kcal: 910, protein: 38, carbs: 105, fat: 37 },
  items: [oats, bread, egg, salad],
}

interface Sent {
  method: string
  path: string
  body: unknown
}

// An op as the fake server reads it: an item's new amount, if it has one.
interface SentOp {
  op: string
  item_id: string
  amount: number | undefined
}

function opsOf(body: unknown): SentOp[] {
  if (typeof body !== 'object' || body === null || !('ops' in body) || !Array.isArray(body.ops)) {
    return []
  }
  return body.ops.flatMap((o: unknown) => {
    if (
      typeof o !== 'object' ||
      o === null ||
      !('op' in o) ||
      !('item_id' in o) ||
      typeof o.op !== 'string' ||
      typeof o.item_id !== 'string'
    ) {
      return []
    }
    const item: object = 'item' in o && typeof o.item === 'object' && o.item !== null ? o.item : {}
    const amount = 'amount' in item && typeof item.amount === 'number' ? item.amount : undefined
    return [{ op: o.op, item_id: o.item_id, amount }]
  })
}

// mealServer is a day with breakfast: a change of the meal stores the
// ops' new amounts and removals as the server would, in a simple way, and
// the day is answered as it stands.
function mealServer(answer: Partial<Record<string, Handler>> = {}) {
  let current: Day = day({ meals: [breakfast] })
  const sent: Sent[] = []
  const summary = () => {
    const { meals: _meals, metrics: _metrics, ...rest } = current
    return rest
  }
  const routes: Record<string, Handler> = {
    ...signedIn(),
    'GET /api/v1/days/today': () => Response.json(current),
    [`PATCH /api/v1/meals/${mealId}`]: async (request) => {
      const body: unknown = await request.json()
      sent.push({ method: 'PATCH', path: `/api/v1/meals/${mealId}`, body })
      const ops = opsOf(body)
      let items = breakfastNow().items
      for (const op of ops) {
        if (op.op === 'remove') {
          items = items.filter((i) => i.id !== op.item_id)
        } else if (op.amount !== undefined) {
          const amount = op.amount
          items = items.map((i) =>
            i.id === op.item_id ? { ...i, amount, quantity: { ...i.quantity, amount } } : i,
          )
        }
      }
      const changed = { ...breakfastNow(), items }
      current = { ...current, meals: [changed] }
      return Response.json({ meal: changed, day: summary(), warnings: [] })
    },
    [`DELETE /api/v1/meals/${mealId}`]: (request) => {
      sent.push({ method: 'DELETE', path: new URL(request.url).pathname, body: undefined })
      current = { ...current, meals: [] }
      return Response.json({ day: summary() })
    },
    ...answer,
  }
  function breakfastNow(): Meal {
    return current.meals[0] ?? breakfast
  }
  // elsewhere changes the meal as another client would.
  function elsewhere(change: (m: Meal) => Meal) {
    current = { ...current, meals: [change(breakfastNow())] }
  }
  return { routes, sent, elsewhere }
}

function meal() {
  return within(screen.getByRole('article'))
}

async function edit() {
  fireEvent.click(await screen.findByRole('button', { name: 'Change the meal: Breakfast' }))
}

test("an item's amount is changed as it was given, in one change of the meal", async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  expect(meal().getByLabelText('Amount: Oats')).toHaveProperty('value', '80')
  expect(meal().getByLabelText('Amount: Banana bread')).toHaveProperty('value', '1.5')
  expect(meal().getByLabelText('Amount: Egg')).toHaveProperty('value', '2')
  // An estimate is of a whole plate: it has no amount to change.
  expect(meal().queryByLabelText('Amount: Salad from the canteen')).toBeNull()
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '100' } })
  fireEvent.change(meal().getByLabelText('Amount: Banana bread'), { target: { value: '2' } })
  fireEvent.change(meal().getByLabelText('Amount: Egg'), { target: { value: '3' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(await meal().findByText('100 g')).toBeDefined()
  expect(server.sent).toEqual([
    {
      method: 'PATCH',
      path: `/api/v1/meals/${mealId}`,
      body: {
        ops: [
          { op: 'replace', item_id: oats.id, item: { food_id: oatsFood, amount: 100, unit: 'g' } },
          {
            op: 'replace',
            item_id: bread.id,
            item: { food_id: bread.food_id, note: 'home-made', portions: 2 },
          },
          {
            op: 'replace',
            item_id: egg.id,
            item: { food_id: egg.food_id, serving: 'egg', servings: 3 },
          },
        ],
      },
    },
  ])
  expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
})

// unitBeside is the unit shown in an amount's field.
const unitBeside = (field: HTMLElement) => field.nextElementSibling?.textContent

test('the word for portions agrees with the number typed', async () => {
  fakeServer(mealServer().routes)
  renderAt('/')

  await edit()
  const slices = meal().getByLabelText('Amount: Banana bread')
  expect(unitBeside(slices)).toBe('portions')
  fireEvent.change(slices, { target: { value: '1' } })
  expect(unitBeside(slices)).toBe('portion')
  fireEvent.change(slices, { target: { value: 'x' } })
  expect(unitBeside(slices)).toBe('portions')
  // Removed and kept again, the field has what was typed, as its word.
  fireEvent.change(slices, { target: { value: '1' } })
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Banana bread' }))
  fireEvent.click(meal().getByRole('button', { name: 'Keep: Banana bread' }))
  expect(meal().getByLabelText('Amount: Banana bread')).toHaveProperty('value', '1')
  expect(unitBeside(meal().getByLabelText('Amount: Banana bread'))).toBe('portion')
  // A serving's name and a food's unit stay as they are.
  expect(unitBeside(meal().getByLabelText('Amount: Egg'))).toBe('egg')
  expect(unitBeside(meal().getByLabelText('Amount: Oats'))).toBe('g')
})

test('items are removed, and one can be kept again before saving', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Salad from the canteen' }))
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Keep: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(meal().queryByText('Salad from the canteen')).toBeNull()
  })
  expect(server.sent.map((s) => s.body)).toEqual([{ ops: [{ op: 'remove', item_id: salad.id }] }])
  expect(meal().getByText('Egg')).toBeDefined()
})

test('a meal is not left without items', async () => {
  fakeServer(mealServer().routes)
  renderAt('/')

  await edit()
  for (const name of ['Oats', 'Banana bread', 'Egg', 'Salad from the canteen']) {
    fireEvent.click(meal().getByRole('button', { name: `Remove: ${name}` }))
  }

  expect(meal().getByText('A meal keeps at least one item: delete the meal instead.')).toBeDefined()
  expect(meal().getByRole('button', { name: 'Save the meal' }).hasAttribute('disabled')).toBe(true)
})

test('a meal left as it was sends nothing', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(await meal().findByText('80 g')).toBeDefined()
  expect(server.sent).toEqual([])
  fireEvent.click(meal().getByRole('button', { name: 'Change the meal: Breakfast' }))
  fireEvent.click(meal().getByRole('button', { name: 'Cancel' }))
  expect(server.sent).toEqual([])
})

test('an amount that is no number is not sent', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: 'lots' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(meal().getByRole('alert').textContent).toBe('Write a number, such as 72.5.')
  expect(meal().getByLabelText('Amount: Oats').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])
})

test("the server's warnings of a change are said in the app's words", async () => {
  fakeServer(
    mealServer({
      [`PATCH /api/v1/meals/${mealId}`]: () =>
        Response.json({
          meal: breakfast,
          day: { date: today, status: 'open', type: 'rest', total: breakfast.total },
          warnings: [{ code: 'duplicate_meal', message: 'a meal with the same items was logged' }],
        }),
    }).routes,
  )
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(
    await meal().findByText(
      'A meal with the same items was logged just before: if this one was sent twice, delete it.',
    ),
  ).toBeDefined()
})

test('a change the server refuses says so, with nothing to try again', async () => {
  fakeServer(mealServer({ [`PATCH /api/v1/meals/${mealId}`]: () => problem(422) }).routes)
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect((await meal().findByRole('alert')).textContent).toBe(
    'The server did not take this change.',
  )
  expect(meal().queryByRole('button', { name: 'Try again' })).toBeNull()
})

test('a meal is deleted after it asks', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Delete the meal' }))
  expect(meal().getByText('Delete this meal?')).toBeDefined()
  fireEvent.click(meal().getByRole('button', { name: 'Keep it' }))
  expect(server.sent).toEqual([])
  fireEvent.click(meal().getByRole('button', { name: 'Delete the meal' }))
  fireEvent.click(meal().getByRole('button', { name: 'Delete' }))

  expect(await screen.findByText('Nothing logged yet.')).toBeDefined()
  expect(server.sent).toEqual([
    { method: 'DELETE', path: `/api/v1/meals/${mealId}`, body: undefined },
  ])
})

test('a delete that failed says so, and is sent again', async () => {
  const server = mealServer()
  let up = false
  const remove = server.routes[`DELETE /api/v1/meals/${mealId}`]
  fakeServer({
    ...server.routes,
    [`DELETE /api/v1/meals/${mealId}`]: (request) =>
      up && remove ? remove(request) : problem(503),
  })
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Delete the meal' }))
  fireEvent.click(meal().getByRole('button', { name: 'Delete' }))
  expect((await meal().findByRole('alert')).textContent).toContain(
    'Not saved. The server failed to answer.',
  )

  up = true
  fireEvent.click(meal().getByRole('button', { name: 'Try again' }))

  expect(await screen.findByText('Nothing logged yet.')).toBeDefined()
})

test('an amount of thousands with decimals starts as it is, and unchanged sends nothing', async () => {
  const milk: Item = {
    ...oats,
    id: '0193c3a5-0000-7000-8000-0000000000b5',
    name: 'Milk',
    quantity: { amount: 1250.5, unit: 'ml' },
    amount: 1250.5,
    unit: 'ml',
  }
  const server = mealServer({
    'GET /api/v1/days/today': () =>
      Response.json(day({ meals: [{ ...breakfast, items: [milk] }] })),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  expect(meal().getByLabelText('Amount: Milk')).toHaveProperty('value', '1250.5')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(meal().queryByLabelText('Amount: Milk')).toBeNull()
  })
  expect(server.sent).toEqual([])
})

test('an amount left as it started is not sent, though the field shows it rounded', async () => {
  const third: Item = { ...oats, quantity: { amount: 66.6667, unit: 'g' }, amount: 66.6667 }
  const server = mealServer({
    'GET /api/v1/days/today': () =>
      Response.json(day({ meals: [{ ...breakfast, items: [third, salad] }] })),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  expect(meal().getByLabelText('Amount: Oats')).toHaveProperty('value', '66.667')
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Salad from the canteen' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([{ ops: [{ op: 'remove', item_id: salad.id }] }])
  })
})

test('an amount given without a unit is changed without one: the food says it', async () => {
  const plain: Item = { ...oats, quantity: { amount: 80 } }
  const server = mealServer({
    'GET /api/v1/days/today': () =>
      Response.json(day({ meals: [{ ...breakfast, items: [plain] }] })),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '90' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      { ops: [{ op: 'replace', item_id: oats.id, item: { food_id: oatsFood, amount: 90 } }] },
    ])
  })
})

test('a change made elsewhere while the meal is open stays, unless it is changed here', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  // A model sets the oats to 120 g; the window gets the focus back.
  server.elsewhere((m) => ({
    ...m,
    items: m.items.map((i) =>
      i.id === oats.id ? { ...i, amount: 120, quantity: { amount: 120, unit: 'g' } } : i,
    ),
  }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(await meal().findByText('120 g')).toBeDefined()
  expect(server.sent.map((s) => s.body)).toEqual([{ ops: [{ op: 'remove', item_id: egg.id }] }])
})

test('a change sent again after it failed closes the meal once saved', async () => {
  const server = mealServer()
  let up = false
  const patch = server.routes[`PATCH /api/v1/meals/${mealId}`]
  fakeServer({
    ...server.routes,
    [`PATCH /api/v1/meals/${mealId}`]: (request) => (up && patch ? patch(request) : problem(503)),
  })
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))
  expect((await meal().findByRole('alert')).textContent).toContain(
    'Not saved. The server failed to answer.',
  )
  up = true
  fireEvent.click(meal().getByRole('button', { name: 'Try again' }))

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(meal().queryByText('Egg')).toBeNull()
})

test('a change of an item changed meanwhile (409) is refused, with nothing to try again', async () => {
  fakeServer(mealServer({ [`PATCH /api/v1/meals/${mealId}`]: () => problem(409) }).routes)
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect((await meal().findByRole('alert')).textContent).toBe(
    'The server did not take this change.',
  )
  expect(meal().queryByRole('button', { name: 'Try again' })).toBeNull()
})

test('a meal already gone when its delete is sent again is taken for deleted', async () => {
  const server = mealServer()
  let deleted = false
  fakeServer({
    ...server.routes,
    'GET /api/v1/days/today': () => Response.json(day({ meals: deleted ? [] : [breakfast] })),
    // The first delete deletes, its answer lost; the second finds none.
    [`DELETE /api/v1/meals/${mealId}`]: () => {
      if (!deleted) {
        deleted = true
        return Promise.reject(new TypeError('Failed to fetch'))
      }
      return problem(404)
    },
  })
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Delete the meal' }))
  fireEvent.click(meal().getByRole('button', { name: 'Delete' }))
  fireEvent.click(await meal().findByRole('button', { name: 'Try again' }))

  expect(await screen.findByText('Nothing logged yet.')).toBeDefined()
  expect(screen.queryByRole('alert')).toBeNull()
})

test('the focus goes into the meal when it opens, to keeping it when asked, and back to the pencil', async () => {
  fakeServer(mealServer().routes)
  renderAt('/')

  await edit()
  await waitFor(() => {
    expect(document.activeElement).toBe(meal().getByLabelText('What it was'))
  })
  fireEvent.click(meal().getByRole('button', { name: 'Delete the meal' }))
  await waitFor(() => {
    expect(document.activeElement).toBe(meal().getByRole('button', { name: 'Keep it' }))
  })
  fireEvent.click(meal().getByRole('button', { name: 'Cancel' }))
  await waitFor(() => {
    expect(document.activeElement).toBe(
      meal().getByRole('button', { name: 'Change the meal: Breakfast' }),
    )
  })
})

test('an item removed here and elsewhere leaves the meal its others', async () => {
  const server = mealServer()
  let gone = false
  fakeServer({
    ...server.routes,
    'GET /api/v1/days/today': () =>
      Response.json(day({ meals: [{ ...breakfast, items: gone ? [oats] : [oats, egg] }] })),
  })
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  gone = true
  focusManager.setFocused(false)
  focusManager.setFocused(true)

  await waitFor(() => {
    expect(meal().queryByText('Egg')).toBeNull()
  })
  expect(meal().queryByText('A meal keeps at least one item: delete the meal instead.')).toBeNull()
  expect(meal().getByRole('button', { name: 'Save the meal' }).hasAttribute('disabled')).toBe(false)
})

test('an item added elsewhere while the meal is open can be changed too', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  // A model adds a milk; the window gets the focus back.
  const milk: Item = { ...oats, id: '0193c3a5-0000-7000-8000-0000000000b6', name: 'Milk' }
  server.elsewhere((m) => ({ ...m, items: [...m.items, milk] }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  const field = await meal().findByLabelText('Amount: Milk')
  expect(field).toHaveProperty('value', '80')
  fireEvent.change(field, { target: { value: '250' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          { op: 'replace', item_id: milk.id, item: { food_id: oatsFood, amount: 250, unit: 'g' } },
        ],
      },
    ])
  })
})

test('an item changed elsewhere says so, and its field keeps what it opened with', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  // The oats go from 80 g to 2 servings of a cup elsewhere.
  server.elsewhere((m) => ({
    ...m,
    items: m.items.map((i) =>
      i.id === oats.id ? { ...i, quantity: { serving: 'cup', servings: 2 } } : i,
    ),
  }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)

  expect(await meal().findByText('Changed elsewhere since this opened.')).toBeDefined()
  expect(meal().getByLabelText('Amount: Oats')).toHaveProperty('value', '80')
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '90' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  // The number typed means what the field showed: grams.
  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          { op: 'replace', item_id: oats.id, item: { food_id: oatsFood, amount: 90, unit: 'g' } },
        ],
      },
    ])
  })
})

test('a change tried again after it failed sends the meal as it is typed then', async () => {
  const server = mealServer()
  let up = false
  const patch = server.routes[`PATCH /api/v1/meals/${mealId}`]
  fakeServer({
    ...server.routes,
    [`PATCH /api/v1/meals/${mealId}`]: (request) => (up && patch ? patch(request) : problem(503)),
  })
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))
  await meal().findByRole('alert')
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '95' } })
  up = true
  fireEvent.click(meal().getByRole('button', { name: 'Try again' }))

  expect(await meal().findByText('95 g')).toBeDefined()
  expect(server.sent.at(-1)?.body).toEqual({
    ops: [
      { op: 'replace', item_id: oats.id, item: { food_id: oatsFood, amount: 95, unit: 'g' } },
      { op: 'remove', item_id: egg.id },
    ],
  })
})

test('a change of a meal deleted elsewhere (404) asks the day again, and the meal goes', async () => {
  let gone = false
  fakeServer({
    ...mealServer().routes,
    'GET /api/v1/days/today': () => Response.json(day({ meals: gone ? [] : [breakfast] })),
    [`PATCH /api/v1/meals/${mealId}`]: () => {
      gone = true
      return problem(404)
    },
  })
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(await screen.findByText('Nothing logged yet.')).toBeDefined()
})

test('an item of another food now, changed elsewhere, is sent as it is now', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  // A model puts rice in the oats' place, under the item's id.
  const rice = '0193c3a5-0000-7000-8000-0000000000f7'
  server.elsewhere((m) => ({
    ...m,
    items: m.items.map((i) => (i.id === oats.id ? { ...i, food_id: rice, name: 'Rice' } : i)),
  }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  const field = await meal().findByLabelText('Amount: Rice')
  expect(meal().getByText('Changed elsewhere since this opened.')).toBeDefined()
  fireEvent.change(field, { target: { value: '100' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: oats.id,
            item: { food_id: rice, amount: 100, unit: 'g' },
          },
        ],
      },
    ])
  })
})

test('a try again keeps the meal its one item', async () => {
  const server = mealServer({ [`PATCH /api/v1/meals/${mealId}`]: () => problem(503) })
  const fetch = fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))
  await meal().findByRole('button', { name: 'Try again' })
  for (const name of ['Oats', 'Banana bread', 'Salad from the canteen']) {
    fireEvent.click(meal().getByRole('button', { name: `Remove: ${name}` }))
  }
  fireEvent.click(meal().getByRole('button', { name: 'Try again' }))

  // Nothing is sent: a change sent would be saving at once.
  expect(meal().queryByText('Saving…')).toBeNull()
  expect(meal().getByRole('button', { name: 'Try again' })).toBeDefined()
  expect(
    fetch.mock.calls.filter(([r]) => r instanceof Request && r.method === 'PATCH'),
  ).toHaveLength(1)
  expect(meal().getByText('A meal keeps at least one item: delete the meal instead.')).toBeDefined()
})

test('an item of another food now starts its field as it is now', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  // Two portions of a stew in the oats' place, under the item's id.
  const stew = '0193c3a5-0000-7000-8000-0000000000f8'
  server.elsewhere((m) => ({
    ...m,
    items: m.items.map((i) =>
      i.id === oats.id
        ? { ...i, food_id: stew, name: 'Stew', note: 'spicy', quantity: { portions: 2 } }
        : i,
    ),
  }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  const field = await meal().findByLabelText('Amount: Stew')
  expect(field).toHaveProperty('value', '2')
  fireEvent.change(field, { target: { value: '3' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          { op: 'replace', item_id: oats.id, item: { food_id: stew, note: 'spicy', portions: 3 } },
        ],
      },
    ])
  })
})

test("an estimate's values are changed, and only those typed anew are new", async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const kcal = meal().getByLabelText('Energy (kcal): Salad from the canteen')
  expect(kcal).toHaveProperty('value', '250')
  expect(meal().getByLabelText('Fat (g): Salad from the canteen')).toHaveProperty('value', '18')
  fireEvent.change(kcal, { target: { value: '320' } })
  fireEvent.change(meal().getByLabelText('Protein (g): Salad from the canteen'), {
    target: { value: '9,5' },
  })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: salad.id,
            item: {
              name: 'Salad from the canteen',
              nutrients: { kcal: 320, protein: 9.5, carbs: 12, fat: 18 },
            },
          },
        ],
      },
    ])
  })
})

test('a value of an estimate that is no number is not sent', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const carbs = meal().getByLabelText('Carbs (g): Salad from the canteen')
  fireEvent.change(carbs, { target: { value: 'lots' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(await meal().findByRole('alert')).toBeDefined()
  expect(carbs.getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])
})

test('an estimate changed elsewhere keeps that change, and its other values', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  // A model sets the salad's fat to 20 g, with its fibre, and a note.
  server.elsewhere((m) => ({
    ...m,
    items: m.items.map((i) =>
      i.id === salad.id
        ? { ...i, note: 'with dressing', nutrients: { ...i.nutrients, fat: 20, fiber: 5 } }
        : i,
    ),
  }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)

  expect(await meal().findByText('Changed elsewhere since this opened.')).toBeDefined()
  fireEvent.change(meal().getByLabelText('Energy (kcal): Salad from the canteen'), {
    target: { value: '300' },
  })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: salad.id,
            item: {
              name: 'Salad from the canteen',
              note: 'with dressing',
              nutrients: { kcal: 300, protein: 8, carbs: 12, fat: 20, fiber: 5 },
            },
          },
        ],
      },
    ])
  })
})

test("a meal's description, time and note are changed, the time within its day", async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  expect(meal().getByLabelText('What it was')).toHaveProperty('value', 'Breakfast')
  // 15:45 UTC is 08:45 in Los Angeles, the user's time zone.
  expect(meal().getByLabelText('Time')).toHaveProperty('value', '08:45')
  fireEvent.change(meal().getByLabelText('What it was'), { target: { value: ' Brunch ' } })
  fireEvent.change(meal().getByLabelText('Time'), { target: { value: '10:15' } })
  fireEvent.click(meal().getByRole('button', { name: 'Add a note' }))
  fireEvent.change(meal().getByLabelText('Note'), { target: { value: 'after the gym\n' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      { description: 'Brunch', eaten_at: '2026-10-23T17:15:00.000Z', note: 'after the gym' },
    ])
  })
})

test("a time before the day's start is of the night after it", async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  // The user's day starts at 4: 02:30 is the night after the 23rd.
  fireEvent.change(meal().getByLabelText('Time'), { target: { value: '02:30' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([{ eaten_at: '2026-10-24T09:30:00.000Z' }])
  })
})

test('words left as they were, or only spaced, and a cleared time send nothing', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('What it was'), { target: { value: 'Breakfast ' } })
  fireEvent.change(meal().getByLabelText('Time'), { target: { value: '' } })
  fireEvent.click(meal().getByRole('button', { name: 'Add a note' }))
  fireEvent.change(meal().getByLabelText('Note'), { target: { value: '  ' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(server.sent).toEqual([])
})

test('a description changed elsewhere stays when only the note is changed here', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  server.elsewhere((m) => ({ ...m, description: 'Second breakfast' }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  fireEvent.click(meal().getByRole('button', { name: 'Add a note' }))
  fireEvent.change(meal().getByLabelText('Note'), { target: { value: 'big one' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([{ note: 'big one' }])
  })
})

test("a meal's note is shown on its card", async () => {
  const server = mealServer()
  server.elsewhere((m) => ({ ...m, note: 'before the run' }))
  fakeServer(server.routes)
  renderAt('/')

  expect(
    await within(await screen.findByRole('article')).findByText('before the run'),
  ).toBeDefined()
})

const rye: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000f9',
  name: 'Rye bread',
  basis: '100g',
  source: 'label',
  archived: false,
  nutrients: { kcal: 259, protein: 8.5, carbs: 48, fat: 3.3 },
}

// withFoods is a meal's server that finds the rye bread.
function withFoods(answer: Partial<Record<string, Handler>> = {}) {
  return mealServer({
    'GET /api/v1/foods': (request) =>
      Response.json({
        foods: new URL(request.url).searchParams.get('query') ? [rye] : [],
        unfound: [],
      }),
    ...answer,
  })
}

const anId = expect.stringMatching(/^[0-9a-f]{8}-[0-9a-f]{4}-7/) as unknown

test("a food of the user's and an estimate are added to the meal, each under its own id", async () => {
  const server = withFoods()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await meal().findByRole('button', { name: /Rye bread/ }))
  // A food never eaten comes as 100 of its unit.
  const amount = meal().getByLabelText('Amount: Rye bread')
  expect(amount).toHaveProperty('value', '100')
  expect(document.activeElement).toBe(amount)
  fireEvent.change(amount, { target: { value: '60' } })
  fireEvent.click(meal().getByRole('button', { name: 'Add an estimate' }))
  fireEvent.change(meal().getByLabelText('What it is'), { target: { value: 'Butter' } })
  for (const [label, value] of [
    ['Energy (kcal)', '70'],
    ['Protein (g)', '0'],
    ['Carbs (g)', '0'],
    ['Fat (g)', '8'],
  ] as const) {
    fireEvent.change(meal().getByLabelText(label), { target: { value } })
  }
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          { op: 'add', item: { id: anId, food_id: rye.id, amount: 60, unit: 'g' } },
          {
            op: 'add',
            item: {
              id: anId,
              name: 'Butter',
              nutrients: { kcal: 70, protein: 0, carbs: 0, fat: 8 },
            },
          },
        ],
      },
    ])
  })
})

test('an item added that cannot be sent as it is holds the meal back', async () => {
  const server = withFoods()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Add an estimate' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(
    await meal().findByText(
      'An amount is more than 0, and an estimate has a name and its values, such as 72.5.',
    ),
  ).toBeDefined()
  expect(server.sent).toEqual([])
  // Removed, it holds nothing back.
  fireEvent.click(meal().getByRole('button', { name: 'Remove: an estimate' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))
  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(server.sent).toEqual([])
})

test('a meal of its items all removed keeps the one added in their place', async () => {
  const server = withFoods()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  for (const name of ['Oats', 'Banana bread', 'Egg', 'Salad from the canteen']) {
    fireEvent.click(meal().getByRole('button', { name: `Remove: ${name}` }))
  }
  expect(meal().getByRole('button', { name: 'Save the meal' }).hasAttribute('disabled')).toBe(true)
  fireEvent.change(meal().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await meal().findByRole('button', { name: /Rye bread/ }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          { op: 'remove', item_id: oats.id },
          { op: 'remove', item_id: bread.id },
          { op: 'remove', item_id: egg.id },
          { op: 'remove', item_id: salad.id },
          { op: 'add', item: { id: anId, food_id: rye.id, amount: 100, unit: 'g' } },
        ],
      },
    ])
  })
})

test('an item added and sent again after a failure goes under the same id', async () => {
  let up = false
  const server = withFoods()
  const patch = server.routes[`PATCH /api/v1/meals/${mealId}`]
  // Each body sent, the one that failed too.
  const bodies: unknown[] = []
  fakeServer({
    ...server.routes,
    [`PATCH /api/v1/meals/${mealId}`]: async (request) => {
      bodies.push(await request.clone().json())
      return up && patch ? patch(request) : problem(503)
    },
  })
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await meal().findByRole('button', { name: /Rye bread/ }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))
  const again = await meal().findByRole('button', { name: 'Try again' })
  // It may have been added: it holds still until it is sent again.
  expect(meal().getByLabelText('Amount: Rye bread').matches(':disabled')).toBe(true)
  expect(meal().getByRole('button', { name: 'Remove: Rye bread' }).matches(':disabled')).toBe(true)
  up = true
  fireEvent.click(again)

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(bodies).toHaveLength(2)
  expect(bodies[1]).toEqual(bodies[0])
})

test('an estimate removed and kept again keeps the values typed in it', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Energy (kcal): Salad from the canteen'), {
    target: { value: '300' },
  })
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Salad from the canteen' }))
  fireEvent.click(meal().getByRole('button', { name: 'Keep: Salad from the canteen' }))
  expect(meal().getByLabelText('Energy (kcal): Salad from the canteen')).toHaveProperty(
    'value',
    '300',
  )
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: salad.id,
            item: {
              name: 'Salad from the canteen',
              nutrients: { kcal: 300, protein: 8, carbs: 12, fat: 18 },
            },
          },
        ],
      },
    ])
  })
})

test('items added elsewhere, then corrected elsewhere, are not undone by fields left as they came', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  // A model adds a milk and a soup; the window gets the focus back.
  const milk: Item = { ...oats, id: '0193c3a5-0000-7000-8000-0000000000b6', name: 'Milk' }
  const soup: Item = { ...salad, id: '0193c3a5-0000-7000-8000-0000000000b7', name: 'Soup' }
  server.elsewhere((m) => ({ ...m, items: [...m.items, milk, soup] }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  expect(await meal().findByLabelText('Amount: Milk')).toHaveProperty('value', '80')
  expect(meal().getByLabelText('Energy (kcal): Soup')).toHaveProperty('value', '250')
  // And corrects them: 120 g of milk, a soup of 300 kcal.
  server.elsewhere((m) => ({
    ...m,
    items: m.items.map((i) =>
      i.id === milk.id
        ? { ...i, amount: 120, quantity: { amount: 120, unit: 'g' } }
        : i.id === soup.id
          ? { ...i, nutrients: { ...i.nutrients, kcal: 300 } }
          : i,
    ),
  }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  await waitFor(() => {
    expect(meal().getAllByText('Changed elsewhere since this opened.')).toHaveLength(2)
  })
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([{ ops: [{ op: 'remove', item_id: egg.id }] }])
  })
})

test("an item added by a change that got no answer is the meal's, and not added again", async () => {
  let up = false
  const server = withFoods()
  const patch = server.routes[`PATCH /api/v1/meals/${mealId}`]
  fakeServer({
    ...server.routes,
    [`PATCH /api/v1/meals/${mealId}`]: async (request) => {
      if (up && patch) {
        return patch(request)
      }
      // Taken, but its answer is lost: the meal has the bread now.
      const added = /"op":"add","item":\{"id":"([^"]+)"/.exec(await request.text())?.[1]
      server.elsewhere((m) => ({
        ...m,
        items: [
          ...m.items,
          { ...oats, id: added ?? '', food_id: rye.id, name: 'Rye bread', amount: 100 },
        ],
      }))
      return problem(503)
    },
  })
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await meal().findByRole('button', { name: /Rye bread/ }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))
  const again = await meal().findByRole('button', { name: 'Try again' })
  // The window gets the focus back: the day comes with the bread in it.
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  // One field of it, the meal's, which can be changed again.
  await waitFor(() => {
    expect(meal().getByLabelText('Amount: Rye bread').matches(':disabled')).toBe(false)
  })
  // Unwanted after all: removed, and not added again with the retry.
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Rye bread' }))
  up = true
  fireEvent.click(again)

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  const retried = server.sent.at(-1)?.body
  expect(opsOf(retried).map((o) => o.op)).toEqual(['remove'])
  expect(JSON.stringify(retried)).not.toContain('"add"')
})

test('an item added by a change that got no answer, then removed elsewhere, stays removed', async () => {
  let up = false
  let addedId = ''
  const server = withFoods()
  const patch = server.routes[`PATCH /api/v1/meals/${mealId}`]
  fakeServer({
    ...server.routes,
    [`PATCH /api/v1/meals/${mealId}`]: async (request) => {
      if (up && patch) {
        return patch(request)
      }
      // Taken, but its answer is lost: the meal has the bread now.
      addedId = /"op":"add","item":\{"id":"([^"]+)"/.exec(await request.text())?.[1] ?? ''
      server.elsewhere((m) => ({
        ...m,
        items: [...m.items, { ...oats, id: addedId, food_id: rye.id, name: 'Rye bread' }],
      }))
      return problem(503)
    },
  })
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await meal().findByRole('button', { name: /Rye bread/ }))
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))
  const again = await meal().findByRole('button', { name: 'Try again' })
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  await waitFor(() => {
    expect(meal().getByLabelText('Amount: Rye bread').matches(':disabled')).toBe(false)
  })
  // A model removes the bread.
  server.elsewhere((m) => ({ ...m, items: m.items.filter((i) => i.id !== addedId) }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  await waitFor(() => {
    expect(meal().queryByLabelText('Amount: Rye bread')).toBeNull()
  })
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Egg' }))
  up = true
  fireEvent.click(again)

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(server.sent.at(-1)?.body).toEqual({ ops: [{ op: 'remove', item_id: egg.id }] })
})

test("an estimate's name is changed, and one cleared keeps the name it had", async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const name = meal().getByLabelText('What it is: Salad from the canteen')
  expect(name).toHaveProperty('value', 'Salad from the canteen')
  fireEvent.change(name, { target: { value: ' Caesar salad ' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: salad.id,
            item: {
              name: 'Caesar salad',
              nutrients: { kcal: 250, protein: 8, carbs: 12, fat: 18 },
            },
          },
        ],
      },
    ])
  })
})

test('an estimate whose name is cleared sends nothing', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('What it is: Salad from the canteen'), {
    target: { value: ' ' },
  })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(server.sent).toEqual([])
})

test('a meal is moved to a day before, at its time or another', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  expect(meal().getByLabelText('Day')).toHaveProperty('value', '2026-10-23')
  fireEvent.change(meal().getByLabelText('Day'), { target: { value: '2026-10-22' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  // 08:45 in Los Angeles, a day before.
  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([{ eaten_at: '2026-10-22T15:45:00.000Z' }])
  })
})

test('a meal moved to a day and a time is sent as both', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Day'), { target: { value: '2026-10-21' } })
  fireEvent.change(meal().getByLabelText('Time'), { target: { value: '19:00' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([{ eaten_at: '2026-10-22T02:00:00.000Z' }])
  })
})

test('a day after today is not sent, and says why; a cleared day keeps its own', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const dayField = meal().getByLabelText('Day')
  fireEvent.change(dayField, { target: { value: '2026-10-24' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(
    (await meal().findByText('A meal is of today or a day before.')).getAttribute('role'),
  ).toBe('alert')
  expect(dayField.getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])

  fireEvent.change(dayField, { target: { value: '' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))
  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(server.sent).toEqual([])
})

// food is a food of the user's, as GET /api/v1/foods/{id} answers.
function food(id: string, over: Partial<Food>): Food {
  return {
    id,
    name: 'A food',
    basis: '100g',
    source: 'label',
    archived: false,
    nutrients: { kcal: 100, protein: 1, carbs: 1, fat: 1 },
    ...over,
  }
}

// foodsOf are the routes of the foods of the breakfast's oats and bread.
function foodsOf(): Partial<Record<string, Handler>> {
  return {
    [`GET /api/v1/foods/${oatsFood}`]: () =>
      Response.json(food(oatsFood, { name: 'Oats', servings: [{ name: 'cup', amount: 90 }] })),
    [`GET /api/v1/foods/${bread.food_id ?? ''}`]: () =>
      Response.json(
        food(bread.food_id ?? '', { name: 'Banana bread', basis: 'portion', portion_grams: 120 }),
      ),
  }
}

test('an amount is changed to another unit of its food, worked out anew', async () => {
  const server = mealServer(foodsOf())
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const unit = await meal().findByLabelText('Unit: Oats')
  expect(unit).toHaveProperty('value', 'g')
  // 80 g are 0.89 of a cup of 90 g.
  fireEvent.change(unit, { target: { value: 'serving:cup' } })
  expect(meal().getByLabelText('Amount: Oats')).toHaveProperty('value', '0.89')
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '1' } })
  // 1.5 portions of 120 g are 180 g.
  fireEvent.change(await meal().findByLabelText('Unit: Banana bread'), { target: { value: 'g' } })
  expect(meal().getByLabelText('Amount: Banana bread')).toHaveProperty('value', '180')
  // The egg's food did not come: its unit stays as it is.
  expect(meal().queryByLabelText('Unit: Egg')).toBeNull()
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: oats.id,
            item: { food_id: oatsFood, serving: 'cup', servings: 1 },
          },
          {
            op: 'replace',
            item_id: bread.id,
            item: { food_id: bread.food_id, note: 'home-made', amount: 180, unit: 'g' },
          },
        ],
      },
    ])
  })
})

test('a unit chosen and chosen back sends the amount as it was', async () => {
  const server = mealServer(foodsOf())
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const unit = await meal().findByLabelText('Unit: Oats')
  fireEvent.change(unit, { target: { value: 'serving:cup' } })
  fireEvent.change(unit, { target: { value: 'g' } })
  // Not 0.89 of a cup back in grams, 80.1: the 80 it was.
  expect(meal().getByLabelText('Amount: Oats')).toHaveProperty('value', '80')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(server.sent).toEqual([])
})

// swap finds the rye bread for the item named name.
async function swap(name: string) {
  fireEvent.click(meal().getByRole('button', { name: `Another food in place of: ${name}` }))
  fireEvent.change(meal().getByLabelText(`A food of yours in place of ${name}`), {
    target: { value: 'rye' },
  })
  fireEvent.click(await meal().findByRole('button', { name: /^Rye bread/ }))
}

test("another food takes an item's place, under its id and with its note", async () => {
  const server = withFoods()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  await swap('Banana bread')
  expect(meal().getByText('In place of Banana bread:')).toBeDefined()
  const amount = meal().getByLabelText('Amount: Rye bread')
  expect(document.activeElement).toBe(amount)
  fireEvent.change(amount, { target: { value: '60' } })
  // An estimate's place too.
  await swap('Salad from the canteen')
  fireEvent.change(meal().getAllByLabelText('Amount: Rye bread')[1] ?? amount, {
    target: { value: '40' },
  })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: bread.id,
            item: { food_id: rye.id, amount: 60, unit: 'g', note: 'home-made' },
          },
          { op: 'replace', item_id: salad.id, item: { food_id: rye.id, amount: 40, unit: 'g' } },
        ],
      },
    ])
  })
})

test('another food taken back leaves the item as it was', async () => {
  const server = withFoods()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '90' } })
  await swap('Oats')
  expect(meal().getByLabelText('Amount: Oats', { selector: 'input' }).closest('li')?.hidden).toBe(
    true,
  )
  fireEvent.click(meal().getByRole('button', { name: 'Remove: Rye bread' }))
  expect(meal().getByLabelText('Amount: Oats')).toHaveProperty('value', '90')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          { op: 'replace', item_id: oats.id, item: { food_id: oatsFood, amount: 90, unit: 'g' } },
        ],
      },
    ])
  })
})

test('another food of no amount holds the meal back', async () => {
  const server = withFoods()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  await swap('Oats')
  fireEvent.change(meal().getByLabelText('Amount: Rye bread'), { target: { value: '0' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(
    await meal().findByText(
      'An amount is more than 0, and an estimate has a name and its values, such as 72.5.',
    ),
  ).toBeDefined()
  expect(server.sent).toEqual([])
})

test("an estimate is saved as a food of the user's, a portion of it, as it is typed", async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const save = meal().getByRole('button', {
    name: 'Save as a food of yours: Salad from the canteen',
  })
  fireEvent.click(save)
  expect(save.getAttribute('aria-pressed')).toBe('true')
  expect(meal().getByText('Saved as a food of yours, a portion of it eaten.')).toBeDefined()
  fireEvent.change(meal().getByLabelText('What it is: Salad from the canteen'), {
    target: { value: 'Canteen salad' },
  })
  fireEvent.change(meal().getByLabelText('Energy (kcal): Salad from the canteen'), {
    target: { value: '260' },
  })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: salad.id,
            item: {
              food: {
                name: 'Canteen salad',
                basis: 'portion',
                source: 'estimate',
                nutrients: { kcal: 260, protein: 8, carbs: 12, fat: 18 },
              },
              portions: 1,
            },
          },
        ],
      },
    ])
  })
})

test('an estimate saved as a food and taken back sends nothing', async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const save = meal().getByRole('button', {
    name: 'Save as a food of yours: Salad from the canteen',
  })
  fireEvent.click(save)
  fireEvent.click(save)
  expect(save.getAttribute('aria-pressed')).toBe('false')
  expect(meal().queryByText('Saved as a food of yours, a portion of it eaten.')).toBeNull()
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(server.sent).toEqual([])
})

test('a food of the same name the user has already is said to be kept', async () => {
  const server = mealServer({
    [`PATCH /api/v1/meals/${mealId}`]: () =>
      Response.json({
        meal: breakfast,
        day: { ...day(), meals: undefined, metrics: undefined },
        warnings: [{ code: 'food_kept' }],
      }),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.click(
    meal().getByRole('button', { name: 'Save as a food of yours: Salad from the canteen' }),
  )
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(
    await meal().findByText('A food of yours was logged with its own values, not those sent.'),
  ).toBeDefined()
})

test("a serving given in another case is the food's, worked out from its grams", async () => {
  const server = mealServer({
    ...foodsOf(),
    [`GET /api/v1/foods/${egg.food_id ?? ''}`]: () =>
      Response.json(
        food(egg.food_id ?? '', { name: 'Egg', servings: [{ name: 'Egg', amount: 50 }] }),
      ),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  // Given as "egg", the food's "Egg": one choice of it, not two.
  const unit = await meal().findByLabelText('Unit: Egg')
  expect(unit).toHaveProperty('value', 'serving:Egg')
  expect(
    within(unit)
      .getAllByRole('option')
      .map((o) => o.textContent),
  ).toEqual(['Egg', 'g'])
  fireEvent.change(unit, { target: { value: 'g' } })
  expect(meal().getByLabelText('Amount: Egg')).toHaveProperty('value', '100')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: egg.id,
            item: { food_id: egg.food_id, amount: 100, unit: 'g' },
          },
        ],
      },
    ])
  })
})

test('a small amount in a big unit is not worked out to nothing', async () => {
  const server = mealServer({
    [`GET /api/v1/foods/${oatsFood}`]: () =>
      Response.json(food(oatsFood, { name: 'Oats', servings: [{ name: 'sack', amount: 5000 }] })),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const unit = await meal().findByLabelText('Unit: Oats')
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '1' } })
  fireEvent.change(unit, { target: { value: 'serving:sack' } })
  expect(meal().getByLabelText('Amount: Oats')).toHaveProperty('value', '0.0002')
})

test('a unit chosen for an item whose food changes elsewhere starts again with the new one', async () => {
  const server = mealServer(foodsOf())
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(await meal().findByLabelText('Unit: Oats'), { target: { value: 'serving:cup' } })
  // A model puts two portions of the banana bread in the oats' place.
  server.elsewhere((m) => ({
    ...m,
    items: m.items.map((i) =>
      i.id === oats.id
        ? { ...i, food_id: bread.food_id ?? '', name: 'Rye loaf', quantity: { portions: 2 } }
        : i,
    ),
  }))
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  expect(await meal().findByLabelText('Unit: Rye loaf')).toHaveProperty('value', 'portion')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(server.sent).toEqual([])
})

test('an estimate saved as a food is found by a search that was open', async () => {
  let saved = false
  const canteen: Food = {
    ...rye,
    id: '0193c3a5-0000-7000-8000-0000000000fa',
    name: 'Canteen salad',
  }
  const server = mealServer({
    'GET /api/v1/foods': () => Response.json({ foods: saved ? [canteen] : [] }),
  })
  const patch = server.routes[`PATCH /api/v1/meals/${mealId}`]
  fakeServer({
    ...server.routes,
    [`PATCH /api/v1/meals/${mealId}`]: (request) => {
      saved = true
      return patch ? patch(request) : Response.error()
    },
  })
  renderAt('/')

  // A meal being logged, its search finding nothing yet.
  fireEvent.click(await screen.findByRole('button', { name: 'Log a meal' }))
  const log = within(screen.getByRole('form', { name: 'A new meal' }))
  fireEvent.change(log.getByLabelText('Find a food of yours'), { target: { value: 'canteen' } })
  expect(await log.findByText('No food of yours matches: give it as an estimate.')).toBeDefined()
  await edit()
  fireEvent.click(
    meal().getByRole('button', { name: 'Save as a food of yours: Salad from the canteen' }),
  )
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(await log.findByRole('button', { name: /^Canteen salad/ })).toBeDefined()
})

test('an amount worked out in another unit is sent as worked out, not as rounded to be shown', async () => {
  const server = mealServer({
    [`GET /api/v1/foods/${oatsFood}`]: () =>
      Response.json(food(oatsFood, { name: 'Oats', servings: [{ name: 'sack', amount: 5000 }] })),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '30' } })
  fireEvent.change(await meal().findByLabelText('Unit: Oats'), {
    target: { value: 'serving:sack' },
  })
  // 30 g are 0.006 of a sack, shown as 0.01.
  expect(meal().getByLabelText('Amount: Oats')).toHaveProperty('value', '0.01')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: oats.id,
            item: { food_id: oatsFood, serving: 'sack', servings: 0.006 },
          },
        ],
      },
    ])
  })
})

test('an amount typed after a unit worked it out is sent as typed', async () => {
  const server = mealServer({
    [`GET /api/v1/foods/${oatsFood}`]: () =>
      Response.json(food(oatsFood, { name: 'Oats', servings: [{ name: 'sack', amount: 5000 }] })),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '30' } })
  fireEvent.change(await meal().findByLabelText('Unit: Oats'), {
    target: { value: 'serving:sack' },
  })
  // Typed again as it was shown: what is typed, not what was worked out.
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '1' } })
  fireEvent.change(meal().getByLabelText('Amount: Oats'), { target: { value: '0.01' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: oats.id,
            item: { food_id: oatsFood, serving: 'sack', servings: 0.01 },
          },
        ],
      },
    ])
  })
})

test('a unit chosen and chosen back of no even ratio sends nothing', async () => {
  const server = mealServer({
    [`GET /api/v1/foods/${oatsFood}`]: () =>
      Response.json(food(oatsFood, { name: 'Oats', servings: [{ name: 'scoop', amount: 77 }] })),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const unit = await meal().findByLabelText('Unit: Oats')
  fireEvent.change(unit, { target: { value: 'serving:scoop' } })
  fireEvent.change(unit, { target: { value: 'g' } })
  expect(meal().getByLabelText('Amount: Oats')).toHaveProperty('value', '80')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(server.sent).toEqual([])
})

test('an amount not typed is worked out from what it is, not from what its field shows', async () => {
  const server = mealServer({
    [`GET /api/v1/foods/${bread.food_id ?? ''}`]: () =>
      Response.json(
        food(bread.food_id ?? '', { name: 'Banana bread', basis: 'portion', portion_grams: 500 }),
      ),
  })
  // A third of a portion, which its field shows as 0.333.
  server.elsewhere((m) => ({
    ...m,
    items: m.items.map((i) => (i.id === bread.id ? { ...i, quantity: { portions: 1 / 3 } } : i)),
  }))
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(await meal().findByLabelText('Unit: Banana bread'), { target: { value: 'g' } })
  expect(meal().getByLabelText('Amount: Banana bread')).toHaveProperty('value', '166.67')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: bread.id,
            item: { food_id: bread.food_id, note: 'home-made', amount: 500 / 3, unit: 'g' },
          },
        ],
      },
    ])
  })
})

test('a serving chosen that its food no longer has stays chosen, with its amount', async () => {
  let cup = true
  let asked = 0
  const server = mealServer({
    [`GET /api/v1/foods/${oatsFood}`]: () => {
      asked++
      return Response.json(
        food(oatsFood, { name: 'Oats', servings: cup ? [{ name: 'cup', amount: 90 }] : [] }),
      )
    },
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(await meal().findByLabelText('Unit: Oats'), { target: { value: 'serving:cup' } })
  // The cup is removed from the oats elsewhere; the window gets the focus back.
  cup = false
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  await waitFor(() => {
    expect(asked).toBeGreaterThan(1)
  })
  expect(meal().getByLabelText('Unit: Oats')).toHaveProperty('value', 'serving:cup')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: oats.id,
            item: { food_id: oatsFood, serving: 'cup', servings: 80 / 90 },
          },
        ],
      },
    ])
  })
})

test('a serving renamed elsewhere keeps the item in it, untouched', async () => {
  let name = 'egg'
  let asked = 0
  const server = mealServer({
    [`GET /api/v1/foods/${egg.food_id ?? ''}`]: () => {
      asked++
      return Response.json(
        food(egg.food_id ?? '', { name: 'Egg', servings: [{ name, amount: 50 }] }),
      )
    },
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  expect(await meal().findByLabelText('Unit: Egg')).toHaveProperty('value', 'serving:egg')
  name = 'Egg'
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  await waitFor(() => {
    expect(meal().getByLabelText('Unit: Egg')).toHaveProperty('value', 'serving:Egg')
  })
  expect(asked).toBeGreaterThan(1)
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(meal().queryByRole('button', { name: 'Save the meal' })).toBeNull()
  })
  expect(server.sent).toEqual([])
})

test('a unit chosen that cannot be worked out from the last asks for the amount anew', async () => {
  let cup = true
  let asked = 0
  const server = mealServer({
    [`GET /api/v1/foods/${oatsFood}`]: () => {
      asked++
      return Response.json(
        food(oatsFood, { name: 'Oats', servings: cup ? [{ name: 'cup', amount: 90 }] : [] }),
      )
    },
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  const unit = await meal().findByLabelText('Unit: Oats')
  fireEvent.change(unit, { target: { value: 'serving:cup' } })
  // The cup goes from the oats elsewhere: grams cannot be worked out of it.
  cup = false
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  await waitFor(() => {
    expect(asked).toBeGreaterThan(1)
  })
  fireEvent.change(meal().getByLabelText('Unit: Oats'), { target: { value: 'g' } })
  expect(meal().getByLabelText('Amount: Oats')).toHaveProperty('value', '')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  expect(await meal().findByRole('alert')).toBeDefined()
  expect(server.sent).toEqual([])
})

test("the meal's note has its field once asked for, and an item's note is kept", async () => {
  const server = mealServer()
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  // Seldom given: no field until it is asked for, and none for an item.
  expect(meal().queryByLabelText('Note')).toBeNull()
  expect(meal().queryByLabelText(/^Note: /)).toBeNull()
  fireEvent.click(meal().getByRole('button', { name: 'Add a note' }))
  expect(document.activeElement).toBe(meal().getByLabelText('Note'))
  // An item replaced keeps the note it has.
  fireEvent.change(meal().getByLabelText('Amount: Banana bread'), { target: { value: '2' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: bread.id,
            item: { food_id: bread.food_id, note: 'home-made', portions: 2 },
          },
        ],
      },
    ])
  })
})

test("a meal's note it has is changed from its own button", async () => {
  const server = mealServer()
  server.elsewhere((m) => ({ ...m, note: 'before the run' }))
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.click(meal().getByRole('button', { name: 'Change the note' }))
  expect(meal().getByLabelText('Note')).toHaveProperty('value', 'before the run')
  fireEvent.change(meal().getByLabelText('Note'), { target: { value: 'after the run' } })
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([{ note: 'after the run' }])
  })
})

test("a unit chosen for another food in an item's place works its amount out anew", async () => {
  const server = withFoods({
    'GET /api/v1/foods': () =>
      Response.json({ foods: [{ ...rye, servings: [{ name: 'slice', amount: 40 }] }] }),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  await swap('Banana bread')
  const amount = meal().getByLabelText('Amount: Rye bread')
  expect(amount).toHaveProperty('value', '100')
  fireEvent.change(meal().getByLabelText('Unit: Rye bread'), {
    target: { value: 'serving:slice' },
  })
  expect(amount).toHaveProperty('value', '2.5')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'replace',
            item_id: bread.id,
            item: { food_id: rye.id, serving: 'slice', servings: 2.5, note: 'home-made' },
          },
        ],
      },
    ])
  })
})

test('a unit chosen for a food added to the meal works its amount out anew', async () => {
  const server = withFoods({
    'GET /api/v1/foods': () =>
      Response.json({ foods: [{ ...rye, servings: [{ name: 'slice', amount: 40 }] }] }),
  })
  fakeServer(server.routes)
  renderAt('/')

  await edit()
  fireEvent.change(meal().getByLabelText('Find a food of yours'), { target: { value: 'rye' } })
  fireEvent.click(await meal().findByRole('button', { name: /Rye bread/ }))
  fireEvent.change(meal().getByLabelText('Amount: Rye bread'), { target: { value: '60' } })
  fireEvent.change(meal().getByLabelText('Unit: Rye bread'), {
    target: { value: 'serving:slice' },
  })
  expect(meal().getByLabelText('Amount: Rye bread')).toHaveProperty('value', '1.5')
  fireEvent.click(meal().getByRole('button', { name: 'Save the meal' }))

  await waitFor(() => {
    expect(server.sent.map((s) => s.body)).toEqual([
      {
        ops: [
          {
            op: 'add',
            item: { id: anId, food_id: rye.id, serving: 'slice', servings: 1.5 },
          },
        ],
      },
    ])
  })
})
