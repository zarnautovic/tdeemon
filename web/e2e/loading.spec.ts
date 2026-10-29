// The checks of where the first meal is run code in the page, which has
// the DOM's types.
/// <reference lib="dom" />

import { expect, test, type Route } from '@playwright/test'

import { signIn } from './user.ts'

// slow answers a request of the API after 1.5 s, as a slow network does.
async function slow(route: Route) {
  await new Promise((resolve) => setTimeout(resolve, 1500))
  await route.continue()
}

test('on a slow network today is a skeleton of its shape, then all at once: no meal moves', async ({
  page,
}) => {
  await signIn(page)
  // A food logged today, which the chips over the meals then offer.
  const food = await page.request.post('/api/v1/foods/new', {
    data: {
      name: 'Greek yoghurt',
      basis: '100g',
      source: 'label',
      nutrients: { kcal: 97, protein: 9, carbs: 4, fat: 5 },
    },
  })
  expect(food.ok()).toBe(true)
  const saved: unknown = await food.json()
  const id =
    typeof saved === 'object' && saved !== null && 'id' in saved && typeof saved.id === 'string'
      ? saved.id
      : ''
  const meal = await page.request.post('/api/v1/meals', {
    data: { description: 'Yoghurt bowl', items: [{ food_id: id, amount: 200 }] },
  })
  expect(meal.ok()).toBe(true)

  // Where the first meal is, in the page, every frame from the first it
  // is shown in.
  await page.addInitScript(() => {
    const tops: number[] = []
    Object.assign(globalThis, { mealTops: tops })
    const look = () => {
      const first = document.querySelector('main article')
      if (first && first.getClientRects().length > 0) {
        tops.push(Math.round(first.getBoundingClientRect().top + window.scrollY))
      }
      requestAnimationFrame(look)
    }
    requestAnimationFrame(look)
  })
  await page.route('**/api/v1/**', slow)
  await page.goto('/')

  // The skeleton first, under the page's header.
  await expect(page.getByText('Loading…')).toBeAttached({ timeout: 10_000 })
  await expect(page.getByRole('heading', { name: 'Today', exact: true })).toBeVisible()
  await expect(page.getByRole('article').filter({ hasText: 'Yoghurt bowl' })).toBeVisible({
    timeout: 15_000,
  })
  await expect(page.getByText('Loading…')).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Quick add' })).toBeVisible()
  // Whatever comes after the meals, nothing comes above them.
  await page.waitForLoadState('networkidle')
  const tops = await page.evaluate(() => {
    const seen: unknown = Reflect.get(globalThis, 'mealTops')
    return Array.isArray(seen)
      ? seen.filter((top: unknown): top is number => typeof top === 'number')
      : []
  })
  expect(tops.length).toBeGreaterThan(0)
  expect(new Set(tops)).toEqual(new Set([tops[0]]))
})

test('a day that does not come keeps its header and the navigation, and comes when asked again', async ({
  page,
}) => {
  await signIn(page)
  await page.route('**/api/v1/days/**', (route) =>
    route.fulfill({
      status: 500,
      contentType: 'application/problem+json',
      body: JSON.stringify({ status: 500, title: 'Internal Server Error' }),
    }),
  )
  await page.goto('/')

  const alert = page.getByRole('alert')
  await expect(alert).toContainText("Couldn't load today.")
  await expect(alert).toContainText('The server failed to answer.')
  await expect(page.getByRole('heading', { name: 'Today', exact: true })).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Sections' })).toBeVisible()

  await page.unroute('**/api/v1/days/**')
  await page.getByRole('button', { name: 'Try again' }).click()
  await expect(page.getByRole('heading', { name: 'Meals' })).toBeVisible()
  await expect(alert).toHaveCount(0)
})

test('without the server what came stays, with a bar that says since when; what did not says so', async ({
  page,
}) => {
  await signIn(page)
  await expect(page.getByRole('heading', { name: 'Meals' })).toBeVisible()
  await page.route('**/api/v1/**', (route) => route.abort('internetdisconnected'))
  const sections = page.getByRole('navigation', { name: 'Sections' })

  // The foods, not asked for before, say so in their place, in the frame.
  await sections.getByRole('link', { name: 'Foods' }).click()
  await expect(page.getByRole('alert')).toContainText("Couldn't load your foods.")
  await expect(page.getByRole('heading', { name: 'Foods', level: 1 })).toBeVisible()
  await expect(sections).toBeVisible()

  // Today, which could not be asked again, stays as it was.
  await sections.getByRole('link', { name: 'Today' }).click()
  await expect(page.getByText(/^Offline · data from \d{1,2}:\d{2}/)).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Meals' })).toBeVisible()
  await expect(page.getByRole('alert')).toHaveCount(0)

  // The server back, and the window in focus again, the bar goes.
  await page.unroute('**/api/v1/**')
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByText(/^Offline/)).toHaveCount(0)
})
