import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import type { Food } from '@/api/types'
import { EntryRow } from '@/components/day/entry-row'
import type { FoodEntry } from '@/components/day/meal-entries'
import { IngredientRow } from '@/components/recipes/ingredient-row'

afterEach(cleanup)

// A made-up recipe, and a food.
const dal: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000c1',
  name: 'Dal',
  basis: '100g',
  source: 'recipe',
  archived: false,
  nutrients: { kcal: 120, protein: 7, carbs: 18, fat: 2 },
  servings: [{ name: 'recipe', amount: 1000 }],
}
const rice: Food = {
  ...dal,
  id: '0193c3a5-0000-7000-8000-0000000000f1',
  name: 'Rice',
  source: 'label',
}

const entry = (food: Food): FoodEntry => ({
  key: food.id,
  kind: 'food',
  food,
  amount: '1/4',
  unit: food.source === 'recipe' ? 'serving:recipe' : 'g',
})

const none = () => undefined

// modes are the keyboards of the amounts of the food named name.
function modes(name: string) {
  return screen.getAllByLabelText(`Amount: ${name}`).map((el) => el.getAttribute('inputmode'))
}

test("a recipe's amount takes a keyboard with a /, a food's one of digits", () => {
  render(
    <ul>
      <IngredientRow entry={entry(dal)} wrong={undefined} onChange={none} />
      <IngredientRow entry={entry(rice)} wrong={undefined} onChange={none} />
      <EntryRow
        entry={{ ...entry(dal), key: 'e1' }}
        wrong={false}
        describedBy={undefined}
        focus={false}
        onChange={none}
        onRemove={none}
      />
      <EntryRow
        entry={{ ...entry(rice), key: 'e2' }}
        wrong={false}
        describedBy={undefined}
        focus={false}
        onChange={none}
        onRemove={none}
      />
    </ul>,
  )
  expect(modes('Dal')).toEqual(['text', 'text'])
  expect(modes('Rice')).toEqual(['decimal', 'decimal'])
})
