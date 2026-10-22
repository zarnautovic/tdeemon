-- An item of a new food a client sent with the meal is told by that food
-- as sent (meal.SentFood), not by the food it is stored as: a retry is
-- known however the food is renamed or corrected since. NULL for an item
-- of a food of the user's, or an estimate.
ALTER TABLE meal_items ADD COLUMN sent_food TEXT CHECK (
    sent_food IS NULL OR (length(sent_food) = 64 AND food_id IS NOT NULL)
);
