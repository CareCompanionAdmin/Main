-- +goose Up
-- Add location field to behavior_logs
ALTER TABLE behavior_logs ADD COLUMN location VARCHAR(50);
ALTER TABLE behavior_logs ADD COLUMN location_other VARCHAR(255);

-- +goose Down
ALTER TABLE behavior_logs DROP COLUMN IF EXISTS location_other;
ALTER TABLE behavior_logs DROP COLUMN IF EXISTS location;
