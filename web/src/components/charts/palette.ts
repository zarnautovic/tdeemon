import { useSyncExternalStore } from 'react'

// ChartColors are the colours a chart draws with, each from a token of
// the page's (index.css), so a chart takes the theme's: its text and grid
// as the page's, its series in the chart tokens, and the plan's lines (a
// goal, a target) in the warning's colour.
export interface ChartColors {
  // A tooltip's text, and its background and edge.
  text: string
  surface: string
  // The axes' labels and the legends', and the grid's lines.
  muted: string
  grid: string
  // The series: the trend weight, the TDEE, a night of 6 hours, the body
  // fat, and the dots of the weigh-ins and of what was eaten.
  primary: string
  blue: string
  amber: string
  pink: string
  neutral: string
  // The plan's lines, and what falls short (a short night).
  warning: string
  bad: string
}

// tokens are the custom properties each colour is read from.
export const tokens: Record<keyof ChartColors, string> = {
  text: '--foreground',
  surface: '--card',
  muted: '--muted-foreground',
  grid: '--border',
  primary: '--chart-1',
  blue: '--chart-2',
  amber: '--chart-3',
  pink: '--chart-4',
  neutral: '--chart-5',
  warning: '--warning',
  bad: '--destructive',
}

// palette is the charts' colours in the dark theme, health-api's validated
// palette (design/tokens), which the dark theme's tokens in index.css were
// made from and keep to (palette.test.ts). Blue and violet never come as
// series next to each other: they read as one colour to many eyes. A page
// without the stylesheet (a test's) draws with these.
export const palette: ChartColors = {
  text: '#e8ecf4',
  surface: '#121722',
  muted: '#a9b2c4',
  grid: '#253045',
  primary: '#4fd1a5',
  blue: '#7aa2f7',
  amber: '#f0b45c',
  pink: '#f07fa0',
  neutral: '#8b95ab',
  warning: '#f0b45c',
  bad: '#f07178',
}

// chartColors are the colours of the theme the page is in now, as its
// stylesheet gives the tokens. A token that names another (--chart-1 is
// the primary's) is followed to its value, which a browser gives already;
// one the page does not have is the palette's.
export function chartColors(
  style: CSSStyleDeclaration = getComputedStyle(document.documentElement),
): ChartColors {
  const read = (name: string, depth = 0): string => {
    const value = style.getPropertyValue(name).trim()
    const named = /^var\((--[\w-]+)\)$/.exec(value)?.[1]
    return named !== undefined && depth < 4 ? read(named, depth + 1) : value
  }
  const of = (key: keyof ChartColors) => read(tokens[key]) || palette[key]
  return {
    text: of('text'),
    surface: of('surface'),
    muted: of('muted'),
    grid: of('grid'),
    primary: of('primary'),
    blue: of('blue'),
    amber: of('amber'),
    pink: of('pink'),
    neutral: of('neutral'),
    warning: of('warning'),
    bad: of('bad'),
  }
}

// subscribe listens for what changes the tokens: the theme's class on
// <html> (lib/theme.ts), and the system's scheme, which a theme left to
// the system follows.
function subscribe(listener: () => void) {
  const observer = new MutationObserver(listener)
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  const media =
    typeof globalThis.matchMedia === 'function'
      ? globalThis.matchMedia('(prefers-color-scheme: dark)')
      : undefined
  media?.addEventListener('change', listener)
  return () => {
    observer.disconnect()
    media?.removeEventListener('change', listener)
  }
}

// The colours last read, kept while they are the same, so React sees no
// change until there is one.
let last: { key: string; colors: ChartColors } | undefined

function snapshot(): ChartColors {
  const colors = chartColors()
  const key = JSON.stringify(colors)
  if (last?.key !== key) {
    last = { key, colors }
  }
  return last.colors
}

// useChartColors are the colours of the theme the page is in, read again
// when it changes, so the charts draw again in the new one.
export function useChartColors(): ChartColors {
  return useSyncExternalStore(subscribe, snapshot)
}
