import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test('a meal of before is logged again in two taps', async ({ page }) => {
  await signIn(page)
  // A meal logged as a model would, through the API: the latest.
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Lentil soup',
      items: [{ name: 'Lentil soup', nutrients: { kcal: 310, protein: 18, carbs: 40, fat: 8 } }],
    },
  })
  expect(logged.ok()).toBe(true)
  await page.goto('/')
  const soups = page.getByRole('article').filter({ hasText: 'Lentil soup' })
  await expect(soups).toHaveCount(1)

  // One: the bar's +. Two: the meal's Repeat.
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Log a meal' })
    .click()
  await page
    .getByRole('dialog', { name: 'A new meal' })
    .getByRole('button', { name: 'Repeat: Lentil soup' })
    .click()

  await expect(page.getByText('Logged: Lentil soup · 310 kcal')).toBeVisible()
  await expect(page.getByRole('dialog', { name: 'A new meal' })).toBeHidden()
  await expect(soups).toHaveCount(2)
})

test("the day's chips scroll on their own, not the page", async ({ page }) => {
  await signIn(page)
  // Foods of long names, each eaten: chips wider than a phone together.
  const names = ['Greek yoghurt with honey', 'Wholegrain sourdough', 'Smoked salmon slices']
  const logged = await Promise.all(
    names.map(async (name) => {
      const food = await page.request.post('/api/v1/foods', {
        data: {
          name,
          basis: '100g',
          source: 'label',
          nutrients: { kcal: 200, protein: 10, carbs: 20, fat: 8 },
        },
      })
      const saved: unknown = await food.json()
      const id =
        typeof saved === 'object' && saved !== null && 'id' in saved && typeof saved.id === 'string'
          ? saved.id
          : ''
      const meal = await page.request.post('/api/v1/meals', {
        data: { items: [{ food_id: id, amount: 150 }] },
      })
      return meal.ok()
    }),
  )
  expect(logged).toEqual([true, true, true])
  await page.goto('/')
  await expect(page.getByRole('region', { name: 'Quick add' })).toBeVisible()

  // A string: the tests' types know no DOM.
  const width = await page.evaluate<number>('document.documentElement.scrollWidth')
  expect(width).toBe(page.viewportSize()?.width)
})
