-- The scheduler's record of the jobs it ran for a user: a job (such as
-- the morning summary) for an occurrence of its own naming (such as the
-- day it is for). A restart finds the row and does not run the job for
-- that occurrence again.
CREATE TABLE job_runs (
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    job TEXT NOT NULL CHECK (length(job) BETWEEN 1 AND 64),
    occurrence TEXT NOT NULL CHECK (length(occurrence) BETWEEN 1 AND 64),
    ran_at TEXT NOT NULL,
    PRIMARY KEY (user_id, job, occurrence)
) STRICT;
