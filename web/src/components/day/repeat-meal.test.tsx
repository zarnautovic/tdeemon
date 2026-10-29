import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Day, Meal } from '@/api/types'
import { day, fakeServer, problem, renderAt, signedIn, today, type Handler } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  localStorage.clear()
})

const yesterday = '2026-10-22'
const oatsFood = '0193c3a5-0000-7000-8000-0000000000f1'

const breakfast: Meal = {
  id: '0193c3a5-0000-7000-8000-0000000000a1',
  eaten_at: '2026-10-22T14:30:00Z',
  description: 'Breakfast',
  note: 'before the run',
  client: 'mcp',
  total: { kcal: 550, protein: 19, carbs: 66, fat: 24 },
  items: [
    {
      id: '0193c3a5-0000-7000-8000-0000000000b1',
      food_id: oatsFood,
      name: 'Oats',
      quantity: { amount: 80, unit: 'g' },
      amount: 80,
      unit: 'g',
      nutrients: { kcal: 300, protein: 11, carbs: 54, fat: 6 },
      source: 'label',
    },
    {
      id: '0193c3a5-0000-7000-8000-0000000000b4',
      name: 'Salad from the canteen',
      quantity: { portions: 1 },
      amount: 1,
      unit: 'portion',
      nutrients: { kcal: 250, protein: 8, carbs: 12, fat: 18, fiber: 4 },
      source: 'estimate',
    },
  ],
}

// repeatServer is yesterday with breakfast, and today with meals: a meal
// logged is today's, and each body sent is kept. post answers in place of
// the server when given.
function repeatServer(post?: Handler, meals: Meal[] = []) {
  let todays: Day = day({ date: today, meals })
  const sent: unknown[] = []
  const routes: Record<string, Handler> = {
    ...signedIn(),
    'GET /api/v1/days/today': () => Response.json(todays),
    [`GET /api/v1/days/${today}`]: () => Response.json(todays),
    [`GET /api/v1/days/${yesterday}`]: () =>
      Response.json(day({ date: yesterday, meals: [breakfast] })),
    'POST /api/v1/meals': async (request) => {
      const body: unknown = await request.json()
      sent.push(body)
      if (post) {
        return post(request)
      }
      const meal: Meal = { ...breakfast, id: '0193c3a5-0000-7000-8000-0000000000a2', client: 'web' }
      todays = { ...todays, meals: [...todays.meals, meal] }
      const { meals: _meals, metrics: _metrics, ...summary } = todays
      return Response.json({ meal, day: summary, warnings: [{ code: 'atwater' }] }, { status: 201 })
    },
  }
  fakeServer(routes)
  return { sent, routes }
}

// repeat opens the meal's row, and finds its repeat.
async function repeat() {
  const meal = (await screen.findAllByRole('article'))[0]
  if (meal === undefined) {
    throw new Error('no meal')
  }
  fireEvent.click(within(meal).getByRole('button', { expanded: false }))
  return within(meal).getByRole('button', { name: 'Repeat today: Breakfast' })
}

test("yesterday's meal is repeated today at this time, its toast says which and its kcal, and today is a tap away", async () => {
  const server = repeatServer()
  renderAt(`/?date=${yesterday}`)

  fireEvent.click(await repeat())

  expect(await screen.findByText('Repeated: Breakfast · 550 kcal')).toBeDefined()
  expect(screen.getByText('The kcal do not match the protein, carbs and fat.')).toBeDefined()
  expect(server.sent).toEqual([
    {
      id: expect.stringMatching(/^[0-9a-f]{8}-[0-9a-f]{4}-7/) as unknown,
      description: 'Breakfast',
      note: 'before the run',
      items: [
        { food_id: oatsFood, amount: 80, unit: 'g' },
        {
          name: 'Salad from the canteen',
          nutrients: { kcal: 250, protein: 8, carbs: 12, fat: 18, fiber: 4 },
        },
      ],
    },
  ])
  // Yesterday is as it was: the meal is today's.
  expect(screen.getAllByRole('article')).toHaveLength(1)
  fireEvent.click(screen.getByRole('link', { name: 'See it today' }))
  expect(await screen.findByRole('heading', { name: 'Today' })).toBeDefined()
  expect(within(await screen.findByRole('article')).getByText('Breakfast')).toBeDefined()
})

test('a repeat with no answer is sent again under its id, one meal however often', async () => {
  let fail = true
  const server = repeatServer(() => {
    if (fail) {
      return problem(503)
    }
    return Response.json(
      {
        meal: breakfast,
        day: { ...day(), meals: undefined, metrics: undefined },
        warnings: [],
      },
      { status: 201 },
    )
  })
  renderAt(`/?date=${yesterday}`)

  fireEvent.click(await repeat())
  const again = await screen.findByRole('button', { name: 'Try again' })
  fail = false
  fireEvent.click(again)

  expect(await screen.findByText('Repeated: Breakfast · 550 kcal')).toBeDefined()
  expect(server.sent).toHaveLength(2)
  const [first, second] = server.sent
  expect(second).toEqual(first)
})

test("a repeat the server refuses says why, in the server's words", async () => {
  repeatServer(() => problem(422))
  renderAt(`/?date=${yesterday}`)

  fireEvent.click(await repeat())

  expect(
    await screen.findByText('The server did not take this meal: what went wrong'),
  ).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()
})

test("today's meal repeated needs no way to today", async () => {
  const server = repeatServer(undefined, [breakfast])
  renderAt('/')

  fireEvent.click(await repeat())

  expect(await screen.findByText('Repeated: Breakfast · 550 kcal')).toBeDefined()
  expect(screen.queryByRole('link', { name: 'See it today' })).toBeNull()
  expect(server.sent).toHaveLength(1)
  await waitFor(() => {
    expect(screen.getAllByRole('article')).toHaveLength(2)
  })
})

test("a repeat's toast takes the repeat back, and no other change", async () => {
  const server = repeatServer()
  const undos: unknown[] = []
  fakeServer({
    ...server.routes,
    'POST /api/v1/undo': async (request) => {
      undos.push(await request.json())
      return Response.json({
        undone: {
          action: 'meal.create',
          entity: '0193c3a5-0000-7000-8000-0000000000a2',
          at: '2026-10-23T16:00:00Z',
          client: 'web',
          days: [today],
        },
      })
    },
  })
  renderAt(`/?date=${yesterday}`)

  fireEvent.click(await repeat())
  fireEvent.click(await screen.findByRole('button', { name: 'Undo' }))

  // Taken back on today, which yesterday's page does not show.
  expect(await screen.findByText('Undone: a meal logged.')).toBeDefined()
  expect(screen.getByRole('link', { name: 'See it today' })).toBeDefined()
  expect(undos).toEqual([
    {
      id: expect.any(String) as unknown,
      last: { action: 'meal.create', entity: '0193c3a5-0000-7000-8000-0000000000a2' },
    },
  ])
})
