-- The search over foods finds a đ as d and as dj ("grozde" and "grozdje"
-- find "grožđe"), which unicode61's remove_diacritics does not do: no d
-- and mark make up a đ. The store indexes a text with a đ in it three
-- times, as it is, with d and with dj in its place (searchText in
-- foods.go), and this indexes the foods saved before it the same way.
-- Aliases are joined in the order the store reads them, by rowid.
DELETE FROM foods_fts;

INSERT INTO foods_fts (food_id, user_id, name, brand, aliases)
WITH texts AS (
    SELECT
        f.id,
        f.user_id,
        f.name,
        f.brand,
        coalesce((
            SELECT group_concat(a.alias, char(10))
            FROM (
                SELECT fa.alias
                FROM food_aliases AS fa
                WHERE fa.food_id = f.id AND fa.user_id = f.user_id
                ORDER BY fa.rowid
            ) AS a
        ), '') AS aliases
    FROM foods AS f
)

SELECT
    id,
    user_id,
    CASE
        WHEN instr(name, 'đ') > 0 OR instr(name, 'Đ') > 0
            THEN
                name || char(10)
                || replace(replace(name, 'đ', 'd'), 'Đ', 'D') || char(10)
                || replace(replace(name, 'đ', 'dj'), 'Đ', 'Dj')
        ELSE name
    END AS name,
    CASE
        WHEN instr(brand, 'đ') > 0 OR instr(brand, 'Đ') > 0
            THEN
                brand || char(10)
                || replace(replace(brand, 'đ', 'd'), 'Đ', 'D') || char(10)
                || replace(replace(brand, 'đ', 'dj'), 'Đ', 'Dj')
        ELSE brand
    END AS brand,
    CASE
        WHEN instr(aliases, 'đ') > 0 OR instr(aliases, 'Đ') > 0
            THEN
                aliases || char(10)
                || replace(replace(aliases, 'đ', 'd'), 'Đ', 'D') || char(10)
                || replace(replace(aliases, 'đ', 'dj'), 'Đ', 'Dj')
        ELSE aliases
    END AS aliases
FROM texts;
