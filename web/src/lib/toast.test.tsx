import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Toasts } from '@/components/toasts'
import { sayChange, toasts } from '@/lib/toast'

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

const date = '2026-10-23'

// A delete asks nothing first: its toast, and its Undo, last longer, as
// a toast that warns does.
test("a delete's toast lasts 10 s, as one that warns; another change's 6 s", () => {
  const add = vi.spyOn(toasts, 'add')

  sayChange({ title: 'Logged' }, { last: { action: 'meal.create', entity: 'a' }, date })
  sayChange({ title: 'Deleted' }, { last: { action: 'meal.delete', entity: 'a' }, date })
  sayChange(
    { title: 'Deleted' },
    { last: { action: 'metric.delete', entity: `${date}/rhr` }, date },
  )
  sayChange({ title: 'Logged', lines: ['The kcal do not match.'] })
  sayChange({ title: 'Saved' })

  expect(add.mock.calls.map(([toast]) => toast.timeout)).toEqual([
    6000, 10_000, 10_000, 10_000, 6000,
  ])
})

// Base UI keeps the toasts while the pointer is over them (and while the
// keyboard's focus is in them, or a finger touched them last): their
// time runs again once it leaves.
test('a toast stays while the pointer is over the toasts, and goes in its time after', () => {
  vi.useFakeTimers()
  render(<Toasts />)
  act(() => {
    sayChange({ title: 'The check-in is saved' })
  })
  const region = screen.getByRole('region', { name: 'Notifications' })

  fireEvent.mouseEnter(region)
  act(() => {
    vi.advanceTimersByTime(20_000)
  })
  expect(screen.queryByText('The check-in is saved')).not.toBeNull()

  fireEvent.mouseLeave(region)
  act(() => {
    vi.advanceTimersByTime(7000)
  })
  act(() => {
    vi.runAllTimers()
  })
  expect(screen.queryByText('The check-in is saved')).toBeNull()
})
