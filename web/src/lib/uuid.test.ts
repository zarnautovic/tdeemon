import { expect, test } from 'vitest'

import { uuidv7 } from '@/lib/uuid'

test('a UUIDv7 carries its time, its version and its variant', () => {
  const at = Date.UTC(2026, 9, 23, 10, 0, 0, 123)
  const id = uuidv7(at)

  expect(id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  expect(Number.parseInt(id.replaceAll('-', '').slice(0, 12), 16)).toBe(at)
  expect(uuidv7(at)).not.toBe(id)
})
