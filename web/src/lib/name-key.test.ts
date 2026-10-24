import { expect, test } from 'vitest'

import { nameKey } from '@/lib/name-key'
import keys from '@/test/name-keys.json'

// The names and the keys the server's food.Key gives them, written with
// the table by go test ./internal/domain/food -run TestWebKey -update.
test.each(keys)('%j keys as the server keys it', (name, key) => {
  expect(nameKey(name)).toBe(key)
})

test('the names a folding of its own got wrong are told apart, or not, as the server does', () => {
  expect(nameKey('µg')).toBe(nameKey('μg'))
  expect(nameKey('ﬂ oz')).toBe(nameKey('FL  OZ'))
  expect(nameKey('GROẞES STÜCK')).toBe(nameKey('Großes Stück'))
  expect(nameKey('I')).not.toBe(nameKey('ı'))
})
