-- Journal entries may have no activity: they are Unassigned until filed
-- (SPEC §10 item 91). SQLite cannot drop NOT NULL, so the table is rebuilt.
CREATE TABLE journal_entries_new (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    activity_id INTEGER REFERENCES activities (id) ON DELETE CASCADE,
    date        TEXT    NOT NULL CHECK (date(date) = date),
    kind        TEXT    NOT NULL CHECK (kind IN ('update', 'decision', 'blocker', 'meeting', 'scope_ack', 'task')),
    text        TEXT    NOT NULL CHECK (length(trim(text)) > 0),
    logged_at   TEXT    CHECK (logged_at IS NULL OR datetime(logged_at) IS NOT NULL AND datetime(logged_at) = logged_at || ':00'),
    closed_on   TEXT    CHECK (closed_on IS NULL OR (kind = 'task' AND date(closed_on) = closed_on AND closed_on >= date)),
    closed_at   TEXT    CHECK (closed_at IS NULL OR (
        closed_on IS NOT NULL AND datetime(closed_at) IS NOT NULL AND datetime(closed_at) = closed_at || ':00'
        AND substr(closed_at, 1, 10) = closed_on)),
    CHECK (kind <> 'scope_ack' OR activity_id IS NOT NULL)
);

INSERT INTO journal_entries_new (id, activity_id, date, kind, text, logged_at, closed_on, closed_at)
    SELECT id, activity_id, date, kind, text, logged_at, closed_on, closed_at FROM journal_entries;

-- Keep the id counter: ids of deleted entries must never come back
-- (scope_acks.last_task_id relies on it).
INSERT INTO sqlite_sequence (name, seq)
    SELECT 'journal_entries_new', 0
    WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'journal_entries_new');
UPDATE sqlite_sequence SET seq = max(seq, coalesce((SELECT seq FROM sqlite_sequence WHERE name = 'journal_entries'), 0))
    WHERE name = 'journal_entries_new';

DROP TABLE journal_entries;
ALTER TABLE journal_entries_new RENAME TO journal_entries;
CREATE INDEX journal_activity_date ON journal_entries (activity_id, date, id);
CREATE INDEX journal_activity_kind ON journal_entries (activity_id, kind, id);
CREATE INDEX journal_date ON journal_entries (date, id);
