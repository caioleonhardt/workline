-- An entry may belong to a project instead of an activity, and `note` is a
-- kind like the others (SPEC §10 item 120). At most one of activity_id and
-- project_id is set; neither is Unassigned. The table is rebuilt to change
-- its CHECKs (as in 0008); every row keeps its id, links and star.
CREATE TABLE journal_entries_new (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    activity_id INTEGER REFERENCES activities (id) ON DELETE CASCADE,
    project_id  INTEGER REFERENCES projects (id) ON DELETE CASCADE,
    date        TEXT    NOT NULL CHECK (date(date) = date),
    kind        TEXT    NOT NULL CHECK (kind IN ('update', 'decision', 'blocker', 'meeting', 'note', 'scope_ack', 'task')),
    text        TEXT    NOT NULL CHECK (length(trim(text)) > 0),
    logged_at   TEXT    CHECK (logged_at IS NULL OR datetime(logged_at) IS NOT NULL AND datetime(logged_at) = logged_at || ':00'),
    closed_at   TEXT    CHECK (closed_at IS NULL OR (
        kind = 'task' AND datetime(closed_at) IS NOT NULL AND datetime(closed_at) = closed_at || ':00'
        AND substr(closed_at, 1, 10) >= date)),
    priority    TEXT    NOT NULL DEFAULT ''
        CHECK (priority IN ('', 'high', 'medium', 'low') AND (priority = '' OR kind = 'task')),
    starred     INTEGER NOT NULL DEFAULT 0 CHECK (starred IN (0, 1)),
    CHECK (kind <> 'scope_ack' OR activity_id IS NOT NULL),
    CHECK (activity_id IS NULL OR project_id IS NULL)
);

INSERT INTO journal_entries_new (id, activity_id, date, kind, text, logged_at, closed_at, priority, starred)
    SELECT id, activity_id, date, kind, text, logged_at, closed_at, priority, starred FROM journal_entries;

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
CREATE INDEX journal_project ON journal_entries (project_id, id);
CREATE INDEX journal_date ON journal_entries (date, id);
