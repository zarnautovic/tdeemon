import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test('a meal is corrected, deleted, and brought back by undo', async ({ page }) => {
  await signIn(page)
  // A food and a meal of it, logged as a model would, through the API.
  const food = await page.request.post('/api/v1/foods', {
    data: {
      name: 'Rolled oats',
      basis: '100g',
      source: 'label',
      nutrients: { kcal: 370, protein: 13, carbs: 60, fat: 7 },
    },
  })
  expect(food.ok()).toBe(true)
  const saved: unknown = await food.json()
  const foodId =
    typeof saved === 'object' && saved !== null && 'id' in saved && typeof saved.id === 'string'
      ? saved.id
      : ''
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Porridge',
      items: [
        { food_id: foodId, amount: 80 },
        { name: 'Blueberries', nutrients: { kcal: 40, protein: 0.5, carbs: 9, fat: 0.2 } },
      ],
    },
  })
  expect(logged.ok()).toBe(true)

  await page.goto('/')
  const porridge = page.getByRole('article').filter({ hasText: 'Porridge' })
  await expect(porridge).toContainText('80 g')

  await porridge.getByRole('button', { name: 'Change the meal: Porridge' }).click()
  await porridge.getByLabel('Amount: Rolled oats').fill('100')
  await porridge.getByRole('button', { name: 'Remove: Blueberries' }).click()
  await porridge.getByRole('button', { name: 'Save the meal' }).click()
  await expect(porridge).toContainText('100 g')
  await expect(porridge).toContainText('370 kcal')
  await expect(porridge).not.toContainText('Blueberries')

  await porridge.getByRole('button', { name: 'Change the meal: Porridge' }).click()
  await porridge.getByRole('button', { name: 'Delete the meal' }).click()
  await porridge.getByRole('button', { name: 'Delete', exact: true }).click()
  await expect(porridge).toHaveCount(0)

  await page.getByRole('button', { name: 'Undo the latest change' }).click()
  await expect(page.getByText('Taken back: a meal deleted.')).toBeVisible()
  await expect(porridge).toContainText('100 g')
})

test('a meal is repeated today in a tap', async ({ page }) => {
  await signIn(page)
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Toast',
      items: [{ name: 'Toast and jam', nutrients: { kcal: 260, protein: 6, carbs: 48, fat: 4 } }],
    },
  })
  expect(logged.ok()).toBe(true)

  await page.goto('/')
  const toast = page.getByRole('article').filter({ hasText: 'Toast' })
  await expect(toast).toHaveCount(1)
  await toast.getByRole('button', { name: 'Repeat today: Toast' }).click()
  await expect(page.getByText('Logged again, today at this time.')).toBeVisible()
  await expect(toast).toHaveCount(2)
})

test("an item's note changed alone keeps what it brought, its food corrected since", async ({
  page,
}) => {
  await signIn(page)
  const created = await page.request.post('/api/v1/foods', {
    data: {
      name: 'Spelt bread',
      basis: '100g',
      source: 'label',
      nutrients: { kcal: 250, protein: 9, carbs: 45, fat: 3 },
    },
  })
  expect(created.ok()).toBe(true)
  const food: unknown = await created.json()
  if (typeof food !== 'object' || food === null || !('id' in food)) {
    throw new Error('no food')
  }
  const logged = await page.request.post('/api/v1/meals', {
    data: { description: 'Toast break', items: [{ food_id: food.id, amount: 100 }] },
  })
  expect(logged.ok()).toBe(true)
  // The bread is corrected after the meal.
  const corrected = await page.request.post('/api/v1/foods', {
    data: { ...food, nutrients: { kcal: 300, protein: 9, carbs: 45, fat: 3 } },
  })
  expect(corrected.ok()).toBe(true)

  await page.goto('/')
  const toast = page.getByRole('article').filter({ hasText: 'Toast break' })
  await expect(toast).toContainText('250 kcal')
  await toast.getByRole('button', { name: 'Change the meal: Toast break' }).click()
  await toast.getByLabel('Note: Spelt bread').fill('with honey')
  await toast.getByRole('button', { name: 'Save the meal' }).click()

  await expect(toast.getByRole('button', { name: 'Save the meal' })).toHaveCount(0)
  await expect(toast).toContainText('250 kcal')
})
