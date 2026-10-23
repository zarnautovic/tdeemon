import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Food } from '@/api/types'
import { setLocale } from '@/i18n/locale'
import { fakeServer, problem, renderAt, signedIn } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

function food(i: number, over: Partial<Food> = {}): Food {
  return {
    id: `0193c3a5-0000-7000-8000-${String(i).padStart(12, '0')}`,
    name: `Food ${String(i).padStart(2, '0')}`,
    basis: '100g',
    source: 'label',
    archived: false,
    nutrients: { kcal: 100 + i, protein: 5, carbs: 10, fat: 3 },
    ...over,
  }
}

const oats = food(1, {
  name: 'Rolled oats',
  brand: 'Mill',
  nutrients: { kcal: 370, protein: 13, carbs: 60, fat: 7, fiber: 10 },
  servings: [{ name: 'cup', amount: 80 }],
  aliases: ['porridge oats'],
  note: 'from the label',
  last_use: {
    at: '2026-10-21T07:00:00Z',
    meal_id: '0193c3a5-0000-7000-8000-0000000000a1',
    quantity: { amount: 80, unit: 'g' },
    amount: 80,
    unit: 'g',
  },
})

// listOf answers the food list from foods, a page as the request asks.
function listOf(foods: Food[]) {
  return (request: Request) => {
    const url = new URL(request.url)
    const offset = Number(url.searchParams.get('offset') ?? 0)
    const limit = Number(url.searchParams.get('limit') ?? 50)
    return Response.json({ foods: foods.slice(offset, offset + limit), total: foods.length })
  }
}

function asked(fetch: ReturnType<typeof fakeServer>, path: string): URL[] {
  return fetch.mock.calls
    .map(([r]) => (r instanceof Request ? new URL(r.url) : undefined))
    .filter((u): u is URL => u?.pathname === path)
}

test('the foods in use are listed by name, a page at a time', async () => {
  const foods = [oats, ...Array.from({ length: 52 }, (_, i) => food(i + 2))]
  const fetch = fakeServer({ ...signedIn(), 'GET /api/v1/foods/list': listOf(foods) })
  renderAt('/foods')

  const list = await screen.findByRole('list', { name: 'In use' })
  expect(within(list).getAllByRole('listitem')).toHaveLength(50)
  const first = within(list).getAllByRole('link')[0]
  expect(first?.textContent).toBe(
    'Rolled oats · MillLast eaten Oct 21370 kcal · P 13 · C 60 · F 7per 100 g',
  )
  expect(first?.getAttribute('href')).toBe(`/foods/${oats.id}`)
  expect(screen.getByText('50 of 53')).toBeDefined()

  fireEvent.click(screen.getByRole('button', { name: 'Show more' }))

  expect(await screen.findByText('53 of 53')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Show more' })).toBeNull()
  const pages = asked(fetch, '/api/v1/foods/list')
  expect(pages.map((u) => u.searchParams.get('offset'))).toEqual(['0', '50'])
  expect(pages[0]?.searchParams.get('archived')).toBe('false')
  expect(pages[0]?.searchParams.get('order')).toBe('name')
})

test('the archived foods, and the order of the last use, are in the address', async () => {
  const fetch = fakeServer({ ...signedIn(), 'GET /api/v1/foods/list': listOf([]) })
  const router = renderAt('/foods')

  expect(
    await screen.findByText(
      'No foods yet: a meal logged through a model saves its foods, or add one.',
    ),
  ).toBeDefined()
  fireEvent.click(screen.getByRole('button', { name: 'Archived' }))
  expect(await screen.findByText('No archived foods.')).toBeDefined()
  fireEvent.change(screen.getByLabelText('Order'), { target: { value: 'use' } })

  await waitFor(() => {
    expect(router.state.location.search).toEqual({ archived: true, order: 'use' })
  })
  const last = asked(fetch, '/api/v1/foods/list').at(-1)
  expect(last?.searchParams.get('archived')).toBe('true')
  expect(last?.searchParams.get('order')).toBe('use')
  expect(screen.getByRole('button', { name: 'Archived' }).getAttribute('aria-pressed')).toBe('true')
})

test('a search finds the foods, or says none matches', async () => {
  const fetch = fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([oats]),
    'GET /api/v1/foods': (request) =>
      Response.json({
        foods: new URL(request.url).searchParams.get('query') === 'oat' ? [oats] : [],
      }),
  })
  const router = renderAt('/foods')

  fireEvent.change(await screen.findByLabelText('Search the foods'), { target: { value: 'oat' } })

  const found = await screen.findByRole('list', { name: 'Found' })
  expect(within(found).getByText('Rolled oats')).toBeDefined()
  expect(router.state.location.search).toEqual({ q: 'oat' })
  expect(asked(fetch, '/api/v1/foods').at(-1)?.searchParams.get('query')).toBe('oat')

  fireEvent.change(screen.getByLabelText('Search the foods'), { target: { value: 'kale' } })
  expect(await screen.findByText('No food matches.')).toBeDefined()
})

test('a list that did not come says why, and asks again', async () => {
  let up = false
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': (request) => (up ? listOf([oats])(request) : problem(500)),
  })
  renderAt('/foods')

  expect((await screen.findByRole('alert')).textContent).toBe('The server failed to answer.')
  up = true
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

  expect(await screen.findByText('Rolled oats')).toBeDefined()
})

test('a food on two pages, as the list changed between them, is shown once', async () => {
  const foods = Array.from({ length: 60 }, (_, i) => food(i + 1))
  let first = true
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': (request) => {
      // A food came on top of the list after its first page.
      const now = first ? foods : [food(99, { name: 'Food 00' }), ...foods]
      first = false
      return listOf(now)(request)
    },
  })
  renderAt('/foods')

  fireEvent.click(await screen.findByRole('button', { name: 'Show more' }))

  expect(await screen.findByText('Food 60')).toBeDefined()
  expect(screen.getAllByText('Food 50')).toHaveLength(1)
})

test('what the last search found stays while the next one comes', async () => {
  let answer: ((r: Response) => void) | undefined
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([oats]),
    'GET /api/v1/foods': (request) =>
      new URL(request.url).searchParams.get('query') === 'oat'
        ? Response.json({ foods: [oats] })
        : new Promise<Response>((resolve) => {
            answer = resolve
          }),
  })
  renderAt('/foods')

  fireEvent.change(await screen.findByLabelText('Search the foods'), { target: { value: 'oat' } })
  expect(await screen.findByRole('list', { name: 'Found' })).toBeDefined()
  fireEvent.change(screen.getByLabelText('Search the foods'), { target: { value: 'oats' } })

  expect(screen.getByRole('list', { name: 'Found' })).toBeDefined()
  answer?.(Response.json({ foods: [] }))
  expect(await screen.findByText('No food matches.')).toBeDefined()
})

test('a food eaten last in another year says the year', async () => {
  const old = food(7, {
    name: 'Old cereal',
    last_use: {
      at: '2024-10-21T07:00:00Z',
      meal_id: '0193c3a5-0000-7000-8000-0000000000a1',
      quantity: { amount: 80, unit: 'g' },
      amount: 80,
      unit: 'g',
    },
  })
  fakeServer({ ...signedIn(), 'GET /api/v1/foods/list': listOf([old]) })
  renderAt('/foods')

  expect(await screen.findByText('Last eaten Oct 21, 2024')).toBeDefined()
})

test('the search field follows the address it did not write, and keeps what is typed', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([oats]),
    'GET /api/v1/foods': () => Response.json({ foods: [oats] }),
  })
  const router = renderAt('/foods?q=oat')

  const field = await screen.findByLabelText('Search the foods')
  expect(field).toHaveProperty('value', 'oat')
  // Typed fast: the address comes after each letter.
  for (const typed of ['oatm', 'oatme', 'oatmea']) {
    fireEvent.change(field, { target: { value: typed } })
  }
  expect(field).toHaveProperty('value', 'oatmea')
  await waitFor(() => {
    expect(router.state.location.search).toEqual({ q: 'oatmea' })
  })
  expect(field).toHaveProperty('value', 'oatmea')

  // A link to the foods, as the section's: the search is gone.
  await router.navigate({ to: '/foods' })
  await waitFor(() => {
    expect(screen.getByLabelText('Search the foods')).toHaveProperty('value', '')
  })
  expect(await screen.findByRole('list', { name: 'In use' })).toBeDefined()
})

test("a food's page goes back to the foods as they were left", async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([{ ...oats, archived: true }]),
    [`GET /api/v1/foods/${oats.id}`]: () => Response.json({ ...oats, archived: true }),
  })
  renderAt('/foods?archived=true&order=use')

  fireEvent.click(await screen.findByRole('link', { name: /Rolled oats/ }))

  const back = await screen.findByRole('link', { name: 'Back to the foods' })
  expect(back.getAttribute('href')).toBe('/foods?archived=true&order=use')
})

test('an address with a search typed on the way to another puts it in the field', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([]),
    'GET /api/v1/foods': () => Response.json({ foods: [] }),
  })
  const router = renderAt('/foods')
  const field = await screen.findByLabelText('Search the foods')
  for (const typed of ['o', 'oa', 'oat']) {
    fireEvent.change(field, { target: { value: typed } })
  }
  await waitFor(() => {
    expect(router.state.location.search).toEqual({ q: 'oat' })
  })

  await router.navigate({ to: '/foods', search: { q: 'oa' } })

  await waitFor(() => {
    expect(screen.getByLabelText('Search the foods')).toHaveProperty('value', 'oa')
  })
})

test("the section's link clears the search after a typo corrected fast", async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([]),
    'GET /api/v1/foods': () => Response.json({ foods: [] }),
  })
  const router = renderAt('/foods')
  const field = await screen.findByLabelText('Search the foods')
  // Some of these never reach the address: the next navigation takes
  // their place.
  for (const typed of ['x', '', 'b']) {
    fireEvent.change(field, { target: { value: typed } })
  }
  await waitFor(() => {
    expect(router.state.location.search).toEqual({ q: 'b' })
  })

  await router.navigate({ to: '/foods' })

  await waitFor(() => {
    expect(screen.getByLabelText('Search the foods')).toHaveProperty('value', '')
  })
})
