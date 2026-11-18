-- The local date and time (to the minute) a subtask was closed; NULL while open and for older rows.
ALTER TABLE subtasks ADD COLUMN closed_at TEXT
    CHECK (closed_at IS NULL OR (
        closed_on IS NOT NULL AND datetime(closed_at) IS NOT NULL AND datetime(closed_at) = closed_at || ':00'
        AND substr(closed_at, 1, 10) = closed_on));
