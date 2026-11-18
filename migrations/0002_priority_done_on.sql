ALTER TABLE activities ADD COLUMN priority TEXT
    CHECK (priority IS NULL OR priority IN ('high', 'medium', 'low'));

-- done_on is set only by a status change to done; see SPEC §10 item 21.
ALTER TABLE activities ADD COLUMN done_on TEXT
    CHECK (done_on IS NULL OR (date(done_on) = done_on AND status = 'done'));

UPDATE activities SET done_on = due_date WHERE status = 'done';
