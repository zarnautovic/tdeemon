import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import { InfoTip } from '@/components/ui/info-tip'

afterEach(cleanup)

function renderTip() {
  render(
    <p>
      TDEE 2,409
      <InfoTip label="What TDEE is" title="TDEE">
        TDEE is how much energy you burn a day.
      </InfoTip>
    </p>,
  )
  return screen.getByRole('button', { name: 'What TDEE is' })
}

test('the ⓘ is a button named for what it explains, closed at first', () => {
  const tip = renderTip()
  expect(tip.getAttribute('aria-expanded')).toBe('false')
  expect(screen.queryByRole('dialog')).toBeNull()
})

// To a screen reader the sentences are a dialog, named by the number's
// name and described by them.
test('a tap opens what the number is, as a dialog named by it', async () => {
  const tip = renderTip()
  fireEvent.click(tip)

  const dialog = await screen.findByRole('dialog', { name: 'TDEE' })
  expect(dialog.getAttribute('aria-describedby')).not.toBeNull()
  expect(document.getElementById(dialog.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'TDEE is how much energy you burn a day.',
  )
  expect(tip.getAttribute('aria-expanded')).toBe('true')
})

// The keyboard's Enter is a native button's click; the end-to-end tests
// press it (over-target.spec.ts).
test('Escape closes it, the focus back on the ⓘ', async () => {
  const tip = renderTip()
  tip.focus()
  fireEvent.click(tip)

  const dialog = await screen.findByRole('dialog', { name: 'TDEE' })
  fireEvent.keyDown(dialog, { key: 'Escape' })
  await waitFor(() => {
    expect(screen.queryByRole('dialog')).toBeNull()
  })
  expect(document.activeElement).toBe(tip)
})
