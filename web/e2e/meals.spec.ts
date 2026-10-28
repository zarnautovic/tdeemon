import { expect, test } from '@playwright/test'

import { openMeal, signIn } from './user.ts'

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
  await openMeal(porridge)
  await expect(porridge).toContainText('80 g')

  await porridge.getByRole('button', { name: 'Change the meal: Porridge' }).click()
  // The editor is a sheet over the day, its total said as it changes.
  const editor = page.getByRole('dialog', { name: 'Change the meal' })
  await editor.getByLabel('Amount: Rolled oats').fill('100')
  await expect(editor).toContainText('370 kcal · P 13 · was 296')
  await editor.getByRole('button', { name: 'Remove: Blueberries' }).click()
  await expect(editor).toContainText('+34 kcal from the changes')
  await editor.getByRole('button', { name: 'Save the meal' }).click()
  await expect(editor).toBeHidden()
  await expect(porridge).toContainText('100 g')
  await expect(porridge).toContainText('370 kcal')
  await expect(porridge).not.toContainText('Blueberries')

  // The change taken back by its toast's Undo: the item removed is back.
  await page.getByRole('button', { name: 'Undo' }).click()
  await expect(page.getByText('Taken back: a meal changed.')).toBeVisible()
  await expect(porridge).toContainText('Blueberries')
  await expect(porridge).toContainText('80 g')

  // Deleted at once, and brought back by its toast's Undo.
  await porridge.getByRole('button', { name: 'Change the meal: Porridge' }).click()
  await editor.getByRole('button', { name: 'Delete the meal' }).click()
  await expect(porridge).toHaveCount(0)
  await expect(page.getByText('Deleted: Porridge')).toBeVisible()

  await page.getByRole('button', { name: 'Undo' }).click()
  await expect(page.getByText('Taken back: a meal deleted.')).toBeVisible()
  await openMeal(porridge)
  await expect(porridge).toContainText('Blueberries')
})

test('a meal moved to the day before says so, and its toast brings it back', async ({ page }) => {
  await signIn(page)
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Late snack',
      items: [{ name: 'Crackers', nutrients: { kcal: 150, protein: 3, carbs: 20, fat: 6 } }],
    },
  })
  expect(logged.ok()).toBe(true)
  const snack = page.getByRole('article').filter({ hasText: 'Late snack' })

  await page.goto('/')
  await openMeal(snack)
  await snack.getByRole('button', { name: 'Change the meal: Late snack' }).click()
  const editor = page.getByRole('dialog', { name: 'Change the meal' })
  await editor.getByLabel('Day').selectOption({ label: 'Yesterday' })
  await editor.getByRole('button', { name: 'Save the meal' }).click()
  await expect(snack).toHaveCount(0)
  await expect(page.getByText(/^Moved to /)).toBeVisible()

  // Taken back only while it is the latest change: it is.
  await page.getByRole('button', { name: 'Undo' }).click()
  await expect(page.getByText('Taken back: a meal changed.')).toBeVisible()
  await expect(snack).toHaveCount(1)
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
  await openMeal(toast)
  await toast.getByRole('button', { name: 'Repeat today: Toast' }).click()
  await expect(page.getByText('Logged again, today at this time.')).toBeVisible()
  await expect(toast).toHaveCount(2)
})

test("a meal's item is given in another unit, and an estimate saved as a food", async ({
  page,
}) => {
  await signIn(page)
  const food = await page.request.post('/api/v1/foods', {
    data: {
      name: 'Muesli',
      basis: '100g',
      source: 'label',
      nutrients: { kcal: 380, protein: 10, carbs: 64, fat: 8 },
      servings: [{ name: 'cup', amount: 90 }],
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
      description: 'Office lunch',
      items: [
        { food_id: foodId, amount: 45, unit: 'g' },
        { name: 'Canteen soup', nutrients: { kcal: 180, protein: 6, carbs: 20, fat: 8 } },
      ],
    },
  })
  expect(logged.ok()).toBe(true)

  await page.goto('/')
  const lunch = page.getByRole('article').filter({ hasText: 'Office lunch' })
  await openMeal(lunch)
  await lunch.getByRole('button', { name: 'Change the meal: Office lunch' }).click()
  const editor = page.getByRole('dialog', { name: 'Change the meal' })
  // 45 g are half a cup of 90 g.
  await editor.getByLabel('Unit: Muesli').selectOption({ label: 'cup' })
  await expect(editor.getByLabel('Amount: Muesli')).toHaveValue('0.5')
  await editor.getByLabel('Amount: Muesli').fill('1')
  await editor.getByRole('button', { name: 'Save as a food of yours: Canteen soup' }).click()
  await editor.getByRole('button', { name: 'Save the meal' }).click()

  await expect(lunch).toContainText('90 g')
  await expect(lunch).toContainText('Canteen soup')
  const found = await page.request.get('/api/v1/foods?query=Canteen%20soup')
  expect(JSON.stringify(await found.json())).toContain('"basis":"portion"')
})
