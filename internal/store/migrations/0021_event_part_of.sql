-- A write made as a part of another's change names that change's event:
-- the new foods a meal brings with it, and an archived food it brings
-- back, are part of the meal's create or edit. Undo takes such a write
-- back with the change it is part of, never on its own.
ALTER TABLE events ADD COLUMN part_of TEXT CHECK (
    part_of IS NULL OR length(part_of) = 36
);

CREATE INDEX events_user_part_of ON events (user_id, part_of)
WHERE part_of IS NOT NULL;
