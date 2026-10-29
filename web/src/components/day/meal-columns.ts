// The columns of a desktop's meals, a table (MealList): the time, as
// wide as the language's times, the meal, its kcal, protein, carbs and
// fat, and the mark that it opens. The heads, each meal's row and the
// total share them, so they line up.
export function mealColumns(hour12: boolean): string {
  return hour12
    ? 'grid-cols-[3.75rem_minmax(0,1fr)_4.5rem_repeat(3,3.5rem)_1rem] gap-x-2.5 px-4'
    : 'grid-cols-[2.5rem_minmax(0,1fr)_4.5rem_repeat(3,3.5rem)_1rem] gap-x-2.5 px-4'
}
