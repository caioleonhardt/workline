-- Tasks have a priority like activities: '' (none), high, medium or low
-- (SPEC §10 item 92). Other kinds always have none.
ALTER TABLE journal_entries ADD COLUMN priority TEXT NOT NULL DEFAULT ''
    CHECK (priority IN ('', 'high', 'medium', 'low') AND (priority = '' OR kind = 'task'));
