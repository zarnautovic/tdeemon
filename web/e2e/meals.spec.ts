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
