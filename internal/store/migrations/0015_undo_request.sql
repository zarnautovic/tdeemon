-- The id a client made for an undo, on the change that undo settled:
-- taken back, or passed over as one that cannot be. The same undo sent
-- again finds it and answers as before, rather than taking back the
-- change before it.
ALTER TABLE events ADD COLUMN undo_request TEXT CHECK (
    undo_request IS NULL OR length(undo_request) = 36
);

CREATE UNIQUE INDEX events_user_undo_request ON events (user_id, undo_request)
WHERE undo_request IS NOT NULL;
