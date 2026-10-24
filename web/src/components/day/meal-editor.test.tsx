import { focusManager } from '@tanstack/react-query'
import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Day, Item, Meal } from '@/api/types'
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
