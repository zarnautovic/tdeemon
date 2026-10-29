// A check runs code in the page, which has the DOM's types.
/// <reference lib="dom" />

import { expect, test } from '@playwright/test'

import { openMeal, signIn } from './user.ts'

test('a meal is corrected, deleted, and brought back by undo', async ({ page }) => {
  await signIn(page)
  // A food and a meal of it, logged as a model would, through the API.
  const food = await page.request.post('/api/v1/foods', {
    data: {
      name: 'Rolled oats',
      basis: '100g',
      source: 'label',
      nutrients: { kcal: 370, protein: 13, carbs: 60, fat: 7 },
    },
  })
  expect(food.ok()).toBe(true)
  const saved: unknown = await food.json()
  const foodId =
    typeof saved === 'object' && saved !== null && 'id' in saved && typeof saved.id === 'string'
      ? saved.id
      : ''
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Porridge',
      items: [
        { food_id: foodId, amount: 80 },
        { name: 'Blueberries', nutrients: { kcal: 40, protein: 0.5, carbs: 9, fat: 0.2 } },
      ],
    },
  })
  expect(logged.ok()).toBe(true)

  await page.goto('/')
  const porridge = page.getByRole('article').filter({ hasText: 'Porridge' })
  await openMeal(porridge)
  await expect(porridge).toContainText('80 g')

  await porridge.getByRole('button', { name: 'Change the meal: Porridge' }).click()
  // The editor is a sheet over the day, its total said as it changes.
  const editor = page.getByRole('dialog', { name: 'Change the meal' })
  await editor.getByLabel('Amount: Rolled oats').fill('100')
  await expect(editor).toContainText('370 kcal · P 13 · was 296')
  // An item is removed from its menu.
  await editor.getByRole('button', { name: 'More: Blueberries' }).click()
  await page.getByRole('menuitem', { name: 'Remove: Blueberries' }).click()
  await expect(editor).toContainText('+34 kcal from the changes')
  await editor.getByRole('button', { name: 'Save the meal' }).click()
  await expect(editor).toBeHidden()
  await expect(porridge).toContainText('100 g')
  await expect(porridge).toContainText('370 kcal')
  await expect(porridge).not.toContainText('Blueberries')

  // The change taken back by its toast's Undo: the item removed is back.
  await page.getByRole('button', { name: 'Undo' }).click()
  await expect(page.getByText('Taken back: a meal changed.')).toBeVisible()
  await expect(porridge).toContainText('Blueberries')
  await expect(porridge).toContainText('80 g')

  // Deleted at once, and brought back by its toast's Undo. The focus stays
  // on the day: on its meals, which the meal went from.
  await porridge.getByRole('button', { name: 'Change the meal: Porridge' }).click()
  await editor.getByRole('button', { name: 'Delete the meal' }).click()
  await expect(porridge).toHaveCount(0)
  await expect(page.getByText('Deleted: Porridge')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Meals', exact: true })).toBeFocused()

  await page.getByRole('button', { name: 'Undo' }).click()
  await expect(page.getByText('Taken back: a meal deleted.')).toBeVisible()
  await openMeal(porridge)
  await expect(porridge).toContainText('Blueberries')
})

test('a meal moved to the day before says so, and its toast brings it back', async ({ page }) => {
  await signIn(page)
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Late snack',
      items: [{ name: 'Crackers', nutrients: { kcal: 150, protein: 3, carbs: 20, fat: 6 } }],
    },
  })
  expect(logged.ok()).toBe(true)
  const snack = page.getByRole('article').filter({ hasText: 'Late snack' })

  await page.goto('/')
  await openMeal(snack)
  await snack.getByRole('button', { name: 'Change the meal: Late snack' }).click()
  const editor = page.getByRole('dialog', { name: 'Change the meal' })
  // The day is a button with its name, which lists the days by theirs.
  const day = editor.getByRole('combobox', { name: 'Day' })
  await expect(day).toHaveText('Today')
  await day.click()
  await page.getByRole('option', { name: 'Yesterday' }).click()
  await expect(day).toHaveText('Yesterday')
  await editor.getByRole('button', { name: 'Save the meal' }).click()
  await expect(snack).toHaveCount(0)
  await expect(page.getByText(/^Moved to /)).toBeVisible()

  // Taken back only while it is the latest change: it is.
  await page.getByRole('button', { name: 'Undo' }).click()
  await expect(page.getByText('Taken back: a meal changed.')).toBeVisible()
  await expect(snack).toHaveCount(1)
})

test('a meal is repeated today in a tap', async ({ page }) => {
  await signIn(page)
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Toast',
      items: [{ name: 'Toast and jam', nutrients: { kcal: 260, protein: 6, carbs: 48, fat: 4 } }],
    },
  })
  expect(logged.ok()).toBe(true)

  await page.goto('/')
  const toast = page.getByRole('article').filter({ hasText: 'Toast' })
  await expect(toast).toHaveCount(1)
  await openMeal(toast)
  await toast.getByRole('button', { name: 'Repeat today: Toast' }).click()
  await expect(page.getByText('Repeated: Toast · 260 kcal')).toBeVisible()
  await expect(toast).toHaveCount(2)
})

test("a meal's item is given in another unit, and an estimate saved as a food", async ({
  page,
}) => {
  await signIn(page)
  const food = await page.request.post('/api/v1/foods', {
    data: {
      name: 'Muesli',
      basis: '100g',
      source: 'label',
      nutrients: { kcal: 380, protein: 10, carbs: 64, fat: 8 },
      servings: [{ name: 'cup', amount: 90 }],
    },
  })
  expect(food.ok()).toBe(true)
  const saved: unknown = await food.json()
  const foodId =
    typeof saved === 'object' && saved !== null && 'id' in saved && typeof saved.id === 'string'
      ? saved.id
      : ''
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Office lunch',
      items: [
        { food_id: foodId, amount: 45, unit: 'g' },
        { name: 'Canteen soup', nutrients: { kcal: 180, protein: 6, carbs: 20, fat: 8 } },
      ],
    },
  })
  expect(logged.ok()).toBe(true)

  await page.goto('/')
  const lunch = page.getByRole('article').filter({ hasText: 'Office lunch' })
  await openMeal(lunch)
  await lunch.getByRole('button', { name: 'Change the meal: Office lunch' }).click()
  const editor = page.getByRole('dialog', { name: 'Change the meal' })
  // 45 g are half a cup of 90 g.
  await editor.getByLabel('Unit: Muesli').selectOption({ label: 'cup' })
  await expect(editor.getByLabel('Amount: Muesli')).toHaveValue('0.5')
  await editor.getByLabel('Amount: Muesli').fill('1')
  await editor.getByRole('button', { name: 'More: Canteen soup' }).click()
  await page
    .getByRole('menuitemcheckbox', { name: 'Save as a food of yours: Canteen soup' })
    .click()
  await expect(editor).toContainText('Saved as a food of yours, a portion of it eaten.')
  await editor.getByRole('button', { name: 'Save the meal' }).click()

  await expect(lunch).toContainText('90 g')
  await expect(lunch).toContainText('Canteen soup')
  const found = await page.request.get('/api/v1/foods?query=Canteen%20soup')
  expect(JSON.stringify(await found.json())).toContain('"basis":"portion"')
})

test.describe('on a phone 390 px wide', () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test("a meal's items keep room for their names, and the focus follows their menu", async ({
    page,
  }) => {
    await signIn(page)
    const tuna = 'Tuna chunks in spring water'
    const chard = 'Chard with olive oil and garlic'
    const food = await page.request.post('/api/v1/foods', {
      data: {
        name: tuna,
        basis: '100g',
        source: 'label',
        nutrients: { kcal: 116, protein: 26, carbs: 0, fat: 1 },
        servings: [{ name: 'can', amount: 80 }],
      },
    })
    expect(food.ok()).toBe(true)
    const saved: unknown = await food.json()
    const foodId =
      typeof saved === 'object' && saved !== null && 'id' in saved && typeof saved.id === 'string'
        ? saved.id
        : ''
    const logged = await page.request.post('/api/v1/meals', {
      data: {
        description: 'Tuna plate',
        items: [
          { food_id: foodId, amount: 160 },
          { name: chard, nutrients: { kcal: 120, protein: 3, carbs: 6, fat: 9 } },
        ],
      },
    })
    expect(logged.ok()).toBe(true)

    await page.goto('/')
    const plate = page.getByRole('article').filter({ hasText: 'Tuna plate' })
    await openMeal(plate)
    await plate.getByRole('button', { name: 'Change the meal: Tuna plate' }).click()
    const editor = page.getByRole('dialog', { name: 'Change the meal' })
    await expect(editor.getByLabel(`Amount: ${tuna}`)).toHaveValue('160')
    await expect(editor.getByLabel(`Unit: ${tuna}`)).toHaveValue('g')

    // Nothing runs past the sheet's width, and a name takes the lines its
    // words need beside the amount, not a line a word.
    const body = editor.locator('form > div').first()
    expect(await body.evaluate((el) => el.scrollWidth - el.clientWidth)).toBe(0)
    const lines = (name: string) =>
      editor.evaluate((el, text) => {
        const walk = document.createTreeWalker(el, NodeFilter.SHOW_TEXT)
        for (let node = walk.nextNode(); node !== null; node = walk.nextNode()) {
          if (node.textContent === text) {
            const range = document.createRange()
            range.selectNodeContents(node)
            return new Set([...range.getClientRects()].map((r) => Math.round(r.top))).size
          }
        }
        return 0
      }, name)
    expect(await lines(tuna)).toBeGreaterThan(0)
    expect(await lines(tuna)).toBeLessThanOrEqual(2)
    expect(await lines(chard)).toBe(1)

    // The unit after the amount is a choice of the food's: 160 g are two
    // cans.
    await editor.getByLabel(`Unit: ${tuna}`).selectOption({ label: 'can' })
    await expect(editor.getByLabel(`Amount: ${tuna}`)).toHaveValue('2')

    // Another food chosen, by a tap or the keyboard, its search has the
    // focus, and takes what is typed; Cancel gives the focus back.
    const more = editor.getByRole('button', { name: `More: ${tuna}` })
    const search = editor.getByLabel(`A food of yours in place of ${tuna}`)
    await more.click()
    await page.getByRole('menuitem', { name: `Another food in place of: ${tuna}` }).click()
    await expect(search).toBeFocused()
    await page.keyboard.type('rice')
    await expect(search).toHaveValue('rice')
    await editor.getByRole('button', { name: 'Cancel' }).click()
    await expect(search).toHaveCount(0)
    await expect(more).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(
      page.getByRole('menuitem', { name: `Another food in place of: ${tuna}` }),
    ).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(search).toBeFocused()

    // Removed, the item's Keep has the focus; kept, its menu.
    await editor.getByRole('button', { name: `More: ${chard}` }).click()
    await page.getByRole('menuitem', { name: `Remove: ${chard}` }).click()
    const keep = editor.getByRole('button', { name: `Keep: ${chard}` })
    await expect(keep).toBeFocused()
    await keep.click()
    await expect(editor.getByRole('button', { name: `More: ${chard}` })).toBeFocused()
  })
})
