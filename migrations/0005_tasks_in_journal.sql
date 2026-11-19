-- Tasks become journal entries of kind 'task' (SPEC §10 item 84). The table is
-- rebuilt to change its CHECKs and to use AUTOINCREMENT: scope_acks.last_task_id
-- relies on ids never being reused.
CREATE TABLE journal_entries_new (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    activity_id INTEGER NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    date        TEXT    NOT NULL CHECK (date(date) = date),
    kind        TEXT    NOT NULL CHECK (kind IN ('update', 'decision', 'blocker', 'meeting', 'scope_ack', 'task')),
    text        TEXT    NOT NULL CHECK (length(trim(text)) > 0),
    logged_at   TEXT    CHECK (logged_at IS NULL OR datetime(logged_at) IS NOT NULL AND datetime(logged_at) = logged_at || ':00'),
    -- A task's date is the day it was opened; closed_on/closed_at are set by toggling it.
    closed_on   TEXT    CHECK (closed_on IS NULL OR (kind = 'task' AND date(closed_on) = closed_on AND closed_on >= date)),
    closed_at   TEXT    CHECK (closed_at IS NULL OR (
        closed_on IS NOT NULL AND datetime(closed_at) IS NOT NULL AND datetime(closed_at) = closed_at || ':00'
        AND substr(closed_at, 1, 10) = closed_on))
);

INSERT INTO journal_entries_new (id, activity_id, date, kind, text, logged_at)
    SELECT id, activity_id, date, kind, text, logged_at FROM journal_entries;

-- Tasks get ids after every existing entry, in their old order.
CREATE TEMP TABLE task_ids AS
    SELECT id AS old_id, (SELECT coalesce(max(id), 0) FROM journal_entries) + row_number() OVER (ORDER BY id) AS new_id
    FROM subtasks;

INSERT INTO journal_entries_new (id, activity_id, date, kind, text, closed_on, closed_at)
    SELECT t.new_id, s.activity_id, s.created_on, 'task', s.text, s.closed_on, s.closed_at
    FROM subtasks s JOIN task_ids t ON t.old_id = s.id;

-- The renumbering keeps the order, so the tasks newer than an ack stay the same.
ALTER TABLE scope_acks RENAME COLUMN last_subtask_id TO last_task_id;
UPDATE scope_acks SET last_task_id = coalesce(
    (SELECT max(new_id) FROM task_ids WHERE old_id <= scope_acks.last_task_id), 0);

DROP TABLE task_ids;
DROP TABLE subtasks;
DROP TABLE journal_entries;
ALTER TABLE journal_entries_new RENAME TO journal_entries;
CREATE INDEX journal_activity_date ON journal_entries (activity_id, date, id);
CREATE INDEX journal_activity_kind ON journal_entries (activity_id, kind, id);
