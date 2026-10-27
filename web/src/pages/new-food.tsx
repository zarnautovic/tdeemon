import { useNavigate } from '@tanstack/react-router'

import { BackToFoods } from '@/components/foods/back-to-foods'
import { FoodForm } from '@/components/foods/food-form'
import { useMessages } from '@/i18n/messages'

// NewFoodPage makes a food; once saved, the food's own page shows it.
export function NewFoodPage() {
  const t = useMessages()
  const navigate = useNavigate()

  return (
    <section className="max-w-3xl space-y-4">
      <BackToFoods />
      <h1 className="text-2xl font-semibold">{t.foods.newTitle}</h1>
      <FoodForm
        onSaved={(food) => {
          void navigate({
            to: '/foods/$id',
            params: { id: food.id },
            replace: true,
            // Back from the food is back to the foods it was made from.
            state: (s) => s,
          })
        }}
      />
    </section>
  )
}
