import { cleanup, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { fakeServer, renderAt, signedIn, today } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  localStorage.clear()
})

// A day of the week's history, eaten of a target of 2,250.
function eaten(date: string, kcal: number) {
  return {
    date,
    status: 'complete',
    type: 'rest',
    meals: 3,
    intake: { kcal, protein: 150, carbs: 250, fat: 70 },
    target: { kcal: 2250, protein: 160, carbs: 250, fat: 70 },
  }
}

// Wednesday 21 a little over its target, on it; Thursday 22 well over.
function serveWeek() {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/history': () =>
      Response.json({
        from: '2026-10-17',
        to: today,
        days: [eaten('2026-10-21', 2300), eaten('2026-10-22', 2600)],
        weeks: [],
      }),
  })
}

async function dayOf(name: string) {
  const week = await screen.findByRole('navigation', { name: 'The week' })
  return within(week).findByRole('link', { name })
}

// On its target, the mini ring and the bar are the brand's colour alone.
test('a day a little over its target is on it in the week, in one colour', async () => {
  serveWeek()
  renderAt('/')

  const day = await dayOf('Wed 21: 2,300 of 2,250 kcal')
  expect(day.querySelector('.stroke-over, .bg-over')).toBeNull()
  expect(day.querySelectorAll('.stroke-primary, .bg-primary')).toHaveLength(2)
})

// Well over it, only what is above the target's tick is in the over
// colour, and thinner, so it shows without the colour too; nothing is an
// error's.
test('a day well over its target shows its excess above the target alone, thinner', async () => {
  serveWeek()
  renderAt('/')

  const day = await dayOf('Thu 22: 2,600 of 2,250 kcal')
  const ring = day.querySelector('circle.stroke-over')
  expect(ring?.getAttribute('stroke-width')).toBe('2')
  expect(day.querySelector('circle.stroke-primary')?.getAttribute('stroke-width')).toBe('4')

  // The week's most is 2,600: the brand's bar up to the target, the
  // excess on it, narrower.
  const bar = day.querySelector<HTMLElement>('span.bg-primary')
  const excess = day.querySelector<HTMLElement>('span.bg-over')
  expect(Number.parseFloat(bar?.style.height ?? '')).toBeCloseTo((2250 / 2600) * 100, 5)
  expect(Number.parseFloat(excess?.style.bottom ?? '')).toBeCloseTo((2250 / 2600) * 100, 5)
  expect(Number.parseFloat(excess?.style.height ?? '')).toBeCloseTo((350 / 2600) * 100, 5)
  expect(excess?.classList.contains('inset-x-1.5')).toBe(true)
  expect(day.querySelector('.stroke-destructive, .bg-destructive')).toBeNull()
})
