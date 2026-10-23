import { Link, useRouterState } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'

import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// BackToFoods goes back to the foods page as it was left, its search, its
// list and its order, which the link to this page carried.
export function BackToFoods() {
  const t = useMessages()
  const from = useRouterState({ select: (s) => s.location.state.foods })

  return (
    <Link
      to="/foods"
      search={from ?? {}}
      className={buttonVariants({ variant: 'ghost', className: '-ml-2' })}
    >
      <ChevronLeft aria-hidden />
      {t.foods.back}
    </Link>
  )
}
