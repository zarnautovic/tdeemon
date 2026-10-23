import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
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

// foodServer is a server with oats, which takes a save as the API does,
// and keeps the bodies it was sent.
function foodServer(save?: (body: unknown) => Response, locale: 'en' | 'hr' = 'en') {
  let current = oats
  const sent: unknown[] = []
  const fetch = fakeServer({
    ...signedIn(locale),
    [`GET /api/v1/foods/${id}`]: () => Response.json(current),
    'POST /api/v1/foods': async (request) => {
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
    },
  })
  return { fetch, sent }
}

test('a food is corrected, the whole of it saved', async () => {
  const server = foodServer()
  renderAt(`/foods/${id}`)

  expect(await screen.findByRole('heading', { name: 'Rolled oats' })).toBeDefined()
  expect(screen.getByLabelText('Energy (kcal)')).toHaveProperty('value', '370')
  expect(screen.getByLabelText('Fibre (g)')).toHaveProperty('value', '10')
  expect(screen.getByLabelText('Serving')).toHaveProperty('value', 'cup')
  expect(screen.getByLabelText('Amount (g)')).toHaveProperty('value', '80')
  expect(screen.getByLabelText('Other name')).toHaveProperty('value', 'porridge oats')
  fireEvent.change(screen.getByLabelText('Energy (kcal)'), { target: { value: '372.5' } })
  fireEvent.click(screen.getByRole('button', { name: 'Remove the name: porridge oats' }))
  fireEvent.click(screen.getByRole('button', { name: 'Add a serving' }))
  const servings = screen.getAllByLabelText('Serving')
  fireEvent.change(servings[1] ?? document.body, { target: { value: 'spoon' } })
  fireEvent.change(screen.getAllByLabelText('Amount (g)')[1] ?? document.body, {
    target: { value: '10' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  expect(await screen.findByText('Saved')).toBeDefined()
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
})

test('what a food needs, and what is no number, is said and not sent', async () => {
  const server = foodServer()
  renderAt(`/foods/${id}`)

  fireEvent.change(await screen.findByLabelText('Name'), { target: { value: ' ' } })
  fireEvent.change(screen.getByLabelText('Fat (g)'), { target: { value: 'some' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  expect(screen.getByLabelText('Name').getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByText('Needed.')).toBeDefined()
  expect(screen.getByLabelText('Fat (g)').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])
})

test("a name another food has is refused, in the server's words", async () => {
  foodServer(() =>
    Response.json(
      { status: 409, title: 'Conflict', detail: 'the name and brand are taken by food x ("Oats")' },
      { status: 409, headers: { 'Content-Type': 'application/problem+json' } },
    ),
  )
  renderAt(`/foods/${id}`)

  fireEvent.click(await screen.findByRole('button', { name: 'Save the food' }))

  expect((await screen.findByRole('alert')).textContent).toBe(
    'Another food is in the way: the name and brand are taken by food x ("Oats")',
  )
})

test('a food the server refuses says why', async () => {
  foodServer(() => problem(422))
  renderAt(`/foods/${id}`)

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
    await screen.findByText('Archived: no search finds it, and its names stay taken.'),
  ).toBeDefined()
  expect(server.sent[0]).toEqual({ ...oats, archived: true })
  fireEvent.click(screen.getByRole('button', { name: 'Bring back' }))
  await waitFor(() => {
    expect(server.sent[1]).toMatchObject({ archived: false })
  })
})

test('a food of a portion has its grams, and none else', async () => {
  const server = foodServer()
  renderAt(`/foods/${id}`)

  fireEvent.change(await screen.findByLabelText('The values are'), { target: { value: 'portion' } })
  fireEvent.change(screen.getByLabelText('Grams of a portion'), { target: { value: '450' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save the food' }))

  await waitFor(() => {
    expect(server.sent[0]).toMatchObject({ basis: 'portion', portion_grams: 450 })
  })
  fireEvent.change(screen.getByLabelText('The values are'), { target: { value: '100ml' } })
  expect(screen.queryByLabelText('Grams of a portion')).toBeNull()
  expect(screen.getByLabelText('Amount (ml)')).toBeDefined()
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
  fireEvent.change(screen.getByLabelText('The values are from'), { target: { value: 'estimate' } })
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
  renderAt(`/foods/${id}`)

  fireEvent.change(await screen.findByLabelText('Proteini (g)'), { target: { value: '13,5' } })
  fireEvent.click(screen.getByRole('button', { name: 'Spremi namirnicu' }))

  await waitFor(() => {
    expect(server.sent[0]).toMatchObject({ nutrients: { protein: 13.5 } })
  })
})
