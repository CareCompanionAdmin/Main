-- Optional notes captured when a user resolves an alert from the dashboard
-- (acknowledge stays note-less; only resolve gets a textarea in the UI).
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS resolution_notes TEXT;
