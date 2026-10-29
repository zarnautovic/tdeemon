import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { Sheet } from '@/components/sheet'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

// Opener is a button that opens a sheet of a field, as the app's do.
function Opener() {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button
        type="button"
        onClick={() => {
          setOpen(true)
        }}
      >
        Measure
      </button>
      <Sheet open={open} onOpenChange={setOpen} title="Measurements">
        <label>
          Weight
          <input />
        </label>
      </Sheet>
    </>
  )
}

test('a sheet given no field to focus focuses its first control, as a dialog does', async () => {
  render(<Opener />)
  fireEvent.click(screen.getByRole('button', { name: 'Measure' }))

  const sheet = await screen.findByRole('dialog', { name: 'Measurements' })
  await waitFor(() => {
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Close' }))
  })
  expect(sheet.contains(document.activeElement)).toBe(true)
})

// desktop stubs the window's width as a desktop's, or a phone's.
function desktop(is: boolean) {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: is && query === '(min-width: 64rem)',
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }))
}

test.each([
  ['a phone', false, 'down'],
  ['a desktop', true, 'right'],
])('on %s a sheet is dragged away the way it came', async (_, wide, way) => {
  desktop(wide)
  render(<Opener />)
  fireEvent.click(screen.getByRole('button', { name: 'Measure' }))

  const sheet = await screen.findByRole('dialog', { name: 'Measurements' })
  expect(sheet.getAttribute('data-swipe-direction')).toBe(way)
})
