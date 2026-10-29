// The checks run code in the page, which has the DOM's types.
/// <reference lib="dom" />

import { AxeBuilder } from '@axe-core/playwright'
import { expect, test, type Page, type TestInfo } from '@playwright/test'

import { openMeal, signIn } from './user.ts'

// The checks of every screen of the app, on a phone and a desktop, in
// the light and the dark theme, in English and Croatian: axe finds
// nothing serious; on a phone, what a finger taps is 44 px or more; what
// the keyboard reaches shows it has the focus. Each screen is
// photographed too, to set beside the design: a phone's in Croatian and
// a desktop's in English, as the design has them.

const sizes = [
  { name: 'phone', viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true },
  { name: 'desktop', viewport: { width: 1440, height: 900 }, isMobile: false, hasTouch: false },
] as const
const themes = ['light', 'dark'] as const
const locales = ['en', 'hr'] as const

// What the screens show: a food, a meal of it and an estimate, and a
// weigh-in, made up, under names of their own each run.
interface Made {
  foodId: string
  meal: string
}

async function make(page: Page, tag: string): Promise<Made> {
  const api = page.request
  const food = await api.post('/api/v1/foods', {
    data: {
      name: `Rolled oats ${tag}`,
      basis: '100g',
      source: 'label',
      nutrients: { kcal: 370, protein: 13, carbs: 60, fat: 7 },
      servings: [{ name: 'cup', amount: 90 }],
    },
  })
  expect(food.ok()).toBe(true)
  const saved: unknown = await food.json()
  const foodId =
    typeof saved === 'object' && saved !== null && 'id' in saved && typeof saved.id === 'string'
      ? saved.id
      : ''
  const meal = `Overnight oats ${tag}`
  const logged = await api.post('/api/v1/meals', {
    data: {
      description: meal,
      items: [
        { food_id: foodId, amount: 80 },
        { name: 'Blueberries', nutrients: { kcal: 40, protein: 0.5, carbs: 9, fat: 0.2 } },
      ],
    },
  })
  expect(logged.ok()).toBe(true)
  expect((await api.put('/api/v1/days/today/metrics/weight', { data: { value: 72.5 } })).ok()).toBe(
    true,
  )
  // A day of a week long past, which the other tests leave alone: its
  // week has a day logged to say (today's has none over yet).
  const before = await api.post('/api/v1/meals', {
    data: {
      eaten_at: `${weekDay}T12:00:00Z`,
      items: [
        {
          name: `Barley risotto ${tag}`,
          nutrients: { kcal: 520, protein: 28, carbs: 70, fat: 12 },
        },
      ],
    },
  })
  expect(before.ok()).toBe(true)
  return { foodId, meal }
}

// The day of the week whose sheet is checked.
const weekDay = '2025-03-12'

// A screen is a page of the app, or a sheet over one, opened and settled.
interface Screen {
  name: string
  open: (page: Page, made: Made) => Promise<void>
}

// settled waits until the page has what it shows: nothing is loading,
// and what came has come (its entries ended), as axe sees colours as
// they are drawn.
async function settled(page: Page) {
  await expect(page.getByText(/^(Loading|Učitavanje)…$/)).toHaveCount(0)
  await expect(page.getByRole('heading').first()).toBeVisible()
  // Nothing asked again: a part that waits for its data is faded, which
  // axe would take for its colours.
  await expect(page.locator('[aria-busy="true"]')).toHaveCount(0)
  await page.evaluate(() =>
    Promise.race([
      Promise.all(
        document
          .getAnimations()
          .filter((a) => a.effect?.getComputedTiming().endTime !== Infinity)
          .map((a) => a.finished.catch(() => undefined)),
      ),
      // An entry paused, or one that starts again: not waited for long.
      new Promise((resolve) => setTimeout(resolve, 1000)),
    ]),
  )
}

const screens: Screen[] = [
  {
    name: 'today',
    open: async (page) => {
      await page.goto('/')
      await settled(page)
    },
  },
  {
    name: 'day-before',
    open: async (page) => {
      await page.goto('/')
      await settled(page)
      await page.getByRole('link', { name: /^(The day before|Dan prije)$/ }).click()
      await expect(
        page.getByRole('link', { name: /^(Back to today|Natrag na danas)$/ }),
      ).toBeVisible()
      await settled(page)
    },
  },
  {
    name: 'week',
    open: async (page) => {
      await page.goto(`/?date=${weekDay}`)
      await settled(page)
      await page.getByRole('button', { name: /^(That week|Taj tjedan): / }).click()
      await expect(page.getByRole('dialog')).toBeVisible()
      await settled(page)
    },
  },
  {
    name: 'quick-add',
    open: async (page) => {
      await page.goto('/?log=true')
      await expect(page.getByRole('dialog')).toBeVisible()
      await settled(page)
    },
  },
  {
    name: 'quick-add-food',
    open: async (page) => {
      await page.goto('/?log=true')
      await page.getByRole('button', { name: /^(A new food|Nova namirnica)$/ }).click()
      await expect(
        page.getByRole('button', { name: /^(Add to meal|Dodaj u obrok)$/ }),
      ).toBeVisible()
      await settled(page)
    },
  },
  {
    name: 'meal-editor',
    open: async (page, made) => {
      await page.goto('/')
      const meal = page.getByRole('article').filter({ hasText: made.meal })
      await openMeal(meal)
      await meal
        .getByRole('button', { name: new RegExp(`: ${made.meal}$`) })
        .first()
        .click()
      await expect(page.getByRole('dialog')).toBeVisible()
      await settled(page)
    },
  },
  {
    name: 'foods',
    open: async (page) => {
      await page.goto('/foods')
      await settled(page)
    },
  },
  {
    name: 'food',
    open: async (page, made) => {
      await page.goto(`/foods/${made.foodId}`)
      await settled(page)
    },
  },
  {
    name: 'progress',
    open: async (page) => {
      await page.goto('/progress')
      await settled(page)
    },
  },
  {
    name: 'plan',
    open: async (page) => {
      await page.goto('/plan')
      await settled(page)
    },
  },
  {
    name: 'settings',
    open: async (page) => {
      await page.goto('/settings')
      await settled(page)
    },
  },
  // The pages the plan's and the settings' rows lead to.
  ...[
    '/plan/profile',
    '/plan/limits',
    '/plan/notes',
    '/settings/appearance',
    '/settings/account',
    '/settings/devices',
  ].map((path) => ({
    name: path.slice(1).replace('/', '-'),
    open: async (page: Page) => {
      await page.goto(path)
      await settled(page)
    },
  })),
  // An AI coach's access key just made, with the steps to connect it.
  {
    name: 'connect',
    open: async (page) => {
      await page.goto('/settings/devices?connect=coach')
      const sheet = page.getByRole('dialog')
      await sheet
        .getByRole('button', { name: /^(Create the access key|Napravi pristupni ključ)$/ })
        .click()
      await expect(
        sheet.getByText(/^(Waiting for the first call…|Čeka prvi poziv…)$/),
      ).toBeVisible()
      await settled(page)
    },
  },
]

// serious is what axe finds of a serious or critical impact, against
// WCAG 2.2 AA, each said with where.
async function serious(page: Page): Promise<string[]> {
  const found = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
    .analyze()
  return found.violations
    .filter((v) => v.impact === 'serious' || v.impact === 'critical')
    .flatMap((v) =>
      v.nodes.map((n) => `${v.id}: ${n.target.join(' ')} (${n.failureSummary ?? ''})`),
    )
}

// smallTargets are what a finger taps on the page, outside a line of
// text, smaller than 44 px either way. What a control draws past its box
// (an ::after laid over it, as a chip does) counts as its own.
async function smallTargets(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const taps = document.querySelectorAll<HTMLElement>(
      'button, a[href], select, textarea, input:not([type=hidden]), [role=button], [role=tab], [role=switch]',
    )
    const small: string[] = []
    for (const el of taps) {
      // A radio or a checkbox hidden for its label: the label is tapped.
      const label = el.closest('label')
      const target =
        el instanceof HTMLInputElement &&
        (el.type === 'radio' || el.type === 'checkbox') &&
        label !== null
          ? label
          : el
      const box = target.getBoundingClientRect()
      const style = getComputedStyle(target)
      const hidden =
        box.width <= 1 ||
        box.height <= 1 ||
        style.visibility === 'hidden' ||
        target.closest('[inert], [aria-hidden=true]') !== null
      // A link in a sentence is the sentence's size (WCAG 2.5.8).
      const inline = target.tagName === 'A' && style.display === 'inline'
      if (hidden || inline) {
        continue
      }
      // How far an ::after laid over it reaches past its box, each way.
      const after = getComputedStyle(target, '::after')
      const [top, bottom, left, right] =
        after.content !== 'none' && after.position === 'absolute'
          ? [after.top, after.bottom, after.left, after.right].map((v) =>
              Math.max(0, -Number.parseFloat(v) || 0),
            )
          : [0, 0, 0, 0]
      const height = box.height + top + bottom
      const width = box.width + left + right
      if (height < 43.5 || width < 43.5) {
        const name = target.getAttribute('aria-label') ?? target.textContent.trim().slice(0, 40)
        small.push(
          `${target.tagName.toLowerCase()} "${name}" ${String(Math.round(width))}×${String(Math.round(height))}`,
        )
      }
    }
    return small
  })
}

// unfocused are the keyboard's first stops on the page, up to 12, that
// show no sign of having the focus: nothing of it, or of the label or
// field around it, looks otherwise than without the focus (an outline, a
// ring, a border, a background). Transitions are off while it looks, from
// before the first stop, so each state is drawn at once.
async function unfocused(page: Page): Promise<string[]> {
  // A sheet the script makes, which the page's policy (no inline style)
  // leaves be.
  await page.evaluate(() => {
    const still = new CSSStyleSheet()
    still.replaceSync('*, ::before, ::after { transition: none !important; }')
    document.adoptedStyleSheets = [...document.adoptedStyleSheets, still]
  })
  const missing = await stops(page, 12)
  await page.evaluate(() => {
    document.adoptedStyleSheets = document.adoptedStyleSheets.slice(0, -1)
  })
  return missing
}

async function stops(page: Page, left: number): Promise<string[]> {
  if (left === 0) {
    return []
  }
  await page.keyboard.press('Tab')
  const seen = await page.evaluate(() => {
    const el = document.activeElement
    if (!(el instanceof HTMLElement) || el === document.body) {
      return 'none'
    }
    const around = [el, el.parentElement, el.parentElement?.parentElement].filter(
      (e) => e instanceof HTMLElement,
    )
    const look = () =>
      around.map((e) => {
        const s = getComputedStyle(e)
        // An outline of no style is none, whatever its width says.
        const outline =
          s.outlineStyle === 'none'
            ? 'none'
            : `${s.outlineStyle} ${s.outlineWidth} ${s.outlineColor}`
        return [outline, s.boxShadow, s.borderColor, s.backgroundColor].join(' | ')
      })
    const focused = look()
    el.blur()
    const blurred = look()
    el.focus()
    const name = el.getAttribute('aria-label') ?? el.textContent.trim().slice(0, 40)
    return focused.some((f, i) => f !== blurred[i])
      ? 'shown'
      : `${el.tagName.toLowerCase()} "${name}"`
  })
  if (seen === 'none') {
    return []
  }
  const rest = await stops(page, left - 1)
  return seen === 'shown' ? rest : [seen, ...rest]
}

// inTurn runs a step for each item, each once the one before is done, as
// one page takes them.
async function inTurn<T>(items: readonly T[], step: (item: T) => Promise<void>) {
  await items.reduce<Promise<void>>(
    (before, item) => before.then(() => step(item)),
    Promise.resolve(),
  )
}

// photograph keeps the whole page as it is, in the test's output, where
// CI keeps it with the run (e2e-screens), and in its report.
async function photograph(page: Page, testInfo: TestInfo, name: string) {
  const path = testInfo.outputPath(`screen-${name}.png`)
  await page.screenshot({ path, fullPage: true })
  await testInfo.attach(`screen-${name}`, { path, contentType: 'image/png' })
}

for (const size of sizes) {
  for (const theme of themes) {
    for (const locale of locales) {
      test.describe(`${size.name}, ${theme}, ${locale}`, () => {
        test.use({
          viewport: size.viewport,
          isMobile: size.isMobile,
          hasTouch: size.hasTouch,
          colorScheme: theme,
        })

        test.afterEach(async ({ page }) => {
          // The e2e user's language back for the other tests.
          await page.request.patch('/api/v1/me', { data: { locale: 'en' } })
        })

        test('every screen passes axe, its targets and its focus', async ({ page }, testInfo) => {
          // Long: every screen, its checks and its photograph.
          test.setTimeout(180_000)
          await signIn(page)
          const made = await make(
            page,
            `${size.name}-${theme}-${locale}-${String(testInfo.repeatEachIndex)}`,
          )
          expect((await page.request.patch('/api/v1/me', { data: { locale } })).ok()).toBe(true)
          const photographed =
            (size.name === 'phone' && locale === 'hr') ||
            (size.name === 'desktop' && locale === 'en')

          const found: Record<string, string[]> = {}
          await inTurn(screens, async (screen) => {
            await screen.open(page, made)
            const problems = [
              ...(await serious(page)),
              ...(size.name === 'phone' ? await smallTargets(page) : await unfocused(page)),
            ]
            if (problems.length > 0) {
              found[screen.name] = problems
            }
            if (photographed) {
              await photograph(page, testInfo, screen.name)
            }
          })
          expect(found).toEqual({})
        })
      })
    }
  }
}

// motions are what the page's elements would move, other than by
// fading: the properties their transitions change, and those of the
// animations that run.
async function motions(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const found = new Set<string>()
    for (const el of document.querySelectorAll('*')) {
      const style = getComputedStyle(el)
      const lasts = style.transitionDuration.split(',').some((d) => Number.parseFloat(d) > 0)
      if (lasts) {
        for (const p of style.transitionProperty.split(',').map((x) => x.trim())) {
          if (p !== 'opacity' && p !== 'none') {
            found.add(p)
          }
        }
      }
    }
    for (const a of document.getAnimations()) {
      const keyframes = a.effect instanceof KeyframeEffect ? a.effect.getKeyframes() : []
      for (const p of keyframes.flatMap((k) => Object.keys(k))) {
        if (!['offset', 'easing', 'composite', 'computedOffset', 'opacity'].includes(p)) {
          found.add(p)
        }
      }
    }
    return [...found].toSorted()
  })
}

test.describe('with less motion asked for', () => {
  test.use({ reducedMotion: 'reduce' })

  test('the day and a sheet only fade', async ({ page }) => {
    await signIn(page)
    await make(page, 'motion')
    // The ring and the bars fill, a meal's chevron turns, a sheet slides
    // up: none of it, only fades.
    await page.goto('/')
    await settled(page)
    expect(await motions(page)).toEqual([])
    await page.goto('/?log=true')
    await expect(page.getByRole('dialog')).toBeVisible()
    expect(await motions(page)).toEqual([])
  })
})
