-- On its own, outside sqlfluff (see .sqlfluffignore): its SQLite dialect
-- cannot parse the arguments of an FTS5 table.
--
-- Search over a food's name, brand and aliases, without regard to case or
-- diacritics ("cevapi" finds "Ćevapi"), by word prefixes. The store keeps
-- it in step with foods and food_aliases in the same transactions; the
-- trigger below removes a food's row however the food goes, a user's
-- deletion included, so no name outlives its food here.
CREATE VIRTUAL TABLE foods_fts USING fts5 (
    food_id UNINDEXED,
    user_id UNINDEXED,
    name,
    brand,
    aliases,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER foods_fts_delete AFTER DELETE ON foods
BEGIN
    DELETE FROM foods_fts WHERE food_id = old.id;
END;
