import { afterEach, expect, test } from 'vitest'

import { chartColors, palette, tokens } from '@/components/charts/palette'
import { themeStyle, themeTokens } from '@/test/charts'

afterEach(() => {
  document.documentElement.classList.remove('dark')
  for (const style of document.head.querySelectorAll('style')) {
    style.remove()
  }
})

// resolved is a token's value in a theme, following the tokens it names.
function resolved(theme: Record<string, string>, name: string): string {
  const value = theme[name] ?? ''
  const named = /^var\((--[\w-]+)\)$/.exec(value)?.[1]
  return named === undefined ? value : resolved(theme, named)
}

test("the dark theme's tokens are the palette's", () => {
  const { dark } = themeTokens()
  expect(
    Object.fromEntries(Object.entries(tokens).map(([key, token]) => [key, resolved(dark, token)])),
  ).toEqual(palette)
})

test("the charts' colours are the page's tokens, light or dark as the page is", () => {
  const { light, dark } = themeTokens()
  document.head.append(themeStyle())

  expect(chartColors().primary).toBe(resolved(light, '--primary'))
  expect(chartColors().grid).toBe(resolved(light, '--border'))
  document.documentElement.classList.add('dark')
  expect(chartColors()).toEqual(palette)
  expect(chartColors().blue).toBe(resolved(dark, '--protein'))
})

test("a page without the tokens draws in the palette's", () => {
  expect(chartColors()).toEqual(palette)
})
