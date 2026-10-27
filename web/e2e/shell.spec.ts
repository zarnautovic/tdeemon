import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test('a phone has the bar at the bottom, a desktop the sidebar, and the keys work there', async ({
  page,
}) => {
  await signIn(page)
  const sections = page.getByRole('navigation', { name: 'Sections' })

  // A phone: the bar's sections, Log a meal in its middle, the settings
  // the plan's.
  const viewport = page.viewportSize()
  const bar = await sections.boundingBox()
  expect(bar && viewport && Math.round(bar.y + bar.height)).toBe(viewport?.height)
  await expect(sections.getByRole('link', { name: 'Progress' })).toBeVisible()
  await expect(sections.getByRole('link', { name: 'Settings' })).toBeHidden()
  await sections.getByRole('link', { name: 'Foods' }).click()
  await sections.getByRole('link', { name: 'Log a meal' }).click()
  await expect(page.getByRole('form', { name: 'A new meal' })).toBeVisible()
  await expect(page).toHaveURL('/')

  // A desktop: the sidebar, 240 px at the left, the settings at its foot.
  await page.setViewportSize({ width: 1440, height: 900 })
  const side = await sections.boundingBox()
  expect(side && [side.x, side.width, side.height]).toEqual([0, 240, 900])
  await expect(sections.getByRole('link', { name: 'Settings' })).toBeVisible()
  await expect(sections.getByRole('button', { name: 'Dark theme' })).toBeVisible()

  await page.getByRole('heading', { name: 'Today', exact: true }).click()
  await page.keyboard.press('/')
  await expect(page.getByRole('searchbox', { name: 'Search the foods' })).toBeFocused()
  await page.keyboard.press('Escape')
  await page.getByRole('heading', { name: 'Foods' }).click()
  await page.keyboard.press('?')
  const dialog = page.getByRole('dialog', { name: 'Keyboard shortcuts' })
  await expect(dialog).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
})

test('the addresses of before lead where their pages are now', async ({ page }) => {
  await signIn(page)
  await page.goto('/charts?days=30')
  await expect(page).toHaveURL('/progress?days=30')
  await expect(page.getByRole('heading', { name: 'Progress' })).toBeVisible()
  await page.goto('/settings/plan')
  await expect(page).toHaveURL('/plan')
  await expect(page.getByRole('heading', { name: 'Plan', level: 1 })).toBeVisible()
})
