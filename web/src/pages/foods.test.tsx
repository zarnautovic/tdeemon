import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Food } from '@/api/types'
import { setLocale } from '@/i18n/locale'
import { fakeServer, problem, renderAt, signedIn, today } from '@/test/app'

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

// lastUse is a food's last use on day, of 80 g.
function lastUse(day: string): NonNullable<Food['last_use']> {
  return {
    at: `${day}T15:30:00Z`,
    day,
    meal_id: '0193c3a5-0000-7000-8000-0000000000a1',
    quantity: { amount: 80, unit: 'g' },
    amount: 80,
    unit: 'g',
  }
}

// Eaten two days before today.
const oats = food(1, {
  name: 'Rolled oats',
  brand: 'Mill',
  nutrients: { kcal: 370, protein: 13, carbs: 60, fat: 7, fiber: 10 },
  servings: [{ name: 'cup', amount: 80 }],
  aliases: ['porridge oats'],
  note: 'from the label',
  last_use: lastUse('2026-10-21'),
})

// listOf answers the food list from foods, a page as the request asks.
function listOf(foods: Food[]) {
  return (request: Request) => {
    const url = new URL(request.url)
    const offset = Number(url.searchParams.get('offset') ?? 0)
    const limit = Number(url.searchParams.get('limit') ?? 50)
    return Response.json({
      foods: foods.slice(offset, offset + limit),
      total: foods.length,
    })
  }
}

// markOf is the classes of a food's mark, the first thing in its row.
function markOf(row: HTMLElement | undefined): string {
  return row?.firstElementChild?.className ?? ''
}

function asked(fetch: ReturnType<typeof fakeServer>, path: string): URL[] {
  return fetch.mock.calls
    .map(([r]) => (r instanceof Request ? new URL(r.url) : undefined))
    .filter((u): u is URL => u?.pathname === path)
}

// desktop makes the window a desktop's width, as matchMedia says.
function desktop() {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: query === '(min-width: 64rem)',
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }))
}

test('the foods in use are listed by name, a page at a time', async () => {
  const foods = [oats, ...Array.from({ length: 52 }, (_, i) => food(i + 2))]
  const fetch = fakeServer({ ...signedIn(), 'GET /api/v1/foods/list': listOf(foods) })
  renderAt('/foods?order=name')

  const list = await screen.findByRole('list', { name: 'In use' })
  expect(within(list).getAllByRole('listitem')).toHaveLength(50)
  const first = within(list).getAllByRole('link')[0]
  // Its initials, name and brand, macros, and kcal for its basis.
  expect(first?.textContent).toBe('RORolled oats · MillP 13 · C 60 · F 7370 kcal/100 g')
  expect(first?.getAttribute('href')).toBe(`/foods/${oats.id}?order=name`)
  expect(screen.getByText('50 of 53')).toBeDefined()

  fireEvent.click(screen.getByRole('button', { name: 'Show more' }))

  expect(await screen.findByText('53 of 53')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Show more' })).toBeNull()
  const pages = asked(fetch, '/api/v1/foods/list')
  expect(pages.map((u) => u.searchParams.get('offset'))).toEqual(['0', '50'])
  expect(pages[0]?.searchParams.get('archived')).toBe('false')
  expect(pages[0]?.searchParams.get('order')).toBe('name')
  expect(pages[0]?.searchParams.has('source')).toBe(false)
})

test('by how many meals had them, the foods eaten this week come first', async () => {
  const yoghurt = food(2, { name: 'Greek yoghurt', last_use: lastUse(today), source: 'estimate' })
  // A week ago to the day: earlier.
  const rice = food(3, { name: 'Basmati rice', last_use: lastUse('2026-10-16') })
  const never = food(4, { name: 'Chia seeds' })
  const fetch = fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([rice, oats, yoghurt, never]),
  })
  renderAt('/foods')

  const week = await screen.findByRole('list', { name: 'In use: This week' })
  expect(
    within(week)
      .getAllByRole('link')
      .map((a) => a.textContent),
  ).toEqual([
    'RORolled oats · MillP 13 · C 60 · F 7370 kcal/100 g',
    // An estimate says so.
    'GYGreek yoghurtestimateP 5 · C 10 · F 3102 kcal/100 g',
  ])
  const earlier = screen.getByRole('list', { name: 'In use: Earlier' })
  expect(
    within(earlier)
      .getAllByRole('link')
      .map((a) => a.textContent),
  ).toEqual([
    // How long ago it was eaten, when not this week.
    'BRBasmati riceP 5 · C 10 · F 3 · 7 days ago103 kcal/100 g',
    'CSChia seedsP 5 · C 10 · F 3104 kcal/100 g',
  ])
  // The mark of a food eaten this week is in colour, of the others grey.
  expect(markOf(within(week).getAllByRole('link')[0])).toContain('bg-primary-soft')
  for (const row of within(earlier).getAllByRole('link')) {
    expect(markOf(row)).toContain('bg-surface-2')
    expect(markOf(row)).not.toContain('bg-primary-soft')
  }
  expect(screen.getByRole('heading', { name: 'This week' })).toBeDefined()
  expect(asked(fetch, '/api/v1/foods/list')[0]?.searchParams.get('order')).toBe('frequent')
  expect(screen.getByRole('button', { name: 'Order: Most eaten' })).toBeDefined()
  // The list chosen says how many foods it has.
  expect(screen.getByRole('radio', { name: 'In use · 4' })).toHaveProperty('checked', true)
  expect(screen.getByRole('radio', { name: 'Archive' })).toHaveProperty('checked', false)
})

test('the list chosen says how many foods it has, its order is a menu, in Croatian', async () => {
  const foods = [
    oats,
    food(2, { name: 'Basmati riža', last_use: lastUse('2026-10-14') }),
    ...Array.from({ length: 1200 }, (_, i) => food(i + 3)),
  ]
  const fetch = fakeServer({ ...signedIn('hr'), 'GET /api/v1/foods/list': listOf(foods) })
  const router = renderAt('/foods')

  expect(await screen.findByRole('radio', { name: 'U upotrebi · 1.202' })).toHaveProperty(
    'checked',
    true,
  )
  expect(screen.getByRole('link', { name: 'Nova' }).getAttribute('href')).toBe('/foods/new')
  const earlier = screen.getByRole('list', { name: 'U upotrebi: Ranije' })
  expect(within(earlier).getByRole('link', { name: /^Basmati riža/ }).textContent).toContain(
    'M 3 · prije 9 dana',
  )

  fireEvent.click(screen.getByRole('button', { name: 'Redoslijed: Najčešće' }))
  const menu = within(await screen.findByRole('menu'))
  expect(
    menu.getByRole('menuitemradio', { name: 'Prvo najčešće jedene' }).getAttribute('aria-checked'),
  ).toBe('true')
  fireEvent.click(menu.getByRole('menuitemradio', { name: 'Po nazivu' }))

  expect(await screen.findByRole('button', { name: 'Redoslijed: Po nazivu' })).toBeDefined()
  expect(router.state.location.search).toEqual({ order: 'name' })
  expect(asked(fetch, '/api/v1/foods/list').at(-1)?.searchParams.get('order')).toBe('name')
  // By name, the foods are one list, of the same count.
  expect(await screen.findByRole('list', { name: 'U upotrebi' })).toBeDefined()
  expect(screen.getByRole('radio', { name: 'U upotrebi · 1.202' })).toBeDefined()
})

test('the archived foods, and the order of the last use, are in the address', async () => {
  const fetch = fakeServer({ ...signedIn(), 'GET /api/v1/foods/list': listOf([]) })
  const router = renderAt('/foods')

  expect(
    await screen.findByText(
      'No foods yet. They appear when your AI coach logs a meal, or add one here.',
    ),
  ).toBeDefined()
  // The one action of an empty list: the header has none beside it.
  expect(screen.getByRole('link', { name: 'Add a food' }).getAttribute('href')).toBe('/foods/new')
  expect(screen.queryByRole('link', { name: 'New food' })).toBeNull()
  fireEvent.click(screen.getByRole('radio', { name: 'Archive' }))
  expect(await screen.findByText('No archived foods.')).toBeDefined()
  // The archive's list adds none, and the header does.
  expect(screen.queryByRole('link', { name: 'Add a food' })).toBeNull()
  expect(screen.getByRole('link', { name: 'New food' }).getAttribute('href')).toMatch(
    /^\/foods\/new\b/,
  )
  fireEvent.click(screen.getByRole('button', { name: 'Order: Most eaten' }))
  fireEvent.click(
    within(await screen.findByRole('menu')).getByRole('menuitemradio', {
      name: 'Recently eaten first',
    }),
  )

  await waitFor(() => {
    expect(router.state.location.search).toEqual({ filter: 'archived', order: 'use' })
  })
  const last = asked(fetch, '/api/v1/foods/list').at(-1)
  expect(last?.searchParams.get('archived')).toBe('true')
  expect(last?.searchParams.get('order')).toBe('use')
  expect(await screen.findByRole('radio', { name: 'Archive · 0' })).toHaveProperty('checked', true)
  expect(screen.getByRole('button', { name: 'Order: Recent' })).toBeDefined()
})

test("the archive's address before the lists is the archive", async () => {
  const fetch = fakeServer({ ...signedIn(), 'GET /api/v1/foods/list': listOf([]) })
  const router = renderAt('/foods?archived=true')

  expect(await screen.findByText('No archived foods.')).toBeDefined()
  expect(screen.getByRole('radio', { name: /^Archive/ })).toHaveProperty('checked', true)
  expect(asked(fetch, '/api/v1/foods/list').at(-1)?.searchParams.get('archived')).toBe('true')
  // The address is the lists' own now, so the foods in use are a tap away.
  await waitFor(() => {
    expect(router.state.location.search).toEqual({ filter: 'archived' })
  })
  fireEvent.click(screen.getByRole('radio', { name: 'In use' }))
  await waitFor(() => {
    expect(screen.getByRole('radio', { name: /^In use/ })).toHaveProperty('checked', true)
  })
  expect(router.state.location.search).toEqual({})
})

test('the estimates are the foods in use of values estimated, listed and searched', async () => {
  const guess = food(5, { name: 'Canteen goulash', source: 'estimate' })
  const soup = food(6, { name: 'Canteen soup', source: 'estimate' })
  // More estimates than a page of the list holds.
  const others = Array.from({ length: 205 }, (_, i) =>
    food(100 + i, { name: `Plate ${String(i)}`, source: 'estimate' }),
  )
  const fetch = fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': (request) =>
      new URL(request.url).searchParams.get('source') === 'estimate'
        ? listOf([guess, ...others, soup])(request)
        : listOf([oats, guess])(request),
    // A search of all the foods: the first ones it finds, none of them an
    // estimate.
    'GET /api/v1/foods': () => Response.json({ foods: [oats], unfound: [] }),
  })
  const router = renderAt('/foods')

  fireEvent.click(await screen.findByRole('radio', { name: 'Estimates' }))

  const list = await screen.findByRole('list', { name: 'Estimates: Earlier' })
  expect(within(list).getAllByRole('link').length).toBeGreaterThan(1)
  expect(router.state.location.search).toEqual({ filter: 'estimates' })
  const last = asked(fetch, '/api/v1/foods/list').at(-1)
  expect(last?.searchParams.get('source')).toBe('estimate')
  expect(last?.searchParams.get('archived')).toBe('false')

  // A search of the estimates is of every one of them, as find_foods
  // finds (the start of a word), not of what a search of all found.
  fireEvent.change(screen.getByLabelText('Search the foods'), { target: { value: 'canteen' } })
  const found = await screen.findByRole('list', { name: 'Found' })
  expect(
    within(found)
      .getAllByRole('link')
      .map((a) => a.getAttribute('href')),
  ).toEqual([
    `/foods/${guess.id}?q=canteen&filter=estimates`,
    `/foods/${soup.id}?q=canteen&filter=estimates`,
  ])
  expect(asked(fetch, '/api/v1/foods')).toEqual([])
})

test('a search finds the foods, or says none matches', async () => {
  const fetch = fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([oats]),
    'GET /api/v1/foods': (request) =>
      Response.json({
        foods: new URL(request.url).searchParams.get('query') === 'oat' ? [oats] : [],
        unfound: [],
      }),
  })
  const router = renderAt('/foods')

  fireEvent.change(await screen.findByLabelText('Search the foods'), { target: { value: 'oat' } })

  const found = await screen.findByRole('list', { name: 'Found' })
  expect(within(found).getByText('Rolled oats')).toBeDefined()
  expect(router.state.location.search).toEqual({ q: 'oat' })
  expect(asked(fetch, '/api/v1/foods').at(-1)?.searchParams.get('query')).toBe('oat')
  // A search is ordered by how well it matches.
  expect(screen.queryByRole('button', { name: /^Order/ })).toBeNull()

  fireEvent.change(screen.getByLabelText('Search the foods'), { target: { value: 'kale' } })
  expect(await screen.findByText('No food matches.')).toBeDefined()
})

test('a search in the archive finds the archived foods, and the archive stays', async () => {
  const stew = { ...oats, id: '0193c3a5-0000-7000-8000-0000000000f9', name: 'Canteen stew' }
  const fetch = fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([]),
    'GET /api/v1/foods': (request) =>
      Response.json({
        foods: new URL(request.url).searchParams.get('archived') === 'true' ? [stew] : [],
        unfound: [],
      }),
  })
  const router = renderAt('/foods?filter=archived')

  fireEvent.change(await screen.findByLabelText('Search the foods'), { target: { value: 'stew' } })

  const found = await screen.findByRole('list', { name: 'Found' })
  expect(within(found).getByText('Canteen stew')).toBeDefined()
  expect(asked(fetch, '/api/v1/foods').at(-1)?.searchParams.get('archived')).toBe('true')
  expect(screen.getByRole('radio', { name: /^Archive/ })).toHaveProperty('checked', true)
  // The foods in use, searched for the same.
  fireEvent.click(screen.getByRole('radio', { name: 'In use' }))
  expect(await screen.findByText('No food matches.')).toBeDefined()
  expect(asked(fetch, '/api/v1/foods').at(-1)?.searchParams.has('archived')).toBe(false)
  expect(router.state.location.search).toEqual({ q: 'stew' })
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

test('a list on its way is rows of its shape, and says it is loading', async () => {
  let answer: ((r: Response) => void) | undefined
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': () =>
      new Promise<Response>((resolve) => {
        answer = resolve
      }),
  })
  renderAt('/foods')

  expect(await screen.findByText('Loading…')).toBeDefined()
  answer?.(Response.json({ foods: [oats], total: 1 }))
  expect(await screen.findByText('Rolled oats')).toBeDefined()
  expect(screen.queryByText('Loading…')).toBeNull()
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
        ? Response.json({ foods: [oats], unfound: [] })
        : new Promise<Response>((resolve) => {
            answer = resolve
          }),
  })
  renderAt('/foods')

  fireEvent.change(await screen.findByLabelText('Search the foods'), { target: { value: 'oat' } })
  expect(await screen.findByRole('list', { name: 'Found' })).toBeDefined()
  fireEvent.change(screen.getByLabelText('Search the foods'), { target: { value: 'oats' } })

  expect(screen.getByRole('list', { name: 'Found' })).toBeDefined()
  answer?.(Response.json({ foods: [], unfound: [] }))
  expect(await screen.findByText('No food matches.')).toBeDefined()
})

test('the search field follows the address it did not write, and keeps what is typed', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([oats]),
    'GET /api/v1/foods': () => Response.json({ foods: [oats], unfound: [] }),
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
  expect(await screen.findByRole('list', { name: 'In use: This week' })).toBeDefined()
})

test("a food's page goes back to the foods as they were left", async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([{ ...oats, archived: true }]),
    [`GET /api/v1/foods/${oats.id}`]: () => Response.json({ ...oats, archived: true }),
  })
  renderAt('/foods?filter=archived&order=use')

  fireEvent.click(await screen.findByRole('link', { name: /Rolled oats/ }))

  const back = await screen.findByRole('link', { name: 'Back to the foods' })
  expect(back.getAttribute('href')).toBe('/foods?filter=archived&order=use')
  // A phone shows the food alone.
  expect(screen.queryByLabelText('Search the foods')).toBeNull()
})

test('a desktop has the food beside the list, which stays, and / finds a food', async () => {
  desktop()
  const rice = food(3, { name: 'Basmati rice' })
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([oats, rice]),
    [`GET /api/v1/foods/${oats.id}`]: () => Response.json(oats),
    [`GET /api/v1/foods/${rice.id}`]: () => Response.json(rice),
  })
  const router = renderAt('/foods?order=name')

  // Before a food is picked, the pane says to pick one.
  expect(await screen.findByText('Pick a food to see what it brings, or add one.')).toBeDefined()
  fireEvent.click(await screen.findByRole('link', { name: /Rolled oats/ }))

  expect(await screen.findByRole('heading', { name: 'Rolled oats', level: 1 })).toBeDefined()
  // The list stays beside it, the food marked in it, and no way back.
  const list = screen.getByRole('list', { name: 'In use' })
  expect(
    within(list)
      .getByRole('link', { name: /Rolled oats/ })
      .getAttribute('aria-current'),
  ).toBe('page')
  expect(screen.queryByRole('link', { name: 'Back to the foods' })).toBeNull()
  expect(router.state.location.search).toEqual({ order: 'name' })

  // Another list keeps the food open.
  fireEvent.click(screen.getByRole('radio', { name: 'Archive' }))
  await waitFor(() => {
    expect(router.state.location.pathname).toBe(`/foods/${oats.id}`)
  })
  expect(router.state.location.search).toEqual({ order: 'name', filter: 'archived' })
  expect(screen.getByRole('heading', { name: 'Rolled oats', level: 1 })).toBeDefined()

  fireEvent.keyDown(document.body, { key: '/' })
  expect(document.activeElement).toBe(screen.getByRole('searchbox', { name: 'Search the foods' }))
})

test('an address with a search typed on the way to another puts it in the field', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': listOf([]),
    'GET /api/v1/foods': () => Response.json({ foods: [], unfound: [] }),
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
    'GET /api/v1/foods': () => Response.json({ foods: [], unfound: [] }),
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
