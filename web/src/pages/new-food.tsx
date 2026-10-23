import { Link, useNavigate } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'

import { FoodForm } from '@/components/foods/food-form'
import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// NewFoodPage makes a food; once saved, the food's own page shows it.
export function NewFoodPage() {
  const t = useMessages()
  const navigate = useNavigate()

  return (
    <section className="space-y-4">
      <Link to="/foods" className={buttonVariants({ variant: 'ghost', className: '-ml-2' })}>
        <ChevronLeft aria-hidden />
        {t.foods.back}
      </Link>
      <h1 className="text-2xl font-semibold">{t.foods.newTitle}</h1>
      <FoodForm
        onSaved={(food) => {
          void navigate({ to: '/foods/$id', params: { id: food.id }, replace: true })
        }}
      />
    </section>
  )
}
