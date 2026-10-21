-- On its own, outside sqlfluff (see .sqlfluffignore): its SQLite dialect
-- cannot parse the arguments of an FTS5 table.
--
-- Search over a food's name, brand and aliases, without regard to case or
-- diacritics ("cevapi" finds "Ćevapi"), by word prefixes. The store keeps
-- it in step with foods and food_aliases in the same transactions.
CREATE VIRTUAL TABLE foods_fts USING fts5 (
    food_id UNINDEXED,
    user_id UNINDEXED,
    name,
    brand,
    aliases,
    tokenize = 'unicode61 remove_diacritics 2'
);
