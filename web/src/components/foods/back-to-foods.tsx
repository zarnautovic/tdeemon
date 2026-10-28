import { Link, useSearch } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'

import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { useDesktop } from '@/lib/media'

// BackToFoods goes back to the foods as they were left, their search,
// list and order, which a food's address keeps. A desktop has them
// beside the food, and no need of it.
export function BackToFoods() {
  const t = useMessages()
  const desktop = useDesktop()
  const { q, filter, order } = useSearch({ from: '/app/foods' })

  if (desktop) {
    return null
  }
  return (
    <Link
      to="/foods"
      search={{ q, filter, order }}
      className={buttonVariants({ variant: 'ghost', size: 'sm', className: '-ml-2' })}
    >
      <ChevronLeft aria-hidden />
      {t.foods.back}
    </Link>
  )
}
