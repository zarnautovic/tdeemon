-- An item keeps its food's brand apart from its name (meal.Item.Brand),
-- so the app shows them as the foods are shown, the name first. An item
-- logged before had the brand and a space before the name; it is split
-- where its food's brand still starts the name, and left as it is where
-- the brand has changed since.
ALTER TABLE meal_items ADD COLUMN brand TEXT NOT NULL DEFAULT '';

UPDATE meal_items
SET
    brand = (
        SELECT f.brand
        FROM foods AS f
        WHERE f.user_id = meal_items.user_id AND f.id = meal_items.food_id
    ),
    name = substr(name, length((
        SELECT f.brand
        FROM foods AS f
        WHERE f.user_id = meal_items.user_id AND f.id = meal_items.food_id
    )) + 2)
WHERE EXISTS (
    SELECT 1
    FROM foods AS f
    WHERE
        f.user_id = meal_items.user_id
        AND f.id = meal_items.food_id
        AND f.brand <> ''
        AND length(meal_items.name) > length(f.brand) + 1
        AND substr(meal_items.name, 1, length(f.brand) + 1) = f.brand || ' '
);
