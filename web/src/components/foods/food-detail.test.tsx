import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import type { Food } from '@/api/types'
import { setLocale } from '@/i18n/locale'
import { day, fakeServer, renderAt, signedIn, today, type Handler } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

type Use = components['schemas']['FoodUseOutputBody']

const id = '0193c3a5-0000-7000-8000-0000000000f1'

const oats: Food = {
  id,
  name: 'Rolled oats',
  brand: 'Mill',
  basis: '100g',
  source: 'label',
  archived: false,
  nutrients: { kcal: 370, protein: 13, carbs: 60, fat: 7, fiber: 10 },
  servings: [{ name: 'cup', amount: 80 }],
  aliases: ['porridge oats'],
  barcodes: ['3850000000015'],
  note: 'from the label',
}

// eaten is a use of 14 meals, the last of 80 g on a day.
function eaten(on = '2026-10-21', meals = 14): Use {
  return {
    meals,
    last_use: {
      at: `${on}T15:30:00Z`,
      day: on,
      meal_id: '0193c3a5-0000-7000-8000-0000000000a1',
      quantity: { amount: 80, unit: 'g' },
      amount: 80,
      unit: 'g',
    },
  }
}

// foodServer is a server with a food and its use, which takes a save,
// a meal and an undo, and keeps what it was sent.
function foodServer({
  food = oats,
  use = eaten(),
  locale = 'en',
  over = {},
}: {
  food?: Food
  use?: Use
  locale?: 'en' | 'hr'
  over?: Record<string, Handler>
} = {}) {
  let current = food
  const sent: { path: string; body: unknown }[] = []
  async function keep(request: Request) {
    const body: unknown = await request.json()
    sent.push({ path: new URL(request.url).pathname, body })
    return body
  }
  fakeServer({
    ...signedIn(locale),
    'GET /api/v1/foods/list': () => Response.json({ foods: [current], total: 1 }),
    [`GET /api/v1/foods/${food.id}`]: () => Response.json(current),
    [`GET /api/v1/foods/${food.id}/use`]: () => Response.json(use),
    'POST /api/v1/foods': async (request) => {
      const body = await keep(request)
      current = { ...current, ...(typeof body === 'object' ? body : {}) }
      return Response.json(current)
    },
    'POST /api/v1/meals': async (request) => {
      await keep(request)
      const meal = {
        id: '0193c3a5-0000-7000-8000-0000000000a9',
        eaten_at: `${today}T15:30:00Z`,
        description: 'Breakfast',
        client: 'web',
        total: { kcal: 296, protein: 10.4, carbs: 48, fat: 5.6 },
        items: [],
      } as const
      const { meals: _meals, metrics: _metrics, ...summary } = day()
      return Response.json({ meal, day: summary, warnings: [] }, { status: 201 })
    },
    'POST /api/v1/undo': async (request) => {
      await keep(request)
      return Response.json({
        undone: {
          action: 'food.save',
          entity: food.id,
          at: `${today}T16:00:00Z`,
          client: 'web',
          days: [],
        },
      })
    },
    ...over,
  })
  return { sent }
}

// table is the nutrition table's rows, as they read.
function table() {
  const rows = within(screen.getByRole('table', { name: /Nutrition|Nutritivne/ })).getAllByRole(
    'row',
  )
  return rows.map((r) => [...r.children].map((c) => c.textContent).join(': '))
}

test('the read mode opens from the list, and Edit goes to the form', async () => {
  foodServer()
  const router = renderAt('/foods?order=name')

  fireEvent.click(await screen.findByRole('link', { name: /Rolled oats/ }))

  expect(await screen.findByRole('heading', { name: 'Rolled oats', level: 1 })).toBeDefined()
  expect(screen.queryByLabelText('Energy (kcal)')).toBeNull()
  fireEvent.click(screen.getByRole('link', { name: 'Edit' }))

  expect(await screen.findByRole('heading', { name: 'Edit the food' })).toBeDefined()
  expect(screen.getByLabelText('Energy (kcal)')).toHaveProperty('value', '370')
  expect(router.state.location.search).toEqual({ order: 'name', edit: true })
  // Given up, the form goes back to the food it was opened from.
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(await screen.findByRole('heading', { name: 'Rolled oats', level: 1 })).toBeDefined()
  expect(router.state.location.search).toEqual({ order: 'name' })
  expect(router.state.location.pathname).toBe(`/foods/${id}`)
})

test('a food says its energy, where it comes from, and what it brings for 100 g or a serving', async () => {
  foodServer()
  renderAt(`/foods/${id}`)

  expect(await screen.findByRole('heading', { name: 'Rolled oats', level: 1 })).toBeDefined()
  const energy = within(screen.getByRole('region', { name: 'Where the energy comes from' }))
  expect(energy.getByText('370')).toBeDefined()
  expect(energy.getByText('kcal per 100 g')).toBeDefined()
  // Protein's 52 kcal, carbs' 240 and fat's 63 of 355.
  expect(
    energy
      .getAllByRole('term')
      .map((dt, i) => [dt.textContent, energy.getAllByRole('definition')[i]?.textContent]),
  ).toEqual([
    ['Protein', '13 g · 15%'],
    ['Carbs', '60 g · 68%'],
    ['Fat', '7 g · 18%'],
  ])
  expect(table()).toEqual([
    'Energy: 370 kcal',
    'Protein: 13 g',
    'Carbs: 60 g',
    'Fibre: 10 g',
    'Fat: 7 g',
  ])
  expect(screen.getByRole('radio', { name: '100 g' })).toHaveProperty('checked', true)

  fireEvent.click(screen.getByRole('radio', { name: 'cup (80 g)' }))

  expect(table()).toEqual([
    'Energy: 296 kcal',
    'Protein: 10 g',
    'Carbs: 48 g',
    'Fibre: 8 g',
    'Fat: 5.6 g',
  ])
  expect(screen.getByText('80 g · 296 kcal')).toBeDefined()
  expect(screen.getByText('porridge oats')).toBeDefined()
  expect(screen.getByRole('heading', { name: 'Barcodes' })).toBeDefined()
  expect(screen.getByText('3850000000015')).toBeDefined()
  expect(screen.getByText('from the label')).toBeDefined()
  expect(screen.getByText('Label')).toBeDefined()
  expect(await screen.findByText('14× · last Oct 21')).toBeDefined()
})

test('a food eaten last in another year says the year', async () => {
  foodServer({ use: eaten('2024-10-21', 3) })
  renderAt(`/foods/${id}`)

  expect(await screen.findByText('3× · last Oct 21, 2024')).toBeDefined()
})

test('a portion is said in Croatian, of 100 g too when its grams are known', async () => {
  const lasagne: Food = {
    id,
    name: 'Lazanje',
    basis: 'portion',
    source: 'estimate',
    archived: false,
    portion_grams: 450,
    nutrients: { kcal: 2150, protein: 90, carbs: 180, fat: 118, salt: 4.25 },
  }
  foodServer({ food: lasagne, use: { meals: 0 }, locale: 'hr' })
  renderAt(`/foods/${id}`)

  expect(await screen.findByRole('heading', { name: 'Lazanje', level: 1 })).toBeDefined()
  expect(screen.getByText('2.150')).toBeDefined()
  expect(screen.getByText('kcal po porciji')).toBeDefined()
  expect(screen.getAllByText('procjena')).toHaveLength(1)
  expect(table()).toEqual([
    'Energija: 2.150 kcal',
    'Proteini: 90 g',
    'Ugljikohidrati: 180 g',
    'Masti: 118 g',
    'Sol: 4,25 g',
  ])
  fireEvent.click(screen.getByRole('radio', { name: '100 g' }))
  expect(table()[0]).toBe('Energija: 478 kcal')
  expect(await screen.findByText('još nije')).toBeDefined()
  // Not eaten yet: logged as the quick add starts it.
  expect(screen.getByRole('button', { name: 'Upiši' })).toBeDefined()
})

test('a food of many servings picks the amount in a list', async () => {
  const servings = ['slice', 'cup', 'spoon'].map((name, i) => ({ name, amount: 10 * (i + 1) }))
  foodServer({ food: { ...oats, servings } })
  renderAt(`/foods/${id}`)

  fireEvent.change(await screen.findByLabelText('Values for'), { target: { value: '3' } })

  expect(table()[0]).toBe('Energy: 111 kcal')
})

test('Log 80 g opens the meal to log with the food in it, as it was eaten last', async () => {
  const server = foodServer()
  renderAt(`/foods/${id}`)

  fireEvent.click(await screen.findByRole('button', { name: 'Log 80 g' }))

  const sheet = within(await screen.findByRole('dialog', { name: 'New meal' }))
  const basket = within(sheet.getByRole('list', { name: 'In the meal' })).getAllByRole('listitem')
  expect(basket.map((li) => li.textContent.replace('×', ''))).toEqual([
    'Rolled oats · 80 g · 296 kcal',
  ])
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 296 kcal' }))

  expect(await screen.findByText('Logged: Breakfast · 296 kcal')).toBeDefined()
  expect(server.sent).toEqual([
    {
      path: '/api/v1/meals',
      body: {
        id: expect.any(String) as unknown,
        items: [{ id: expect.any(String) as unknown, food_id: id, amount: 80, unit: 'g' }],
      },
    },
  ])
  // The food's page stays, the sheet closed.
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'New meal' })).toBeNull()
  })
  expect(screen.getByRole('heading', { name: 'Rolled oats', level: 1 })).toBeDefined()
})

test('a food archived says so, and its toast takes it back', async () => {
  const server = foodServer()
  renderAt(`/foods/${id}`)

  fireEvent.click(await screen.findByRole('button', { name: 'Archive' }))

  expect(await screen.findByText('Archived: Rolled oats')).toBeDefined()
  // An archived food is brought back, and not logged.
  expect(await screen.findByRole('button', { name: 'Bring back' })).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Log 80 g' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Undo' }))

  expect(await screen.findByText('Undone: a food saved.')).toBeDefined()
  expect(server.sent.map((s) => s.body)).toEqual([
    { ...oats, archived: true },
    { id: expect.any(String) as unknown, last: { action: 'food.save', entity: id } },
  ])
})

test("a recipe's food is archived without its recipe", async () => {
  const curry: Food = {
    id,
    name: 'Chicken curry',
    basis: '100g',
    source: 'recipe',
    archived: false,
    nutrients: { kcal: 81, protein: 14, carbs: 1.4, fat: 2.6 },
    servings: [
      { name: 'recipe', amount: 1000 },
      { name: 'portion', amount: 250 },
    ],
    recipe: {
      weight: 1000,
      yield_g: 1000,
      portions: 4,
      total: { kcal: 810, protein: 140, carbs: 14, fat: 26 },
    },
  }
  const server = foodServer({ food: curry })
  renderAt(`/foods/${id}`)

  fireEvent.click(await screen.findByRole('button', { name: 'Archive' }))

  expect(await screen.findByText('Archived: Chicken curry')).toBeDefined()
  const { recipe: _recipe, ...fields } = curry
  expect(server.sent.map((s) => s.body)).toEqual([{ ...fields, archived: true }])
})
