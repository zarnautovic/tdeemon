import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test('the profile is set, and a limit tightened and loosened again', async ({ page }) => {
  await signIn(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Plan' })
    .click()

  const profile = page.getByRole('form', { name: 'Profile' })
  await profile.getByRole('radio', { name: 'Female' }).check()
  await profile.getByLabel('Birth date').fill('1990-06-15')
  await profile.getByRole('textbox', { name: /Height/ }).fill('168')
  await profile.getByRole('button', { name: 'Save' }).click()
  await expect(profile.getByText('A plan has all it needs of you.')).toBeVisible()

  const limits = page.getByRole('form', { name: 'Limits of the plan' })
  const cut = limits.getByRole('textbox', { name: /^Cut, fastest loss, BMI below 30/ })
  await cut.fill('0.5')
  await limits.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(cut).toHaveValue('0.5')

  // Back to the default, a loosening, which the user confirms.
  await cut.fill('0.7')
  await limits.getByRole('button', { name: 'Save' }).click()
  await limits.getByRole('button', { name: 'Loosen' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(cut).toHaveValue('0.7')

  // The profile cleared again, for the other tests.
  await profile.getByRole('radio', { name: 'Not given' }).check()
  await profile.getByLabel('Birth date').fill('')
  await profile.getByRole('textbox', { name: /Height/ }).fill('')
  await profile.getByRole('button', { name: 'Save' }).click()
  await expect(profile.getByText('A plan still needs: sex, birth date, height.')).toBeVisible()
})
