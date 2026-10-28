import type { Meal } from '@/api/types'

// foodsOf is what a meal is of, whatever the amounts: its foods, an
// estimate by its name, each as often as it is in it, in any order.
function foodsOf(meal: Meal): string {
  return JSON.stringify(meal.items.map((i) => i.food_id ?? `estimate:${i.name}`).toSorted())
}

// pastMeals are meals of before, the latest first, as the meals to repeat
// list them: the meals of the same foods once, the latest, whatever their
// amounts and names. The server's list has a meal eaten again in the same
// amounts once; eaten in others, it is the same meal to repeat, which
// would show as the same row twice.
export function pastMeals(meals: readonly Meal[]): Meal[] {
  const seen = new Set<string>()
  return meals.filter((m) => {
    const foods = foodsOf(m)
    if (seen.has(foods)) {
      return false
    }
    seen.add(foods)
    return true
  })
}
