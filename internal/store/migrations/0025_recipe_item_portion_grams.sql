-- The weight of a portion of a recipe's ingredient, for a food per
-- portion that says it, as it was when the recipe was saved: an
-- ingredient in portions adds it to the raw ingredients' weight. NULL
-- when not known, as for the ingredients saved before.
ALTER TABLE recipe_items ADD COLUMN portion_grams REAL CHECK (portion_grams > 0);
