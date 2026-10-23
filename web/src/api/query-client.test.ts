import { expect, test } from 'vitest'

import { retry } from '@/api/errors'
import { createQueryClient } from '@/api/query-client'

test("the app's queries are asked again by its rule", () => {
  expect(createQueryClient().getDefaultOptions().queries?.retry).toBe(retry)
})
