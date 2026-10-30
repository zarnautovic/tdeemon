import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Food } from '@/api/types'
import { setLocale } from '@/i18n/locale'
import { fakeServer, problem, renderAt, signedIn, type Handler } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

// Made-up foods, and a curry of them.
const chicken: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000f1',
  name: 'Chicken breast',
  basis: '100g',
  source: 'label',
  archived: false,
  nutrients: { kcal: 110, protein: 23, carbs: 0, fat: 1.5 },
}

const rice: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000f2',
  name: 'Basmati rice',
  basis: '100g',
  source: 'label',
  archived: false,
  nutrients: { kcal: 350, protein: 7, carbs: 78, fat: 1 },
  servings: [{ name: 'cup', amount: 180 }],
}

const lentils: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000f3',
  name: 'Red lentils',
  basis: '100g',
  source: 'label',
  archived: false,
  nutrients: { kcal: 360, protein: 24, carbs: 60, fat: 1.2 },
}

const curryId = '0193c3a5-0000-7000-8000-0000000000c1'

// The curry: 800 g of chicken and 2 cups of rice, weighed at 1500 g in
// 5 portions; the rice was corrected since.
const curry: Food = {
  id: curryId,
  name: 'Chicken curry',
  basis: '100g',
  source: 'recipe',
  archived: false,
  aliases: ['curry'],
  note: 'Simmer for 20 minutes',
  nutrients: { kcal: 142.67, protein: 13.95, carbs: 18.72, fat: 1.04 },
  servings: [
    { name: 'recipe', amount: 1500 },
    { name: 'portion', amount: 300 },
  ],
  recipe: {
    weight: 1500,
    yield_g: 1500,
    raw_weight: 1160,
    portions: 5,
    pot_g: 1200,
    total: { kcal: 2140, protein: 209.2, carbs: 280.8, fat: 15.6 },
    portion: { kcal: 428, protein: 41.84, carbs: 56.16, fat: 3.12 },
    items: [
      {
        food_id: chicken.id,
        name: 'Chicken breast',
        quantity: { amount: 800, unit: 'g' },
        amount: 800,
        unit: 'g',
        nutrients: { kcal: 880, protein: 184, carbs: 0, fat: 12 },
        source: 'label',
      },
      {
        food_id: rice.id,
        name: 'Basmati rice',
        quantity: { servings: 2, serving: 'cup' },
        amount: 360,
        unit: 'g',
        nutrients: { kcal: 1260, protein: 25.2, carbs: 280.8, fat: 3.6 },
        source: 'label',
        changed: true,
      },
    ],
  },
}

// notWeighed is the curry as it is without a weight, nor portions.
const notWeighed: Food = {
  ...curry,
  nutrients: { kcal: 184.48, protein: 18.03, carbs: 24.21, fat: 1.34 },
  servings: [{ name: 'recipe', amount: 1160 }],
  recipe: {
    weight: 1160,
    raw_weight: 1160,
    total: { kcal: 2140, protein: 209.2, carbs: 280.8, fat: 15.6 },
    items: curry.recipe?.items ?? [],
  },
}

// recipeServer is a server with the curry and its foods, which takes a
// recipe saved as the API does, and keeps the bodies it was sent.
function recipeServer(start: Food = curry, over: Record<string, Handler> = {}) {
  const sent: { path: string; body: unknown }[] = []
  async function saved(request: Request, status: number) {
    const body: unknown = await request.json()
    sent.push({ path: new URL(request.url).pathname, body })
    const id =
      typeof body === 'object' && body !== null && 'id' in body && typeof body.id === 'string'
        ? body.id
        : '0193c3a5-0000-7000-8000-0000000000c9'
    const name =
      typeof body === 'object' && body !== null && 'name' in body && typeof body.name === 'string'
        ? body.name
        : start.name
    return Response.json({ food: { ...start, id, name }, warnings: [] }, { status })
  }
  const fetch = fakeServer({
    ...signedIn(),
    [`GET /api/v1/foods/${start.id}`]: () => Response.json(start),
    [`GET /api/v1/foods/${start.id}/use`]: () =>
      Response.json({
        meals: 3,
        last_use: {
          at: '2026-10-21T12:00:00Z',
          day: '2026-10-21',
          meal_id: '0193c3a5-0000-7000-8000-0000000000a1',
          quantity: { servings: 1, serving: 'portion' },
          amount: 300,
          unit: 'g',
        },
      }),
    [`GET /api/v1/foods/${chicken.id}`]: () => Response.json(chicken),
    [`GET /api/v1/foods/${rice.id}`]: () =>
      Response.json({ ...rice, nutrients: { ...rice.nutrients, kcal: 355 } }),
    [`GET /api/v1/foods/${lentils.id}`]: () => Response.json(lentils),
    'GET /api/v1/foods': (request) =>
      Response.json({
        foods: new URL(request.url).searchParams.get('query') === 'lent' ? [lentils] : [],
        unfound: [],
      }),
    'GET /api/v1/foods/list': () => Response.json({ foods: [], total: 0 }),
    'POST /api/v1/recipes': (request) => saved(request, 200),
    'POST /api/v1/recipes/new': (request) => saved(request, 201),
    ...over,
  })
  return { fetch, sent }
}

test("a recipe's page says a portion's energy, the dish's weight and its ingredients", async () => {
  recipeServer()
  renderAt(`/foods/${curryId}`)

  const page = await screen.findByRole('article', { name: 'Chicken curry' })
  expect(within(page).getByText('2 ingredients · 5 portions')).toBeDefined()
  const energy = within(page).getByRole('region', { name: 'Energy of the recipe' })
  expect(within(energy).getByText('428')).toBeDefined()
  expect(within(energy).getByText('kcal per portion')).toBeDefined()
  expect(within(energy).getByText('300 g')).toBeDefined()
  expect(within(energy).getByText('143 kcal')).toBeDefined()
  expect(within(energy).getByText('2,140 kcal')).toBeDefined()
  expect(within(energy).getByText('Weighed: 1,500 g')).toBeDefined()
  expect(
    within(energy).getByText('5 portions of 300 g · 129% of the raw ingredients'),
  ).toBeDefined()
  // Weighed, nothing is approximate.
  expect(within(energy).queryByText(/≈/)).toBeNull()
  const ingredients = within(page).getByRole('list', { name: 'Ingredients' })
  expect(within(ingredients).getByText('800 g')).toBeDefined()
  expect(within(ingredients).getByText('2 cup (360 g) · values changed')).toBeDefined()
  expect(within(page).getByText('1,160 g · 2,140 kcal')).toBeDefined()
  expect(within(page).getByText('Simmer for 20 minutes')).toBeDefined()
  // Logged as it was last: a portion.
  expect(await within(page).findByRole('button', { name: 'Log 1 portion' })).toBeDefined()
  expect(within(page).getByText('3× · last Oct 21')).toBeDefined()
  // The table: a portion, 100 g, or the whole dish.
  expect(within(page).getByRole('radio', { name: '1 portion' })).toHaveProperty('checked', true)
  fireEvent.click(within(page).getByRole('radio', { name: 'Whole recipe' }))
  expect(within(page).getByRole('row', { name: 'Energy 2,140 kcal' })).toBeDefined()
})

test('a recipe not weighed says so plainly, ≈ on its grams, and is weighed in a sheet', async () => {
  const server = recipeServer(notWeighed)
  renderAt(`/foods/${curryId}`)

  const page = await screen.findByRole('article', { name: 'Chicken curry' })
  const energy = within(page).getByRole('region', { name: 'Energy of the recipe' })
  expect(within(energy).getByText('≈ 184')).toBeDefined()
  expect(within(energy).getByText('kcal per 100 g')).toBeDefined()
  expect(within(energy).getByText('Not weighed')).toBeDefined()
  expect(
    within(energy).getByText(
      "The raw ingredients' sum, 1,160 g, stands for the weight. Portions and shares are exact, grams approximate.",
    ),
  ).toBeDefined()
  fireEvent.click(within(page).getByRole('radio', { name: '100 g' }))
  expect(within(page).getByRole('row', { name: 'Energy ≈ 184 kcal' })).toBeDefined()
  expect(
    within(page).getByText('Per 100 g is approximate: the cooked dish was not weighed.'),
  ).toBeDefined()

  fireEvent.click(within(energy).getByRole('button', { name: 'Enter the weight' }))
  const sheet = await screen.findByRole('dialog', { name: 'Weigh the dish' })
  // Without a weight, nothing is saved.
  fireEvent.click(within(sheet).getByRole('button', { name: 'Save the weight' }))
  expect(within(sheet).getByRole('alert').textContent).toBe(
    'Enter the weight, or work it out from the pot.',
  )
  fireEvent.change(within(sheet).getByLabelText('Weight of the cooked dish'), {
    target: { value: '1450' },
  })
  expect(within(sheet).getByText("125% of the raw ingredients' sum (1,160 g)")).toBeDefined()
  fireEvent.click(within(sheet).getByRole('button', { name: 'Save the weight' }))

  expect(await screen.findByText('Saved: Chicken curry · 184 kcal per 100 g')).toBeDefined()
  expect(server.sent).toEqual([
    {
      path: '/api/v1/recipes',
      body: {
        id: curryId,
        name: 'Chicken curry',
        aliases: ['curry'],
        note: 'Simmer for 20 minutes',
        items: [
          { food_id: chicken.id, amount: 800, unit: 'g' },
          { food_id: rice.id, serving: 'cup', servings: 2 },
        ],
        yield_g: 1450,
      },
    },
  ])
})

test('the new values of a food changed are taken by saving the recipe again', async () => {
  const server = recipeServer()
  renderAt(`/foods/${curryId}`)

  const page = await screen.findByRole('article', { name: 'Chicken curry' })
  expect(
    within(page).getByText(
      'Changed since the recipe was saved: Basmati rice. The recipe still uses the values it had.',
    ),
  ).toBeDefined()
  fireEvent.click(within(page).getByRole('button', { name: 'Take the new values' }))

  expect(await screen.findByText('Saved: Chicken curry · 428 kcal per portion')).toBeDefined()
  expect(server.sent).toEqual([
    {
      path: '/api/v1/recipes',
      body: {
        id: curryId,
        name: 'Chicken curry',
        aliases: ['curry'],
        note: 'Simmer for 20 minutes',
        items: [
          { food_id: chicken.id, amount: 800, unit: 'g' },
          { food_id: rice.id, serving: 'cup', servings: 2 },
        ],
        yield_g: 1500,
        portions: 5,
        pot_g: 1200,
      },
    },
  ])
})

test('cooked again changes the amounts, the weight from the pot and the portions', async () => {
  const server = recipeServer()
  renderAt(`/foods/${curryId}`)

  fireEvent.click(await screen.findByRole('button', { name: 'Cooked again' }))
  const sheet = await screen.findByRole('dialog', { name: 'Cooked again' })
  const amount = await within(sheet).findByLabelText('Amount: Chicken breast')
  expect(amount).toHaveProperty('value', '800')
  expect(within(sheet).getByLabelText('Unit: Basmati rice')).toHaveProperty('value', 'serving:cup')
  fireEvent.change(amount, { target: { value: '900' } })
  expect(within(sheet).getByText('990 kcal · was 800 g')).toBeDefined()
  // The weight is weighed anew, the empty pot remembered.
  expect(within(sheet).getByLabelText('Weight of the cooked dish')).toHaveProperty('value', '')
  expect(within(sheet).getByLabelText('Empty pot · remembered')).toHaveProperty('value', '1200')
  expect(within(sheet).getByText('Last time: 1,500 g.')).toBeDefined()
  fireEvent.change(within(sheet).getByLabelText('Full pot'), { target: { value: '2900' } })
  expect(within(sheet).getByText('= 1,700 g of cooked dish')).toBeDefined()
  expect(within(sheet).getByLabelText('Weight of the cooked dish')).toHaveProperty('value', '1700')
  fireEvent.click(within(sheet).getByRole('button', { name: 'One portion more' }))
  // 880 + 990 kcal... the rice as it is now, 355 kcal per 100 g.
  expect(within(sheet).getByText(/^was 428 · /)).toBeDefined()
  fireEvent.click(within(sheet).getByRole('button', { name: 'Save' }))

  expect(await screen.findByText('Saved: Chicken curry · 428 kcal per portion')).toBeDefined()
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'Cooked again' })).toBeNull()
  })
  expect(server.sent).toEqual([
    {
      path: '/api/v1/recipes',
      body: {
        id: curryId,
        name: 'Chicken curry',
        aliases: ['curry'],
        note: 'Simmer for 20 minutes',
        items: [
          { food_id: chicken.id, amount: 900, unit: 'g' },
          { food_id: rice.id, serving: 'cup', servings: 2 },
        ],
        yield_g: 1700,
        portions: 6,
        pot_g: 1200,
      },
    },
  ])
})

test('a new recipe is written: its ingredients found, its weight warned of, and saved', async () => {
  const server = recipeServer()
  const router = renderAt('/foods/new-recipe')

  expect(await screen.findByRole('heading', { name: 'New recipe' })).toBeDefined()
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Lentil soup' } })
  fireEvent.click(screen.getByRole('button', { name: 'Add ingredients' }))
  const sheet = await screen.findByRole('dialog', { name: 'Add ingredients' })
  expect(within(sheet).getByText('Into Lentil soup · 0 / 50')).toBeDefined()
  // No estimate: an ingredient is a food.
  expect(within(sheet).queryByRole('button', { name: 'An estimate' })).toBeNull()
  fireEvent.change(within(sheet).getByRole('searchbox'), { target: { value: 'lent' } })
  fireEvent.click(await within(sheet).findByRole('button', { name: /^Red lentils/ }))
  fireEvent.change(within(sheet).getByLabelText('Amount: Red lentils'), {
    target: { value: '300' },
  })
  fireEvent.click(within(sheet).getByRole('button', { name: 'Add: Red lentils' }))
  const added = within(sheet).getByRole('list', { name: 'Added to the recipe' })
  expect(within(added).getByText('Red lentils · 300 g · 1,080 kcal')).toBeDefined()
  fireEvent.click(within(sheet).getByRole('button', { name: 'Done · 1 added' }))
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'Add ingredients' })).toBeNull()
  })

  expect(screen.getByLabelText('Amount: Red lentils')).toHaveProperty('value', '300')
  // Not weighed, the raw ingredients stand for the dish.
  expect(screen.getByText('Whole recipe 1,080 kcal · ≈ 300 g')).toBeDefined()
  const weight = screen.getByLabelText('Weight of the cooked dish')
  fireEvent.change(weight, { target: { value: '60' } })
  expect(
    screen.getByText(
      "60 g is 20% of the raw ingredients' sum (300 g). Was the empty pot taken off? It saves anyway.",
    ),
  ).toBeDefined()
  fireEvent.change(weight, { target: { value: '900' } })
  expect(screen.getByText("300% of the raw ingredients' sum (300 g)")).toBeDefined()
  const more = screen.getByRole('button', { name: 'One portion more' })
  fireEvent.click(more)
  fireEvent.click(more)
  fireEvent.click(more)
  expect(screen.getByLabelText('Portions')).toHaveProperty('value', '3')
  expect(screen.getByText('300 g each')).toBeDefined()
  expect(screen.getByText('Per portion')).toBeDefined()
  expect(screen.getByText('360 kcal')).toBeDefined()
  fireEvent.click(screen.getByRole('button', { name: 'Save the recipe' }))

  expect(await screen.findByText('Saved: Lentil soup')).toBeDefined()
  expect(server.sent).toEqual([
    {
      path: '/api/v1/recipes/new',
      body: {
        name: 'Lentil soup',
        items: [{ food_id: lentils.id, amount: 300, unit: 'g' }],
        yield_g: 900,
        portions: 3,
      },
    },
  ])
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/foods/0193c3a5-0000-7000-8000-0000000000c9')
  })
})

test('a recipe without a name or ingredients says so, and one whose name is taken names the food', async () => {
  recipeServer(curry, {
    'POST /api/v1/recipes/new': () =>
      Response.json(
        {
          status: 409,
          title: 'Conflict',
          detail: 'a food of the name',
          errors: [{ location: 'body.name', value: chicken.id, message: 'taken' }],
        },
        { status: 409, headers: { 'Content-Type': 'application/problem+json' } },
      ),
  })
  renderAt('/foods/new-recipe')

  fireEvent.click(await screen.findByRole('button', { name: 'Save the recipe' }))
  expect(await screen.findByText('Needed.')).toBeDefined()
  expect(screen.getByText('Add at least one ingredient.')).toBeDefined()
  expect(document.activeElement).toBe(screen.getByLabelText('Name'))

  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Chicken breast' } })
  fireEvent.click(screen.getByRole('button', { name: 'Add ingredients' }))
  const sheet = await screen.findByRole('dialog', { name: 'Add ingredients' })
  fireEvent.change(within(sheet).getByRole('searchbox'), { target: { value: 'lent' } })
  fireEvent.click(await within(sheet).findByRole('button', { name: 'Add: Red lentils' }))
  fireEvent.click(within(sheet).getByRole('button', { name: 'Done · 1 added' }))
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'Add ingredients' })).toBeNull()
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save the recipe' }))

  expect(await screen.findByText('Another food of yours has this name and brand:')).toBeDefined()
  expect(await screen.findByRole('link', { name: 'Chicken breast' })).toBeDefined()
  expect(screen.getByLabelText('Name').getAttribute('aria-invalid')).toBe('true')
})

test("a recipe is edited from its ingredients' foods: moved, removed, and saved whole", async () => {
  const server = recipeServer()
  const router = renderAt(`/foods/${curryId}?edit=true`)

  expect(await screen.findByRole('heading', { name: 'Edit the recipe' })).toBeDefined()
  expect(await screen.findByLabelText('Amount: Chicken breast')).toHaveProperty('value', '800')
  expect(screen.getByLabelText('Name')).toHaveProperty('value', 'Chicken curry')
  expect(screen.getByLabelText('Weight of the cooked dish')).toHaveProperty('value', '1500')
  expect(screen.getByLabelText('Portions')).toHaveProperty('value', '5')
  expect(screen.getByLabelText('Note · optional')).toHaveProperty('value', 'Simmer for 20 minutes')

  fireEvent.click(screen.getByRole('button', { name: 'More: Chicken breast' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Move down' }))
  await waitFor(() => {
    expect(
      within(screen.getByRole('list', { name: 'Ingredients' }))
        .getAllByRole('listitem')
        .map((li) => li.textContent.split(/\d/)[0]),
    ).toEqual(['Basmati rice', 'Chicken breast'])
  })
  fireEvent.change(screen.getByLabelText('Unit: Basmati rice'), { target: { value: 'g' } })
  expect(screen.getByLabelText('Amount: Basmati rice')).toHaveProperty('value', '360')
  fireEvent.change(screen.getByLabelText('Portions'), { target: { value: '0' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the recipe' }))
  expect(await screen.findByText('From 1 to 100 portions.')).toBeDefined()
  fireEvent.change(screen.getByLabelText('Portions'), { target: { value: '' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the recipe' }))

  expect(await screen.findByText('Saved: Chicken curry')).toBeDefined()
  expect(server.sent).toEqual([
    {
      path: '/api/v1/recipes',
      body: {
        id: curryId,
        name: 'Chicken curry',
        aliases: ['curry'],
        items: [
          { food_id: rice.id, amount: 360, unit: 'g' },
          { food_id: chicken.id, amount: 800, unit: 'g' },
        ],
        yield_g: 1500,
        pot_g: 1200,
        note: 'Simmer for 20 minutes',
      },
    },
  ])
  // Saved, the recipe is read again.
  await waitFor(() => {
    expect(router.state.location.search).toEqual({})
  })
})

test('the recipes are a list of their own, their kcal for a portion, and a new one is offered', async () => {
  const recipes: Food[] = [curry]
  const fetch = fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': (request) => {
      const source = new URL(request.url).searchParams.get('source')
      const foods = source === 'recipe' ? recipes : [chicken, curry]
      return Response.json({ foods, total: foods.length })
    },
  })
  const router = renderAt('/foods')

  // In use, a recipe is marked, its kcal those of a portion.
  const row = await screen.findByRole('link', { name: /^Chicken curry/ })
  expect(row.textContent).toContain('recipe')
  expect(row.textContent).toContain('428 kcal/portion')
  fireEvent.click(screen.getByRole('radio', { name: 'Recipes' }))
  expect(await screen.findByRole('radio', { name: 'Recipes · 1' })).toHaveProperty('checked', true)
  expect(router.state.location.search).toEqual({ filter: 'recipes' })
  expect(
    fetch.mock.calls.some(
      ([r]) => r instanceof Request && new URL(r.url).searchParams.get('source') === 'recipe',
    ),
  ).toBe(true)
  expect(screen.getByRole('link', { name: 'New recipe' }).getAttribute('href')).toBe(
    '/foods/new-recipe?filter=recipes',
  )

  // None yet: what a recipe is for, and the way to one.
  recipes.pop()
  cleanup()
  renderAt('/foods?filter=recipes')
  expect(await screen.findByRole('heading', { name: 'No recipes yet' })).toBeDefined()
  expect(screen.getAllByRole('link', { name: 'New recipe' })).toHaveLength(1)
})

test('the new button is a choice of a food or a recipe', async () => {
  fakeServer({
    ...signedIn('hr'),
    'GET /api/v1/foods/list': () => Response.json({ foods: [chicken], total: 1 }),
  })
  const router = renderAt('/foods')

  fireEvent.click(await screen.findByRole('button', { name: 'Nova namirnica ili recept' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Recept' }))
  expect(await screen.findByRole('heading', { name: 'Novi recept' })).toBeDefined()
  expect(router.state.location.pathname).toBe('/foods/new-recipe')
  expect(
    screen.getByText(
      'Još nema sastojaka. Dodaj ih kao u obrok. Sastojak može biti i drugi recept, npr. umak.',
    ),
  ).toBeDefined()
})

test('a recipe that failed to save says why', async () => {
  recipeServer(curry, { 'POST /api/v1/recipes': () => problem(422) })
  renderAt(`/foods/${curryId}?edit=true`)

  await screen.findByLabelText('Amount: Chicken breast')
  fireEvent.click(screen.getByRole('button', { name: 'Save the recipe' }))
  expect((await screen.findByRole('alert')).textContent).toBe(
    'The server did not take this recipe: what went wrong',
  )
})
