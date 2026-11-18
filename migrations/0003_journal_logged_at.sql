-- The local date and time (to the minute) an entry was logged; NULL for older entries.
ALTER TABLE journal_entries ADD COLUMN logged_at TEXT
    CHECK (logged_at IS NULL OR datetime(logged_at) IS NOT NULL AND datetime(logged_at) = logged_at || ':00');
