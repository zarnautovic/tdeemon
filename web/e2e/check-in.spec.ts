import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

// A day long past, which the other tests leave alone.
const date = '2025-06-02'

test("a day's check-in and measurements are given, corrected and kept", async ({ page }) => {
  await signIn(page)
  await page.goto(`/?date=${date}`)
  await expect(page.getByRole('heading', { name: 'Monday, June 2' })).toBeVisible()
  const checkIn = page.getByRole('region', { name: 'Check-in' })

  await checkIn.getByRole('group', { name: 'Mood' }).getByRole('button', { name: '4 good' }).click()
  await checkIn
    .getByRole('group', { name: 'Hunger' })
    .getByRole('button', { name: '2 calm' })
    .click()
  await checkIn.getByRole('button', { name: '+ Add a note' }).click()
  await checkIn.getByLabel('Note').fill('long walk')
  await checkIn.getByLabel('Note').blur()
  // The day's type is the header's chip, a menu.
  await page.getByRole('button', { name: 'Type of day: Rest day' }).click()
  await page.getByRole('menuitemradio', { name: 'Training' }).click()
  await expect(page.getByRole('button', { name: 'Type of day: Training day' })).toBeVisible()
  await checkIn.getByRole('switch', { name: 'Counted in calibration' }).click()
  await checkIn.getByRole('button', { name: 'sick' }).click()
  await checkIn.getByRole('button', { name: 'Leave out of calibration' }).click()
  await expect(checkIn.getByText('Out of calibration: sick')).toBeVisible()

  // The body's tiles, and all its measurements in a sheet.
  await page.getByRole('button', { name: 'All measurements' }).click()
  const measurements = page.getByRole('dialog', { name: 'Measurements' })
  await measurements.getByLabel('Weight').fill('72.5')
  await measurements.getByRole('button', { name: 'Save' }).click()
  await expect(measurements).toContainText('72.5 kg')
  await measurements.getByRole('button', { name: 'Change: Weight' }).click()
  await measurements.getByLabel('Weight').fill('72.8')
  await measurements.getByRole('button', { name: 'Save' }).click()
  await expect(measurements).toContainText('72.8 kg')
  await measurements.getByRole('button', { name: 'Add a measurement' }).click()
  await measurements.getByLabel('Measurement').selectOption({ label: 'Sleep' })
  await measurements.getByLabel('Value').fill('7.5')
  await measurements.getByRole('button', { name: 'Save' }).click()
  await expect(measurements).toContainText('7.5 h')
  await measurements.getByRole('button', { name: 'Change: Sleep' }).click()
  await measurements.getByRole('button', { name: 'Delete' }).click()
  // Gone once the server answered: its form, which names it, goes too.
  // The value alone is gone as soon as the form opens, and a reload
  // before the answer would cancel the delete.
  await expect(measurements).not.toContainText('Sleep')

  // What the server kept comes back with the page.
  await page.reload()
  await expect(checkIn.getByRole('button', { name: '4 good' })).toHaveAttribute(
    'aria-pressed',
    'true',
  )
  await expect(checkIn.getByRole('button', { name: '2 calm' })).toHaveAttribute(
    'aria-pressed',
    'true',
  )
  await expect(checkIn.getByLabel('Note')).toHaveValue('long walk')
  await expect(page.getByRole('button', { name: 'Type of day: Training day' })).toBeVisible()
  await expect(checkIn.getByText('Out of calibration: sick')).toBeVisible()
  await expect(page.getByRole('region', { name: 'Body', exact: true })).toContainText('72.8 kg')
  await page.getByRole('button', { name: 'All measurements' }).click()
  await expect(measurements).toContainText('72.8 kg')
  await expect(measurements).not.toContainText('Sleep')
})
