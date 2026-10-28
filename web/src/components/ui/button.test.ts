import { expect, test } from 'vitest'

import { buttonVariants } from '@/components/ui/button'

test("a variant's border and colour win over the base's", () => {
  const outline = buttonVariants({ variant: 'outline' }).split(' ')
  expect(outline).toContain('border-border')
  expect(outline).not.toContain('border-transparent')
  expect(buttonVariants({ variant: 'outline', className: 'w-full' })).toContain('w-full')
})
