import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Food } from '@/api/types'
import { setLocale } from '@/i18n/locale'
import { askAgain, fakeServer, problem, renderAt, signedIn, unreachable } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

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
  note: 'from the label',
}

// foodServer is a server with oats, and others, which takes a save as the
// API does, and keeps the bodies it was sent.
function foodServer(
  save?: (body: unknown) => Response,
  locale: 'en' | 'hr' = 'en',
  start: Food = oats,
  others: Food[] = [],
) {
  let current = start
  const sent: unknown[] = []
  const fetch = fakeServer({
    ...signedIn(locale),
    ...Object.fromEntries(
      others.map((o) => [`GET /api/v1/foods/${o.id}`, () => Response.json(o)] as const),
    ),
    [`GET /api/v1/foods/${id}`]: () => Response.json(current),
    'POST /api/v1/foods/new': (request) => saved(request),
    'POST /api/v1/foods': (request) => saved(request),
  })
  async function saved(request: Request) {
    {
      const body: unknown = await request.json()
      sent.push(body)
      if (save) {
        return save(body)
      }
      const fields = typeof body === 'object' && body !== null ? body : {}
      current = {
        ...oats,
        brand: '',
        servings: [],
        aliases: [],
        note: '',
        ...fields,
        id:
          'id' in fields && typeof fields.id === 'string'
            ? fields.id
            : '0193c3a5-0000-7000-8000-0000000000f9',
      }
      return Response.json(current)
    }
  }
  return { fetch, sent }
}

test('a food that came stays when it cannot be asked again, the bar says since when', async () => {
  foodServer()
  const router = renderAt(`/foods/${id}`)
  const food = await screen.findByRole('article', { name: 'Rolled oats' })

  unreachable()
  await askAgain(router)

  expect(await screen.findByText(/^Offline · data from /)).toBeDefined()
  expect(within(food).getByRole('heading', { name: 'Rolled oats' })).toBeDefined()
  expect(screen.queryByRole('alert')).toBeNull()
})

test('a food that did not come says so in its place, and asks again', async () => {
  let up = false
  fakeServer({
    ...signedIn(),
    [`GET /api/v1/foods/${id}`]: () => (up ? Response.json(oats) : problem(500)),
  })
  renderAt(`/foods/${id}`)

  const alert = await screen.findByRole('alert')
  expect(within(alert).getByText("Couldn't load this food.")).toBeDefined()
  expect(within(alert).getByText('The server failed to answer.')).toBeDefined()
  up = true
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
  expect(await screen.findByRole('article', { name: 'Rolled oats' })).toBeDefined()
})

test('a food is corrected, the whole of it saved', async () => {
  const server = foodServer()
  const router = renderAt(`/foods/${id}?edit=true`)

  expect(await screen.findByRole('heading', { name: 'Edit the food' })).toBeDefined()
  expect(screen.getByLabelText('Energy (kcal)')).toHaveProperty('value', '370')
  expect(screen.getByLabelText('Fibre (g)')).toHaveProperty('value', '10')
  expect(screen.getByLabelText('Serving 1')).toHaveProperty('value', 'cup')
  expect(screen.getByLabelText('Amount 1 (g)')).toHaveProperty('value', '80')
  expect(screen.getByLabelText('Other name 1')).toHaveProperty('value', 'porridge oats')
  fireEvent.change(screen.getByLabelText('Energy (kcal)'), { target: { value: '372.5' } })
  fireEvent.click(screen.getByRole('button', { name: 'Remove the name: porridge oats' }))
  fireEvent.click(screen.getByRole('button', { name: 'Add a serving' }))
  fireEvent.change(screen.getByLabelText('Serving 2'), { target: { value: 'spoon' } })
  fireEvent.change(screen.getByLabelText('Amount 2 (g)'), { target: { value: '10' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  expect(await screen.findByText('Saved: Rolled oats')).toBeDefined()
  expect(server.sent).toEqual([
    {
      id,
      archived: false,
      name: 'Rolled oats',
      brand: 'Mill',
      basis: '100g',
      nutrients: { kcal: 372.5, protein: 13, carbs: 60, fat: 7, fiber: 10 },
      servings: [
        { name: 'cup', amount: 80 },
        { name: 'spoon', amount: 10 },
      ],
      source: 'label',
      note: 'from the label',
    },
  ])
  // Saved, the food is read again, in the form's place.
  expect(await screen.findByRole('heading', { name: 'Rolled oats' })).toBeDefined()
  expect(router.state.location.search).toEqual({})
})

test('what a food needs, and what is no number, is said and not sent', async () => {
  const server = foodServer()
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('Name'), { target: { value: ' ' } })
  fireEvent.change(screen.getByLabelText('Fat (g)'), { target: { value: 'some' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  expect(screen.getByLabelText('Name').getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByText('Needed.')).toBeDefined()
  expect(screen.getByLabelText('Fat (g)').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])
})

test('a value below 0 is said to be one, not no number', async () => {
  const server = foodServer()
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('Carbs (g)'), { target: { value: '-5' } })
  fireEvent.change(screen.getByLabelText('Fat (g)'), { target: { value: '\u22122,5' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  expect(screen.getAllByText('It cannot be less than 0.')).toHaveLength(2)
  expect(screen.getByLabelText('Carbs (g)').getAttribute('aria-invalid')).toBe('true')
  expect(screen.queryByText('Write a number, such as 72.5.')).toBeNull()
  expect(server.sent).toEqual([])
})

test("a field's error goes once it is changed, and the others' stay", async () => {
  const server = foodServer(undefined, 'hr')
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('Ugljikohidrati (g)'), {
    target: { value: '-5' },
  })
  fireEvent.change(screen.getByLabelText('Masti (g)'), { target: { value: '-1' } })
  fireEvent.change(screen.getByLabelText('Naziv'), { target: { value: '' } })
  fireEvent.click(screen.getByRole('button', { name: 'Spremi namirnicu' }))
  expect(screen.getAllByText('Ne može biti manje od 0.')).toHaveLength(2)

  fireEvent.change(screen.getByLabelText('Ugljikohidrati (g)'), { target: { value: '4' } })

  const carbs = screen.getByLabelText('Ugljikohidrati (g)')
  expect(carbs.getAttribute('aria-invalid')).toBe('false')
  expect(carbs.getAttribute('aria-describedby')).toBeNull()
  expect(screen.getAllByText('Ne može biti manje od 0.')).toHaveLength(1)
  expect(screen.getByLabelText('Masti (g)').getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByLabelText('Naziv').getAttribute('aria-invalid')).toBe('true')
  // The next save tells each field again: the one changed back is wrong.
  fireEvent.change(screen.getByLabelText('Ugljikohidrati (g)'), { target: { value: '-4' } })
  expect(screen.getAllByText('Ne može biti manje od 0.')).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Spremi namirnicu' }))
  expect(screen.getAllByText('Ne može biti manje od 0.')).toHaveLength(2)
  expect(server.sent).toEqual([])
})

test('grams past what 100 g holds are no longer said wrong once one of them changes', async () => {
  foodServer()
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('Protein (g)'), { target: { value: '200' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))
  expect(screen.getByLabelText('Carbs (g)').getAttribute('aria-invalid')).toBe('true')

  fireEvent.change(screen.getByLabelText('Protein (g)'), { target: { value: '20' } })

  expect(screen.queryByText(/add up to at most/)).toBeNull()
  expect(screen.getByLabelText('Carbs (g)').getAttribute('aria-invalid')).toBe('false')
  expect(screen.getByLabelText('Protein (g)').getAttribute('aria-invalid')).toBe('false')
})

test('a serving or another name given twice, as the server tells names apart, is said there and not sent', async () => {
  const server = foodServer(undefined, 'hr', {
    ...oats,
    servings: [{ name: 'čaša', amount: 250 }],
    aliases: ['grcki jogurt'],
  })
  renderAt(`/foods/${id}?edit=true`)

  // The same name in another case, its č of two code points.
  fireEvent.click(await screen.findByRole('button', { name: 'Dodaj mjeru' }))
  fireEvent.change(screen.getByLabelText('Mjera 2'), { target: { value: ' ČAŠA ' } })
  fireEvent.change(screen.getByLabelText('Količina 2 (g)'), { target: { value: '200' } })
  fireEvent.click(screen.getByRole('button', { name: 'Dodaj drugi naziv' }))
  fireEvent.change(screen.getByLabelText('Drugi naziv 2'), { target: { value: 'GRCKI  jogurt' } })
  // Another letter is another name.
  fireEvent.click(screen.getByRole('button', { name: 'Dodaj drugi naziv' }))
  fireEvent.change(screen.getByLabelText('Drugi naziv 3'), { target: { value: 'grčki jogurt' } })
  fireEvent.click(screen.getByRole('button', { name: 'Spremi namirnicu' }))

  const serving = screen.getByLabelText('Mjera 2')
  expect(serving.getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByText('Mjera tog naziva već postoji.').id).toBe(
    serving.getAttribute('aria-describedby'),
  )
  const alias = screen.getByLabelText('Drugi naziv 2')
  expect(alias.getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByText('Taj drugi naziv je već upisan.').id).toBe(
    alias.getAttribute('aria-describedby'),
  )
  expect(screen.getByLabelText('Mjera 1').getAttribute('aria-invalid')).toBe('false')
  expect(screen.getByLabelText('Drugi naziv 1').getAttribute('aria-invalid')).toBe('false')
  expect(screen.getByLabelText('Drugi naziv 3').getAttribute('aria-invalid')).toBe('false')
  expect(server.sent).toEqual([])

  // Named apart, they are sent.
  fireEvent.change(serving, { target: { value: 'velika čaša' } })
  fireEvent.click(screen.getByRole('button', { name: 'Ukloni naziv: GRCKI jogurt' }))
  fireEvent.click(screen.getByRole('button', { name: 'Spremi namirnicu' }))
  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  expect(server.sent[0]).toMatchObject({
    servings: [
      { name: 'čaša', amount: 250 },
      { name: 'velika čaša', amount: 200 },
    ],
    aliases: ['grcki jogurt', 'grčki jogurt'],
  })
})

// conflict is the API's 409 of a save another food is in the way of, at
// location.
function conflict(location: string, foodId: string): Response {
  return Response.json(
    {
      status: 409,
      title: 'Conflict',
      detail: `the name and brand are taken by food ${foodId}`,
      errors: [{ location, message: `taken by food ${foodId}`, value: foodId }],
    },
    { status: 409, headers: { 'Content-Type': 'application/problem+json' } },
  )
}

const otherOats: Food = { ...oats, id: '0193c3a5-0000-7000-8000-0000000000f7', aliases: [] }

test('a name and brand another food has says which food, under the name, until changed', async () => {
  foodServer(() => conflict('body.name', otherOats.id), 'en', oats, [otherOats])
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.click(await screen.findByRole('button', { name: 'Save the food' }))

  const link = await screen.findByRole('link', { name: 'Rolled oats · Mill' })
  expect(link.getAttribute('href')).toBe(`/foods/${otherOats.id}`)
  expect(screen.getByRole('alert').textContent).toBe(
    'Another food of yours has this name and brand: Rolled oats · Mill',
  )
  // A refusal, said at the name, which takes the focus: sent again, it
  // would be refused again.
  const name = screen.getByLabelText('Name')
  expect(name.getAttribute('aria-invalid')).toBe('true')
  expect(name.getAttribute('aria-describedby')).toBe(screen.getByRole('alert').id)
  await waitFor(() => {
    expect(document.activeElement).toBe(name)
  })
  expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()
  expect(screen.queryByText(/taken by food/)).toBeNull()
  // A change of the form makes it stale.
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Jumbo oats' } })
  expect(screen.queryByRole('link', { name: 'Rolled oats · Mill' })).toBeNull()
})

test('an other name another food has says which food, under it, in Croatian', async () => {
  const archived = { ...otherOats, name: 'Zobene pahuljice', brand: '', archived: true }
  foodServer(() => conflict('body.aliases[0]', archived.id), 'hr', oats, [archived])
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.click(await screen.findByRole('button', { name: 'Spremi namirnicu' }))

  await screen.findByRole('link', { name: 'Zobene pahuljice' })
  expect(screen.getByRole('alert').textContent).toBe(
    'Ovaj drugi naziv već ima tvoja namirnica: Zobene pahuljice (arhivirano)',
  )
  expect(screen.getByLabelText('Drugi naziv 1').getAttribute('aria-invalid')).toBe('true')
})

test('a barcode another food took meanwhile says which food, under the save, marking no field', async () => {
  const wafer = { ...otherOats, name: 'Caramel wafer', brand: '' }
  foodServer(
    () => conflict('body.barcodes[0]', wafer.id),
    'en',
    { ...oats, barcodes: ['96385074'] },
    [wafer],
  )
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.click(await screen.findByRole('button', { name: 'Save the food' }))

  await screen.findByRole('link', { name: 'Caramel wafer' })
  expect(screen.getByRole('alert').textContent).toBe(
    'Another food of yours has one of its barcodes: Caramel wafer',
  )
  expect(screen.getByLabelText('Name').getAttribute('aria-invalid')).toBe('false')
})

test('a conflict of an other name removed while the save was on its way marks no field', async () => {
  let answer: ((r: Response) => void) | undefined
  const withTwo = { ...oats, aliases: ['porridge oats', 'muesli'] }
  fakeServer({
    ...signedIn(),
    [`GET /api/v1/foods/${id}`]: () => Response.json(withTwo),
    [`GET /api/v1/foods/${otherOats.id}`]: () => Response.json(otherOats),
    'POST /api/v1/foods': () =>
      new Promise<Response>((resolve) => {
        answer = resolve
      }),
  })
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.click(await screen.findByRole('button', { name: 'Save the food' }))
  await waitFor(() => {
    expect(answer).toBeDefined()
  })
  fireEvent.click(screen.getByRole('button', { name: 'Remove the name: porridge oats' }))
  answer?.(conflict('body.aliases[0]', otherOats.id))

  expect((await screen.findByRole('alert')).textContent).toContain(
    'Another food of yours has this other name:',
  )
  // The other name it was is gone; muesli, now the first, is not it.
  expect(screen.getByLabelText('Other name 1').getAttribute('aria-invalid')).toBe('false')
})

test("a conflict that does not say its food is said in the server's words", async () => {
  foodServer(() =>
    Response.json(
      { status: 409, title: 'Conflict', detail: 'the name and brand are taken by food x ("Oats")' },
      { status: 409, headers: { 'Content-Type': 'application/problem+json' } },
    ),
  )
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.click(await screen.findByRole('button', { name: 'Save the food' }))

  expect((await screen.findByRole('alert')).textContent).toBe(
    'Another food has this name or other name: the name and brand are taken by food x ("Oats")',
  )
})

test('a food the server refuses says why', async () => {
  foodServer(() => problem(422))
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.click(await screen.findByRole('button', { name: 'Save the food' }))

  expect((await screen.findByRole('alert')).textContent).toBe(
    'The server did not take this food: what went wrong',
  )
})

test('a food is archived, and brought back', async () => {
  const server = foodServer()
  renderAt(`/foods/${id}`)

  fireEvent.click(await screen.findByRole('button', { name: 'Archive' }))

  expect(
    await screen.findByText('Archived. Find it among the archived foods; its names stay taken.'),
  ).toBeDefined()
  expect(server.sent[0]).toEqual({ ...oats, archived: true })
  fireEvent.click(screen.getByRole('button', { name: 'Bring back' }))
  await waitFor(() => {
    expect(server.sent[1]).toMatchObject({ archived: false })
  })
})

test("values past what 100 g can hold are said with the server's bounds, and not sent", async () => {
  const server = foodServer()
  renderAt(`/foods/${id}?edit=true`)

  // 200 g of protein in 100 g, more energy than fat, more sugars than 100 g.
  fireEvent.change(await screen.findByLabelText('Protein (g)'), { target: { value: '200' } })
  fireEvent.change(screen.getByLabelText('Energy (kcal)'), { target: { value: '950' } })
  fireEvent.change(screen.getByLabelText('Sugars (g)'), { target: { value: '120' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  expect(
    screen.getByText('Protein, carbs, fat and alcohol add up to at most 101 g per 100 g.'),
  ).toBeDefined()
  expect(screen.getByLabelText('Protein (g)').getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByLabelText('Carbs (g)').getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByText('At most 900 kcal per 100 g, the energy of pure fat.')).toBeDefined()
  expect(screen.getByText('At most 101 g per 100 g.')).toBeDefined()
  expect(server.sent).toEqual([])
})

test('a portion is no feast, and of its grams known holds what 100 g can, in Croatian', async () => {
  const server = foodServer(undefined, 'hr')
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('Vrijednosti su'), { target: { value: 'portion' } })
  fireEvent.change(screen.getByLabelText('Energija (kcal)'), { target: { value: '10500' } })
  fireEvent.click(screen.getByRole('button', { name: 'Spremi namirnicu' }))
  expect(screen.getByText('Najviše 10.000 kcal u porciji.')).toBeDefined()

  // 1,000 kcal in a portion of 50 g is 2,000 in 100 g.
  fireEvent.change(screen.getByLabelText('Energija (kcal)'), { target: { value: '1000' } })
  fireEvent.change(screen.getByLabelText('Grama u porciji'), { target: { value: '50' } })
  fireEvent.click(screen.getByRole('button', { name: 'Spremi namirnicu' }))
  expect(screen.getByText('Najviše 900 kcal na 100 g, energija čiste masti.')).toBeDefined()
  expect(server.sent).toEqual([])
})

test('a food of a portion has its grams, and none else', async () => {
  const server = foodServer()
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('The values are'), { target: { value: 'portion' } })
  fireEvent.change(screen.getByLabelText('Grams of a portion'), { target: { value: '450' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  await waitFor(() => {
    expect(server.sent[0]).toMatchObject({ basis: 'portion', portion_grams: 450 })
  })
  // Saved, the food is read; edited again, it is of 100 ml.
  fireEvent.click(await screen.findByRole('link', { name: 'Edit' }))
  fireEvent.change(await screen.findByLabelText('The values are'), { target: { value: '100ml' } })
  expect(screen.queryByLabelText('Grams of a portion')).toBeNull()
  expect(screen.getByLabelText('Amount 1 (ml)')).toBeDefined()
  // The grams typed for a portion do not go with a food per 100 ml.
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))
  await waitFor(() => {
    expect(server.sent).toHaveLength(2)
  })
  expect(server.sent[1]).not.toHaveProperty('portion_grams')
})

test('a food that is not there says so', async () => {
  fakeServer({ ...signedIn(), [`GET /api/v1/foods/${id}`]: () => problem(404) })
  renderAt(`/foods/${id}`)

  expect(await screen.findByText('There is no such food.')).toBeDefined()
  expect(screen.getByRole('link', { name: 'Back to the foods' }).getAttribute('href')).toBe(
    '/foods',
  )
})

test('a new food is made, and its page shown', async () => {
  const server = foodServer()
  const router = renderAt('/foods/new')

  fireEvent.change(await screen.findByLabelText('Name'), { target: { value: 'Kefir' } })
  fireEvent.change(screen.getByLabelText('The values are'), { target: { value: '100ml' } })
  for (const [label, value] of [
    ['Energy (kcal)', '41'],
    ['Protein (g)', '3,4'],
    ['Carbs (g)', '4'],
    ['Fat (g)', '1.5'],
  ] as const) {
    fireEvent.change(screen.getByLabelText(label), { target: { value } })
  }
  fireEvent.change(screen.getByLabelText('Values from'), { target: { value: 'estimate' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/foods/0193c3a5-0000-7000-8000-0000000000f9')
  })
  expect(server.sent).toEqual([
    {
      name: 'Kefir',
      basis: '100ml',
      nutrients: { kcal: 41, protein: 3.4, carbs: 4, fat: 1.5 },
      source: 'estimate',
    },
  ])
})

test('a food is corrected in Croatian, its numbers too', async () => {
  const server = foodServer(undefined, 'hr')
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('Proteini (g)'), { target: { value: '13,5' } })
  fireEvent.click(screen.getByRole('button', { name: 'Spremi namirnicu' }))

  await waitFor(() => {
    expect(server.sent[0]).toMatchObject({ nutrients: { protein: 13.5 } })
  })
})

test('an archived food corrected stays archived', async () => {
  const server = foodServer(undefined, 'en', { ...oats, archived: true })
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('Note'), { target: { value: 'from the box' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  await waitFor(() => {
    expect(server.sent[0]).toMatchObject({ archived: true, note: 'from the box' })
  })
})

test('a new food of a name another food has is refused, not saved over it', async () => {
  const paths: string[] = []
  foodServer((body) => {
    paths.push(JSON.stringify(body))
    return Response.json(
      {
        status: 409,
        title: 'Conflict',
        detail: 'the name and brand are taken by food x ("Rolled oats", archived)',
      },
      { status: 409, headers: { 'Content-Type': 'application/problem+json' } },
    )
  })
  const fetch = vi.mocked(globalThis.fetch)
  renderAt('/foods/new')

  fireEvent.change(await screen.findByLabelText('Name'), { target: { value: 'rolled OATS' } })
  for (const label of ['Energy (kcal)', 'Protein (g)', 'Carbs (g)', 'Fat (g)']) {
    fireEvent.change(screen.getByLabelText(label), { target: { value: '1' } })
  }
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  expect((await screen.findByRole('alert')).textContent).toBe(
    'Another food has this name or other name: the name and brand are taken by food x ("Rolled oats", archived)',
  )
  const posts = fetch.mock.calls
    .map(([r]) => (r instanceof Request && r.method === 'POST' ? new URL(r.url).pathname : ''))
    .filter(Boolean)
  expect(posts).toEqual(['/api/v1/foods/new'])
})

test('a try again with a draft that cannot be sent says what is wrong', async () => {
  foodServer(() => problem(500))
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.click(await screen.findByRole('button', { name: 'Save the food' }))
  await screen.findByRole('button', { name: 'Try again' })
  fireEvent.change(screen.getByLabelText('Fat (g)'), { target: { value: '' } })
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

  expect(screen.getByLabelText('Fat (g)').getAttribute('aria-invalid')).toBe('true')
  await waitFor(() => {
    expect(document.activeElement).toBe(screen.getByLabelText('Fat (g)'))
  })
})

test('an address that names no id says there is no such food', async () => {
  fakeServer({ ...signedIn(), 'GET /api/v1/foods/abc': () => problem(422) })
  renderAt('/foods/abc')

  expect(await screen.findByText('There is no such food.')).toBeDefined()
})

test('a value saved untouched keeps its decimals, and words get a keyboard of letters', async () => {
  const server = foodServer(undefined, 'en', {
    ...oats,
    nutrients: { ...oats.nutrients, salt: 0.012345 },
  })
  renderAt(`/foods/${id}?edit=true`)

  expect(await screen.findByLabelText('Salt (g)')).toHaveProperty('value', '0.012345')
  expect(screen.getByLabelText('Other name 1').getAttribute('inputmode')).toBeNull()
  expect(screen.getByLabelText('Serving 1').getAttribute('inputmode')).toBeNull()
  expect(screen.getByLabelText('Amount 1 (g)').getAttribute('inputmode')).toBe('decimal')
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  await waitFor(() => {
    expect(server.sent[0]).toMatchObject({ nutrients: { salt: 0.012345 } })
  })
})

test('a serving of no amount, or of more than the server takes, is not sent', async () => {
  const server = foodServer()
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('Amount 1 (g)'), { target: { value: '0' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  expect(screen.getByText('More than 0, at most 10000.')).toBeDefined()
  expect(server.sent).toEqual([])
})

test('a serving or another name past the most a food has is said there, and not sent', async () => {
  const server = foodServer(undefined, 'en', {
    ...oats,
    servings: Array.from({ length: 20 }, (_, i) => ({ name: `cup ${String(i + 1)}`, amount: 80 })),
    aliases: Array.from({ length: 20 }, (_, i) => `oats ${String(i + 1)}`),
  })
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.click(await screen.findByRole('button', { name: 'Add a serving' }))
  fireEvent.change(screen.getByLabelText('Serving 21'), { target: { value: 'bowl' } })
  fireEvent.change(screen.getByLabelText('Amount 21 (g)'), { target: { value: '120' } })
  fireEvent.click(screen.getByRole('button', { name: 'Add another name' }))
  fireEvent.change(screen.getByLabelText('Other name 21'), { target: { value: 'muesli base' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  const serving = screen.getByLabelText('Serving 21')
  expect(serving.getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByText('A food has at most 20 servings.').id).toBe(
    serving.getAttribute('aria-describedby'),
  )
  expect(document.activeElement).toBe(serving)
  const alias = screen.getByLabelText('Other name 21')
  expect(alias.getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByText('A food has at most 20 other names.').id).toBe(
    alias.getAttribute('aria-describedby'),
  )
  expect(screen.getByLabelText('Serving 20').getAttribute('aria-invalid')).toBe('false')
  expect(screen.getByLabelText('Other name 20').getAttribute('aria-invalid')).toBe('false')
  expect(server.sent).toEqual([])

  // One of each taken out, the rest are sent.
  fireEvent.click(screen.getByRole('button', { name: 'Remove the serving: cup 1' }))
  fireEvent.click(screen.getByRole('button', { name: 'Remove the name: oats 1' }))
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))
  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  expect(server.sent[0]).toMatchObject({
    servings: expect.arrayContaining([{ name: 'bowl', amount: 120 }]) as unknown,
    aliases: expect.arrayContaining(['muesli base']) as unknown,
  })
})

test("a food's barcodes, which a scan adds, go with it when the form saves it", async () => {
  const server = foodServer(undefined, 'en', { ...oats, barcodes: ['3850000000015', '96385074'] })
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('Energy (kcal)'), { target: { value: '372' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))
  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  expect(server.sent[0]).toMatchObject({ id, barcodes: ['3850000000015', '96385074'] })
})

test('a food from Open Food Facts keeps it as its source when saved; one typed in is not offered it', async () => {
  const server = foodServer(undefined, 'en', {
    ...oats,
    source: 'off',
    barcodes: ['3850000000015'],
  })
  renderAt(`/foods/${id}?edit=true`)

  const source = await screen.findByLabelText('Values from')
  expect(source).toHaveProperty('value', 'off')
  expect(within(source).getByRole('option', { name: 'Open Food Facts' })).toBeDefined()
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))
  await waitFor(() => {
    expect(server.sent).toHaveLength(1)
  })
  expect(server.sent[0]).toMatchObject({ source: 'off', barcodes: ['3850000000015'] })
  cleanup()

  foodServer()
  renderAt(`/foods/${id}?edit=true`)
  const typed = await screen.findByLabelText('Values from')
  expect(within(typed).queryByRole('option', { name: 'Open Food Facts' })).toBeNull()
})

test('energy far from what the macros make is warned of, with theirs to take, and saved all the same', async () => {
  const server = foodServer()
  const router = renderAt('/foods/new')

  fireEvent.change(await screen.findByLabelText('Name'), { target: { value: 'Skyr' } })
  // 300 kcal of P 20 / C 5 / F 1.5, which make about 114.
  for (const [label, value] of [
    ['Energy (kcal)', '300'],
    ['Protein (g)', '20'],
    ['Carbs (g)', '5'],
  ] as const) {
    fireEvent.change(screen.getByLabelText(label), { target: { value } })
  }
  // Not before the four are there.
  expect(screen.queryByText(/The macros make about/)).toBeNull()
  fireEvent.change(screen.getByLabelText('Fat (g)'), { target: { value: '1.5' } })

  const warning = screen.getByText('The macros make about 114 kcal, not 300: check the values.')
  expect(screen.getByLabelText('Energy (kcal)').getAttribute('aria-describedby')).toBe(warning.id)
  // A warning, not a refusal: the field is not wrong.
  expect(screen.getByLabelText('Energy (kcal)').getAttribute('aria-invalid')).toBe('false')
  fireEvent.click(screen.getByRole('button', { name: 'Use 114 kcal' }))

  expect(screen.getByLabelText('Energy (kcal)')).toHaveProperty('value', '114')
  expect(screen.queryByText(/The macros make about/)).toBeNull()
  // Typed again, it is saved as typed.
  fireEvent.change(screen.getByLabelText('Energy (kcal)'), { target: { value: '300' } })
  expect(screen.getByText(/The macros make about 114 kcal/)).toBeDefined()
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/foods/0193c3a5-0000-7000-8000-0000000000f9')
  })
  expect(server.sent).toEqual([
    {
      name: 'Skyr',
      basis: '100g',
      nutrients: { kcal: 300, protein: 20, carbs: 5, fat: 1.5 },
      source: 'label',
    },
  ])
})

test('the energy of fibre and polyols counts, in Croatian too', async () => {
  foodServer(undefined, 'hr')
  renderAt('/foods/new')

  // Sugar-free sweets: 64 g of carbs, 60 of them polyols, of 2.4 kcal.
  await screen.findByLabelText('Naziv')
  for (const [label, value] of [
    ['Energija (kcal)', '160'],
    ['Proteini (g)', '0'],
    ['Ugljikohidrati (g)', '64'],
    ['Masti (g)', '0'],
  ] as const) {
    fireEvent.change(screen.getByLabelText(label), { target: { value } })
  }
  expect(
    screen.getByText('Makroi daju oko 256 kcal, a ne 160: provjeri vrijednosti.'),
  ).toBeDefined()
  fireEvent.change(screen.getByLabelText('Polioli (g)'), { target: { value: '60' } })

  expect(screen.queryByText(/Makroi daju oko/)).toBeNull()
  // Fibre's 2 kcal a gram, apart from the carbs.
  fireEvent.change(screen.getByLabelText('Energija (kcal)'), { target: { value: '1500,5' } })
  fireEvent.change(screen.getByLabelText('Vlakna (g)'), { target: { value: '10' } })
  expect(
    screen.getByText('Makroi daju oko 180 kcal, a ne 1.501: provjeri vrijednosti.'),
  ).toBeDefined()
  expect(screen.getByRole('button', { name: 'Postavi 180 kcal' })).toBeDefined()
})

test('a serving or another name added takes the focus to its first field', async () => {
  foodServer()
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.click(await screen.findByRole('button', { name: 'Add a serving' }))
  await waitFor(() => {
    expect(document.activeElement).toBe(screen.getByLabelText('Serving 2'))
  })
  fireEvent.click(screen.getByRole('button', { name: 'Add another name' }))
  await waitFor(() => {
    expect(document.activeElement).toBe(screen.getByLabelText('Other name 2'))
  })
  fireEvent.click(screen.getByRole('button', { name: 'Add a serving' }))
  await waitFor(() => {
    expect(document.activeElement).toBe(screen.getByLabelText('Serving 3'))
  })
})

test('a save says so in a toast whose Undo takes it back', async () => {
  const undos: unknown[] = []
  foodServer()
  const fetch = vi.mocked(globalThis.fetch)
  const serve = fetch.getMockImplementation()
  fetch.mockImplementation(async (input, init) => {
    const request = input instanceof Request ? input : new Request(input, init)
    if (new URL(request.url).pathname === '/api/v1/undo') {
      undos.push(await request.json())
      return Response.json({
        undone: {
          action: 'food.save',
          entity: id,
          at: '2026-10-23T16:00:00Z',
          client: 'web',
          days: [],
        },
      })
    }
    return serve ? serve(input, init) : Response.error()
  })
  renderAt(`/foods/${id}?edit=true`)

  fireEvent.change(await screen.findByLabelText('Note'), { target: { value: 'from the box' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))
  expect(await screen.findByText('Saved: Rolled oats')).toBeDefined()
  fireEvent.click(screen.getByRole('button', { name: 'Undo' }))

  expect(await screen.findByText('Undone: a food saved.')).toBeDefined()
  expect(undos).toEqual([
    { id: expect.any(String) as unknown, last: { action: 'food.save', entity: id } },
  ])
})
