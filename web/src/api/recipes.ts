import { useQueries, useQueryClient } from '@tanstack/react-query'

import { inputOf } from '@/components/day/meal-entries'

import { $api } from './client'
import { keep } from './foods'
import type { components } from './schema'
import type { Food } from './types'

export type Ingredient = components['schemas']['IngredientBody']
export type RecipeBody = components['schemas']['RecipeBody']
export type RecipeInput = components['schemas']['UpsertRecipeInputBody']
export type RecipeWarning = components['schemas']['RecipeWarningBody']

// useSaveRecipe saves a recipe of the user's, corrected or cooked again
// (upsert_recipe): its food, with the recipe, is kept, and the lists and
// the searches asked again.
export function useSaveRecipe() {
  const queryClient = useQueryClient()
  return $api.useMutation('post', '/api/v1/recipes', {
    onSuccess: (answer) => keep(queryClient, answer.food),
  })
}

// useCreateRecipe makes a new recipe; a food of its name, archived too,
// is in the way (409), not corrected.
export function useCreateRecipe() {
  const queryClient = useQueryClient()
  return $api.useMutation('post', '/api/v1/recipes/new', {
    onSuccess: (answer) => keep(queryClient, answer.food),
  })
}

// ingredientInput is an ingredient as a recipe saved again sends it: its
// food in the quantity it was given, which takes the food's values now.
// (inputOf reads an item of a meal, which an ingredient is but for its
// id, which it does not read.)
export function ingredientInput(ingredient: Ingredient) {
  return inputOf({ ...ingredient, id: ingredient.food_id })
}

// recipeInput is a recipe's food as upsert_recipe takes it to save it
// again as it is: by its id, its ingredients in the quantities they were
// given, which take their foods' values now, and archived as it is.
export function recipeInput(food: Food): RecipeInput {
  const recipe = food.recipe
  return {
    id: food.id,
    name: food.name,
    ...(food.aliases === undefined || food.aliases.length === 0 ? {} : { aliases: food.aliases }),
    items: (recipe?.items ?? []).map(ingredientInput),
    ...(recipe?.yield_g === undefined ? {} : { yield_g: recipe.yield_g }),
    ...(recipe?.portions === undefined ? {} : { portions: recipe.portions }),
    ...(recipe?.pot_g === undefined ? {} : { pot_g: recipe.pot_g }),
    ...(food.note ? { note: food.note } : {}),
    ...(food.archived ? { archived: true } : {}),
  }
}

// useIngredientFoods are the foods of a recipe's ingredients as they are
// now, which the recipe's editor gives amounts of: all of them once each
// came (foods), else whether one is still on its way (pending) or
// failed (error, and retry to ask again).
export function useIngredientFoods(ids: readonly string[]) {
  return useQueries({
    queries: [...new Set(ids)].map((id) =>
      $api.queryOptions('get', '/api/v1/foods/{id}', { params: { path: { id } } }),
    ),
    combine: (results) => {
      const byId = new Map<string, Food>()
      for (const r of results) {
        if (r.data !== undefined) {
          byId.set(r.data.id, r.data)
        }
      }
      const failed = results.find((r) => r.data === undefined && r.isError)
      return {
        foods: results.every((r) => r.data !== undefined) ? byId : undefined,
        error: failed?.error,
        retry: () => {
          for (const r of results) {
            if (r.isError) {
              void r.refetch()
            }
          }
        },
      }
    },
  })
}
