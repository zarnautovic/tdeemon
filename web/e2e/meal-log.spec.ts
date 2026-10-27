import { expect, test } from '@playwright/test'

import { openMeal, signIn } from './user.ts'

// A day long past, which the other tests leave alone.
const date = '2025-06-03'

test('a meal is logged by hand, of a food and an estimate', async ({ page }) => {
  await signIn(page)
  const food = await page.request.post('/api/v1/foods/new', {
    data: {
      name: 'Basmati rice',
      basis: '100g',
      source: 'label',
      nutrients: { kcal: 350, protein: 8, carbs: 78, fat: 1 },
      servings: [{ name: 'cup', amount: 60 }],
    },
  })
  expect(food.ok()).toBe(true)

  await page.goto(`/?date=${date}`)
  await page.getByRole('button', { name: 'Log a meal' }).click()
  const sheet = page.getByRole('dialog', { name: 'A new meal' })
  await sheet.getByRole('button', { name: 'Change the name and time' }).click()
  await sheet.getByLabel('What it was').fill('Dinner')
  await sheet.getByLabel('Time', { exact: true }).fill('19:30')
  await sheet.getByLabel('Find a food or a meal').fill('basm')
  await expect(sheet.getByRole('button', { name: /^Basmati rice/ })).toBeVisible()
  // Enter opens the food found for its amount; it does not send the meal.
  await sheet.getByLabel('Find a food or a meal').press('Enter')
  await sheet.getByLabel('Unit: Basmati rice').selectOption({ label: 'cup' })
  // 100 g, worked out in cups of 60 g.
  await expect(sheet.getByLabel('Amount: Basmati rice')).toHaveValue('1.67')
  await sheet.getByLabel('Amount: Basmati rice').fill('2')
  await sheet.getByLabel('Amount: Basmati rice').press('Enter')
  const meal = sheet.getByRole('list', { name: 'In the meal' })
  await expect(meal).toContainText('Basmati rice · 2 cup · 420 kcal')
  await sheet.getByRole('button', { name: 'An estimate', exact: true }).click()
  await sheet.getByLabel('What it is').fill('Curry')
  await sheet.getByLabel('Energy (kcal)').fill('400')
  await sheet.getByLabel('Protein (g)').fill('20')
  await sheet.getByLabel('Carbs (g)').fill('15')
  await sheet.getByLabel('Fat (g)', { exact: true }).fill('28')
  await sheet.getByRole('button', { name: 'Add: Curry' }).click()
  await expect(meal).toContainText('Curry · 400 kcal')
  await sheet.getByRole('button', { name: 'Log 2 items · 820 kcal' }).click()

  await expect(page.getByText(/^Logged: Dinner · 820 kcal/)).toBeVisible()
  await expect(sheet).toBeHidden()
  const dinner = page.getByRole('article').filter({ hasText: 'Dinner' })
  // Two cups of rice, 120 g: 420 kcal, and the curry's 400.
  await expect(dinner).toContainText('820 kcal')
  await expect(dinner).toContainText('7:30 PM')
  await openMeal(dinner)
  await expect(dinner).toContainText('Curry')
})
