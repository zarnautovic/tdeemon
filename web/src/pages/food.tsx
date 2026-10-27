import { useParams } from '@tanstack/react-router'

import { $api } from '@/api/client'
import { statusOf } from '@/api/errors'
import { fieldsOf, useSaveFood } from '@/api/foods'
import { SaveStatus } from '@/components/day/save-status'
import { BackToFoods } from '@/components/foods/back-to-foods'
import { FoodForm } from '@/components/foods/food-form'
import { LoadError } from '@/components/load-error'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { sayChange } from '@/lib/toast'

// FoodPage is a food of the user's, to correct, archive or bring back.
export function FoodPage() {
  const t = useMessages()
  const { id } = useParams({ from: '/app/foods/$id' })
  const food = $api.useQuery('get', '/api/v1/foods/{id}', { params: { path: { id } } })
  const archive = useSaveFood()

  let body
  if (food.isPending) {
    body = <output className="block text-muted-foreground">{t.loading}</output>
  } else if (food.isError && [404, 422].includes(statusOf(food.error) ?? 0)) {
    // 422: the address names no food's id at all.
    body = <p className="text-muted-foreground">{t.foods.notFound}</p>
  } else if (food.isError) {
    body = <LoadError error={food.error} onRetry={() => void food.refetch()} />
  } else {
    const f = food.data
    body = (
      <>
        <h1 className="text-2xl font-semibold">{f.name}</h1>
        {f.archived && (
          <p className="rounded-lg bg-muted px-3 py-2 text-sm">{t.foods.archivedNote}</p>
        )}
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            disabled={archive.isPending}
            onClick={() => {
              archive.mutate(
                { body: { ...fieldsOf(f), archived: !f.archived } },
                {
                  onSuccess: (saved) => {
                    sayChange({
                      title: saved.archived
                        ? t.toast.archived(saved.name)
                        : t.toast.inUse(saved.name),
                    })
                  },
                },
              )
            }}
          >
            {f.archived ? t.foods.unarchive : t.foods.archive}
          </Button>
          <SaveStatus change={archive} />
        </div>
        {/* A food corrected elsewhere, or archived here, starts the form
            again only when it is another food. */}
        <FoodForm key={f.id} food={f} />
      </>
    )
  }

  return (
    <section className="max-w-3xl space-y-4">
      <BackToFoods />
      {body}
    </section>
  )
}
