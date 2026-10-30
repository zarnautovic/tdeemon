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

import type { Day, Food, Item, Meal } from '@/api/types'
import { setLocale } from '@/i18n/locale'
import { day, fakeServer, renderAt, signedIn, type Handler } from '@/test/app'

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-10-23T22:00:00Z'))
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

// Made-up recipes. The curry: 2,000 kcal in 1,000 g weighed, in 4
// portions of 250 g and 500 kcal.
const curry: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000c1',
  name: 'Chicken curry',
  basis: '100g',
  source: 'recipe',
  archived: false,
  nutrients: { kcal: 200, protein: 15, carbs: 20, fat: 6 },
  servings: [
    { name: 'recipe', amount: 1000 },
    { name: 'portion', amount: 250 },
  ],
  recipe: {
    weight: 1000,
    yield_g: 1000,
    portions: 4,
    total: { kcal: 2000, protein: 150, carbs: 200, fat: 60 },
    portion: { kcal: 500, protein: 37.5, carbs: 50, fat: 15 },
    items: [],
  },
}

// The stew: 1,200 kcal, not weighed nor divided, its raw ingredients'
// 800 g standing for its weight.
const stew: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000c2',
  name: 'Bean stew',
  basis: '100g',
  source: 'recipe',
  archived: false,
  nutrients: { kcal: 150, protein: 10, carbs: 12.5, fat: 5 },
  servings: [{ name: 'recipe', amount: 800 }],
  recipe: {
    weight: 800,
    raw_weight: 800,
    total: { kcal: 1200, protein: 80, carbs: 100, fat: 40 },
    items: [],
  },
}

// eatenAs is a recipe eaten last in a quantity.
function eatenAs(food: Food, quantity: Item['quantity'], amount: number): Food {
  return {
    ...food,
    last_use: {
      at: '2026-10-21T19:00:00Z',
      day: '2026-10-21',
      meal_id: '0193c3a5-0000-7000-8000-0000000000a9',
      quantity,
      amount,
      unit: 'g',
    },
  }
}

// recipeServer is a day that takes a meal, whose search finds foods
// (each search, all of them), and keeps the bodies it was sent.
function recipeServer(foods: Food[], over: Record<string, Handler> = {}) {
  const current: Day = day()
  const sent: unknown[] = []
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () => Response.json(current),
    'GET /api/v1/foods': () => Response.json({ foods, unfound: [] }),
    'GET /api/v1/foods/list': () => Response.json({ foods: [], total: 0 }),
    'GET /api/v1/meals/recent': () => Response.json({ meals: [] }),
    'POST /api/v1/meals': async (request) => {
      const body: unknown = await request.json()
      sent.push(body)
      const meal: Meal = {
        id: '0193c3a5-0000-7000-8000-0000000000a1',
        eaten_at: '2026-10-23T22:00:00Z',
        description: '',
        client: 'web',
        total: { kcal: 667, protein: 50, carbs: 67, fat: 20 },
        items: [],
      }
      const { meals: _meals, metrics: _metrics, ...summary } = current
      return Response.json({ meal, day: summary, warnings: [] }, { status: 201 })
    },
    ...over,
  })
  return { sent }
}

type Sheet = BoundFunctions<typeof queries>

const logMeal =
  /^(Log a meal|Upiši obrok|Add (breakfast|lunch|a snack|dinner)|Dodaj (doručak|ručak|užinu|večeru))/

// find opens the meal to log, searches, and picks the food named name.
async function find(name: string, title = 'New meal', search = 'Find a food or a meal') {
  const main = within(await screen.findByRole('main'))
  fireEvent.click(await main.findByRole('button', { name: logMeal }))
  const sheet = within(await screen.findByRole('dialog', { name: title }))
  fireEvent.change(sheet.getByLabelText(search), { target: { value: name.slice(0, 3) } })
  fireEvent.click(await sheet.findByRole('button', { name: new RegExp(`^${name}`) }))
  return sheet
}

function basket(sheet: Sheet) {
  return within(sheet.getByRole('list', { name: 'In the meal' }))
    .getAllByRole('listitem')
    .map((li) => li.textContent.replace('×', ''))
}

const options = (select: HTMLElement) =>
  [...select.querySelectorAll('option')].map((o) => [o.value, o.textContent])

test('a recipe found is marked, a portion its energy, and opens at a portion; ⅓ of it is sent as a third', async () => {
  const server = recipeServer([curry])
  renderAt('/')

  const main = within(await screen.findByRole('main'))
  fireEvent.click(await main.findByRole('button', { name: logMeal }))
  const sheet = within(await screen.findByRole('dialog', { name: 'New meal' }))
  fireEvent.change(sheet.getByLabelText('Find a food or a meal'), { target: { value: 'cur' } })
  const row = await sheet.findByRole('button', { name: /^Chicken curry/ })
  expect(within(row).getByText('recipe')).toBeDefined()
  expect(row.textContent).toContain('500 kcal per portion')
  fireEvent.click(row)

  // Never eaten: a portion.
  const amount = sheet.getByLabelText('Amount: Chicken curry')
  expect(amount.getAttribute('inputmode')).toBe('text')
  const unit = sheet.getByLabelText('Unit: Chicken curry')
  expect(amount).toHaveProperty('value', '1')
  expect(unit).toHaveProperty('value', 'serving:portion')
  expect(options(unit)).toEqual([
    ['serving:portion', 'portion'],
    ['serving:recipe', 'of the recipe'],
    ['g', 'g'],
  ])
  // Weighed: no ≈ anywhere.
  expect(sheet.getByText('= 500 kcal · P 38 · C 50 · F 15 · 250 g')).toBeDefined()
  const quick = within(
    sheet.getByRole('group', { name: 'Quick: a portion or a share of the recipe' }),
  )
  expect(quick.getByRole('button', { name: '1 portion (250 g)' })).toHaveProperty(
    'ariaPressed',
    'true',
  )
  fireEvent.click(quick.getByRole('button', { name: '⅓ of the recipe' }))
  expect(amount).toHaveProperty('value', '1/3')
  expect(unit).toHaveProperty('value', 'serving:recipe')
  expect(quick.getByRole('button', { name: '⅓ of the recipe' })).toHaveProperty(
    'ariaPressed',
    'true',
  )
  expect(sheet.getByText('= 667 kcal · P 50 · C 67 · F 20 · 333 g')).toBeDefined()
  // In grams and back, the third it was.
  fireEvent.change(unit, { target: { value: 'g' } })
  expect(amount).toHaveProperty('value', '333.33')
  fireEvent.change(unit, { target: { value: 'serving:recipe' } })
  expect(amount).toHaveProperty('value', '1/3')
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Chicken curry' }))

  expect(basket(sheet)).toEqual(['Chicken curry · ⅓ of the recipe · 667 kcal'])
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 667 kcal' }))
  expect(await screen.findByText(/^Logged: /)).toBeDefined()
  expect(server.sent).toEqual([
    {
      id: expect.any(String) as unknown,
      items: [
        {
          id: expect.any(String) as unknown,
          food_id: curry.id,
          serving: 'recipe',
          // Every decimal: a third, not 0.33.
          servings: 1 / 3,
        },
      ],
    },
  ])
})

test('a recipe not weighed opens at a quarter, takes a fraction typed, and its grams are about so', async () => {
  const server = recipeServer([stew])
  renderAt('/')

  const sheet = await find('Bean stew')
  const row = sheet.getByRole('button', { name: /^Bean stew/ })
  // Per 100 g, about so.
  expect(row.textContent).toContain('≈ 150 kcal per 100 g')
  const amount = sheet.getByLabelText('Amount: Bean stew')
  const unit = sheet.getByLabelText('Unit: Bean stew')
  // Not divided: a quarter of it.
  expect(amount).toHaveProperty('value', '1/4')
  expect(unit).toHaveProperty('value', 'serving:recipe')
  expect(options(unit)).toEqual([
    ['serving:recipe', 'of the recipe'],
    ['g', 'g'],
  ])
  // A share is exact, its grams are not.
  expect(sheet.getByText('= 300 kcal · P 20 · C 25 · F 10 · ≈ 200 g')).toBeDefined()
  const quick = within(
    sheet.getByRole('group', { name: 'Quick: a portion or a share of the recipe' }),
  )
  expect(quick.queryByRole('button', { name: /portion/ })).toBeNull()

  // 1/… types 1/ and waits for the denominator.
  fireEvent.click(quick.getByRole('button', { name: '1/… of the recipe: type the denominator' }))
  expect(amount).toHaveProperty('value', '1/')
  expect(document.activeElement).toBe(amount)
  expect(sheet.getByText('Type the denominator: 1/10 is a tenth of the recipe.')).toBeDefined()
  fireEvent.change(amount, { target: { value: '1/10' } })
  expect(sheet.getByText('= 120 kcal · P 8 · C 10 · F 4 · ≈ 80 g')).toBeDefined()
  fireEvent.change(amount, { target: { value: '2/3' } })
  expect(sheet.getByText('= 800 kcal · P 53 · C 67 · F 27 · ≈ 533 g')).toBeDefined()
  fireEvent.change(amount, { target: { value: '0.5' } })
  expect(sheet.getByText('= 600 kcal · P 40 · C 50 · F 20 · ≈ 400 g')).toBeDefined()

  // In grams, what it brings is about so too, and why is said.
  fireEvent.change(unit, { target: { value: 'g' } })
  expect(amount).toHaveProperty('value', '400')
  expect(sheet.getByText('= ≈ 600 kcal · P 40 · C 50 · F 20')).toBeDefined()
  expect(
    sheet.getByText(
      '≈ The cooked dish was not weighed, so grams are approximate. A portion or a share is exact.',
    ),
  ).toBeDefined()
  fireEvent.change(amount, { target: { value: '200' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Bean stew' }))
  expect(basket(sheet)).toEqual(['Bean stew · 200 g · 300 kcal'])
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 300 kcal' }))
  expect(await screen.findByText(/^Logged: /)).toBeDefined()
  expect(server.sent).toEqual([
    {
      id: expect.any(String) as unknown,
      items: [{ id: expect.any(String) as unknown, food_id: stew.id, amount: 200, unit: 'g' }],
    },
  ])
})

test('a share that is no number is not added', async () => {
  recipeServer([stew])
  renderAt('/')

  const sheet = await find('Bean stew')
  const amount = sheet.getByLabelText('Amount: Bean stew')
  fireEvent.change(amount, { target: { value: '1/0' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Bean stew' }))
  expect(sheet.getByRole('alert').textContent).toBe('Write a number, such as 72.5.')
  expect(screen.queryByRole('list', { name: 'In the meal' })).toBeNull()
})

test('a recipe eaten before opens as it was eaten last, its share every decimal of it', async () => {
  const server = recipeServer([eatenAs(curry, { servings: 1 / 3, serving: 'recipe' }, 333.33)])
  renderAt('/')

  const sheet = await find('Chicken curry')
  expect(sheet.getByRole('button', { name: /^Chicken curry/ }).textContent).toContain(
    'last ⅓ of the recipe',
  )
  expect(sheet.getByLabelText('Amount: Chicken curry')).toHaveProperty('value', '1/3')
  expect(sheet.getByLabelText('Unit: Chicken curry')).toHaveProperty('value', 'serving:recipe')
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Chicken curry' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Log 1 item · 667 kcal' }))
  expect(await screen.findByText(/^Logged: /)).toBeDefined()
  expect(server.sent).toEqual([
    {
      id: expect.any(String) as unknown,
      items: [
        {
          id: expect.any(String) as unknown,
          food_id: curry.id,
          serving: 'recipe',
          servings: 1 / 3,
        },
      ],
    },
  ])
})

test("a recipe's portions eaten last are said in Croatian, with its chips", async () => {
  recipeServer([eatenAs(curry, { servings: 2, serving: 'portion' }, 500)], {
    ...signedIn('hr'),
    'GET /api/v1/days/today': () => Response.json(day()),
  })
  setLocale('hr')
  renderAt('/')

  const sheet = await find('Chicken curry', 'Novi obrok', 'Pronađi namirnicu ili obrok')
  expect(sheet.getByRole('button', { name: /^Chicken curry/ }).textContent).toContain('2 porcije')
  expect(sheet.getByLabelText('Količina: Chicken curry')).toHaveProperty('value', '2')
  expect(options(sheet.getByLabelText('Jedinica: Chicken curry'))).toEqual([
    ['serving:portion', 'porcije'],
    ['serving:recipe', 'recepta'],
    ['g', 'g'],
  ])
  const quick = within(sheet.getByRole('group', { name: 'Brzo: porcija ili dio recepta' }))
  expect(quick.getByRole('button', { name: '1 porcija (250 g)' })).toBeDefined()
  fireEvent.click(quick.getByRole('button', { name: '¼ recepta' }))
  expect(sheet.getByText('= 500 kcal · P 38 · UH 50 · M 15 · 250 g')).toBeDefined()
})

test("Log on a recipe's page opens Log a meal on its amount, to choose a portion or a share", async () => {
  // A meal of before, which comes first of the lists but for the recipe.
  const before: Meal = { ...curryMeal, eaten_at: '2026-10-22T19:00:00Z' }
  recipeServer([], {
    'GET /api/v1/meals/recent': () =>
      Response.json({
        meals: [{ ...before, eaten: [{ at: before.eaten_at, day: '2026-10-22' }] }],
      }),
    [`GET /api/v1/foods/${stew.id}`]: () => Response.json(stew),
    [`GET /api/v1/foods/${stew.id}/use`]: () => Response.json({ meals: 0 }),
  })
  renderAt(`/foods/${stew.id}`)

  const page = await screen.findByRole('article', { name: 'Bean stew' })
  fireEvent.click(await within(page).findByRole('button', { name: 'Log it' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'New meal' }))
  const amount = sheet.getByLabelText('Amount: Bean stew')
  expect(amount).toHaveProperty('value', '1/4')
  await waitFor(() => {
    expect(document.activeElement).toBe(amount)
  })
  // First, above the meals of before.
  const results = within(await sheet.findByRole('list', { name: 'What there is to add' }))
  await results.findByRole('button', { name: /^Repeat: / })
  expect(results.getAllByRole('listitem')[0]?.textContent).toMatch(/^BSBean stew/)
  // Not in the meal until it is added.
  expect(sheet.queryByRole('list', { name: 'In the meal' })).toBeNull()
  fireEvent.click(sheet.getByRole('button', { name: '⅓ of the recipe' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Add: Bean stew' }))
  expect(basket(sheet)).toEqual(['Bean stew · ⅓ of the recipe · 400 kcal'])
  // Added, it is no longer first among the foods.
  expect(sheet.queryByRole('button', { name: /^Bean stew/ })).toBeNull()
})

// A meal of the curry: a tenth of it, and two portions.
const tenth: Item = {
  id: '0193c3a5-0000-7000-8000-0000000000b1',
  food_id: curry.id,
  name: 'Chicken curry',
  quantity: { servings: 0.1, serving: 'recipe' },
  amount: 100,
  unit: 'g',
  nutrients: { kcal: 200, protein: 15, carbs: 20, fat: 6 },
  source: 'recipe',
}
const twoPortions: Item = {
  ...tenth,
  id: '0193c3a5-0000-7000-8000-0000000000b2',
  quantity: { servings: 2, serving: 'portion' },
  amount: 500,
  nutrients: { kcal: 1000, protein: 75, carbs: 100, fat: 30 },
}
const curryMeal: Meal = {
  id: '0193c3a5-0000-7000-8000-0000000000a2',
  eaten_at: '2026-10-23T19:00:00Z',
  description: 'Lunch',
  client: 'web',
  total: { kcal: 1200, protein: 90, carbs: 120, fat: 36 },
  items: [tenth, twoPortions],
}

test("a meal's share of a recipe reads as a fraction, its portions in the language's plural", async () => {
  fakeServer({
    ...signedIn('hr'),
    'GET /api/v1/days/today': () => Response.json(day({ meals: [curryMeal] })),
  })
  setLocale('hr')
  renderAt('/')

  const article = await screen.findByRole('article')
  fireEvent.click(within(article).getByRole('button', { expanded: false }))
  expect(within(article).getByText('⅒ recepta')).toBeDefined()
  expect(within(article).getByText('2 porcije')).toBeDefined()
})

test("a meal's share of a recipe opens in its editor as a fraction, and one typed is sent whole", async () => {
  const sent: unknown[] = []
  const meal: Meal = {
    ...curryMeal,
    items: [{ ...tenth, quantity: { servings: 1 / 3, serving: 'recipe' }, amount: 333.33 }],
  }
  fakeServer({
    ...signedIn(),
    'GET /api/v1/days/today': () => Response.json(day({ meals: [meal] })),
    [`GET /api/v1/foods/${curry.id}`]: () => Response.json(curry),
    [`PATCH /api/v1/meals/${meal.id}`]: async (request) => {
      sent.push(await request.json())
      const { meals: _meals, metrics: _metrics, ...summary } = day({ meals: [meal] })
      return Response.json({ meal, day: summary, warnings: [] })
    },
  })
  renderAt('/')

  const article = await screen.findByRole('article')
  fireEvent.click(within(article).getByRole('button', { expanded: false }))
  expect(within(article).getByText('⅓ of the recipe')).toBeDefined()
  fireEvent.click(await screen.findByRole('button', { name: 'Change the meal: Lunch' }))
  const editor = within(await screen.findByRole('dialog', { name: 'Change the meal' }))
  const amount = editor.getByLabelText('Amount: Chicken curry')
  expect(amount).toHaveProperty('value', '1/3')
  // A keyboard with a / for the fraction.
  expect(amount.getAttribute('inputmode')).toBe('text')
  const unit = await editor.findByLabelText('Unit: Chicken curry')
  expect(options(unit)).toEqual([
    // As it was given first, then the others.
    ['serving:recipe', 'of the recipe'],
    ['serving:portion', 'portions'],
    ['g', 'g'],
  ])
  fireEvent.change(amount, { target: { value: '1/6' } })
  fireEvent.click(editor.getByRole('button', { name: 'Save the meal' }))
  await waitFor(() => {
    expect(sent).toHaveLength(1)
  })
  expect(sent).toEqual([
    {
      ops: [
        {
          op: 'replace',
          item_id: tenth.id,
          item: { food_id: curry.id, serving: 'recipe', servings: 1 / 6 },
        },
      ],
    },
  ])
})
