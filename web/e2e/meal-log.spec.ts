import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

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
  const form = page.getByRole('form', { name: 'A new meal' })
  await form.getByLabel('What it was').fill('Dinner')
  await form.getByLabel('Time').fill('19:30')
  await form.getByLabel('Find a food of yours').fill('basm')
  await expect(form.getByRole('button', { name: /Basmati rice/ })).toBeVisible()
  // Enter picks the food found; it does not send the meal yet.
  await form.getByLabel('Find a food of yours').press('Enter')
  await expect(form).toBeVisible()
  await form.getByLabel('Unit: Basmati rice').selectOption({ label: 'cup' })
  // 100 g, worked out in cups of 60 g.
  await expect(form.getByLabel('Amount: Basmati rice')).toHaveValue('1.67')
  await form.getByLabel('Amount: Basmati rice').fill('2')
  await form.getByRole('button', { name: 'Add an estimate' }).click()
  await form.getByLabel('What it is').fill('Curry')
  await form.getByLabel('Energy (kcal)').fill('400')
  await form.getByLabel('Protein (g)').fill('20')
  await form.getByLabel('Carbs (g)').fill('15')
  await form.getByLabel('Fat (g)', { exact: true }).fill('28')
  await form.getByRole('button', { name: 'Log the meal' }).click()

  await expect(page.getByText(/^Logged: /)).toBeVisible()
  const dinner = page.getByRole('article').filter({ hasText: 'Dinner' })
  // Two cups of rice, 120 g: 420 kcal, and the curry's 400.
  await expect(dinner).toContainText('820 kcal')
  await expect(dinner).toContainText('7:30 PM')
  await expect(dinner).toContainText('Curry')
})
