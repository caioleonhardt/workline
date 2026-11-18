CREATE TABLE projects (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL COLLATE NOCASE UNIQUE CHECK (length(trim(name)) > 0),
    archived   INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE activities (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name         TEXT    NOT NULL CHECK (length(trim(name)) > 0),
    requested_by TEXT    NOT NULL DEFAULT '',
    status       TEXT    NOT NULL CHECK (status IN ('planned', 'active', 'blocked', 'done')),
    start_date   TEXT    NOT NULL CHECK (date(start_date) = start_date),
    due_date     TEXT    NOT NULL CHECK (date(due_date) = due_date),
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    CHECK (due_date >= start_date)
);
CREATE INDEX activities_project ON activities (project_id);

-- AUTOINCREMENT: scope_acks.last_subtask_id relies on ids never being reused.
CREATE TABLE subtasks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    activity_id INTEGER NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    text        TEXT    NOT NULL CHECK (length(trim(text)) > 0),
    created_on  TEXT    NOT NULL CHECK (date(created_on) = created_on),
    closed_on   TEXT    CHECK (closed_on IS NULL OR (date(closed_on) = closed_on AND closed_on >= created_on))
);
CREATE INDEX subtasks_activity ON subtasks (activity_id);

CREATE TABLE journal_entries (
    id          INTEGER PRIMARY KEY,
    activity_id INTEGER NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    date        TEXT    NOT NULL CHECK (date(date) = date),
    kind        TEXT    NOT NULL CHECK (kind IN ('update', 'decision', 'blocker', 'meeting', 'scope_ack')),
    text        TEXT    NOT NULL CHECK (length(trim(text)) > 0)
);
CREATE INDEX journal_activity_date ON journal_entries (activity_id, date, id);

CREATE TABLE scope_acks (
    id              INTEGER PRIMARY KEY,
    activity_id     INTEGER NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    date            TEXT    NOT NULL CHECK (date(date) = date),
    baseline_count  INTEGER NOT NULL CHECK (baseline_count >= 0),
    last_subtask_id INTEGER NOT NULL CHECK (last_subtask_id >= 0)
);
CREATE INDEX scope_acks_activity ON scope_acks (activity_id, id);
