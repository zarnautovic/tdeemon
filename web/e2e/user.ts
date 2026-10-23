import { expect, type Page } from '@playwright/test'

// The user scripts/e2e-server.sh creates for the tests (made up).
export const user = { email: 'ana@example.com', password: 'only-for-the-e2e-tests' }

// signIn signs the user in from the sign-in page, and waits for the day.
export async function signIn(page: Page) {
  await page.goto('/login')
  await page.getByLabel('Email').fill(user.email)
  await page.getByLabel('Password').fill(user.password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Today' })).toBeVisible()
}
