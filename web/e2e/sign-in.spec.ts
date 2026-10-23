import { expect, test } from '@playwright/test'

import { user } from './user.ts'

test('a user signs in, moves between the sections and signs out', async ({ page }) => {
  await page.goto('/settings')
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()

  await page.getByLabel('Email').fill(user.email)
  await page.getByLabel('Password').fill(user.password)
  await page.getByRole('button', { name: 'Sign in' }).click()

  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible()
  await expect(page.getByText(`Signed in as Ana (${user.email}).`)).toBeVisible()
  const sections = page.getByRole('navigation', { name: 'Sections' })
  await sections.getByRole('link', { name: 'Foods' }).click()
  await expect(page.getByRole('heading', { name: 'Foods' })).toBeVisible()
  await expect(page).toHaveURL('/foods')
  await sections.getByRole('link', { name: 'Settings' }).click()

  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()
})

test('a wrong password is refused', async ({ page }) => {
  await page.goto('/login')

  await page.getByLabel('Email').fill(user.email)
  await page.getByLabel('Password').fill('not the password')
  await page.getByRole('button', { name: 'Sign in' }).click()

  await expect(page.getByRole('alert')).toHaveText('Wrong email or password.')
  await expect(page).toHaveURL('/login')
})
