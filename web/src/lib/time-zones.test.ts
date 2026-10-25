import { expect, test } from 'vitest'

import { renamed } from '@/lib/time-zones'

test.each(Object.entries(renamed))('%s is %s now, a zone of its own name', (old, now) => {
  expect(now).not.toBe(old)
  const at = new Date('2026-07-01T12:00:00Z')
  const clock = (timeZone: string) =>
    new Intl.DateTimeFormat('en', { timeZone, hour: 'numeric', minute: 'numeric' }).format(at)
  expect(clock(now)).toBe(clock(old))
})
