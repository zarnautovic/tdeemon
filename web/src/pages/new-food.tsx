import { useNavigate, useSearch } from '@tanstack/react-router'

import { BackToFoods } from '@/components/foods/back-to-foods'
import { FoodForm } from '@/components/foods/food-form'
import { useMessages } from '@/i18n/messages'

// NewFoodPage makes a food; once saved, the food's own page shows it in
// its place, the foods' list as it was.
export function NewFoodPage() {
  const t = useMessages()
  const navigate = useNavigate()
  const { q, filter, order } = useSearch({ from: '/app/foods' })

  return (
    <div className="space-y-3">
      <BackToFoods />
      <h1 className="text-h2">{t.foods.newTitle}</h1>
      <div className="rounded-lg bg-card p-4 shadow-card lg:p-6">
        <FoodForm
          onSaved={(food) => {
            void navigate({
              to: '/foods/$id',
              params: { id: food.id },
              search: { q, filter, order },
              replace: true,
            })
          }}
        />
      </div>
    </div>
  )
}
