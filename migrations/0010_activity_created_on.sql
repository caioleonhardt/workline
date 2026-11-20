-- The local day an activity was created, from the clock (SPEC §10 item 111).
-- created_at is UTC from SQLite and stays unused. Existing activities take the
-- date of their "Activity created" entry, else the day of created_at.
ALTER TABLE activities ADD COLUMN created_on TEXT CHECK (created_on IS NULL OR date(created_on) = created_on);

UPDATE activities SET created_on = coalesce(
    (SELECT min(date) FROM journal_entries
     WHERE activity_id = activities.id AND kind = 'update' AND text = 'Activity created'),
    substr(created_at, 1, 10));
