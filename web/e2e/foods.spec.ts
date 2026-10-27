import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test('a food is made, found, corrected, archived and brought back', async ({ page }) => {
  await signIn(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Foods' })
    .click()

  await page.getByRole('link', { name: 'Add a food' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Buckwheat')
  await page.getByLabel('Energy (kcal)').fill('343')
  await page.getByLabel('Protein (g)').fill('13.3')
  await page.getByLabel('Carbs (g)').fill('71.5')
  await page.getByLabel('Fat (g)', { exact: true }).fill('3.4')
  await page.getByRole('button', { name: 'Add another name' }).click()
  await page.getByLabel('Other name').fill('kasha')
  await page.getByRole('button', { name: 'Save the food' }).click()
  await expect(page.getByRole('heading', { name: 'Buckwheat' })).toBeVisible()

  await page.getByRole('link', { name: 'Back to the foods' }).click()
  await page.getByLabel('Search the foods').fill('kash')
  await page
    .getByRole('list', { name: 'Found' })
    .getByRole('link', { name: /Buckwheat/ })
    .click()
  await page.getByLabel('Energy (kcal)').fill('346')
  await page.getByRole('button', { name: 'Save the food' }).click()
  await expect(page.getByText('Saved: Buckwheat')).toBeVisible()

  await page.getByRole('button', { name: 'Archive' }).click()
  await expect(
    page.getByText('Archived: only a search of the archived foods finds it'),
  ).toBeVisible()
  await page.getByRole('link', { name: 'Back to the foods' }).click()
  await expect(page.getByRole('list', { name: 'Found' })).toHaveCount(0)
  await page.getByLabel('Search the foods').fill('')
  await page.getByRole('button', { name: 'Archived' }).click()
  const archived = page.getByRole('list', { name: 'Archived' })
  await expect(archived).toContainText('346 kcal')
  // A search on the archived tab is of the archived foods.
  await page.getByLabel('Search the foods').fill('buck')
  await expect(page.getByRole('list', { name: 'Found' })).toContainText('Buckwheat')
  await page.getByLabel('Search the foods').fill('')
  await archived.getByRole('link', { name: /Buckwheat/ }).click()
  await page.getByRole('button', { name: 'Bring back' }).click()
  await expect(page.getByRole('button', { name: 'Archive' })).toBeVisible()
})
