import { Link, useNavigate, useSearch } from '@tanstack/react-router'

import { BackToFoods } from '@/components/foods/back-to-foods'
import { RecipeEditor } from '@/components/recipes/recipe-editor'
import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// NewRecipePage writes a new recipe; once saved, its page shows it in
// its place, the foods' list as it was.
export function NewRecipePage() {
  const t = useMessages()
  const navigate = useNavigate()
  const { q, filter, order } = useSearch({ from: '/app/foods' })

  return (
    <div className="space-y-3">
      <BackToFoods />
      <header className="flex items-center justify-between gap-2">
        <h1 className="text-h2">{t.recipes.newTitle}</h1>
        <Link
          to="/foods"
          search={{ q, filter, order }}
          className={buttonVariants({ variant: 'ghost', size: 'sm' })}
        >
          {t.foods.cancel}
        </Link>
      </header>
      <RecipeEditor
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
  )
}
