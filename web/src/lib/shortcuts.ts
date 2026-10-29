import { useEffect, useRef } from 'react'

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

// How long the second key of a sequence (g, then d) is waited for.
const sequenceMs = 1500

// apple tells a Mac's or an iPad's keyboard, whose ⌘ is another's Ctrl.
export function apple(): boolean {
  return /Mac|iPhone|iPad/.test(globalThis.navigator.platform)
}

// keyOf is a key pressed as an action names it: its KeyboardEvent.key
// alone (Shift aside, which makes ?), or with Ctrl or ⌘ as "Mod+z"; with
// Alt, or Ctrl and ⌘ both, none.
function keyOf(event: KeyboardEvent): string | undefined {
  if (event.altKey || (event.ctrlKey && event.metaKey)) {
    return undefined
  }
  return event.ctrlKey || event.metaKey ? `Mod+${event.key}` : event.key
}

// useShortcuts runs the action of a key pressed on the page, outside a
// field or a control that takes keys itself: by its KeyboardEvent.key,
// "Mod+z" for one with Ctrl or ⌘ (Ctrl+Shift+Z is "Mod+Z", another), or
// a sequence of two ("g d", the second within a second and a half of the
// first). The actions of the latest render run, and a sequence begun
// before it goes on.
export function useShortcuts(actions: Readonly<Record<string, () => void>>) {
  const latest = useRef(actions)
  useEffect(() => {
    latest.current = actions
  }, [actions])
  useEffect(() => {
    // The first key of a sequence, while its second is waited for.
    let first: string | undefined
    let waiting: ReturnType<typeof setTimeout> | undefined
    function onKey(event: KeyboardEvent) {
      const now = latest.current
      if (event.defaultPrevented || event.isComposing) {
        return
      }
      if (event.target instanceof Element && event.target.closest(ownKeys)) {
        return
      }
      const key = keyOf(event)
      if (key === undefined) {
        return
      }
      const before = first
      first = undefined
      clearTimeout(waiting)
      const action = (before === undefined ? undefined : now[`${before} ${key}`]) ?? now[key]
      if (action) {
        event.preventDefault()
        action()
        return
      }
      if (Object.keys(now).some((name) => name.startsWith(`${key} `))) {
        event.preventDefault()
        first = key
        waiting = setTimeout(() => {
          first = undefined
        }, sequenceMs)
      }
    }
    globalThis.addEventListener('keydown', onKey)
    return () => {
      clearTimeout(waiting)
      globalThis.removeEventListener('keydown', onKey)
    }
  }, [])
}
