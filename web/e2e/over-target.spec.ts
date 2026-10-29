// The checks run code in the page, which has the DOM's types.
/// <reference lib="dom" />

import { AxeBuilder } from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'

import { signIn } from './user.ts'

// A day well over its target, as the day's summary draws it: its ring's
// number fits within the ring, its colours pass axe in both themes, and
// the ⓘ beside the TDEE is a finger's target that the keyboard opens and
// closes. The e2e user has no plan, so today's answers are the server's
// with a target of 2,250, 3,460 eaten (+1,210, the widest number a day
// shows) and a measured TDEE laid over them.

const sizes = [
  { name: 'phone', viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true },
  { name: 'desktop', viewport: { width: 1440, height: 900 }, isMobile: false, hasTouch: false },
] as const
const themes = ['light', 'dark'] as const

// isObject tells a JSON object, as the server's answers are.
function isObject(body: unknown): body is Record<string, unknown> {
  return typeof body === 'object' && body !== null
}

async function overTarget(page: Page) {
  await page.route('**/api/v1/days/today', async (route) => {
    const response = await route.fetch()
    const day: unknown = await response.json()
    await route.fulfill({
      response,
      json: {
        ...(isObject(day) ? day : {}),
        target: { kcal: 2250, protein: 160, carbs: 250, fat: 70 },
        total: { kcal: 3460, protein: 186, carbs: 278, fat: 98 },
        remaining: { kcal: -1210, protein: -26, carbs: -28, fat: -28 },
      },
    })
  })
  await page.route('**/api/v1/summary', async (route) => {
    const response = await route.fetch()
    const summary: unknown = await response.json()
    const tdee = isObject(summary) && isObject(summary.tdee) ? summary.tdee : {}
    await route.fulfill({
      response,
      json: {
        ...(isObject(summary) ? summary : {}),
        tdee: { ...tdee, measured: true, trusted: true, kcal: 2409, half_width: 37 },
      },
    })
  })
}

for (const size of sizes) {
  for (const theme of themes) {
    test.describe(`${size.name}, ${theme}`, () => {
      test.use({
        viewport: size.viewport,
        isMobile: size.isMobile,
        hasTouch: size.hasTouch,
        colorScheme: theme,
      })

      test('a day well over its target fits its ring and passes axe', async ({ page }) => {
        await signIn(page)
        await overTarget(page)
        await page.goto('/')
        const summary = page.getByRole('region', { name: 'Energy and macros' })
        const number = summary.getByText('+1,210', { exact: true })
        await expect(number).toBeVisible()
        await expect(summary.getByText('over target', { exact: true })).toBeVisible()
        await expect(summary.getByText('Over TDEE')).toBeVisible()

        // The number within the ring's inner edge, at its height: the
        // circle of radius 56, less half its stroke of 12, in a box of
        // 132.
        const ring = await summary.locator('svg').first().boundingBox()
        const box = await number.boundingBox()
        expect(ring).not.toBeNull()
        expect(box).not.toBeNull()
        if (ring !== null && box !== null) {
          const inner = (50 / 132) * ring.width
          const [cx, cy] = [ring.x + ring.width / 2, ring.y + ring.height / 2]
          const chord = Math.sqrt(inner ** 2 - (box.y + box.height / 2 - cy) ** 2)
          expect(box.x).toBeGreaterThanOrEqual(cx - chord)
          expect(box.x + box.width).toBeLessThanOrEqual(cx + chord)
        }

        // Nothing serious against WCAG 2.2 AA, its colours with it, the
        // ⓘ's popover open too.
        const axe = async () =>
          (
            await new AxeBuilder({ page })
              .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
              .analyze()
          ).violations
            .filter((v) => v.impact === 'serious' || v.impact === 'critical')
            .flatMap((v) => v.nodes.map((n) => `${v.id}: ${n.target.join(' ')}`))
        expect(await axe()).toEqual([])
        await summary.getByRole('button', { name: 'What TDEE is' }).click()
        await expect(page.getByRole('dialog', { name: 'TDEE' })).toBeVisible()
        expect(await axe()).toEqual([])
      })

      if (size.name === 'phone') {
        // A finger's target: 44 px or more either way, the ::after over
        // the ⓘ with it; and its popover within the screen.
        test("the ⓘ is a finger's target, and its popover stays on the screen", async ({
          page,
        }) => {
          await signIn(page)
          await overTarget(page)
          await page.goto('/')
          const tip = page
            .getByRole('region', { name: 'Energy and macros' })
            .getByRole('button', { name: 'What left to TDEE is' })
          const reach = await tip.evaluate((el) => {
            const box = el.getBoundingClientRect()
            const after = getComputedStyle(el, '::after')
            const [top, bottom, left, right] = [
              after.top,
              after.bottom,
              after.left,
              after.right,
            ].map((v) => Math.max(0, -Number.parseFloat(v) || 0))
            return { width: box.width + left + right, height: box.height + top + bottom }
          })
          expect(reach.width).toBeGreaterThanOrEqual(43.5)
          expect(reach.height).toBeGreaterThanOrEqual(43.5)

          await tip.tap()
          const dialog = page.getByRole('dialog', { name: 'Left to TDEE' })
          await expect(dialog).toBeVisible()
          const box = await dialog.boundingBox()
          expect(box).not.toBeNull()
          expect(box?.x ?? -1).toBeGreaterThanOrEqual(0)
          expect((box?.x ?? 0) + (box?.width ?? Infinity)).toBeLessThanOrEqual(390)
        })
      } else {
        // The keyboard opens it, and Escape closes it, the focus back on
        // the ⓘ, which shows it has it.
        test('the keyboard opens the ⓘ and closes it again', async ({ page }) => {
          await signIn(page)
          await overTarget(page)
          await page.goto('/')
          const tip = page
            .getByRole('region', { name: 'Energy and macros' })
            .getByRole('button', { name: 'What TDEE is' })
          await expect(tip).toBeVisible()
          await tip.focus()
          await page.keyboard.press('Enter')
          const dialog = page.getByRole('dialog', { name: 'TDEE' })
          await expect(dialog).toBeVisible()
          await expect(dialog).toContainText('80%')
          await page.keyboard.press('Escape')
          await expect(dialog).toBeHidden()
          await expect(tip).toBeFocused()
          await expect(tip).not.toHaveCSS('box-shadow', 'none')
        })
      }
    })
  }
}
