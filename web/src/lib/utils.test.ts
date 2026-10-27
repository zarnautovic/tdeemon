import { expect, test } from 'vitest'

import { cn } from '@/lib/utils'

test("the theme's text sizes are sizes, kept beside a colour and replaced by a size", () => {
  expect(cn('text-body text-foreground', 'text-destructive')).toBe('text-body text-destructive')
  expect(cn('text-body', 'text-small')).toBe('text-small')
  expect(cn('text-xs', 'text-caption')).toBe('text-caption')
  for (const size of ['display', 'title', 'h2', 'h3']) {
    expect(cn('text-muted-foreground', `text-${size}`)).toBe(`text-muted-foreground text-${size}`)
  }
})

test("the theme's shadows are shadows", () => {
  expect(cn('shadow-card', 'shadow-sheet')).toBe('shadow-sheet')
  expect(cn('shadow-sm', 'shadow-card')).toBe('shadow-card')
})
