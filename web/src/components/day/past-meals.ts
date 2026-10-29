import type { RecentMeal } from '@/api/types'

// foodsOf is what a meal is of, whatever the amounts: its foods, an
// estimate by its name, each as often as it is in it, in any order.
function foodsOf(meal: RecentMeal): string {
  return JSON.stringify(meal.items.map((i) => i.food_id ?? `estimate:${i.name}`).toSorted())
}

// pastMeals are meals of before, the latest first, as the meals to repeat
// list them: the meals of the same foods once, the latest, whatever their
// amounts and names, eaten whenever any of them was (the latest first).
// The server's list has a meal eaten again in the same amounts once;
// eaten in others, it is the same meal to repeat, which would show as the
// same row twice.
export function pastMeals(meals: readonly RecentMeal[]): RecentMeal[] {
  const kept = new Map<string, RecentMeal>()
  for (const m of meals) {
    const foods = foodsOf(m)
    const first = kept.get(foods)
    kept.set(
      foods,
      first === undefined
        ? m
        : {
            ...first,
            eaten: [...first.eaten, ...m.eaten].toSorted(
              (a, b) => Date.parse(b.at) - Date.parse(a.at),
            ),
          },
    )
  }
  return [...kept.values()]
}
