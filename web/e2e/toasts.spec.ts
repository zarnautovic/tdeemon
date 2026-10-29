// The checks run code in the page, which has the DOM's types.
/// <reference lib="dom" />

import { expect, test, type Locator, type Page } from '@playwright/test'

import { signIn } from './user.ts'

// A day long past, which the other tests leave alone.
const date = '2025-06-04'

const sizes = [
  { name: 'phone', viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true },
  { name: 'desktop', viewport: { width: 1440, height: 900 }, isMobile: false, hasTouch: false },
] as const

// onTop tells whether a pointer at the middle of target reaches it, with
// nothing over it.
function onTop(target: Locator): Promise<boolean> {
  return target.evaluate((el) => {
    const box = el.getBoundingClientRect()
    const hit = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2)
    return hit !== null && el.contains(hit)
  })
}

// overlap tells whether two boxes on the screen have any of it in common.
function overlap(a: Box, b: Box): boolean {
  return a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height
}

interface Box {
  x: number
  y: number
  width: number
  height: number
}

// deleteInSheet gives the day a resting heart rate, through the API with
// the page's session, and deletes it in the sheet of the day's
// measurements, which stays open: no question before, as its toast's
// Undo takes it back. It returns the sheet, the toasts and their Undo.
async function deleteInSheet(page: Page) {
  await signIn(page)
  const given = await page.request.put(`/api/v1/days/${date}/metrics/rhr`, { data: { value: 52 } })
  expect(given.ok()).toBe(true)
  await page.goto(`/?date=${date}`)
  await page.getByRole('button', { name: 'All measurements' }).click()
  const sheet = page.getByRole('dialog', { name: 'Measurements' })
  await sheet.getByRole('button', { name: 'Change: Resting heart rate' }).click()
  await sheet.getByRole('button', { name: 'Delete' }).click()
  await expect(sheet).not.toContainText('Resting heart rate')
  const toasts = page.getByRole('region', { name: 'Notifications' })
  await expect(toasts).toContainText('Deleted: Resting heart rate')
  return { sheet, toasts, undo: toasts.getByRole('button', { name: 'Undo' }) }
}

for (const size of sizes) {
  test.describe(size.name, () => {
    test.use({ viewport: size.viewport, isMobile: size.isMobile, hasTouch: size.hasTouch })

    test('a change made in a sheet is taken back from its toast, over the sheet', async ({
      page,
    }) => {
      const { sheet, toasts, undo } = await deleteInSheet(page)

      // Over the page and the sheet, above it on a phone and beside the
      // panel at the right on a desktop: what a finger or the pointer
      // reaches is the Undo.
      expect(await onTop(undo)).toBe(true)
      await (size.hasTouch ? undo.tap() : undo.click())

      await expect(toasts).toContainText('Taken back: a measurement deleted.')
      await expect(sheet).toContainText('Resting heart rate')
    })

    test('a toast keeps clear of a sheet, whose buttons a tap under it would miss', async ({
      page,
    }) => {
      const { sheet, toasts } = await deleteInSheet(page)

      // Above the sheet on a phone, beside it on a desktop.
      const toast = await toasts.boundingBox()
      const panel = await sheet.boundingBox()
      if (toast === null || panel === null) {
        throw new Error('no toast, or no sheet')
      }
      expect(overlap(toast, panel)).toBe(false)
      expect(await onTop(sheet.getByRole('button', { name: 'Add a measurement' }))).toBe(true)
    })
  })
}

test.describe('keyboard', () => {
  test.use({ viewport: { width: 1440, height: 900 }, isMobile: false, hasTouch: false })

  test('F6 takes the focus from a sheet to the toasts, whose Undo takes the change back', async ({
    page,
  }) => {
    const { sheet, toasts, undo } = await deleteInSheet(page)

    // The card deleted took the focus with it: the measurements take it,
    // in the sheet, and F6 goes from there.
    await expect.poll(() => sheet.evaluate((el) => el.contains(document.activeElement))).toBe(true)
    // Tab stays within the sheet; F6, Base UI's key, goes to the toasts.
    await page.keyboard.press('F6')
    await expect(toasts).toBeFocused()
    await page.keyboard.press('Tab')
    await page.keyboard.press('Tab')
    await expect(undo).toBeFocused()
    await page.keyboard.press('Enter')

    await expect(toasts).toContainText('Taken back: a measurement deleted.')
    await expect(sheet).toContainText('Resting heart rate')
  })
})
