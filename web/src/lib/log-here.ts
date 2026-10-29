import { createContext } from 'react'

// LogHere opens the meal to log of the page shown, from (what opened it,
// which the focus goes back to; null, where it was), when the page has
// one of its own: today's does, so the app's Log a meal (its key N, the
// bar's +, the sidebar's button) logs there as on the page itself, and
// the meal's row takes the focus.
export type LogHere = (from: HTMLElement | null) => void

// LogHereContext takes a page's LogHere while it shows, and null when it
// goes; the app's frame gives it.
export const LogHereContext = createContext<((log: LogHere | null) => void) | null>(null)
