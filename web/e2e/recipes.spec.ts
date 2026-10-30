import { expect, test, type Page } from '@playwright/test'

import { signIn } from './user.ts'

// foodOf makes a food of the user's, made up, and gives its id.
async function foodOf(page: Page, name: string, kcal: number): Promise<string> {
  const food = await page.request.post('/api/v1/foods', {
    data: {
      name,
      basis: '100g',
      source: 'label',
      nutrients: { kcal, protein: 10, carbs: 20, fat: 5 },
    },
  })
  expect(food.ok()).toBe(true)
  const saved: unknown = await food.json()
  return typeof saved === 'object' &&
    saved !== null &&
    'id' in saved &&
    typeof saved.id === 'string'
    ? saved.id
    : ''
}

test('a recipe is written, read, cooked again and edited', async ({ page }) => {
  await signIn(page)
  const tag = String(Date.now())
  await foodOf(page, `Split peas ${tag}`, 340)
  await foodOf(page, `Smoked tofu ${tag}`, 180)
  await page.goto('/foods?filter=recipes')

  await page.getByRole('link', { name: 'New recipe' }).first().click()
  await expect(page.getByRole('heading', { name: 'New recipe' })).toBeVisible()
  await page.getByLabel('Name', { exact: true }).fill(`Pea stew ${tag}`)
  await page.getByRole('button', { name: 'Add ingredients' }).click()
  const sheet = page.getByRole('dialog', { name: 'Add ingredients' })
  await sheet.getByRole('searchbox').fill(`split peas ${tag}`)
  await sheet.getByRole('button', { name: new RegExp(`^Split peas ${tag}`) }).click()
  await sheet.getByLabel(`Amount: Split peas ${tag}`).fill('400')
  await sheet.getByRole('button', { name: `Add: Split peas ${tag}` }).click()
  await sheet.getByRole('searchbox').fill(`smoked tofu ${tag}`)
  await sheet.getByRole('button', { name: `Add: Smoked tofu ${tag}` }).click()
  await sheet.getByRole('button', { name: 'Done · 2 added' }).click()
  await expect(sheet).toHaveCount(0)

  // Not weighed: the raw ingredients' 500 g stand for the dish.
  await expect(page.getByText('Whole recipe 1,540 kcal · ≈ 500 g')).toBeVisible()
  await page.getByLabel('Portions').fill('4')
  await page.getByRole('button', { name: 'Save the recipe' }).click()
  await expect(page.getByText(`Saved: Pea stew ${tag}`)).toBeVisible()
  const recipe = page.getByRole('article', { name: `Pea stew ${tag}` })
  await expect(recipe.getByText('Not weighed', { exact: true })).toBeVisible()
  await expect(recipe.getByText('385', { exact: true })).toBeVisible()
  await expect(recipe.getByText('kcal per portion')).toBeVisible()

  // Cooked again, weighed from the pot.
  await recipe.getByRole('button', { name: 'Cooked again' }).click()
  const pot = page.getByRole('dialog', { name: 'Cooked again' })
  await pot.getByLabel(`Amount: Split peas ${tag}`).fill('500')
  await pot.getByRole('button', { name: 'From the pot' }).click()
  await pot.getByLabel('Full pot').fill('2600')
  await pot.getByLabel('Empty pot').fill('1100')
  await expect(pot.getByLabel('Weight of the cooked dish')).toHaveValue('1500')
  await pot.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText(`Saved: Pea stew ${tag} · 470 kcal per portion`)).toBeVisible()
  await expect(pot).toHaveCount(0)
  await expect(recipe.getByText('Weighed: 1,500 g')).toBeVisible()

  // Edited: the empty pot is remembered, and an ingredient removed.
  await recipe.getByRole('link', { name: 'Edit' }).click()
  await expect(page.getByRole('heading', { name: 'Edit the recipe' })).toBeVisible()
  await expect(page.getByLabel('Empty pot · remembered')).toHaveValue('1100')
  await page.getByRole('button', { name: `More: Smoked tofu ${tag}` }).click()
  await page.getByRole('menuitem', { name: 'Remove' }).click()
  await page.getByRole('button', { name: 'Save the recipe' }).click()
  await expect(recipe.getByText('1 ingredient · 4 portions')).toBeVisible()

  // Among the recipes, its kcal for a portion.
  await page.goto('/foods?filter=recipes')
  await expect(
    page.getByRole('main').getByRole('link', { name: new RegExp(`Pea stew ${tag}`) }),
  ).toContainText('/portion')
})
