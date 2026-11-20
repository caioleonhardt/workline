-- Any journal entry can be starred for the 1:1 report (SPEC §10 item 110).
ALTER TABLE journal_entries ADD COLUMN starred INTEGER NOT NULL DEFAULT 0 CHECK (starred IN (0, 1));
