import { useEffect } from 'react'

// Where a key is the user's text or a control's own: a field, and the
// widgets that move by arrows or letters. A shortcut is not taken there.
const ownKeys = [
  'input',
  'textarea',
  'select',
  '[contenteditable]:not([contenteditable="false"])',
  '[role="combobox"]',
  '[role="listbox"]',
  '[role="menu"]',
  '[role="radiogroup"]',
  '[role="slider"]',
  '[role="tablist"]',
  '[role="dialog"]',
].join(', ')

// useShortcuts runs the action of a key pressed on the page, by its
// KeyboardEvent.key: alone (Shift aside, which makes ?), and outside a
// field or a control that takes keys itself.
export function useShortcuts(actions: Readonly<Record<string, () => void>>) {
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey) {
        return
      }
      if (event.isComposing || (event.target instanceof Element && event.target.closest(ownKeys))) {
        return
      }
      const action = actions[event.key]
      if (action) {
        event.preventDefault()
        action()
      }
    }
    globalThis.addEventListener('keydown', onKey)
    return () => {
      globalThis.removeEventListener('keydown', onKey)
    }
  }, [actions])
}
