import { expect, type Locator, type Page } from '@playwright/test'

// The user scripts/e2e-server.sh creates for the tests (made up).
export const user = { email: 'ana@example.com', password: 'only-for-the-e2e-tests' }

// signIn signs the user in from the sign-in page, and waits for the day.
export async function signIn(page: Page) {
  await page.goto('/login')
  await page.getByLabel('Email').fill(user.email)
  await page.getByLabel('Password').fill(user.password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Today', exact: true })).toBeVisible()
}

// toSettings goes to the settings as a phone does: they are a list under
// the plan, each a row to its page.
export async function toSettings(page: Page) {
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Plan' })
    .click()
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible()
}

// openMeal opens a meal's row, unless it is open: its items show, and its
// ways to change it.
export async function openMeal(meal: Locator) {
  const row = meal.getByRole('button', { expanded: false })
  if ((await row.count()) > 0) {
    await row.click()
  }
  await expect(meal.getByRole('button', { expanded: true })).toBeVisible()
}
