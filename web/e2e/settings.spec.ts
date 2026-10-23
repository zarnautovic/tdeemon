import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test("a user's name and notes are set, and a token made and all revoked", async ({ page }) => {
  await signIn(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Settings' })
    .click()

  const you = page.getByRole('form', { name: 'You' })
  await you.getByLabel('Name', { exact: true }).fill('Ana M.')
  await you.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Signed in as Ana M.')).toBeVisible()

  await page.getByLabel('Notes for a coach').fill('Knees: no running.')
  await page
    .getByLabel('Notes for a coach')
    .locator('xpath=ancestor::form')
    .getByRole('button', { name: 'Save' })
    .click()
  await expect(
    page.getByLabel('Notes for a coach').locator('xpath=ancestor::form').getByText('Saved'),
  ).toBeVisible()
  await page.reload()
  await expect(page.getByLabel('Notes for a coach')).toHaveValue('Knees: no running.')

  const tokens = page.getByRole('region', { name: 'API tokens' })
  await tokens.getByLabel('Name of the token').fill('Test phone')
  await tokens.getByRole('button', { name: 'Create a token' }).click()
  const token = (await tokens.locator('code').textContent()) ?? ''
  expect(token).toMatch(/^tdm_/)
  // The token works, until all are revoked.
  const who = await page.request.get('/api/v1/auth/whoami', {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(who.ok()).toBe(true)
  await tokens.getByRole('button', { name: 'Revoke all tokens' }).click()
  await tokens.getByRole('button', { name: 'Revoke', exact: true }).click()
  await expect(tokens).toContainText('revoked.')
  const after = await page.request.get('/api/v1/auth/whoami', {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(after.status()).toBe(401)

  // The name back, for the other tests.
  await you.getByLabel('Name', { exact: true }).fill('Ana')
  await you.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Signed in as Ana (')).toBeVisible()
})
